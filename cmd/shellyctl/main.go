package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
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
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		runHashPassword(os.Args[2:])
		return
	}

	user := getenv("SHELLYADMIN_USER", "admin")
	passHash := services.DecodeSecretValue("SHELLYADMIN_PASS_HASH")
	pass := services.DecodeSecretValue("SHELLYADMIN_PASS")
	if passHash == "" && pass == "" {
		panic("set SHELLYADMIN_PASS_HASH (argon2id PHC from `shellyctl hash-password`) or SHELLYADMIN_PASS (deprecated plaintext)")
	}
	if passHash == "" {
		slog.Warn("SHELLYADMIN_PASS is set as plaintext; migrate to SHELLYADMIN_PASS_HASH (run `shellyctl hash-password`) — plaintext support is scheduled for removal in v0.2.0, no earlier than 2026-07-22 (3-month overlap from v0.0.15)")
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
	mcpBind := getenv("SHELLYADMIN_MCP_BIND", "0.0.0.0")
	backendVersion := resolveBackendVersion()
	backendCommit := resolveBackendCommit()

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewJSONHandler(&lumberjack.Logger{
		Filename:   filepath.Join(dataDir, "shellyctl.log"),
		MaxSize:    5,
		MaxBackups: 3,
	}, nil))
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

	router := api.NewRouter(database, api.Config{
		User:           user,
		Pass:           pass,
		PassHash:       passHash,
		Secret:         secret,
		CookieSecure:   cookieSecure,
		DataDir:        dataDir,
		BackendVersion: backendVersion,
		BackendCommit:  backendCommit,
		StaticFS:       staticFiles,
		HasStatic:      true,
	})
	service := services.NewAppService(database, dataDir, func(_ context.Context, level, msg string) {
		_ = database.AddLog(level, services.SanitizeLogMessage(msg))
	})
	_ = service.RecoverInterruptedJobs()
	service.StartBackgroundWorkers()
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

	// MCP token resolution: env var wins; if unset, fall back to the
	// persisted settings (operator configured via the SPA). Documented in
	// ADR-0011 → "Two equivalent transport-level auth shapes" + the
	// settings precedence section.
	mcpFromEnv := mcpToken != ""
	if !mcpFromEnv {
		if persisted, perr := service.GetSettings(); perr == nil {
			if persisted.MCPEnabled && persisted.MCPToken != "" {
				mcpToken = persisted.MCPToken
				slog.Info("MCP enabled via settings (env var not set)")
			}
		} else {
			slog.Warn("MCP settings read failed; MCP disabled", "err", perr)
		}
	}

	var mcpServer *http.Server
	if mcpToken == "" {
		slog.Info("MCP disabled (no token in env or settings)")
	} else {
		built, err := mcp.Build(database, dataDir, mcpToken, mcpBind, mcpPort, backendVersion)
		if err != nil {
			panic(fmt.Sprintf("mcp init failed: %v", err))
		}
		mcpServer = built
		slog.Info("MCP server starting", "addr", mcpServer.Addr)
		go func() {
			if err := mcpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				panic(err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		panic(err)
	}
	if mcpServer != nil {
		if err := mcpServer.Shutdown(ctx); err != nil {
			slog.Warn("mcp shutdown", "err", err)
		}
	}
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
	logger.Warn("generated new encryption key — back this file up alongside your database",
		"path", keyPath,
		"hint", "set SHELLYADMIN_ENCRYPTION_KEY (base64) to manage the key outside the data directory")
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
