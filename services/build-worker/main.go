// FILE: services/build-worker/main.go
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type Config struct {
	KafkaBrokers        string
	BuildJobsTopic      string
	ConsumerGroupID     string
	DatabaseDSN         string
	MinIOEndpoint       string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinioBucket         string
	WorkDir             string
	MaxConcurrentBuilds int
	Environment         string
	LogLevel            string
}

func loadConfig() Config {
	return Config{
		KafkaBrokers:        getEnv("KAFKA_BROKERS", "localhost:9092"),
		BuildJobsTopic:      getEnv("BUILD_JOBS_TOPIC", "build-jobs"),
		ConsumerGroupID:     getEnv("CONSUMER_GROUP_ID", "build-workers"),
		DatabaseDSN:         getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5433/orchestrator?sslmode=disable"),
		MinIOEndpoint:       getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:      getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:      getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:         getEnv("MINIO_BUCKET", "submissions"),
		WorkDir:             getEnv("WORK_DIR", "/tmp/builds"),
		MaxConcurrentBuilds: getEnvInt("MAX_CONCURRENT_BUILDS", 3),
		Environment:         getEnv("ENVIRONMENT", "development"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
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

	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return err
	}

	logger.Info("build-worker starting",
		"kafka_brokers", cfg.KafkaBrokers,
		"topic", cfg.BuildJobsTopic,
		"consumer_group", cfg.ConsumerGroupID,
		"work_dir", cfg.WorkDir,
		"max_concurrent_builds", cfg.MaxConcurrentBuilds,
		"environment", cfg.Environment,
	)

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("waiting for build jobs...")
			case <-stopCh:
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	<-sigCh
	logger.Info("build-worker shutting down")
	close(stopCh)

	return nil
}
