// FILE: services/leaderboard-api/main.go
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
	Port                  string
	RedisURL              string
	DatabaseDSN           string
	UpdateIntervalMs      int
	MaxWebSocketConns     int
	WebSocketPingInterval int
	AdminAPIKey           string
	Environment           string
	LogLevel              string
}

func loadConfig() Config {
	return Config{
		Port:                  getEnv("PORT", "8084"),
		RedisURL:              getEnv("REDIS_URL", "localhost:6379"),
		DatabaseDSN:           getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5432/tradeeval?sslmode=disable"),
		UpdateIntervalMs:      getEnvInt("UPDATE_INTERVAL_MS", 500),
		MaxWebSocketConns:     getEnvInt("MAX_WEBSOCKET_CONNS", 10000),
		WebSocketPingInterval: getEnvInt("WEBSOCKET_PING_INTERVAL", 30),
		AdminAPIKey:           getEnv("ADMIN_API_KEY", "admin-dev-key"),
		Environment:           getEnv("ENVIRONMENT", "development"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
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

	logger.Info("leaderboard-api starting",
		"port", cfg.Port,
		"max_websocket_conns", cfg.MaxWebSocketConns,
	)

	r := chi.NewRouter()

	r.Get("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "leaderboard-api",
		})
	})

	r.Get("/v1/leaderboard", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"updated_at": 0,
			"entries":    []interface{}{},
		})
	})

	r.Get("/ws", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("websocket endpoint (not yet implemented)"))
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
	logger.Info("leaderboard-api shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}
