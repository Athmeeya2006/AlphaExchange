// FILE: services/submission-api/main.go
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

type Config struct {
	Port          string
	KafkaBrokers  string
	RedisURL      string
	MinIOEndpoint string
	MinIOAccessKey string
	MinIOSecretKey string
	MinioBucket   string
	DatabaseDSN   string
	MaxUploadMB   int64
	Environment   string
	LogLevel      string
}

func loadConfig() Config {
	maxUpload, err := strconv.ParseInt(getEnv("MAX_UPLOAD_MB", "50"), 10, 64)
	if err != nil {
		maxUpload = 50
	}
	return Config{
		Port:          getEnv("PORT", "8080"),
		KafkaBrokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
		RedisURL:      getEnv("REDIS_URL", "localhost:6379"),
		MinIOEndpoint: getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:   getEnv("MINIO_BUCKET", "submissions"),
		DatabaseDSN:   getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5433/orchestrator?sslmode=disable"),
		MaxUploadMB:   maxUpload,
		Environment:   getEnv("ENVIRONMENT", "development"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func initLogger(cfg Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLogLevel(cfg.LogLevel)}
	var handler slog.Handler
	if cfg.Environment == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

func main() {
	if err := run(); err != nil {
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	logger := initLogger(cfg)
	slog.SetDefault(logger)

	logger.Info("submission-api starting", "port", cfg.Port, "environment", cfg.Environment)

	r := chi.NewRouter()
	r.Get("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "submission-api",
		})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
		}
	}()

	<-sigCh
	logger.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	}

	return nil
}
