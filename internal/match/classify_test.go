package match

import (
	"slices"
	"strings"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func TestClassify(t *testing.T) {
	l := []domain.CashEntry{
		led("L1", "A", 1_000_000, "2026-10-12", "TRD-2026-0000001"), // matched
		led("L2", "A", 500_000, "2026-10-12", "TRD-2026-0000002"),   // custodian amount differs
		led("L3", "A", 250_000, "2026-10-05", "TRD-2026-0000003"),   // settles a week late, mangled ref
		led("L4", "A", -7_500, "2026-10-12", "FEE-0000004"),         // never settles
	}
	c := []domain.CashEntry{
		cus("C1", "A", 1_000_000, "2026-10-12", "SETTLEMENT REF TRD20260000001"),
		cus("C2", "A", 1_000_000, "2026-10-12", "SETTLEMENT REF TRD20260000001"), // reported twice
		cus("C3", "A", 540_000, "2026-10-12", "SETTLEMENT REF TRD-2026-0000002"),
		cus("C4", "A", 250_000, "2026-10-14", "SETTLEMENT REF /RFB/20260000030"), // last two digits swapped
		cus("C5", "A", 12_345, "2026-10-12", "DIVIDEND REF DIV-US0378331005-20261012"),
		cus("C6", "A", -2_500, "2026-10-12", "CHARGES REF ACCOUNT MAINTENANCE"),
	}
	cfg := DefaultConfig()
	res := Run(l, c, cfg)
	got := map[string]string{}
	for _, b := range Classify(l, c, res, cfg) {
		var ids []string
		for _, i := range b.Ledger {
			ids = append(ids, l[i].ExternalID)
		}
		for _, i := range b.Custodian {
			ids = append(ids, c[i].ExternalID)
		}
		slices.Sort(ids)
		got[strings.Join(ids, "+")] = string(b.Category)
		if b.Reason == "" {
			t.Errorf("break %v has no reason", ids)
		}
	}
	want := map[string]string{
		"C2":    "DUPLICATE",
		"C3+L2": "AMOUNT",
		"C4+L3": "TIMING",
		"C5":    "CORPORATE_ACTION",
		"C6":    "MISSING_IN_LEDGER",
		"L4":    "MISSING_IN_CUSTODIAN",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("break %s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d breaks %v, want %d", len(got), got, len(want))
	}
}

func TestClassifyAmountBreakSign(t *testing.T) {
	l := []domain.CashEntry{led("L1", "A", 500_000, "2026-10-12", "TRD-2026-0000002")}
	c := []domain.CashEntry{cus("C1", "A", 540_000, "2026-10-12", "SETTLEMENT REF TRD20260000002")}
	cfg := DefaultConfig()
	b := Classify(l, c, Run(l, c, cfg), cfg)
	if len(b) != 1 || b[0].AmountMinor != -40_000 || b[0].OpenedOn != date("2026-10-12") {
		t.Fatalf("got %+v, want one break of ledger minus custodian = -40000", b)
	}
	if !strings.Contains(b[0].Reason, "differs from the ledger 5000.00 USD by 400.00 USD") {
		t.Errorf("reason = %q", b[0].Reason)
	}
}
