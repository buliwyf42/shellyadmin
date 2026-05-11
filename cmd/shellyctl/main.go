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
	"shellyadmin/internal/services"
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
// password/ha1 columns. Resolution order:
//  1. SHELLYADMIN_ENCRYPTION_KEY or SHELLYADMIN_ENCRYPTION_KEY_FILE — base64
//     of a 32-byte key (operator-managed).
//  2. {dataDir}/shellyadmin.key — a previously generated key file, 0600.
//  3. Otherwise generate a fresh key, write it to {dataDir}/shellyadmin.key
//     with 0600 perms, and log a warning so operators know to back it up.
func loadEncryptionKey(dataDir string, logger *slog.Logger) error {
	if raw := services.DecodeSecretValue("SHELLYADMIN_ENCRYPTION_KEY"); raw != "" {
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

	keyPath := filepath.Join(dataDir, "shellyadmin.key")
	if body, err := os.ReadFile(keyPath); err == nil {
		key, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if decErr != nil {
			return fmt.Errorf("%s: %w", keyPath, decErr)
		}
		if err := secretbox.SetKey(key); err != nil {
			return err
		}
		logger.Info("encryption key loaded from file", "path", keyPath)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", keyPath, err)
	}

	// S6 — Phase 2 (v0.2.11) emits a deprecation warning when no
	// external key is provided and we fall back to auto-generating
	// one alongside the database. Phase 4 (v0.3.0) will turn this
	// into a hard error: storing the key on the same volume as the
	// DB means a volume snapshot exfiltrates both, defeating the
	// at-rest encryption. The two-version deprecation window gives
	// operators time to migrate their `.env` to set
	// SHELLYADMIN_ENCRYPTION_KEY_FILE before the breaking change.
	fresh, err := secretbox.GenerateKey()
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(fresh)
	if err := os.WriteFile(keyPath, []byte(encoded+"\n"), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", keyPath, err)
	}
	if err := secretbox.SetKey(fresh); err != nil {
		return err
	}
	logger.Warn(
		"DEPRECATED: encryption key auto-generated next to the database. "+
			"v0.3.0 will refuse to start without an external key. "+
			"Move the file to a separate volume or set SHELLYADMIN_ENCRYPTION_KEY / "+
			"SHELLYADMIN_ENCRYPTION_KEY_FILE before upgrading.",
		"path", keyPath,
		"deprecation_window", "v0.2.11 warn, v0.3.0 hard-fail",
	)
	return nil
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
