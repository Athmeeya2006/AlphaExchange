package kafka

import "sync"

// OffsetTracker records the last processed offset per partition so offsets are
// committed only after a successful downstream write (at-least-once delivery).
type OffsetTracker struct {
	mu      sync.Mutex
	pending map[int]int64
}

// NewOffsetTracker constructs the tracker.
func NewOffsetTracker() *OffsetTracker {
	return &OffsetTracker{pending: make(map[int]int64)}
}

// MarkProcessed records the highest processed offset for a partition.
func (t *OffsetTracker) MarkProcessed(partition int, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if offset > t.pending[partition] {
		t.pending[partition] = offset
	}
}

// Snapshot returns and clears the pending offsets (call after a successful commit).
func (t *OffsetTracker) Snapshot() map[int]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[int]int64, len(t.pending))
	for k, v := range t.pending {
		out[k] = v
	}
	t.pending = make(map[int]int64)
	return out
}
