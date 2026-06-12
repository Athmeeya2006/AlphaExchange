package kafka

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/segmentio/kafka-go"
)

// ParallelConsumerGroup runs N independent group-reader goroutines that feed a
// shared channel, matching the topic's partition count for max throughput.
type ParallelConsumerGroup struct {
	brokers   []string
	topic     string
	groupID   string
	workers   int
	out       chan kafka.Message
	logger    *slog.Logger
}

// NewParallelConsumerGroup constructs the group.
func NewParallelConsumerGroup(brokers, topic, groupID string, workers int, logger *slog.Logger) *ParallelConsumerGroup {
	if workers <= 0 {
		workers = 8
	}
	return &ParallelConsumerGroup{
		brokers: strings.Split(brokers, ","),
		topic:   topic,
		groupID: groupID,
		workers: workers,
		out:     make(chan kafka.Message, 100000),
		logger:  logger,
	}
}

// Messages returns the shared output channel.
func (g *ParallelConsumerGroup) Messages() <-chan kafka.Message { return g.out }

// Run starts the worker goroutines and blocks until ctx is done.
func (g *ParallelConsumerGroup) Run(ctx context.Context) {
	for i := 0; i < g.workers; i++ {
		go g.worker(ctx, i)
	}
	<-ctx.Done()
	close(g.out)
}

func (g *ParallelConsumerGroup) worker(ctx context.Context, id int) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     g.brokers,
		GroupID:     g.groupID,
		Topic:       g.topic,
		MinBytes:    1 << 20,
		MaxBytes:    10 << 20,
		MaxWait:     500_000_000, // 500ms
		StartOffset: kafka.FirstOffset,
	})
	defer r.Close()
	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			g.logger.Error("parallel consumer read", "worker", id, "error", err)
			continue
		}
		select {
		case g.out <- msg:
		case <-ctx.Done():
			return
		}
	}
}
