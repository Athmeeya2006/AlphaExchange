// Command telemetry-ingester consumes the bot-telemetry firehose, validates
// correctness against a reference order book, computes percentiles, and writes
// metrics to TimescaleDB and Redis.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/trade-eval/telemetry-ingester/anomaly"
	"github.com/trade-eval/telemetry-ingester/api"
	tikafka "github.com/trade-eval/telemetry-ingester/kafka"
	"github.com/trade-eval/telemetry-ingester/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("telemetry-ingester failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	logger := initLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("telemetry-ingester starting", "batch_size", cfg.BatchSize)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.TimescaleDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisURL})
	defer rdb.Close()

	tw := storage.NewTimescaleWriter(pool, cfg.BatchSize, logger)
	defer tw.Close()
	rw := storage.NewRedisMetricsWriter(rdb)
	ad := anomaly.NewDetector(rdb)

	pipeline := NewPipeline(tw, rw, ad, logger)

	stopCh := make(chan struct{})
	buffer := NewReorderBuffer(time.Duration(cfg.ReorderBufferMs)*time.Millisecond, pipeline.ProcessBatch)
	go buffer.Run(stopCh)
	go pipeline.RunPublishers(ctx)

	consumer := NewConsumer(cfg, buffer, logger)
	defer consumer.Close()
	go func() {
		if err := consumer.Run(ctx); err != nil {
			logger.Error("consumer stopped", "error", err)
		}
	}()

	// Kafka consumer-group lag monitor (alerts above 100k; mirrored to Prometheus).
	lagMon := tikafka.NewLagMonitor(cfg.KafkaBrokers, cfg.TelemetryTopic, cfg.ConsumerGroupID, 100000, logger)
	go lagMon.Run(ctx)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				consumerLag.Set(float64(lagMon.CurrentLag()))
			}
		}
	}()

	apiSrv := &http.Server{Addr: ":" + cfg.APIPort, Handler: api.NewServer(pool, cfg.InternalToken).Handler()}
	go func() {
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server", "error", err)
		}
	}()
	logger.Info("telemetry-ingester ready", "api_port", cfg.APIPort)

	<-ctx.Done()
	close(stopCh)
	logger.Info("telemetry-ingester shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return apiSrv.Shutdown(shutdownCtx)
}
