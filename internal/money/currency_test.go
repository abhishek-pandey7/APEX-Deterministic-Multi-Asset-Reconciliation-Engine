package money

import "testing"

func TestLookupCurrency(t *testing.T) {
	tests := []struct {
		code string
		exp  uint8
	}{
		{"USD", 2},
		{"JPY", 0},
		{"KWD", 3},
	}
	for _, tt := range tests {
		c, err := LookupCurrency(tt.code)
		if err != nil {
			t.Fatalf("LookupCurrency(%q): %v", tt.code, err)
		}
		if c.Exponent != tt.exp {
			t.Errorf("LookupCurrency(%q).Exponent = %d, want %d", tt.code, c.Exponent, tt.exp)
		}
	}
}

func TestLookupCurrencyRejectsUnknown(t *testing.T) {
	for _, code := range []string{"", "usd", "XXX", "USDX"} {
		if _, err := LookupCurrency(code); err == nil {
			t.Errorf("LookupCurrency(%q) succeeded, want error", code)
		}
	}
}
