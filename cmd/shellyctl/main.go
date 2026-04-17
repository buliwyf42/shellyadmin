package main

import (
	"context"
	"embed"
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
	"shellyadmin/internal/db"
	"shellyadmin/internal/services"
)

//go:embed all:dist
var staticFiles embed.FS

var (
	AppVersion = "dev"
	CommitHash = "unknown"
)

func main() {
	user := getenv("SHELLYADMIN_USER", "admin")
	pass := services.DecodeSecretValue("SHELLYADMIN_PASS")
	if pass == "" {
		panic("SHELLYADMIN_PASS is required")
	}
	secret := services.DecodeSecretValue("SHELLYADMIN_SECRET")
	if secret == "" {
		secret = api.RandomSecret()
	}
	dataDir := getenv("DATA_DIR", "./data")
	port := getenv("PORT", "8080")
	cookieSecure := getenv("COOKIE_SECURE", "true") == "true"
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
		Secret:         secret,
		CookieSecure:   cookieSecure,
		DataDir:        dataDir,
		BackendVersion: backendVersion,
		BackendCommit:  backendCommit,
		StaticFS:       staticFiles,
		HasStatic:      true,
	})
	service := services.NewAppService(database, dataDir, func(level, msg string) {
		_ = database.AddLog(level, services.SanitizeLogMessage(msg))
	})
	_ = service.RecoverInterruptedJobs()
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
	service.Stop(ctx)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
