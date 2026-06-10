// FILE: services/bot-fleet/main.go
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
)

type Config struct {
	KafkaBrokers        string
	OrchestratorTopic   string
	TelemetryTopic      string
	ConsumerGroupID     string
	MaxBotsPerPod       int
	BotRequestTimeoutMs int
	Environment         string
	LogLevel            string
}

func loadConfig() Config {
	return Config{
		KafkaBrokers:        getEnv("KAFKA_BROKERS", "localhost:9092"),
		OrchestratorTopic:   getEnv("ORCHESTRATOR_TOPIC", "orchestrator-events"),
		TelemetryTopic:      getEnv("TELEMETRY_TOPIC", "bot-telemetry"),
		ConsumerGroupID:     getEnv("CONSUMER_GROUP_ID", "bot-fleet-workers"),
		MaxBotsPerPod:       getEnvInt("MAX_BOTS_PER_POD", 200),
		BotRequestTimeoutMs: getEnvInt("BOT_REQUEST_TIMEOUT_MS", 5000),
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

	logger.Info("bot-fleet starting", "max_bots_per_pod", cfg.MaxBotsPerPod)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("# bot fleet metrics placeholder\n"))
	})

	srv := &http.Server{
		Addr:         ":8082",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
		}
	}()

	<-sigCh
	logger.Info("bot-fleet shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}
