package main

import "testing"

func TestCompositeScore_Normalization(t *testing.T) {
	all := []LatencyWindow{
		{ContestantID: "a", TPS: 9000, P99Us: 450, CorrectnessRate: 0.999},
		{ContestantID: "b", TPS: 7500, P99Us: 380, CorrectnessRate: 0.998},
		{ContestantID: "c", TPS: 11000, P99Us: 620, CorrectnessRate: 0.995},
		{ContestantID: "d", TPS: 8000, P99Us: 900, CorrectnessRate: 0.990},
		{ContestantID: "e", TPS: 5000, P99Us: 1200, CorrectnessRate: 0.999},
	}
	for _, m := range all {
		s := computeCompositeScore(m, all)
		if s < 0 || s > 100 {
			t.Fatalf("%s out of range: %v", m.ContestantID, s)
		}
	}

	// Highest TPS contestant should get normalized_tps close to 1.
	cScore := computeCompositeScore(all[2], all) // c has max TPS
	eScore := computeCompositeScore(all[4], all) // e has min TPS and worst latency
	if eScore >= cScore {
		t.Fatalf("expected e (%v) < c (%v)", eScore, cScore)
	}
}

func TestCompositeScore_SingleContestant(t *testing.T) {
	m := LatencyWindow{TPS: 100, P99Us: 100, CorrectnessRate: 1.0}
	s := computeCompositeScore(m, []LatencyWindow{m})
	if s != 60 {
		t.Fatalf("expected 60, got %v", s)
	}
}

func TestLegalTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
	}{
		{"pending", "running", true},
		{"running", "stopping", true},
		{"stopping", "completed", true},
		{"running", "failed", true},
		{"completed", "running", false},
		{"pending", "completed", false},
	}
	for _, c := range cases {
		if got := legalTransitions[c.from][c.to]; got != c.ok {
			t.Errorf("%s->%s: got %v want %v", c.from, c.to, got, c.ok)
		}
	}
}
