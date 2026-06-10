// FILE: services/orchestrator/main.go
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	KafkaBrokers             string
	OrchestratorEventsTopic  string
	ConsumerGroupID          string
	DatabaseDSN              string
	RedisURL                 string
	InstanceID               string
	HeartbeatIntervalSeconds int
	OrphanDetectionSeconds   int
	Environment              string
	LogLevel                 string
}

func loadConfig() Config {
	return Config{
		KafkaBrokers:             getEnv("KAFKA_BROKERS", "localhost:9092"),
		OrchestratorEventsTopic:  getEnv("ORCHESTRATOR_EVENTS_TOPIC", "orchestrator-events"),
		ConsumerGroupID:          getEnv("CONSUMER_GROUP_ID", "orchestrators"),
		DatabaseDSN:              getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5433/orchestrator?sslmode=disable"),
		RedisURL:                 getEnv("REDIS_URL", "localhost:6379"),
		InstanceID:               getEnv("INSTANCE_ID", ""),
		HeartbeatIntervalSeconds: getEnvInt("HEARTBEAT_INTERVAL_SECONDS", 10),
		OrphanDetectionSeconds:   getEnvInt("ORPHAN_DETECTION_SECONDS", 60),
		Environment:              getEnv("ENVIRONMENT", "development"),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
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
	return slog.New(handler).With("instance_id", cfg.InstanceID)
}

func main() {
	if err := run(); err != nil {
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	if cfg.InstanceID == "" {
		cfg.InstanceID = uuid.New().String()
	}
	logger := initLogger(cfg)
	slog.SetDefault(logger)

	logger.Info("orchestrator starting", "instance_id", cfg.InstanceID)

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.HeartbeatIntervalSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("orchestrator heartbeat")
			case <-stopCh:
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	logger.Info("orchestrator shutting down")
	close(stopCh)
	return nil
}
