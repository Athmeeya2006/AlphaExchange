package main

import (
	"sort"
	"sync"
	"time"

	"github.com/trade-eval/telemetry-ingester/model"
)

// ReorderBuffer holds events briefly and emits them sorted by sequence number,
// correcting for out-of-order delivery across distributed bots.
type ReorderBuffer struct {
	mu      sync.Mutex
	pending []model.TelemetryEvent
	flushFn func([]model.TelemetryEvent)
	hold    time.Duration
}

// NewReorderBuffer constructs a buffer that flushes sorted batches every hold.
func NewReorderBuffer(hold time.Duration, flushFn func([]model.TelemetryEvent)) *ReorderBuffer {
	return &ReorderBuffer{flushFn: flushFn, hold: hold}
}

// Add enqueues an event.
func (rb *ReorderBuffer) Add(e model.TelemetryEvent) {
	rb.mu.Lock()
	rb.pending = append(rb.pending, e)
	rb.mu.Unlock()
}

// Run flushes sorted batches until ctx is done.
func (rb *ReorderBuffer) Run(stop <-chan struct{}) {
	t := time.NewTicker(rb.hold)
	defer t.Stop()
	for {
		select {
		case <-stop:
			rb.flush()
			return
		case <-t.C:
			rb.flush()
		}
	}
}

func (rb *ReorderBuffer) flush() {
	rb.mu.Lock()
	if len(rb.pending) == 0 {
		rb.mu.Unlock()
		return
	}
	batch := rb.pending
	rb.pending = nil
	rb.mu.Unlock()

	sort.SliceStable(batch, func(i, j int) bool {
		return batch[i].SequenceNumber < batch[j].SequenceNumber
	})
	rb.flushFn(batch)
}
