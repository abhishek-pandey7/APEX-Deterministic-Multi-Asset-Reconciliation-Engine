package money

import (
	"errors"
	"math"
	"testing"
)

func TestParseFixed(t *testing.T) {
	tests := []struct {
		in    string
		scale uint8
		want  int64
	}{
		{"0", 2, 0},
		{"-0.00", 2, 0},
		{"1234.5", 2, 123450},
		{"-1234.50", 2, -123450},
		{"+7", 2, 700},
		{".25", 2, 25},
		{"5.", 2, 500},
		{"12.3400", 2, 1234},
		{"1500", 0, 1500},
		{"0.125", 3, 125},
		{"1.000001", QuantityScale, 1000001},
		{"101.25", PriceScale, 10125000000},
		{"9223372036854775807", 0, math.MaxInt64},
		{"-9223372036854775808", 0, math.MinInt64},
		{"92233720368547758.07", 2, math.MaxInt64},
	}
	for _, tt := range tests {
		got, err := ParseFixed(tt.in, tt.scale)
		if err != nil {
			t.Errorf("ParseFixed(%q, %d): unexpected error %v", tt.in, tt.scale, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseFixed(%q, %d) = %d, want %d", tt.in, tt.scale, got, tt.want)
		}
	}
}

func TestParseFixedErrors(t *testing.T) {
	tests := []struct {
		in    string
		scale uint8
		want  error
	}{
		{"", 2, ErrSyntax},
		{"-", 2, ErrSyntax},
		{".", 2, ErrSyntax},
		{"1.2.3", 2, ErrSyntax},
		{"1,234.50", 2, ErrSyntax},
		{"1e3", 2, ErrSyntax},
		{" 12", 2, ErrSyntax},
		{"--1", 2, ErrSyntax},
		{"12.345", 2, ErrPrecision},
		{"1.5", 0, ErrPrecision},
		{"9223372036854775808", 0, ErrRange},
		{"-9223372036854775809", 0, ErrRange},
		{"92233720368547758.08", 2, ErrRange},
	}
	for _, tt := range tests {
		_, err := ParseFixed(tt.in, tt.scale)
		if !errors.Is(err, tt.want) {
			t.Errorf("ParseFixed(%q, %d) error = %v, want %v", tt.in, tt.scale, err, tt.want)
		}
	}
}

func TestFormatFixed(t *testing.T) {
	tests := []struct {
		v     int64
		scale uint8
		want  string
	}{
		{0, 2, "0.00"},
		{5, 2, "0.05"},
		{-5, 2, "-0.05"},
		{123450, 2, "1234.50"},
		{1500, 0, "1500"},
		{125, 3, "0.125"},
		{math.MinInt64, 2, "-92233720368547758.08"},
	}
	for _, tt := range tests {
		if got := FormatFixed(tt.v, tt.scale); got != tt.want {
			t.Errorf("FormatFixed(%d, %d) = %q, want %q", tt.v, tt.scale, got, tt.want)
		}
	}
}

func FuzzParseFormatRoundTrip(f *testing.F) {
	for _, seed := range []string{"0", "-1234.50", "0.001", "9223372036854775807", ".5"} {
		f.Add(seed, uint8(2))
	}
	f.Fuzz(func(t *testing.T, s string, scale uint8) {
		scale %= maxScale + 1
		v, err := ParseFixed(s, scale)
		if err != nil {
			return
		}
		out := FormatFixed(v, scale)
		back, err := ParseFixed(out, scale)
		if err != nil || back != v {
			t.Fatalf("round trip %q -> %d -> %q -> %d (%v)", s, v, out, back, err)
		}
	})
}

func TestFormatGrouped(t *testing.T) {
	tests := []struct {
		v     int64
		scale uint8
		want  string
	}{
		{0, 2, "0.00"},
		{99999, 2, "999.99"},
		{100000, 2, "1,000.00"},
		{-436410876, 2, "-4,364,108.76"},
		{1064269742, 3, "1,064,269.742"},
		{1234567, 0, "1,234,567"},
		{math.MinInt64, 0, "-9,223,372,036,854,775,808"},
	}
	for _, tt := range tests {
		if got := FormatGrouped(tt.v, tt.scale); got != tt.want {
			t.Errorf("FormatGrouped(%d, %d) = %q, want %q", tt.v, tt.scale, got, tt.want)
		}
	}
}
