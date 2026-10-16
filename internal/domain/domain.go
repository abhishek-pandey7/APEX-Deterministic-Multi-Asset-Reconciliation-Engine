// Package domain defines the normalised records every input is converted into.
// Whatever a custodian's file looks like, the engine only ever sees these types.
package domain

import "time"

type Side string

const (
	Ledger    Side = "LEDGER"
	Custodian Side = "CUSTODIAN"
)

type Kind string

const (
	KindCash     Kind = "CASH"
	KindPosition Kind = "POSITION"
)

// Date is a calendar date stored as days since 1970-01-01. Comparing and subtracting
// dates is plain integer arithmetic, which keeps the matching loops cheap.
type Date int32

const dateLayout = "2006-01-02"

func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

func ParseDate(layout, s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return 0, err
	}
	return DateOf(t), nil
}

func (d Date) Time() time.Time { return time.Unix(int64(d)*86400, 0).UTC() }

func (d Date) String() string { return d.Time().Format(dateLayout) }

// CashEntry is one cash movement. AmountMinor is signed from the account's point of view
// (positive is money in) and scaled by the currency's ISO 4217 exponent.
type CashEntry struct {
	// ID is the cash_entries row id once stored; zero for entries parsed but not yet loaded.
	ID           int64
	Side         Side
	ExternalID   string
	AccountID    string
	Currency     string
	AmountMinor  int64
	ValueDate    Date
	TradeDate    Date // zero when the source doesn't provide one
	HasTradeDate bool
	TxnType      string
	Reference    string
	Counterparty string
}

// Position is a holding of one security on one date. Quantity is scaled by 10^6,
// price by 10^8, and market value by the currency's exponent.
type Position struct {
	Side             Side
	AccountID        string
	AsOf             Date
	SecurityID       string
	AssetClass       string
	Currency         string
	QuantityE6       int64
	PriceE8          int64
	MarketValueMinor int64
}
