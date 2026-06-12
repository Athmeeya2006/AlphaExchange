package shadowbook

import (
	"sort"

	"github.com/trade-eval/telemetry-ingester/model"
)

// CorrectnessValidator replays events through the authoritative book in
// sequence order and compares expected vs actual fills.
type CorrectnessValidator struct {
	books      map[string]*OrderBook // per contestant
	lastTestID map[string]string     // contestant -> last seen test_id
}

// NewCorrectnessValidator constructs the validator.
func NewCorrectnessValidator() *CorrectnessValidator {
	return &CorrectnessValidator{
		books:      make(map[string]*OrderBook),
		lastTestID: make(map[string]string),
	}
}

// ValidateBatch processes events (sorted by sequence number) and returns a
// verdict per input event.
func (v *CorrectnessValidator) ValidateBatch(events []model.TelemetryEvent) []model.ValidationResult {
	sorted := make([]model.TelemetryEvent, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SequenceNumber < sorted[j].SequenceNumber
	})

	results := make([]model.ValidationResult, len(sorted))
	for i, e := range sorted {
		// Reset the shadow book when a new test starts for this contestant.
		// This keeps the shadow book in sync with the C++ orderbook which is
		// reset (via POST /reset) by the bot-fleet at the start of each test.
		if prev := v.lastTestID[e.ContestantID]; prev != e.TestID {
			v.books[e.ContestantID] = NewOrderBook()
			v.lastTestID[e.ContestantID] = e.TestID
		}

		book := v.books[e.ContestantID]
		expected := book.ProcessOrder(Order{
			ID:           e.OrderID,
			ContestantID: e.ContestantID,
			Type:         e.OrderType,
			Price:        e.Price,
			Quantity:     e.Quantity,
			SubmittedAt:  e.SequenceNumber,
		})
		results[i] = compare(expected, e.ActualFill)
	}
	return results
}

func compare(expected ExpectedFill, actual model.Fill) model.ValidationResult {
	// Normalize a missing actual status.
	if actual.Status == "" {
		return model.ValidationResult{Correct: false, Reason: "missing actual fill"}
	}

	// Cancel requests can legitimately return CANCELLED, NOT_FOUND (order was
	// already consumed by a concurrent fill), or ALREADY_FILLED - all valid.
	if expected.Status == StatusCancelled {
		if actual.Status == StatusCancelled || actual.Status == "NOT_FOUND" || actual.Status == "ALREADY_FILLED" {
			return model.ValidationResult{Correct: true}
		}
		return model.ValidationResult{Correct: false, Reason: "cancel status mismatch: " + actual.Status}
	}

	// Shadow book says NOT_FOUND for a cancel that arrived after the order was
	// consumed; the C++ server may have already removed it too - both valid.
	if expected.Status == "NOT_FOUND" {
		if actual.Status == "NOT_FOUND" || actual.Status == StatusCancelled || actual.Status == "ALREADY_FILLED" {
			return model.ValidationResult{Correct: true}
		}
		return model.ValidationResult{Correct: false, Reason: "not-found status mismatch: " + actual.Status}
	}

	// Market orders with no liquidity: REJECTED from shadow book and from the
	// C++ server should match, but PARTIAL is also fine (partial fill happened).
	if expected.Status == StatusRejected {
		if actual.Status == StatusRejected || actual.Status == StatusPartial || actual.Status == StatusFilled {
			return model.ValidationResult{Correct: true}
		}
		return model.ValidationResult{Correct: false, Reason: "market status mismatch: " + actual.Status}
	}

	if expected.Status != actual.Status {
		// Tolerate FILLED-expected vs PARTIAL-actual (partial fill is a subset).
		// Also tolerate PENDING-expected vs PARTIAL/FILLED-actual (order crossed
		// due to concurrent arrivals re-ordering at the C++ server).
		if expected.Status == StatusFilled && actual.Status == StatusPartial {
			// OK - partial fill
		} else if expected.Status == StatusPending && (actual.Status == StatusFilled || actual.Status == StatusPartial) {
			// OK - order crossed the book in the C++ server before we rested it
			return model.ValidationResult{Correct: true}
		} else if expected.Status == StatusFilled && actual.Status == StatusPending {
			// OK - shadow book matched but C++ hadn't seen the resting order yet
			return model.ValidationResult{Correct: true}
		} else {
			return model.ValidationResult{Correct: false, Reason: "status mismatch: expected " + expected.Status + " got " + actual.Status}
		}
	}
	if actual.Quantity < 0 {
		return model.ValidationResult{Correct: false, Reason: "negative quantity"}
	}
	if expected.Status == StatusFilled && actual.Quantity > expected.FilledQty+1e-9 {
		return model.ValidationResult{Correct: false, Reason: "overfill"}
	}
	if expected.Status == StatusFilled && expected.FilledPrice > 0 && actual.Price > 0 {
		if d := expected.FilledPrice - actual.Price; d > 0.01 || d < -0.01 {
			return model.ValidationResult{Correct: false, Reason: "wrong price"}
		}
	}
	return model.ValidationResult{Correct: true}
}
