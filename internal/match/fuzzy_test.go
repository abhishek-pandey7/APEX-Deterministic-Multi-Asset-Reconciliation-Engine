package match

import (
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func withCpty(e domain.CashEntry, cpty string) domain.CashEntry {
	e.Counterparty = cpty
	return e
}

func TestFuzzyProposesLateMangledSettlement(t *testing.T) {
	l := []domain.CashEntry{withCpty(led("L1", "A", -750_000, "2026-10-05", "TRD-2026-0001234"), "Northbridge Securities LLC")}
	// Five business days late, prefix dropped and truncated: outside the tolerance window.
	c := []domain.CashEntry{cus("C1", "A", -750_000, "2026-10-12", "SETTLEMENT REF 202600012 NORTHBRIDGE SEC")}
	r := Run(l, c, DefaultConfig())
	if got := pairs(r, l, c); got["L1"] != "C1/FUZZY/PROPOSED" {
		t.Fatalf("got %v", got)
	}
	if m := r.Matches[0]; m.Score < DefaultFuzzyThreshold {
		t.Errorf("score %d below threshold", m.Score)
	}
}

func TestFuzzyNeverProposesConflictingReferences(t *testing.T) {
	// Same amount, same counterparty, plausible delay, but a different trade id.
	l := []domain.CashEntry{withCpty(led("L1", "A", -750_000, "2026-10-05", "TRD-2026-0001234"), "Northbridge Securities LLC")}
	c := []domain.CashEntry{cus("C1", "A", -750_000, "2026-10-12", "SETTLEMENT REF TRD20269999999 NORTHBRIDGE SEC")}
	if r := Run(l, c, DefaultConfig()); len(r.Matches) != 0 {
		t.Errorf("got %v, want both left open", pairs(r, l, c))
	}
}

func TestWeightsAreCapped(t *testing.T) {
	for f, levels := range DefaultWeights {
		for lv, w := range levels[:NumLevels(f)] {
			if w > MaxWeight || w < -MaxWeight {
				t.Errorf("%s/%s weight %d exceeds ±%d", FieldName(f), LevelName(f, lv), w, MaxWeight)
			}
		}
	}
}

func TestEstimateWeights(t *testing.T) {
	mk := func(ref, cpty uint8) FuzzyPair { return FuzzyPair{Levels: Levels{ref, 0, 1, cpty}} }
	pairs := []FuzzyPair{mk(0, 0), mk(0, 0), mk(2, 0), mk(4, 1), mk(4, 1), mk(4, 0)}
	truth := map[int]bool{0: true, 1: true, 2: true}
	i := -1
	w, c := EstimateWeights(pairs, func(FuzzyPair) bool { i++; return truth[i] })
	if c.NTrue != 3 || c.NFalse != 3 {
		t.Fatalf("counts %+v", c)
	}
	if w[FieldRef][0] <= 0 || w[FieldRef][4] >= 0 {
		t.Errorf("same id should weigh for, conflicting against: %v", w[FieldRef])
	}
	if w[FieldAmount][0] != 0 {
		t.Errorf("a level seen equally often on both sides should weigh nothing, got %d", w[FieldAmount][0])
	}
}

func TestCounterpartyLevel(t *testing.T) {
	tok := tokens("SETTLEMENT REF X NORTHBRIDGE SEC")
	tests := map[string]uint8{
		"Northbridge Securities LLC": 0,
		"Harbor Point Capital Inc":   1,
		"":                           2,
		"LLC":                        2,
	}
	for cpty, want := range tests {
		if got := counterpartyLevel(cpty, tok); got != want {
			t.Errorf("counterpartyLevel(%q) = %d, want %d", cpty, got, want)
		}
	}
}
