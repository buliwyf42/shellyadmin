package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"shellyadmin/internal/api"
	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
	"shellyadmin/internal/mcp"
	"shellyadmin/internal/observability"
	"shellyadmin/internal/services"
	"shellyadmin/internal/services/runtimelock"
)

//go:embed all:dist
var staticFiles embed.FS

var (
	AppVersion = "dev"
	CommitHash = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			runHashPassword(os.Args[2:])
			return
		case "mcp":
			runMCPStdio()
			return
		case "unlock":
			runUnlock(os.Args[2:])
			return
		}
	}

	user := getenv("SHELLYADMIN_USER", "admin")
	passHash := services.DecodeSecretValue("SHELLYADMIN_PASS_HASH")
	if passHash == "" {
		panic("set SHELLYADMIN_PASS_HASH (argon2id PHC from `shellyctl hash-password`)")
	}
	secret := services.DecodeSecretValue("SHELLYADMIN_SECRET")
	if secret == "" {
		secret = api.RandomSecret()
	}
	dataDir := getenv("DATA_DIR", "./data")
	port := getenv("PORT", "8080")
	cookieSecure := getenv("COOKIE_SECURE", "true") == "true"
	mcpToken := services.DecodeSecretValue("SHELLYADMIN_MCP_TOKEN")
	mcpPort := getenv("SHELLYADMIN_MCP_PORT", "8081")
	// Default to loopback so an enabled MCP listener does not silently expose
	// itself on every container network interface (e.g. a sidecar monitoring
	// network). Operators who want LAN-reachable MCP set _MCP_BIND=0.0.0.0
	// explicitly. The HTTP API on :8080 still defaults to all interfaces
	// because that surface is auth+CSRF+rate-limited; MCP-token-only auth
	// warrants a tighter default.
	mcpBind := getenv("SHELLYADMIN_MCP_BIND", "127.0.0.1")
	// M4 — optional Prometheus metrics listener. Default off (empty bind)
	// because most homelab operators don't run Prometheus and the
	// metrics surface is small; opt in via SHELLYADMIN_METRICS_BIND
	// (e.g. `127.0.0.1:9100`). The endpoint itself is unauthenticated —
	// pair with `127.0.0.1` binding + reverse-proxy auth, or a
	// firewall rule limiting access to the Prometheus host.
	metricsBind := getenv("SHELLYADMIN_METRICS_BIND", "")
	// S11 — comma-separated trusted-proxy CIDR list. Without this, gin's
	// ClientIP() falls back to the X-Forwarded-For header from ANY peer,
	// which lets an attacker on the LAN spoof "client_ip" in audit rows
	// and bypass per-IP rate-limit accounting. With it, only peers in the
	// listed CIDRs may set X-Forwarded-For; everyone else's ClientIP is
	// the direct peer address. Default empty → no trusted proxies → all
	// X-F-F headers ignored (safe but the reverse-proxy IP shows up as
	// the client). Example: SHELLYADMIN_TRUSTED_PROXIES=127.0.0.1/32,10.0.0.0/8
	trustedProxies := getenv("SHELLYADMIN_TRUSTED_PROXIES", "")
	backendVersion := resolveBackendVersion()
	backendCommit := resolveBackendCommit()

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		panic(err)
	}
	// Tee structured logs to BOTH the rotated file sink AND stderr so cluster
	// log-collectors (Loki/Promtail, docker-logs --since, k8s log streams)
	// can ship the same JSON the operator sees in /data/shellyctl.log. Before
	// this change stdout only carried the Gin default request line; the
	// audit-relevant structured slog stayed inside the container's volume.
	logSink := io.MultiWriter(&lumberjack.Logger{
		Filename:   filepath.Join(dataDir, "shellyctl.log"),
		MaxSize:    5,
		MaxBackups: 3,
	}, os.Stderr)
	logger := slog.New(slog.NewJSONHandler(logSink, nil))
	slog.SetDefault(logger)

	if err := loadEncryptionKey(dataDir, logger); err != nil {
		panic(fmt.Sprintf("encryption key init failed: %v", err))
	}

	// T6 — surface OWASP-deprecated argon2id parameters once at startup
	// so the operator gets a single clear line in the log instead of one
	// on every login attempt. Action: rerun `shellyctl hash-password`
	// against the same plaintext and update SHELLYADMIN_PASS_HASH.
	if services.IsLegacyParameters(passHash) {
		logger.Warn("admin password hash uses legacy argon2id parameters (OWASP 2023 defaults). Regenerate with `shellyctl hash-password` and update SHELLYADMIN_PASS_HASH before the next deployment.")
	}

	database, err := db.Open(dataDir)
	if err != nil {
		backupPath, backupErr := db.BackupDatabaseFile(dataDir)
		if backupErr != nil {
			panic(fmt.Sprintf("database open failed: %v (backup attempt failed: %v)", err, backupErr))
		}
		panic(fmt.Sprintf("database open failed: %v (backup created at %s)", err, backupPath))
	}
	defer database.Close()
	_ = database.MarkRunningJobsInterrupted()

	// ADR-0015 — claim the single-instance primary lock. Refuses to
	// start when another container is alive on the same SQLite file
	// (the heartbeat-fresh row is the "alive" signal). A stale row
	// (5+ minutes without heartbeat — kill-9'd previous container)
	// is taken over automatically. Operators who don't want to wait
	// out the staleness window can run `shellyctl unlock --force`.
	lock := runtimelock.New(database)
	lockCtx, lockCancel := context.WithCancel(context.Background())
	defer lockCancel()
	if err := lock.Acquire(lockCtx); err != nil {
		panic(fmt.Sprintf("runtime lock: %v", err))
	}
	lock.StartHeartbeat(lockCtx, func(hbErr error) {
		// A heartbeat failure is logged but not fatal — the SQLite
		// write may have lost a single tick (busy timeout, brief
		// disk hiccup) and the next tick recovers. A sustained
		// failure means the row will go stale; ADR-0015 treats
		// that as "this container is wedged, another can take
		// over". We surface the error so operators see it in
		// `docker logs` if it persists.
		slog.Warn("runtime lock heartbeat failed", "err", hbErr)
	})
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lock.Release(releaseCtx); err != nil {
			slog.Warn("runtime lock release failed", "err", err)
		}
	}()

	// Single shared AppService — backs HTTP handlers, background workers,
	// AND the MCP listener so SaveSettings can reconcile live state in
	// one place (see ADR-0011 v0.1.21 amendment).
	service := services.NewAppService(database, dataDir, func(_ context.Context, level, msg string) {
		_ = database.AddLog(level, services.SanitizeLogMessage(msg))
	})
	router := api.NewRouter(database, api.Config{
		User:           user,
		PassHash:       passHash,
		Secret:         secret,
		CookieSecure:   cookieSecure,
		DataDir:        dataDir,
		BackendVersion: backendVersion,
		BackendCommit:  backendCommit,
		StaticFS:       staticFiles,
		HasStatic:      true,
		Service:        service,
		TrustedProxies: trustedProxies,
	})
	// Wire MCP runtime params into the service so SaveSettings can
	// reconcile the live listener (start / stop / rotate token) without
	// requiring a container restart. envToken!="" means env-locked —
	// settings changes will be ignored at the reconcile point.
	service.SetMCPParams(database, mcp.Build, mcpToken, mcpBind, mcpPort, backendVersion)
	_ = service.RecoverInterruptedJobs()
	service.StartBackgroundWorkers()
	// Initial MCP startup. Service handles env-vs-settings resolution and
	// the "MCP disabled" log line when neither is set.
	service.StartMCPFromConfig()
	if mcpToken == "" && !service.MCPRunning() {
		slog.Info("MCP disabled (no token in env or settings)")
	}

	// M4 — optional Prometheus metrics listener. Empty bind = disabled.
	// Registry is constructed regardless so the service layer can call
	// Inc/Set without nil checks; only the listener and the URL routing
	// are gated on the env var.
	metrics := observability.NewRegistry()
	metrics.RegisterCounter("shellyadmin_http_requests_total", "Total HTTP requests routed through the SPA + API listener.")
	metrics.RegisterGauge("shellyadmin_devices_total", "Number of devices currently tracked in the inventory.")
	metrics.RegisterCounter("shellyadmin_refresh_jobs_total", "Refresh jobs spawned since service start.")
	metrics.RegisterCounter("shellyadmin_firmware_jobs_total", "Firmware-check + firmware-install jobs spawned since service start.")
	metrics.RegisterLabelledCounter("shellyadmin_audit_rows_written_total", "Audit-log rows written, labelled by level (INFO/WARN/ERROR/DEBUG).")
	service.SetMetrics(metrics)
	if metricsBind != "" {
		metricsServer := &http.Server{
			Addr:              metricsBind,
			Handler:           metrics,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
		}
		go func() {
			slog.Info("metrics listener", "addr", metricsBind)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics listener exited", "err", err)
			}
		}()
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		panic(err)
	}
	// service.Stop now also tears down the MCP listener.
	service.Stop(ctx)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// runHashPassword is the `shellyctl hash-password` subcommand. It reads a
// plaintext password from argv[0] (if provided) or stdin and prints the
// argon2id PHC string suitable for SHELLYADMIN_PASS_HASH.
func runHashPassword(args []string) {
	var plain string
	switch {
	case len(args) == 1:
		// argv leaks through `ps`, shell history (~/.zsh_history,
		// ~/.bash_history), and container manager logs (Docker /
		// Kubernetes record the command line). Warn loudly so the
		// operator either pipes via stdin or accepts the leak knowingly.
		warn1 := "WARNING: password passed on the command line will appear in `ps`, shell history, and container logs."
		warn2 := "         Prefer: `shellyctl hash-password` (reads from stdin) or pipe via `printf` from a read -s variable."
		fmt.Fprintln(os.Stderr, warn1)
		fmt.Fprintln(os.Stderr, warn2)
		plain = args[0]
	case len(args) == 0:
		fmt.Fprint(os.Stderr, "password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr, "read password:", err)
			os.Exit(1)
		}
		plain = strings.TrimRight(line, "\r\n")
	default:
		fmt.Fprintln(os.Stderr, "usage: shellyctl hash-password [password]")
		os.Exit(2)
	}
	if plain == "" {
		fmt.Fprintln(os.Stderr, "password cannot be empty")
		os.Exit(1)
	}
	phc, err := services.HashPassword(plain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash:", err)
		os.Exit(1)
	}
	fmt.Println(phc)
}

// loadEncryptionKey resolves the at-rest encryption key used for credential
// password/ha1 columns. v0.3.0 closes the S6 deprecation window: an
// operator-supplied key is REQUIRED. Resolution order:
//
//  1. SHELLYADMIN_ENCRYPTION_KEY — base64 of a 32-byte key. The
//     DecodeSecretValue helper also accepts SHELLYADMIN_ENCRYPTION_KEY_FILE
//     pointing at a file whose trimmed contents are the base64 string
//     (Docker secret pattern, Kubernetes secret-as-volume, etc.).
//  2. Anything else → hard fail with an actionable error pointing at
//     the migration recipe.
//
// The v0.2.x auto-generation-next-to-the-database fallback is GONE.
// Operators upgrading from v0.2.x must copy the existing key out of
// {dataDir}/shellyadmin.key, store it in their secrets manager / Docker
// secret / NixOS secret store, and set SHELLYADMIN_ENCRYPTION_KEY_FILE
// to the new path before starting v0.3.0. The dataDir copy can stay
// on disk as a rollback safety net but is no longer consulted.
//
// Why: the at-rest encryption defends against an offline DB exfil
// (stolen backup, container escape reading /data, misconfigured
// volume). When the key lives ON the same volume, a volume snapshot
// leaks BOTH halves of the envelope — the encryption is ceremonial.
// External key management closes that.
func loadEncryptionKey(dataDir string, logger *slog.Logger) error {
	raw := services.DecodeSecretValue("SHELLYADMIN_ENCRYPTION_KEY")
	if raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("SHELLYADMIN_ENCRYPTION_KEY is not valid base64: %w", err)
		}
		if err := secretbox.SetKey(key); err != nil {
			return err
		}
		logger.Info("encryption key loaded from environment")
		return nil
	}

	// S6 (v0.3.0) — no env key set. Refuse to start. Hint at the
	// existing on-disk file if there is one so the operator's recovery
	// path is one `cat` away from set.
	legacyPath := filepath.Join(dataDir, "shellyadmin.key")
	legacyHint := ""
	if _, err := os.Stat(legacyPath); err == nil {
		legacyHint = fmt.Sprintf(
			" An auto-generated key from v0.2.x is still present at %s; copy its contents "+
				"to your secrets store and set SHELLYADMIN_ENCRYPTION_KEY_FILE to point at the new path.",
			legacyPath,
		)
	}
	return fmt.Errorf(
		"S6: encryption key not configured. Set SHELLYADMIN_ENCRYPTION_KEY (base64 of a "+
			"32-byte key) or SHELLYADMIN_ENCRYPTION_KEY_FILE (path to that base64 string). "+
			"See docs/adr/0013-encryption-key-externalization.md for the migration recipe.%s",
		legacyHint,
	)
}

func resolveBackendVersion() string {
	if value := strings.TrimSpace(os.Getenv("SHELLYADMIN_VERSION")); value != "" {
		return value
	}
	if AppVersion != "" && AppVersion != "dev" {
		return AppVersion
	}
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	if value := gitOutput("describe", "--tags", "--always", "--dirty"); value != "" {
		return value
	}
	return "dev"
}

func resolveBackendCommit() string {
	if value := strings.TrimSpace(os.Getenv("GIT_COMMIT")); value != "" {
		return trimCommit(value)
	}
	if CommitHash != "" && CommitHash != "unknown" {
		return trimCommit(CommitHash)
	}
	info, ok := debug.ReadBuildInfo()
	if ok {
		revision := "unknown"
		dirty := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
		revision = trimCommit(revision)
		if dirty && revision != "unknown" {
			return revision + "-dirty"
		}
		if revision != "unknown" {
			return revision
		}
	}
	revision := gitOutput("rev-parse", "--short=12", "HEAD")
	if revision == "" {
		return "unknown"
	}
	if gitOutput("status", "--porcelain") != "" {
		return revision + "-dirty"
	}
	return revision
}

func trimCommit(value string) string {
	commit := strings.TrimSpace(value)
	if commit == "" {
		return "unknown"
	}
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runUnlock is the `shellyctl unlock --force` subcommand. Clears the
// runtime_locks `primary` row regardless of acquired_at freshness so
// an operator who knows the previous container died can recover
// without waiting the 5-minute staleness window. Idempotent — running
// it when no row exists is a successful no-op.
//
// Refuses to run without --force to avoid a stray invocation
// invalidating an actively-running instance.
func runUnlock(args []string) {
	force := false
	for _, arg := range args {
		switch arg {
		case "--force", "-f":
			force = true
		default:
			fmt.Fprintf(os.Stderr, "unknown arg: %q\n", arg)
			fmt.Fprintln(os.Stderr, "usage: shellyctl unlock --force")
			os.Exit(2)
		}
	}
	if !force {
		fmt.Fprintln(os.Stderr, "usage: shellyctl unlock --force")
		fmt.Fprintln(os.Stderr, "Refusing without --force; an active container's lock would be cleared.")
		os.Exit(2)
	}
	dataDir := getenv("DATA_DIR", "./data")
	database, err := db.Open(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database open:", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := runtimelock.ForceClear(database); err != nil {
		fmt.Fprintln(os.Stderr, "force clear:", err)
		os.Exit(1)
	}
	fmt.Println("runtime lock cleared")
}
