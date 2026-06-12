package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/segmentio/kafka-go"

	"github.com/trade-eval/telemetry-ingester/model"
)

// Consumer reads bot-telemetry and feeds the reorder buffer.
type Consumer struct {
	reader *kafka.Reader
	buffer *ReorderBuffer
	logger *slog.Logger
}

// NewConsumer constructs the telemetry consumer group reader.
func NewConsumer(cfg Config, buffer *ReorderBuffer, logger *slog.Logger) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     strings.Split(cfg.KafkaBrokers, ","),
		GroupID:     cfg.ConsumerGroupID,
		Topic:       cfg.TelemetryTopic,
		MinBytes:    1024,
		MaxBytes:    10 << 20,
		MaxWait:     500 * 1e6, // 500ms
		StartOffset: kafka.FirstOffset,
	})
	return &Consumer{reader: r, buffer: buffer, logger: logger}
}

// Run consumes until ctx is done. Offsets auto-commit after read; the reorder
// buffer + per-second flush keeps the pipeline moving.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("telemetry read", "error", err)
			continue
		}
		var e model.TelemetryEvent
		if json.Unmarshal(msg.Value, &e) != nil {
			continue
		}
		c.buffer.Add(e)
	}
}

// Close closes the reader.
func (c *Consumer) Close() error { return c.reader.Close() }
