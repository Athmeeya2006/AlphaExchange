// Package metrics aggregates per-contestant performance.
package metrics

import (
	"sync"
	"time"
)

// TPSCounter tracks orders-per-second per contestant using 60 one-second buckets.
type TPSCounter struct {
	mu      sync.Mutex
	windows map[string]*[60]int64
	lastSec map[string]int64
}

// NewTPSCounter constructs the counter.
func NewTPSCounter() *TPSCounter {
	return &TPSCounter{
		windows: make(map[string]*[60]int64),
		lastSec: make(map[string]int64),
	}
}

// Record increments the current-second bucket for a contestant.
func (t *TPSCounter) Record(contestantID string) {
	sec := time.Now().Unix()
	idx := sec % 60
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.windows[contestantID]
	if w == nil {
		w = new([60]int64)
		t.windows[contestantID] = w
	}
	if t.lastSec[contestantID] != sec {
		w[idx] = 0
		t.lastSec[contestantID] = sec
	}
	w[idx]++
}

// GetCurrentTPS returns the smoothed TPS over the last 5 seconds.
func (t *TPSCounter) GetCurrentTPS(contestantID string) float64 {
	sec := time.Now().Unix()
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.windows[contestantID]
	if w == nil {
		return 0
	}
	var sum int64
	for i := int64(1); i <= 5; i++ {
		sum += w[((sec-i)%60+60)%60]
	}
	return float64(sum) / 5.0
}

// GetPeakTPS returns the max single-second count observed in the window.
func (t *TPSCounter) GetPeakTPS(contestantID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.windows[contestantID]
	if w == nil {
		return 0
	}
	var max int64
	for _, c := range w {
		if c > max {
			max = c
		}
	}
	return float64(max)
}
