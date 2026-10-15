package ingest

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func mustLayout(t *testing.T, name string) (*CashLayout, *PositionLayout) {
	t.Helper()
	c, p, err := Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}

func TestParseLedgerCash(t *testing.T) {
	l, _ := mustLayout(t, "apex-ledger-cash")
	src := "entry_id,account_id,currency,amount,value_date,trade_date,txn_type,reference,counterparty\n" +
		"L1,ACC1,USD,-1234.50,2026-09-01,2026-08-31,TRADE_SETTLEMENT,TRD-1,Broker LLC\n" +
		"L2,ACC1,JPY,3750,2026-09-02,,FEE,FEE-2,\n"

	f, err := ParseCash(strings.NewReader(src), *l)
	if err != nil {
		t.Fatal(err)
	}
	if !f.OK() || len(f.Records) != 2 {
		t.Fatalf("records=%d errors=%v", len(f.Records), f.Errors)
	}
	if f.SHA256 != sha256.Sum256([]byte(src)) {
		t.Error("hash does not cover the whole file")
	}

	e := f.Records[0]
	if e.AmountMinor != -123450 || e.ValueDate.String() != "2026-09-01" || !e.HasTradeDate ||
		e.TradeDate.String() != "2026-08-31" || e.Side != domain.Ledger {
		t.Errorf("unexpected entry %+v", e)
	}
	if f.Records[1].AmountMinor != 3750 || f.Records[1].HasTradeDate {
		t.Errorf("unexpected entry %+v", f.Records[1])
	}
}

func TestParseCustodianCashDebitCredit(t *testing.T) {
	l, _ := mustLayout(t, "custodian-cash-v1")
	// Columns deliberately reordered: mapping is by header name, not position.
	src := "Narrative,D/C,Amount,Value Dt,Ccy,Acct No,Stmt Ref\n" +
		"SETTLEMENT REF TRD1,D,1234.50,20260901,USD,ACC1,C1\n" +
		"DIVIDEND REF DIV1,C,0.125,20260902,KWD,ACC1,C2\n"

	f, err := ParseCash(strings.NewReader(src), *l)
	if err != nil {
		t.Fatal(err)
	}
	if !f.OK() {
		t.Fatal(f.Errors)
	}
	if got := f.Records[0]; got.AmountMinor != -123450 || got.TxnType != "UNKNOWN" || got.Reference != "SETTLEMENT REF TRD1" {
		t.Errorf("debit row parsed as %+v", got)
	}
	if got := f.Records[1]; got.AmountMinor != 125 || got.ValueDate.String() != "2026-09-02" {
		t.Errorf("credit row parsed as %+v", got)
	}
}

func TestParseCashRejectsBadRows(t *testing.T) {
	l, _ := mustLayout(t, "custodian-cash-v1")
	src := "Stmt Ref,Acct No,Ccy,Value Dt,Amount,D/C,Narrative\n" +
		"C1,ACC1,USD,20260901,10.00,C,ok\n" +
		"C2,ACC1,USD,20260901,10.005,C,too precise\n" +
		"C3,ACC1,XXX,20260901,10.00,C,unknown currency\n" +
		"C4,ACC1,USD,2026-09-01,10.00,C,wrong date format\n" +
		"C5,ACC1,USD,20260901,-10.00,D,signed amount with flag\n" +
		"C6,ACC1,USD,20260901,10.00,X,bad flag\n" +
		"C1,ACC1,USD,20260901,10.00,C,duplicate id\n" +
		",ACC1,USD,20260901,10.00,C,empty id\n"

	f, err := ParseCash(strings.NewReader(src), *l)
	if err != nil {
		t.Fatal(err)
	}
	if f.OK() || f.ErrorCount != 7 || len(f.Records) != 1 {
		t.Fatalf("errors=%d records=%d: %v", f.ErrorCount, len(f.Records), f.Errors)
	}
	wantLines := []int{3, 4, 5, 6, 7, 8, 9}
	for i, e := range f.Errors {
		if e.Line != wantLines[i] {
			t.Errorf("error %d on line %d, want %d (%v)", i, e.Line, wantLines[i], e)
		}
	}
}

func TestParseCashHeaderWithByteOrderMark(t *testing.T) {
	l, _ := mustLayout(t, "custodian-cash-v1")
	src := string(rune(0xFEFF)) + "Stmt Ref,Acct No,Ccy,Value Dt,Amount,D/C,Narrative\n" +
		"C1,ACC1,USD,20260901,10.00,C,ok\n"
	f, err := ParseCash(strings.NewReader(src), *l)
	if err != nil || !f.OK() || f.Records[0].ExternalID != "C1" {
		t.Fatalf("err=%v file=%+v", err, f)
	}
}

func TestParseCashMissingColumn(t *testing.T) {
	l, _ := mustLayout(t, "custodian-cash-v1")
	_, err := ParseCash(strings.NewReader("Stmt Ref,Acct No,Ccy\nC1,ACC1,USD\n"), *l)
	if err == nil || !strings.Contains(err.Error(), "missing columns") {
		t.Fatalf("err = %v, want missing columns", err)
	}
}

func TestParseCustodianPositions(t *testing.T) {
	_, l := mustLayout(t, "custodian-positions-v1")
	src := "Acct No,Pos Date,ISIN,Ccy,Settled Qty,Px,Mkt Val\n" +
		"ACC1,20260930,US0378331005,USD,100,101.25,10125.00\n" +
		"ACC1,20260930,XS0000000001,USD,0.5,10.02,5.01\n" +
		"ACC1,20260930,US0378331005,USD,1,1,1.00\n"

	f, err := ParsePositions(strings.NewReader(src), *l)
	if err != nil {
		t.Fatal(err)
	}
	if f.ErrorCount != 1 || len(f.Records) != 2 {
		t.Fatalf("errors=%v records=%d", f.Errors, len(f.Records))
	}
	p := f.Records[0]
	if p.QuantityE6 != 100_000000 || p.PriceE8 != 10125000000 || p.MarketValueMinor != 1012500 || p.AssetClass != "UNKNOWN" {
		t.Errorf("unexpected position %+v", p)
	}
}
