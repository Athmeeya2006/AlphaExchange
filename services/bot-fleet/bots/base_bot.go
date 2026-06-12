package bots

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/trade-eval/bot-fleet/client"
	"github.com/trade-eval/bot-fleet/telemetry"
)

// Deps bundles everything a bot needs to run.
type Deps struct {
	Client       *client.OrderBookClient
	Breaker      *client.CircuitBreaker
	Producer     *telemetry.Producer
	ContestantID string
	TestID       string
	Seq          *atomic.Int64
}

// RunBot dispatches to the persona implementation. It blocks until ctx is done.
func RunBot(ctx context.Context, botID, persona string, d Deps) {
	switch persona {
	case "market_maker":
		runMarketMaker(ctx, botID, d)
	case "aggressive_taker":
		runAggressiveTaker(ctx, botID, d)
	case "spammer":
		runSpammer(ctx, botID, d)
	case "whale":
		runWhale(ctx, botID, d)
	default:
		runMarketMaker(ctx, botID, d)
	}
}

func generateOrderID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "ord_" + hex.EncodeToString(b[:])
}

// jitter returns base ± pct% random jitter.
func jitter(base time.Duration, pct float64) time.Duration {
	maxDelta := int64(float64(base) * pct)
	if maxDelta <= 0 {
		return base
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(2*maxDelta))
	return base + time.Duration(n.Int64()-maxDelta)
}

func randFloat(min, max float64) float64 {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	frac := float64(n.Int64()) / 1_000_000.0
	return min + frac*(max-min)
}

// measureAndEmit builds a telemetry event and pushes it to Kafka.
// orderID must be the exact ID submitted to the C++ server so the shadow book
// can correlate limit orders with their subsequent cancels.
func (d Deps) measureAndEmit(orderID, botID, persona, orderType string, sentNs, ackedNs int64, price, qty float64, expected ExpectedFill, actualStatus string, actualPrice, actualQty float64, correct, timedOut bool) {
	ev := telemetry.Event{
		ContestantID:   d.ContestantID,
		TestID:         d.TestID,
		BotID:          botID,
		BotPersona:     persona,
		OrderID:        orderID,
		SentAtNs:       sentNs,
		AckedAtNs:      ackedNs,
		LatencyUs:      (ackedNs - sentNs) / 1000,
		OrderType:      orderType,
		Price:          price,
		Quantity:       qty,
		ExpectedFill:   telemetry.Fill{Price: expected.Price, Quantity: expected.Quantity, Status: expected.Status},
		ActualFill:     telemetry.Fill{Price: actualPrice, Quantity: actualQty, Status: actualStatus},
		Correct:        correct,
		TimedOut:       timedOut,
		SequenceNumber: d.Seq.Add(1),
	}
	d.Producer.Emit(ev)
}
