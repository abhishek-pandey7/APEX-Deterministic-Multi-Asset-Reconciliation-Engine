package money

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// Fixed-point scales for non-cash values. Cash amounts use their currency's exponent.
const (
	QuantityScale uint8 = 6
	PriceScale    uint8 = 8

	maxScale uint8 = 18
)

var (
	ErrSyntax    = errors.New("money: invalid decimal")
	ErrPrecision = errors.New("money: more fractional digits than scale allows")
	ErrRange     = errors.New("money: value out of int64 range")
)

// ParseFixed converts a decimal string such as "-1234.50" into an integer scaled by 10^scale.
// It accepts an optional sign, digits and at most one '.'. Fractional digits beyond the scale are
// accepted only when they are zeros; anything else returns ErrPrecision instead of rounding.
func ParseFixed(s string, scale uint8) (int64, error) {
	if scale > maxScale {
		return 0, fmt.Errorf("money: scale %d exceeds %d", scale, maxScale)
	}
	in := s
	neg := false
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	intPart, fracPart, _ := strings.Cut(s, ".")
	if intPart == "" && fracPart == "" || !isDigits(intPart) || !isDigits(fracPart) {
		return 0, fmt.Errorf("%w: %q", ErrSyntax, in)
	}
	if len(fracPart) > int(scale) {
		if strings.Trim(fracPart[scale:], "0") != "" {
			return 0, fmt.Errorf("%w: %q at scale %d", ErrPrecision, in, scale)
		}
		fracPart = fracPart[:scale]
	}

	limit := uint64(math.MaxInt64)
	if neg {
		limit++
	}
	var mag uint64
	push := func(d byte) bool {
		v := uint64(d - '0')
		if mag > (limit-v)/10 {
			return false
		}
		mag = mag*10 + v
		return true
	}
	digits := intPart + fracPart
	for i := 0; i < len(digits); i++ {
		if !push(digits[i]) {
			return 0, fmt.Errorf("%w: %q", ErrRange, in)
		}
	}
	for i := len(fracPart); i < int(scale); i++ {
		if !push('0') {
			return 0, fmt.Errorf("%w: %q", ErrRange, in)
		}
	}

	if neg {
		return int64(-mag), nil // wraps correctly for math.MinInt64
	}
	return int64(mag), nil
}

// FormatFixed renders v, scaled by 10^scale, as a plain decimal string with exactly scale fractional digits.
func FormatFixed(v int64, scale uint8) string {
	mag := uint64(v)
	sign := ""
	if v < 0 {
		mag = -mag
		sign = "-"
	}
	digits := fmt.Sprintf("%0*d", int(scale)+1, mag)
	if scale == 0 {
		return sign + digits
	}
	cut := len(digits) - int(scale)
	return sign + digits[:cut] + "." + digits[cut:]
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatGrouped is FormatFixed with thousands separators, for display only: "-1,234,567.89".
// Machine-readable output keeps FormatFixed, which ParseFixed reads back.
func FormatGrouped(v int64, scale uint8) string {
	s := FormatFixed(v, scale)
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	if hasFrac {
		return sign + b.String() + "." + frac
	}
	return sign + b.String()
}
