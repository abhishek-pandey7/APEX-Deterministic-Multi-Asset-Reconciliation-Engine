package match

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func led(id, account string, amount int64, day, ref string) domain.CashEntry {
	return domain.CashEntry{Side: domain.Ledger, ExternalID: id, AccountID: account, Currency: "USD",
		AmountMinor: amount, ValueDate: date(day), Reference: ref}
}

func cus(id, account string, amount int64, day, narrative string) domain.CashEntry {
	return domain.CashEntry{Side: domain.Custodian, ExternalID: id, AccountID: account, Currency: "USD",
		AmountMinor: amount, ValueDate: date(day), Reference: narrative}
}

// pairs renders matches as {"L1L2": "C1C2/PHASE/DISPOSITION"} with ids sorted, for easy comparison.
func pairs(r Result, l, c []domain.CashEntry) map[string]string {
	ids := func(all []domain.CashEntry, idx []int32) string {
		names := make([]string, len(idx))
		for k, i := range idx {
			names[k] = all[i].ExternalID
		}
		slices.Sort(names)
		return strings.Join(names, "")
	}
	out := map[string]string{}
	for _, m := range r.Matches {
		out[ids(l, m.Ledger)] = fmt.Sprintf("%s/%s/%s", ids(c, m.Custodian), m.Phase, m.Disposition)
	}
	return out
}

func TestExactPhase(t *testing.T) {
	l := []domain.CashEntry{
		led("L1", "A", -500000, "2026-09-01", "TRD-2026-0000001"),
		// Three identical fees on one day: only the reference tells them apart.
		led("L2", "A", -2500, "2026-09-01", "FEE-0000002"),
		led("L3", "A", -2500, "2026-09-01", "FEE-0000003"),
		led("L4", "A", -2500, "2026-09-01", "FEE-0000004"),
	}
	c := []domain.CashEntry{
		cus("C9", "A", -2500, "2026-09-01", "CHARGES REF FEE0000004"),
		cus("C8", "A", -2500, "2026-09-01", "CHARGES REF FEE-0000002 NORTHBRIDGE SEC"),
		cus("C7", "A", -500000, "2026-09-01", "SETTLEMENT REF TRD20260000001"),
		cus("C6", "A", -2500, "2026-09-01", "CHARGES REF FEE0000003"),
	}
	got := pairs(Run(l, c, DefaultConfig()), l, c)
	want := map[string]string{
		"L1": "C7/EXACT/AUTO", "L2": "C8/EXACT/AUTO", "L3": "C6/EXACT/AUTO", "L4": "C9/EXACT/AUTO",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestDuplicateCustodianLineLeavesOneOpen(t *testing.T) {
	l := []domain.CashEntry{led("L1", "A", 10000, "2026-09-01", "DIV-US0378331005-20260901")}
	c := []domain.CashEntry{
		cus("C2", "A", 10000, "2026-09-01", "DIVIDEND REF DIVUS037833100520260901"),
		cus("C1", "A", 10000, "2026-09-01", "DIVIDEND REF DIVUS037833100520260901"),
	}
	r := Run(l, c, DefaultConfig())
	if got := pairs(r, l, c); got["L1"] != "C1/EXACT/AUTO" {
		t.Errorf("got %v, want L1 matched to the lower id C1", got)
	}
	if len(r.UnmatchedCustodian) != 1 || c[r.UnmatchedCustodian[0]].ExternalID != "C2" {
		t.Errorf("unmatched custodian = %v, want [C2]", r.UnmatchedCustodian)
	}
}

func TestTolerancePhase(t *testing.T) {
	l := []domain.CashEntry{
		led("L1", "A", 750000, "2026-10-16", "DST-FND001-0000001"), // settles Monday: 1 business day
		led("L2", "A", 123456, "2026-10-16", "CPN-XS0000000001-20261016"),
		led("L3", "A", 900000, "2026-10-12", "TRD-2026-0000003"), // custodian 5 business days later
		led("L4", "A", 100000, "2026-10-12", "TRD-2026-0000004"), // custodian amount off by 1.00
	}
	c := []domain.CashEntry{
		cus("C1", "A", 750000, "2026-10-19", "DISTRIBUTION REF DSTFND0010000001"),
		cus("C2", "A", 123499, "2026-10-16", "COUPON PMT REF CPN-XS0000000001-20261016"), // 43 minor units off
		cus("C3", "A", 900000, "2026-10-19", "SETTLEMENT REF TRD20260000003"),
		cus("C4", "A", 100100, "2026-10-12", "SETTLEMENT REF TRD20260000004"),
	}
	r := Run(l, c, DefaultConfig())
	got := pairs(r, l, c)
	// L3 is outside the tolerance window, so phase 4 only proposes it; L4's amount is off by
	// more than the tolerance, so nothing matches it.
	want := map[string]string{"L1": "C1/TOLERANCE/AUTO", "L2": "C2/TOLERANCE/AUTO", "L3": "C3/FUZZY/PROPOSED"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if len(r.UnmatchedLedger) != 1 || len(r.UnmatchedCustodian) != 1 {
		t.Errorf("unmatched ledger=%d custodian=%d, want 1 and 1", len(r.UnmatchedLedger), len(r.UnmatchedCustodian))
	}
}

func TestWeakReferencesAreOnlyProposed(t *testing.T) {
	// The narrative carries a mangled id: plausible, but not strong enough to auto-confirm.
	l := []domain.CashEntry{led("L1", "A", -250000, "2026-09-01", "TRD-2026-0001234")}
	c := []domain.CashEntry{cus("C1", "A", -250000, "2026-09-01", "SETTLEMENT REF TRD")}
	cfg := DefaultConfig()
	cfg.MinProposeRefSim = 0
	if got := pairs(Run(l, c, cfg), l, c); got["L1"] != "C1/TOLERANCE/PROPOSED" {
		t.Errorf("got %v, want a proposal rather than an auto match", got)
	}
}

func TestConflictingReferencesStayOpen(t *testing.T) {
	// Same account, amount and date, but the ids are different fees: not a match, not even a proposal.
	l := []domain.CashEntry{led("L1", "A", -2500, "2026-09-01", "FEE-0000001")}
	c := []domain.CashEntry{cus("C1", "A", -2500, "2026-09-01", "CHARGES REF FEE0000002")}
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("got %v, want both entries left open", pairs(r, l, c))
	}
}

func TestPartitionsNeverCross(t *testing.T) {
	l := []domain.CashEntry{led("L1", "A", 1000, "2026-09-01", "X-1")}
	c := []domain.CashEntry{cus("C1", "B", 1000, "2026-09-01", "REF X1")}
	eur := cus("C2", "A", 1000, "2026-09-01", "REF X1")
	eur.Currency = "EUR"
	c = append(c, eur)
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("matched across account or currency: %v", pairs(r, l, c))
	}
}

// TestDeterminism shuffles the input and varies the worker count; the result hash must not move.
func TestDeterminism(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var l, c []domain.CashEntry
	for i := 0; i < 3000; i++ {
		acct := fmt.Sprintf("A%d", i%7)
		day := []string{"2026-10-12", "2026-10-13", "2026-10-14", "2026-10-15", "2026-10-16"}[i%5]
		amount := int64(rng.IntN(20)+1) * 2500 // many equal amounts on purpose
		ref := fmt.Sprintf("FEE-%07d", i)
		l = append(l, led(fmt.Sprintf("L%05d", i), acct, amount, day, ref))
		if i%10 != 0 {
			c = append(c, cus(fmt.Sprintf("C%05d", i), acct, amount+int64(i%3), day, "CHARGES REF "+ref))
		}
	}

	cfg := DefaultConfig()
	cfg.Workers = 1
	want := Run(l, c, cfg).Hash(l, c)

	for trial := 0; trial < 5; trial++ {
		ls, cs := append([]domain.CashEntry(nil), l...), append([]domain.CashEntry(nil), c...)
		rng.Shuffle(len(ls), func(i, j int) { ls[i], ls[j] = ls[j], ls[i] })
		rng.Shuffle(len(cs), func(i, j int) { cs[i], cs[j] = cs[j], cs[i] })
		cfg.Workers = 1 + trial*3
		if got := Run(ls, cs, cfg).Hash(ls, cs); got != want {
			t.Fatalf("trial %d (workers=%d): result changed with input order", trial, cfg.Workers)
		}
	}
}
