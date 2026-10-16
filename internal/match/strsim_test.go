package match

import (
	"testing"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

func TestJaroWinklerKnownValues(t *testing.T) {
	// Reference values from Winkler (1990) and the standard worked examples.
	tests := []struct {
		a, b string
		want int32
	}{
		{"MARTHA", "MARHTA", 961},
		{"DWAYNE", "DUANE", 840},
		{"DIXON", "DICKSONX", 813},
		{"SAME", "SAME", 1000},
		{"ABC", "XYZ", 0},
		{"", "ABC", 0},
	}
	for _, tt := range tests {
		if got := JaroWinkler(tt.a, tt.b); got != tt.want {
			t.Errorf("JaroWinkler(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got := JaroWinkler(tt.b, tt.a); got != tt.want {
			t.Errorf("JaroWinkler(%q, %q) = %d, want %d (not symmetric)", tt.b, tt.a, got, tt.want)
		}
	}
}

func TestRefSim(t *testing.T) {
	ref := compact("TRD-2026-0001234")
	tests := []struct {
		narrative string
		min, max  int32
	}{
		{"SETTLEMENT REF TRD-2026-0001234 NORTHBRIDGE SEC", 1000, 1000},
		{"SETTLEMENT REF TRD20260001234", 1000, 1000},
		{"SETTLEMENT REF /RFB/TRD202600012", 900, 900},        // truncated: matches ten sequential ids
		{"SETTLEMENT REF /RFB/TRD20260001234", 1000, 1000},    // wrapper splits off cleanly
		{"SETTLEMENT REF XTRD20260001234", 950, 950},          // reference embedded in a longer token
		{"SETTLEMENT REF TRD20260002134", 900, 999},           // transposed digits
		{"SETTLEMENT REF TRD20269999999 GRANITE ROW", 0, 500}, // a different trade
		{"SETTLEMENT REF TRD20260001235", 0, 500},             // the next trade in sequence
		{"SETTLEMENT REF 20260001234", 950, 950},              // alphabetic prefix dropped
		{"SETTLEMENT REF 202600021", 900, 900},                // truncated, then two digits swapped
		{"CHARGES REF FEE0000001", 0, 500},
	}
	for _, tt := range tests {
		got := RefSim(ref, tokens(tt.narrative))
		if got < tt.min || got > tt.max {
			t.Errorf("RefSim(%q) = %d, want %d..%d", tt.narrative, got, tt.min, tt.max)
		}
	}
}

func TestSameOrSwapped(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1234", "1234", true},
		{"1234", "2134", true},
		{"1234", "1243", true},
		{"1234", "1235", false}, // substitution: a neighbouring id, not a typo
		{"1234", "4231", false},
		{"1234", "123", false},
	}
	for _, tt := range tests {
		if got := sameOrSwapped(tt.a, tt.b); got != tt.want {
			t.Errorf("sameOrSwapped(%q, %q) = %v", tt.a, tt.b, got)
		}
	}
}

func date(s string) domain.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return domain.DateOf(t)
}

func TestBizGap(t *testing.T) {
	tests := []struct {
		a, b string
		want int32
	}{
		{"2026-10-16", "2026-10-16", 0}, // Friday
		{"2026-10-16", "2026-10-19", 1}, // Friday to Monday
		{"2026-10-16", "2026-10-17", 1}, // Friday to Saturday counts as reaching Monday
		{"2026-10-12", "2026-10-23", 9}, // Monday to the Friday after next
		{"2026-10-19", "2026-10-16", 1}, // order doesn't matter
		{"1969-12-26", "1969-12-29", 1}, // before the epoch: Friday to Monday
	}
	for _, tt := range tests {
		if got := BizGap(date(tt.a), date(tt.b)); got != tt.want {
			t.Errorf("BizGap(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
