// Package ingest converts source files into domain records. Each source has a Layout
// describing its columns, date format and sign convention; adding a custodian means adding
// a Layout, not new parsing code.
package ingest

import (
	"fmt"
	"sort"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

type SignConvention int

const (
	// Signed amounts carry their own sign, positive meaning money in.
	Signed SignConvention = iota
	// DebitCredit amounts are absolute, with a separate flag column giving the direction.
	DebitCredit
)

// CashLayout maps a cash file's columns onto domain.CashEntry. An empty column name means
// the source doesn't have that field.
type CashLayout struct {
	Name       string
	Side       domain.Side
	DateFormat string

	ID, Account, Currency, Amount, ValueDate    string
	TradeDate, TxnType, Reference, Counterparty string

	Sign                  SignConvention
	DirectionColumn       string
	CreditFlag, DebitFlag string
	DefaultTxnType        string
}

// PositionLayout maps a position file's columns onto domain.Position.
type PositionLayout struct {
	Name       string
	Side       domain.Side
	DateFormat string

	Account, AsOf, Security, AssetClass, Currency string
	Quantity, Price, MarketValue                  string

	DefaultAssetClass string
}

var cashLayouts = map[string]CashLayout{
	"apex-ledger-cash": {
		Name: "apex-ledger-cash", Side: domain.Ledger, DateFormat: "2006-01-02",
		ID: "entry_id", Account: "account_id", Currency: "currency", Amount: "amount",
		ValueDate: "value_date", TradeDate: "trade_date", TxnType: "txn_type",
		Reference: "reference", Counterparty: "counterparty",
		Sign: Signed,
	},
	"custodian-cash-v1": {
		Name: "custodian-cash-v1", Side: domain.Custodian, DateFormat: "20060102",
		ID: "Stmt Ref", Account: "Acct No", Currency: "Ccy", Amount: "Amount",
		ValueDate: "Value Dt", Reference: "Narrative",
		Sign: DebitCredit, DirectionColumn: "D/C", CreditFlag: "C", DebitFlag: "D",
		DefaultTxnType: "UNKNOWN",
	},
}

var positionLayouts = map[string]PositionLayout{
	"apex-ledger-positions": {
		Name: "apex-ledger-positions", Side: domain.Ledger, DateFormat: "2006-01-02",
		Account: "account_id", AsOf: "as_of_date", Security: "security_id",
		AssetClass: "asset_class", Currency: "currency", Quantity: "quantity",
		Price: "price", MarketValue: "market_value",
	},
	"custodian-positions-v1": {
		Name: "custodian-positions-v1", Side: domain.Custodian, DateFormat: "20060102",
		Account: "Acct No", AsOf: "Pos Date", Security: "ISIN", Currency: "Ccy",
		Quantity: "Settled Qty", Price: "Px", MarketValue: "Mkt Val",
		DefaultAssetClass: "UNKNOWN",
	},
}

// Lookup returns the layout registered under name; exactly one of the results is non-nil.
func Lookup(name string) (*CashLayout, *PositionLayout, error) {
	if l, ok := cashLayouts[name]; ok {
		return &l, nil, nil
	}
	if l, ok := positionLayouts[name]; ok {
		return nil, &l, nil
	}
	return nil, nil, fmt.Errorf("ingest: unknown layout %q (known: %v)", name, LayoutNames())
}

func LayoutNames() []string {
	var names []string
	for n := range cashLayouts {
		names = append(names, n)
	}
	for n := range positionLayouts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
