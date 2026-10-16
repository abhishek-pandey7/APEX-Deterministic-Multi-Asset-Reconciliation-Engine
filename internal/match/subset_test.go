package match

import (
	"fmt"
	"testing"
)

func TestSubsets(t *testing.T) {
	tests := []struct {
		name             string
		vals             []int64
		target, tol      int64
		minSize, maxSize int
		want             string
	}{
		{"split in three", []int64{300, 999, 500, 200}, 1000, 0, 2, 4, "[[0 2 3]]"},
		{"negative amounts", []int64{-300, -500, -200}, -1000, 0, 2, 4, "[[0 1 2]]"},
		{"two solutions", []int64{100, 100, 100}, 200, 0, 2, 2, "[[0 1] [0 2]]"},
		{"within tolerance", []int64{499, 500}, 1000, 1, 2, 2, "[[0 1]]"},
		{"size cap", []int64{100, 100, 100, 100, 100}, 500, 0, 2, 4, "[]"},
		{"min size", []int64{1000, 400, 600}, 1000, 0, 2, 3, "[[1 2]]"},
		{"mixed signs rejected", []int64{1500, -500}, 1000, 0, 2, 2, "[]"},
		{"nothing fits", []int64{7, 11, 13}, 100, 0, 1, 3, "[]"},
	}
	for _, tt := range tests {
		got := subsets(tt.vals, tt.target, tt.tol, tt.minSize, tt.maxSize, 2)
		if got == nil {
			got = [][]int{}
		}
		if fmt.Sprint(got) != tt.want {
			t.Errorf("%s: got %v, want %s", tt.name, got, tt.want)
		}
	}
}

func BenchmarkSubsets24(b *testing.B) {
	vals := make([]int64, 24)
	for i := range vals {
		vals[i] = int64(1000 + i*37)
	}
	for b.Loop() {
		subsets(vals, 1_000_003, 0, 2, 4, 2) // unreachable: worst case for the search
	}
}
