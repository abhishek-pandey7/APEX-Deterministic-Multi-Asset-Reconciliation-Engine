package match

import (
	"fmt"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func TestSplitAnchoredByReference(t *testing.T) {
	l := []domain.CashEntry{led("L1", "A", -1_000_000, "2026-10-12", "CC-FND001-0000001")}
	c := []domain.CashEntry{
		cus("C1", "A", -400_000, "2026-10-12", "CAPITAL CALL REF CC-FND001-0000001"),
		cus("C2", "A", -350_000, "2026-10-13", "CAPITAL CALL REF CCFND0010000001"),
		cus("C3", "A", -250_000, "2026-10-13", "CAPITAL CALL REF CC-FND001-0000001"),
		cus("C4", "A", -250_000, "2026-10-13", "CHARGES REF FEE0000009"), // same size, unrelated
	}
	if got := pairs(Run(l, c, DefaultConfig()), l, c); got["L1"] != "C1C2C3/N_TO_M/AUTO" {
		t.Errorf("got %v", got)
	}
}

func TestNettedTradesManyToMany(t *testing.T) {
	l := []domain.CashEntry{
		led("L1", "A", 600_000, "2026-10-12", "TRD-2026-0000001"),
		led("L2", "A", 150_000, "2026-10-12", "TRD-2026-0000002"),
		led("L3", "A", 900_000, "2026-10-12", "TRD-2026-0000003"), // has its own exact match
	}
	c := []domain.CashEntry{
		cus("C1", "A", 300_000, "2026-10-12", "SETTLEMENT REF NET-TRD-2026-0000001"),
		cus("C2", "A", 250_000, "2026-10-12", "SETTLEMENT REF NETTRD20260000001"),
		cus("C3", "A", 200_000, "2026-10-12", "SETTLEMENT REF NETTRD20260000001"),
		cus("C4", "A", 900_000, "2026-10-12", "SETTLEMENT REF TRD20260000003"),
	}
	got := pairs(Run(l, c, DefaultConfig()), l, c)
	if got["L1L2"] != "C1C2C3/N_TO_M/AUTO" || got["L3"] != "C4/EXACT/AUTO" {
		t.Errorf("got %v", got)
	}
}

func TestFeeBatch(t *testing.T) {
	l := []domain.CashEntry{
		led("L1", "A", -2_500, "2026-10-12", "FEE-0000001"),
		led("L2", "A", -7_731, "2026-10-12", "FEE-0000002"),
		led("L3", "A", -1_204, "2026-10-12", "FEE-0000003"),
		led("L4", "A", -9_999, "2026-10-13", "FEE-0000004"), // next day: never part of this batch
	}
	c := []domain.CashEntry{cus("C1", "A", -11_435, "2026-10-12", "CHARGES REF FEE BATCH 20261012")}
	if got := pairs(Run(l, c, DefaultConfig()), l, c); got["L1L2L3"] != "C1/N_TO_M/AUTO" {
		t.Errorf("got %v", got)
	}
}

func TestAmbiguousBatchStaysOpen(t *testing.T) {
	// Two different pairs of fees add up to the batch total: matching either would be a guess.
	l := []domain.CashEntry{
		led("L1", "A", -2_500, "2026-10-12", "FEE-0000001"),
		led("L2", "A", -5_000, "2026-10-12", "FEE-0000002"),
		led("L3", "A", -2_500, "2026-10-12", "FEE-0000003"),
	}
	c := []domain.CashEntry{cus("C1", "A", -7_500, "2026-10-12", "CHARGES REF FEE BATCH 20261012")}
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("got %v, want everything left open", pairs(r, l, c))
	}
}

func TestSpecificReferenceIsNotABatch(t *testing.T) {
	// The line names one particular fee, so it can't be these two fees added together.
	l := []domain.CashEntry{
		led("L1", "A", -2_500, "2026-10-12", "FEE-0000001"),
		led("L2", "A", -5_000, "2026-10-12", "FEE-0000002"),
	}
	c := []domain.CashEntry{cus("C1", "A", -7_500, "2026-10-12", "CHARGES REF FEE0000777")}
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("got %v, want everything left open", pairs(r, l, c))
	}
}

func TestAmountMismatchIsNotNetting(t *testing.T) {
	// One line carries the reference but the amount is off by exactly another entry's amount.
	l := []domain.CashEntry{
		led("L1", "A", 1_000_000, "2026-10-12", "TRD-2026-0000001"),
		led("L2", "A", 50_000, "2026-10-12", "DIV-US0378331005-20261012"),
	}
	c := []domain.CashEntry{cus("C1", "A", 1_050_000, "2026-10-12", "SETTLEMENT REF TRD20260000001")}
	r := Run(l, c, DefaultConfig())
	for _, m := range r.Matches {
		if m.Phase == PhaseNToM {
			t.Errorf("amount mismatch was explained away as netting: %v", pairs(r, l, c))
		}
	}
}

func TestContestedBatchesStayOpen(t *testing.T) {
	// Both batch lines can only be completed with the same fee L3, so neither is accepted.
	l := []domain.CashEntry{
		led("L1", "A", -1_111, "2026-10-12", "FEE-0000001"),
		led("L2", "A", -2_222, "2026-10-12", "FEE-0000002"),
		led("L3", "A", -4_444, "2026-10-12", "FEE-0000003"),
	}
	c := []domain.CashEntry{
		cus("C1", "A", -5_555, "2026-10-12", "CHARGES REF FEE BATCH 20261012"),
		cus("C2", "A", -6_666, "2026-10-12", "CHARGES REF FEE BATCH 20261012"),
	}
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("got %v, want both batches left open", fmt.Sprint(pairs(r, l, c)))
	}
}
