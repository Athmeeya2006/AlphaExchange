// Package shadowbook is the authoritative price-time-priority matching engine
// used to validate contestant fills.
package shadowbook

import (
	"sort"
	"sync"
)

// FillStatus values.
const (
	StatusFilled    = "FILLED"
	StatusPartial   = "PARTIAL"
	StatusPending   = "PENDING"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// Order is a tracked order.
type Order struct {
	ID           string
	ContestantID string
	Type         string
	Price        float64
	Quantity     float64
	RemainingQty float64
	SubmittedAt  int64 // sequence number, not wall clock
}

// ExpectedFill is the reference outcome for an order.
type ExpectedFill struct {
	Status       string
	FilledPrice  float64
	FilledQty    float64
	RemainingQty float64
}

type priceLevel struct {
	Price  float64
	Orders []*Order
}

// OrderBook is a price-time priority matching engine.
type OrderBook struct {
	mu     sync.RWMutex
	bids   []priceLevel // descending
	asks   []priceLevel // ascending
	orders map[string]*Order
}

// NewOrderBook constructs an empty book.
func NewOrderBook() *OrderBook {
	return &OrderBook{orders: make(map[string]*Order)}
}

// ProcessOrder matches/rests an order and returns the expected fill.
func (b *OrderBook) ProcessOrder(o Order) ExpectedFill {
	b.mu.Lock()
	defer b.mu.Unlock()
	ord := &o
	ord.RemainingQty = o.Quantity
	switch o.Type {
	case "MARKET_BUY":
		return b.matchMarket(ord, &b.asks)
	case "MARKET_SELL":
		return b.matchMarket(ord, &b.bids)
	case "LIMIT_BUY":
		f := b.matchLimit(ord, &b.asks, true)
		if ord.RemainingQty > 0 {
			b.insert(&b.bids, ord, false)
		}
		return f
	case "LIMIT_SELL":
		f := b.matchLimit(ord, &b.bids, false)
		if ord.RemainingQty > 0 {
			b.insert(&b.asks, ord, true)
		}
		return f
	case "CANCEL":
		if b.cancel(o.ID) {
			return ExpectedFill{Status: StatusCancelled}
		}
		return ExpectedFill{Status: "NOT_FOUND"}
	}
	return ExpectedFill{Status: StatusRejected}
}

func (b *OrderBook) matchMarket(o *Order, levels *[]priceLevel) ExpectedFill {
	filled, lastPx := 0.0, 0.0
	for len(*levels) > 0 && o.RemainingQty > 0 {
		lvl := &(*levels)[0]
		for len(lvl.Orders) > 0 && o.RemainingQty > 0 {
			resting := lvl.Orders[0]
			m := minF(o.RemainingQty, resting.RemainingQty)
			filled += m
			lastPx = lvl.Price
			o.RemainingQty -= m
			resting.RemainingQty -= m
			if resting.RemainingQty == 0 {
				delete(b.orders, resting.ID)
				lvl.Orders = lvl.Orders[1:]
			}
		}
		if len(lvl.Orders) == 0 {
			*levels = (*levels)[1:]
		}
	}
	if filled == 0 {
		return ExpectedFill{Status: StatusRejected}
	}
	if o.RemainingQty > 0 {
		return ExpectedFill{Status: StatusPartial, FilledPrice: lastPx, FilledQty: filled, RemainingQty: o.RemainingQty}
	}
	return ExpectedFill{Status: StatusFilled, FilledPrice: lastPx, FilledQty: filled}
}

func (b *OrderBook) matchLimit(o *Order, levels *[]priceLevel, isBuy bool) ExpectedFill {
	filled, lastPx := 0.0, 0.0
	for len(*levels) > 0 && o.RemainingQty > 0 {
		lvl := &(*levels)[0]
		crosses := (isBuy && lvl.Price <= o.Price) || (!isBuy && lvl.Price >= o.Price)
		if !crosses {
			break
		}
		for len(lvl.Orders) > 0 && o.RemainingQty > 0 {
			resting := lvl.Orders[0]
			m := minF(o.RemainingQty, resting.RemainingQty)
			filled += m
			lastPx = lvl.Price
			o.RemainingQty -= m
			resting.RemainingQty -= m
			if resting.RemainingQty == 0 {
				delete(b.orders, resting.ID)
				lvl.Orders = lvl.Orders[1:]
			}
		}
		if len(lvl.Orders) == 0 {
			*levels = (*levels)[1:]
		}
	}
	if filled == 0 {
		return ExpectedFill{Status: StatusPending, RemainingQty: o.RemainingQty}
	}
	if o.RemainingQty > 0 {
		return ExpectedFill{Status: StatusPartial, FilledPrice: lastPx, FilledQty: filled, RemainingQty: o.RemainingQty}
	}
	return ExpectedFill{Status: StatusFilled, FilledPrice: lastPx, FilledQty: filled}
}

func (b *OrderBook) insert(levels *[]priceLevel, o *Order, ascending bool) {
	b.orders[o.ID] = o
	for i := range *levels {
		if (*levels)[i].Price == o.Price {
			(*levels)[i].Orders = append((*levels)[i].Orders, o) // FIFO by arrival
			return
		}
	}
	*levels = append(*levels, priceLevel{Price: o.Price, Orders: []*Order{o}})
	sort.SliceStable(*levels, func(i, j int) bool {
		if ascending {
			return (*levels)[i].Price < (*levels)[j].Price
		}
		return (*levels)[i].Price > (*levels)[j].Price
	})
}

func (b *OrderBook) cancel(orderID string) bool {
	o, ok := b.orders[orderID]
	if !ok {
		return false
	}
	delete(b.orders, orderID)
	for _, set := range []*[]priceLevel{&b.bids, &b.asks} {
		for i := range *set {
			for j, ord := range (*set)[i].Orders {
				if ord.ID == orderID {
					(*set)[i].Orders = append((*set)[i].Orders[:j], (*set)[i].Orders[j+1:]...)
					return true
				}
			}
		}
	}
	_ = o
	return true
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
