// FILE: services/telemetry-ingester/main.go
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type Config struct {
	KafkaBrokers    string
	TelemetryTopic  string
	ConsumerGroupID string
	TimescaleDSN    string
	RedisURL        string
	WindowSizeSeconds int
	BatchSize         int
	FlushIntervalMs   int
	ReorderBufferMs   int
	Environment       string
	LogLevel          string
}

func loadConfig() Config {
	return Config{
		KafkaBrokers:      getEnv("KAFKA_BROKERS", "localhost:9092"),
		TelemetryTopic:    getEnv("TELEMETRY_TOPIC", "bot-telemetry"),
		ConsumerGroupID:   getEnv("CONSUMER_GROUP_ID", "telemetry-ingesters"),
		TimescaleDSN:      getEnv("TIMESCALE_DSN", "postgres://postgres:postgres@localhost:5432/tradeeval?sslmode=disable"),
		RedisURL:          getEnv("REDIS_URL", "localhost:6379"),
		WindowSizeSeconds: getEnvInt("WINDOW_SIZE_SECONDS", 30),
		BatchSize:         getEnvInt("BATCH_SIZE", 500),
		FlushIntervalMs:   getEnvInt("FLUSH_INTERVAL_MS", 1000),
		ReorderBufferMs:   getEnvInt("REORDER_BUFFER_MS", 100),
		Environment:       getEnv("ENVIRONMENT", "development"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
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

	logger.Info("telemetry-ingester starting")

	logger.Debug("telemetry-ingester config",
		"kafka_brokers", cfg.KafkaBrokers,
		"telemetry_topic", cfg.TelemetryTopic,
		"consumer_group", cfg.ConsumerGroupID,
		"timescale_dsn", cfg.TimescaleDSN,
		"redis_url", cfg.RedisURL,
		"window_size_seconds", cfg.WindowSizeSeconds,
		"batch_size", cfg.BatchSize,
		"flush_interval_ms", cfg.FlushIntervalMs,
		"reorder_buffer_ms", cfg.ReorderBufferMs,
	)

	var processed atomic.Int64
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("ingester alive", "processed_events", processed.Load())
			case <-stopCh:
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	logger.Info("telemetry-ingester shutting down", "total_processed", processed.Load())
	close(stopCh)
	return nil
}
