package scorer

import "testing"

func TestComputeScores_Rankings(t *testing.T) {
	entries := []Entry{
		{ContestantID: "alice", TPS: 9000, P99Us: 450, CorrectnessRate: 0.999},
		{ContestantID: "carol", TPS: 11000, P99Us: 620, CorrectnessRate: 0.995},
		{ContestantID: "eve", TPS: 5000, P99Us: 1200, CorrectnessRate: 0.999},
	}
	ComputeScores(entries)
	for _, e := range entries {
		if e.Score < 0 || e.Score > 100 {
			t.Fatalf("%s out of range %v", e.ContestantID, e.Score)
		}
	}
	var lowest Entry = entries[0]
	for _, e := range entries {
		if e.Score < lowest.Score {
			lowest = e
		}
	}
	if lowest.ContestantID != "eve" {
		t.Fatalf("expected eve lowest, got %s", lowest.ContestantID)
	}
}

func TestComputeScores_TieBreakerData(t *testing.T) {
	entries := []Entry{
		{ContestantID: "a", TPS: 100, P99Us: 100, CorrectnessRate: 1.0},
		{ContestantID: "b", TPS: 100, P99Us: 100, CorrectnessRate: 0.99},
	}
	ComputeScores(entries)
	if entries[0].Score < entries[1].Score {
		t.Fatal("higher correctness should not score lower with equal tps/p99")
	}
}
