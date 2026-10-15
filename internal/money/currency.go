// Package money holds the fixed-point numeric types used on the matching path.
// No value on that path is ever represented as a float.
package money

import "fmt"

// Currency is an ISO 4217 alphabetic code together with its minor-unit exponent,
// i.e. the number of decimal places an amount in that currency carries.
type Currency struct {
	Code     string
	Exponent uint8
}

// exponents lists ISO 4217 minor units for the currencies APEX accepts.
// Currencies outside this table are rejected rather than guessed.
var exponents = map[string]uint8{
	"AUD": 2, "BRL": 2, "CAD": 2, "CHF": 2, "CNY": 2, "CZK": 2, "DKK": 2,
	"EUR": 2, "GBP": 2, "HKD": 2, "HUF": 2, "IDR": 2, "ILS": 2, "INR": 2,
	"MXN": 2, "MYR": 2, "NOK": 2, "NZD": 2, "PHP": 2, "PLN": 2, "SAR": 2,
	"SEK": 2, "SGD": 2, "THB": 2, "TRY": 2, "TWD": 2, "USD": 2, "ZAR": 2,
	"AED": 2, "QAR": 2,
	"CLP": 0, "ISK": 0, "JPY": 0, "KRW": 0, "VND": 0,
	"BHD": 3, "JOD": 3, "KWD": 3, "OMR": 3, "TND": 3,
}

// LookupCurrency returns the Currency for an ISO 4217 code.
func LookupCurrency(code string) (Currency, error) {
	exp, ok := exponents[code]
	if !ok {
		return Currency{}, fmt.Errorf("money: unsupported currency %q", code)
	}
	return Currency{Code: code, Exponent: exp}, nil
}

func (c Currency) String() string { return c.Code }
