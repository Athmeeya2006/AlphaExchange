package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrConcurrentModification means another instance won the optimistic CAS.
var ErrConcurrentModification = errors.New("concurrent modification")

// ErrIllegalTransition means the requested transition is not allowed.
var ErrIllegalTransition = errors.New("illegal state transition")

var legalTransitions = map[string]map[string]bool{
	"pending":  {"running": true, "failed": true},
	"running":  {"stopping": true, "failed": true},
	"stopping": {"completed": true, "failed": true},
}

// TestStateMachine coordinates the lifecycle of every test.
type TestStateMachine struct {
	cfg      Config
	repo     *TestRepo
	redis    *redis.Client
	kafka    *Producer
	metrics  *MetricsWriter
	logger   *slog.Logger
	instance string

	mu     sync.Mutex
	timers map[string]*time.Timer // testID -> stop timer
	active map[string]string      // testID -> contestantID (running set)
}

// NewStateMachine constructs the state machine.
func NewStateMachine(cfg Config, repo *TestRepo, rdb *redis.Client, kp *Producer, mw *MetricsWriter, logger *slog.Logger) *TestStateMachine {
	return &TestStateMachine{
		cfg:      cfg,
		repo:     repo,
		redis:    rdb,
		kafka:    kp,
		metrics:  mw,
		logger:   logger,
		instance: cfg.InstanceID,
		timers:   make(map[string]*time.Timer),
		active:   make(map[string]string),
	}
}

// TransitionTo performs a validated, optimistically-locked status transition.
func (sm *TestStateMachine) TransitionTo(ctx context.Context, testID, newState, reason string) error {
	current, err := sm.repo.CurrentStatus(ctx, testID)
	if err != nil {
		return fmt.Errorf("read status: %w", err)
	}
	if !legalTransitions[current][newState] {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, current, newState)
	}
	ok, err := sm.repo.CASStatus(ctx, testID, current, newState, sm.instance)
	if err != nil {
		return fmt.Errorf("cas status: %w", err)
	}
	if !ok {
		return ErrConcurrentModification
	}
	if sm.redis != nil {
		sm.redis.HSet(ctx, "test:"+testID, "status", newState)
	}
	if reason != "" {
		_ = sm.repo.SetFailureReason(ctx, testID, reason)
	}
	sm.logger.Info("state transition", "test_id", testID, "from", current, "to", newState)
	return nil
}

// StartTest acquires the per-contestant lock and starts the test.
func (sm *TestStateMachine) StartTest(ctx context.Context, ev StartTestEvent) error {
	duration := ev.DurationSeconds
	if duration <= 0 {
		duration = sm.cfg.DefaultDurationSec
	}
	lockTTL := time.Duration(duration+30) * time.Second

	if sm.redis != nil {
		ok, err := sm.redis.SetNX(ctx, "lock:test:"+ev.ContestantID, ev.TestID, lockTTL).Result()
		if err != nil {
			return fmt.Errorf("acquire lock: %w", err)
		}
		if !ok {
			return ErrConcurrentModification
		}
	}

	if err := sm.TransitionTo(ctx, ev.TestID, "running", ""); err != nil {
		sm.releaseLock(ctx, ev.ContestantID)
		return err
	}

	if sm.metrics != nil {
		_ = sm.metrics.ClearMetrics(ctx, ev.ContestantID)
	}

	// Look up contestant name and seed it into the metrics hash so the
	// leaderboard can display the real name immediately (before the telemetry
	// ingester writes any snapshot of its own).
	contestantName := sm.repo.GetContestantName(ctx, ev.ContestantID)
	if sm.redis != nil && contestantName != "" {
		sm.redis.HSet(ctx, "metrics:"+ev.ContestantID, "contestant_name", contestantName)
	}

	if sm.redis != nil {
		sm.redis.HSet(ctx, "test:"+ev.TestID,
			"started_at", time.Now().UnixNano(),
			"target_ip", ev.TargetIP,
			"target_port", ev.TargetPort,
			"contestant_id", ev.ContestantID,
		)
		sm.redis.Set(ctx, "orchestrator_heartbeat:"+ev.TestID, sm.instance, 30*time.Second)
	}

	// Forward START_TEST to the bot-fleet.
	sm.publish(ctx, sm.cfg.BotFleetTopic, ev.ContestantID, ev)

	testsStarted.Inc()
	sm.mu.Lock()
	sm.active[ev.TestID] = ev.ContestantID
	activeTestsGauge.Set(float64(len(sm.active)))
	sm.timers[ev.TestID] = time.AfterFunc(time.Duration(duration)*time.Second, func() {
		bg := context.Background()
		if err := sm.StopTest(bg, ev.TestID, "duration_elapsed"); err != nil {
			sm.logger.Warn("auto-stop failed", "test_id", ev.TestID, "error", err)
		}
	})
	sm.mu.Unlock()

	sm.logger.Info("test started", "test_id", ev.TestID, "duration_s", duration)
	return nil
}

// StopTest drains bots, finalizes the score, and completes the test.
func (sm *TestStateMachine) StopTest(ctx context.Context, testID, reason string) error {
	sm.cancelTimer(testID)

	if err := sm.TransitionTo(ctx, testID, "stopping", ""); err != nil {
		return err
	}

	contestantID := sm.contestantFor(ctx, testID)
	sm.publish(ctx, sm.cfg.OrchEventsTopic, contestantID, StopTestEvent{Event: "STOP_TEST", TestID: testID, Reason: reason})

	// Give telemetry a chance to flush.
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
	}

	score := 0.0
	if sm.metrics != nil && contestantID != "" {
		window, name := sm.metrics.readWindow(ctx, contestantID)
		score = computeCompositeScore(window, []LatencyWindow{window})
		_ = sm.metrics.PublishFinalScore(ctx, contestantID, name, window, score)
	}

	if err := sm.TransitionTo(ctx, testID, "completed", ""); err != nil {
		return err
	}
	_ = sm.repo.MarkEnded(ctx, testID, score)
	sm.finish(ctx, testID, contestantID)
	testsCompleted.Inc()
	sm.logger.Info("test completed", "test_id", testID, "score", score)
	return nil
}

// FailTest marks a test failed for the given reason.
func (sm *TestStateMachine) FailTest(ctx context.Context, testID, reason string) error {
	sm.cancelTimer(testID)
	contestantID := sm.contestantFor(ctx, testID)
	if err := sm.TransitionTo(ctx, testID, "failed", reason); err != nil {
		return err
	}
	sm.publish(ctx, sm.cfg.OrchEventsTopic, contestantID, StopTestEvent{Event: "STOP_TEST", TestID: testID, Reason: reason})
	sm.finish(ctx, testID, contestantID)
	testsFailed.Inc()
	sm.logger.Warn("test failed", "test_id", testID, "reason", reason)
	return nil
}

// ActiveTests returns a snapshot of locally-owned running tests.
func (sm *TestStateMachine) ActiveTests() map[string]string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make(map[string]string, len(sm.active))
	for k, v := range sm.active {
		out[k] = v
	}
	return out
}

// RegisterRecoveredTimer reinstalls a stop timer during crash recovery.
func (sm *TestStateMachine) RegisterRecoveredTimer(testID, contestantID string, remaining time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.active[testID] = contestantID
	sm.timers[testID] = time.AfterFunc(remaining, func() {
		_ = sm.StopTest(context.Background(), testID, "duration_elapsed_after_recovery")
	})
}

func (sm *TestStateMachine) cancelTimer(testID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if t, ok := sm.timers[testID]; ok {
		t.Stop()
		delete(sm.timers, testID)
	}
}

func (sm *TestStateMachine) finish(ctx context.Context, testID, contestantID string) {
	sm.mu.Lock()
	delete(sm.active, testID)
	delete(sm.timers, testID)
	activeTestsGauge.Set(float64(len(sm.active)))
	sm.mu.Unlock()
	sm.releaseLock(ctx, contestantID)
}

func (sm *TestStateMachine) releaseLock(ctx context.Context, contestantID string) {
	if sm.redis != nil && contestantID != "" {
		sm.redis.Del(ctx, "lock:test:"+contestantID)
	}
}

func (sm *TestStateMachine) contestantFor(ctx context.Context, testID string) string {
	sm.mu.Lock()
	if c, ok := sm.active[testID]; ok {
		sm.mu.Unlock()
		return c
	}
	sm.mu.Unlock()
	if sm.redis != nil {
		if c, err := sm.redis.HGet(ctx, "test:"+testID, "contestant_id").Result(); err == nil {
			return c
		}
	}
	return ""
}

func (sm *TestStateMachine) publish(ctx context.Context, topic, key string, payload any) {
	if sm.kafka == nil {
		return
	}
	if b, err := json.Marshal(payload); err == nil {
		if err := sm.kafka.Produce(ctx, topic, []byte(key), b); err != nil {
			sm.logger.Warn("publish failed", "topic", topic, "error", err)
		}
	}
}

func atoiI64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
