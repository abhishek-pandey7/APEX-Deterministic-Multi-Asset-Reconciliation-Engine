package money

import (
	"errors"
	"math"
	"testing"
)

func TestMarketValue(t *testing.T) {
	usd, _ := LookupCurrency("USD")
	jpy, _ := LookupCurrency("JPY")
	kwd, _ := LookupCurrency("KWD")

	tests := []struct {
		name       string
		qty, price string
		ccy        Currency
		want       int64
	}{
		{"whole shares", "100", "101.25", usd, 1012500},
		{"fractional shares", "0.5", "10.02", usd, 501},
		{"half rounds to even (down)", "1", "0.025", usd, 2},
		{"half rounds to even (up)", "1", "0.035", usd, 4},
		{"negative half rounds to even", "-1", "0.035", usd, -4},
		{"above half rounds up", "1", "0.02500001", usd, 3},
		{"zero exponent", "3", "1234.5", jpy, 3704},
		{"three decimal currency", "2", "1.2345", kwd, 2469},
		{"short position", "-250", "40", usd, -1000000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := ParseFixed(tt.qty, QuantityScale)
			if err != nil {
				t.Fatal(err)
			}
			p, err := ParseFixed(tt.price, PriceScale)
			if err != nil {
				t.Fatal(err)
			}
			got, err := MarketValue(q, p, tt.ccy)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("MarketValue(%s × %s %s) = %d, want %d", tt.qty, tt.price, tt.ccy, got, tt.want)
			}
		})
	}
}

func TestMarketValueOverflow(t *testing.T) {
	usd, _ := LookupCurrency("USD")
	if _, err := MarketValue(math.MaxInt64, math.MaxInt64, usd); !errors.Is(err, ErrRange) {
		t.Errorf("MarketValue overflow error = %v, want ErrRange", err)
	}
}
