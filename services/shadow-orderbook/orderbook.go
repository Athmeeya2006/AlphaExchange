// Package shadoworderbook is a standalone, price-time-priority reference
// matching engine. It is the canonical definition of "correct" behaviour that
// the telemetry ingester validates contestants against.
package shadoworderbook

import "sort"

// Status values for fills.
const (
	StatusFilled    = "FILLED"
	StatusPartial   = "PARTIAL"
	StatusPending   = "PENDING"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
	StatusNotFound  = "NOT_FOUND"
)

// Order is a tracked order.
type Order struct {
	ID           string
	Type         string // LIMIT_BUY/LIMIT_SELL/MARKET_BUY/MARKET_SELL
	Price        float64
	Quantity     float64
	RemainingQty float64
	Seq          int64 // arrival sequence for time priority
}

// Fill is the result of submitting an order.
type Fill struct {
	Status       string
	Price        float64
	Quantity     float64
	RemainingQty float64
}

type level struct {
	Price  float64
	Orders []*Order
}

// OrderBook is a price-time priority matching engine. Not safe for concurrent
// use; callers serialize access (the ingester does so per contestant).
type OrderBook struct {
	bids   []level
	asks   []level
	lookup map[string]*Order
}

// New constructs an empty order book.
func New() *OrderBook { return &OrderBook{lookup: make(map[string]*Order)} }

// Submit processes an order and returns its fill.
func (b *OrderBook) Submit(o Order) Fill {
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
			b.rest(&b.bids, ord, false)
		}
		return f
	case "LIMIT_SELL":
		f := b.matchLimit(ord, &b.bids, false)
		if ord.RemainingQty > 0 {
			b.rest(&b.asks, ord, true)
		}
		return f
	}
	return Fill{Status: StatusRejected}
}

// Cancel removes a resting order.
func (b *OrderBook) Cancel(orderID string) Fill {
	if _, ok := b.lookup[orderID]; !ok {
		return Fill{Status: StatusNotFound}
	}
	delete(b.lookup, orderID)
	for _, set := range []*[]level{&b.bids, &b.asks} {
		for i := range *set {
			for j, o := range (*set)[i].Orders {
				if o.ID == orderID {
					(*set)[i].Orders = append((*set)[i].Orders[:j], (*set)[i].Orders[j+1:]...)
					return Fill{Status: StatusCancelled}
				}
			}
		}
	}
	return Fill{Status: StatusCancelled}
}

func (b *OrderBook) matchMarket(o *Order, levels *[]level) Fill {
	filled, last := 0.0, 0.0
	for len(*levels) > 0 && o.RemainingQty > 0 {
		lvl := &(*levels)[0]
		for len(lvl.Orders) > 0 && o.RemainingQty > 0 {
			r := lvl.Orders[0]
			m := minF(o.RemainingQty, r.RemainingQty)
			filled += m
			last = lvl.Price
			o.RemainingQty -= m
			r.RemainingQty -= m
			if r.RemainingQty == 0 {
				delete(b.lookup, r.ID)
				lvl.Orders = lvl.Orders[1:]
			}
		}
		if len(lvl.Orders) == 0 {
			*levels = (*levels)[1:]
		}
	}
	if filled == 0 {
		return Fill{Status: StatusRejected}
	}
	if o.RemainingQty > 0 {
		return Fill{Status: StatusPartial, Price: last, Quantity: filled, RemainingQty: o.RemainingQty}
	}
	return Fill{Status: StatusFilled, Price: last, Quantity: filled}
}

func (b *OrderBook) matchLimit(o *Order, levels *[]level, isBuy bool) Fill {
	filled, last := 0.0, 0.0
	for len(*levels) > 0 && o.RemainingQty > 0 {
		lvl := &(*levels)[0]
		if (isBuy && lvl.Price > o.Price) || (!isBuy && lvl.Price < o.Price) {
			break
		}
		for len(lvl.Orders) > 0 && o.RemainingQty > 0 {
			r := lvl.Orders[0]
			m := minF(o.RemainingQty, r.RemainingQty)
			filled += m
			last = lvl.Price
			o.RemainingQty -= m
			r.RemainingQty -= m
			if r.RemainingQty == 0 {
				delete(b.lookup, r.ID)
				lvl.Orders = lvl.Orders[1:]
			}
		}
		if len(lvl.Orders) == 0 {
			*levels = (*levels)[1:]
		}
	}
	if filled == 0 {
		return Fill{Status: StatusPending, RemainingQty: o.RemainingQty}
	}
	if o.RemainingQty > 0 {
		return Fill{Status: StatusPartial, Price: last, Quantity: filled, RemainingQty: o.RemainingQty}
	}
	return Fill{Status: StatusFilled, Price: last, Quantity: filled}
}

func (b *OrderBook) rest(levels *[]level, o *Order, ascending bool) {
	b.lookup[o.ID] = o
	for i := range *levels {
		if (*levels)[i].Price == o.Price {
			(*levels)[i].Orders = append((*levels)[i].Orders, o)
			return
		}
	}
	*levels = append(*levels, level{Price: o.Price, Orders: []*Order{o}})
	sort.SliceStable(*levels, func(i, j int) bool {
		if ascending {
			return (*levels)[i].Price < (*levels)[j].Price
		}
		return (*levels)[i].Price > (*levels)[j].Price
	})
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
