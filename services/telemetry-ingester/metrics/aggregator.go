package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/trade-eval/telemetry-ingester/latency"
	"github.com/trade-eval/telemetry-ingester/model"
)

// Snapshot is a point-in-time view of a contestant's metrics.
type Snapshot struct {
	ContestantID    string
	P50LatencyUs    int64
	P90LatencyUs    int64
	P99LatencyUs    int64
	CurrentTPS      float64
	PeakTPS         float64
	TotalOrders     int64
	CorrectOrders   int64
	CorrectnessRate float64
}

// Aggregator combines latency, TPS, and correctness for one contestant.
type Aggregator struct {
	hist            *latency.SlidingWindowHistogram
	totalHist       *latency.HDRHistogram // cumulative all-time; never expires
	totalHistMu     sync.Mutex
	tps             *TPSCounter
	contestantID    string
	totalOrders     atomic.Int64
	correctOrders   atomic.Int64
	timedOutOrders  atomic.Int64
	incorrectOrders atomic.Int64
}

// NewAggregator constructs an aggregator sharing a TPS counter.
func NewAggregator(contestantID string, tps *TPSCounter) *Aggregator {
	return &Aggregator{
		hist:         latency.NewSlidingWindow(),
		totalHist:    latency.NewHDRHistogram(),
		tps:          tps,
		contestantID: contestantID,
	}
}

// Advance rotates the latency window (call once per second).
func (a *Aggregator) Advance() { a.hist.Advance() }

// ProcessEvent updates counters for one event.
func (a *Aggregator) ProcessEvent(e model.TelemetryEvent, v model.ValidationResult) {
	a.totalOrders.Add(1)
	a.tps.Record(e.ContestantID)
	a.hist.Record(e.LatencyUs)
	// Also record into the cumulative histogram so percentiles survive the
	// 30-second sliding window expiry (important when the Kafka backlog drains
	// slower than the window rotates).
	a.totalHistMu.Lock()
	a.totalHist.RecordValue(e.LatencyUs)
	a.totalHistMu.Unlock()
	switch {
	case e.TimedOut:
		a.timedOutOrders.Add(1)
	case v.Correct:
		a.correctOrders.Add(1)
	default:
		a.incorrectOrders.Add(1)
	}
}

// GetSnapshot returns the current aggregated metrics.
func (a *Aggregator) GetSnapshot() Snapshot {
	p50, p90, p99 := a.hist.GetPercentiles()
	// If the sliding window has expired (all buckets zeroed after 30 s of
	// inactivity), fall back to the cumulative histogram so the leaderboard
	// keeps showing real values even after a test ends.
	if p99 == 0 && a.totalOrders.Load() > 0 {
		a.totalHistMu.Lock()
		p50 = a.totalHist.Percentile(0.50)
		p90 = a.totalHist.Percentile(0.90)
		p99 = a.totalHist.Percentile(0.99)
		a.totalHistMu.Unlock()
	}
	total := a.totalOrders.Load()
	timedOut := a.timedOutOrders.Load()
	correct := a.correctOrders.Load()
	valid := total - timedOut
	rate := 0.0
	if valid > 0 {
		rate = float64(correct) / float64(valid)
	}
	return Snapshot{
		ContestantID:    a.contestantID,
		P50LatencyUs:    p50,
		P90LatencyUs:    p90,
		P99LatencyUs:    p99,
		CurrentTPS:      a.tps.GetCurrentTPS(a.contestantID),
		PeakTPS:         a.tps.GetPeakTPS(a.contestantID),
		TotalOrders:     total,
		CorrectOrders:   correct,
		CorrectnessRate: rate,
	}
}

// Registry holds per-contestant aggregators.
type Registry struct {
	mu   sync.RWMutex
	tps  *TPSCounter
	aggs map[string]*Aggregator
}

// NewRegistry constructs the registry.
func NewRegistry() *Registry {
	return &Registry{tps: NewTPSCounter(), aggs: make(map[string]*Aggregator)}
}

// Get returns (creating if needed) the aggregator for a contestant.
func (r *Registry) Get(contestantID string) *Aggregator {
	r.mu.RLock()
	a := r.aggs[contestantID]
	r.mu.RUnlock()
	if a != nil {
		return a
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if a = r.aggs[contestantID]; a == nil {
		a = NewAggregator(contestantID, r.tps)
		r.aggs[contestantID] = a
	}
	return a
}

// Snapshots returns a snapshot for every known contestant.
func (r *Registry) Snapshots() []Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Snapshot, 0, len(r.aggs))
	for _, a := range r.aggs {
		out = append(out, a.GetSnapshot())
	}
	return out
}

// AdvanceAll rotates every aggregator's latency window.
func (r *Registry) AdvanceAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.aggs {
		a.Advance()
	}
}
