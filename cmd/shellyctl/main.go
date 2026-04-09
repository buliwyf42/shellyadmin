package main

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"shellyadmin/internal/api"
	"shellyadmin/internal/db"
	"shellyadmin/internal/services"
)

//go:embed all:dist
var staticFiles embed.FS

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
		panic(err)
	}
	defer database.Close()
	_ = database.MarkRunningJobsInterrupted()

	router := api.NewRouter(database, api.Config{
		User:         user,
		Pass:         pass,
		Secret:       secret,
		CookieSecure: cookieSecure,
		DataDir:      dataDir,
		StaticFS:     staticFiles,
		HasStatic:    true,
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
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
