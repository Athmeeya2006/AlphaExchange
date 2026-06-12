package metrics

import (
	"testing"

	"github.com/trade-eval/telemetry-ingester/model"
)

func TestCorrectnessRate(t *testing.T) {
	r := NewRegistry()
	a := r.Get("c1")
	for i := 0; i < 500; i++ {
		a.ProcessEvent(model.TelemetryEvent{ContestantID: "c1", LatencyUs: 100}, model.ValidationResult{Correct: true})
	}
	for i := 0; i < 500; i++ {
		a.ProcessEvent(model.TelemetryEvent{ContestantID: "c1", LatencyUs: 100}, model.ValidationResult{Correct: false})
	}
	s := a.GetSnapshot()
	if s.CorrectnessRate < 0.49 || s.CorrectnessRate > 0.51 {
		t.Fatalf("expected ~0.5, got %v", s.CorrectnessRate)
	}
}

func TestTPSCounter_Counts(t *testing.T) {
	c := NewTPSCounter()
	for i := 0; i < 1000; i++ {
		c.Record("c1")
	}
	if peak := c.GetPeakTPS("c1"); peak < 1 {
		t.Fatalf("expected peak >= 1, got %v", peak)
	}
}
