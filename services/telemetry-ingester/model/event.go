// Package model holds the shared telemetry event schema.
package model

// Fill mirrors proto Fill.
type Fill struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Status   string  `json:"status"`
}

// TelemetryEvent is the JSON consumed from the bot-telemetry topic.
type TelemetryEvent struct {
	ContestantID   string  `json:"contestant_id"`
	TestID         string  `json:"test_id"`
	BotID          string  `json:"bot_id"`
	BotPersona     string  `json:"bot_persona"`
	OrderID        string  `json:"order_id"`
	SentAtNs       int64   `json:"sent_at_ns"`
	AckedAtNs      int64   `json:"acked_at_ns"`
	LatencyUs      int64   `json:"latency_us"`
	OrderType      string  `json:"order_type"`
	Price          float64 `json:"price"`
	Quantity       float64 `json:"quantity"`
	ExpectedFill   Fill    `json:"expected_fill"`
	ActualFill     Fill    `json:"actual_fill"`
	Correct        bool    `json:"correct"`
	TimedOut       bool    `json:"timed_out"`
	BotError       bool    `json:"bot_error"`
	SequenceNumber int64   `json:"sequence_number"`
}

// ValidationResult is the per-event correctness verdict.
type ValidationResult struct {
	Correct bool
	Reason  string
}
