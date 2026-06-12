package shadoworderbook

import "testing"

func TestLimitCross(t *testing.T) {
	b := New()
	b.Submit(Order{ID: "s1", Type: "LIMIT_SELL", Price: 100, Quantity: 10, Seq: 1})
	f := b.Submit(Order{ID: "b1", Type: "LIMIT_BUY", Price: 100, Quantity: 10, Seq: 2})
	if f.Status != StatusFilled || f.Price != 100 || f.Quantity != 10 {
		t.Fatalf("got %+v", f)
	}
}

func TestPartial(t *testing.T) {
	b := New()
	b.Submit(Order{ID: "s1", Type: "LIMIT_SELL", Price: 100, Quantity: 5, Seq: 1})
	f := b.Submit(Order{ID: "b1", Type: "LIMIT_BUY", Price: 100, Quantity: 10, Seq: 2})
	if f.Status != StatusPartial || f.Quantity != 5 || f.RemainingQty != 5 {
		t.Fatalf("got %+v", f)
	}
}

func TestMarketBestPriceFirst(t *testing.T) {
	b := New()
	b.Submit(Order{ID: "s1", Type: "LIMIT_SELL", Price: 102, Quantity: 5, Seq: 1})
	b.Submit(Order{ID: "s2", Type: "LIMIT_SELL", Price: 100, Quantity: 5, Seq: 2})
	f := b.Submit(Order{ID: "m1", Type: "MARKET_BUY", Quantity: 5, Seq: 3})
	if f.Price != 100 {
		t.Fatalf("expected best price 100 first, got %v", f.Price)
	}
}

func TestCancel(t *testing.T) {
	b := New()
	b.Submit(Order{ID: "b1", Type: "LIMIT_BUY", Price: 50, Quantity: 5, Seq: 1})
	if f := b.Cancel("b1"); f.Status != StatusCancelled {
		t.Fatalf("got %+v", f)
	}
	if f := b.Cancel("b1"); f.Status != StatusNotFound {
		t.Fatalf("re-cancel got %+v", f)
	}
}
