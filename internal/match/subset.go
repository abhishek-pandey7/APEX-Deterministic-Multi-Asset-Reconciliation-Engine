package match

import (
	"cmp"
	"slices"
)

// subsets finds index sets of vals, of size minSize..maxSize, whose sum is within tol of
// target. It returns at most limit solutions, each as ascending indices into vals; callers
// ask for 2 to learn whether the first solution is unique.
//
// This is bounded subset-sum (NP-complete in general, see docs/ALGORITHMS.md §6). It stays
// cheap because callers cap len(vals) and maxSize. All vals must share target's sign. The
// search runs over magnitudes sorted largest first, which allows two prunes: skip a value
// that overshoots, and stop when even the largest remaining values cannot reach the target.
// Exploration order is fixed, so the same input always yields the same solutions in the same order.
func subsets(vals []int64, target, tol int64, minSize, maxSize, limit int) [][]int {
	if len(vals) == 0 || target == 0 {
		return nil
	}
	sign := int64(1)
	if target < 0 {
		sign = -1
	}
	order := make([]int, len(vals))
	for i := range order {
		order[i] = i
	}
	mag := func(i int) int64 { return sign * vals[i] }
	for _, v := range vals {
		if sign*v <= 0 {
			return nil
		}
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Or(cmp.Compare(mag(b), mag(a)), cmp.Compare(a, b)) })

	// prefix[i] is the sum of the i largest magnitudes, for the reachability bound.
	prefix := make([]int64, len(order)+1)
	for i, o := range order {
		prefix[i+1] = prefix[i] + mag(o)
	}
	goal := sign * target

	var out [][]int
	chosen := make([]int, 0, maxSize)
	var walk func(start int, sum int64)
	walk = func(start int, sum int64) {
		if len(out) >= limit {
			return
		}
		if len(chosen) >= minSize && sum >= goal-tol && sum <= goal+tol {
			sol := slices.Clone(chosen)
			slices.Sort(sol)
			out = append(out, sol)
		}
		room := maxSize - len(chosen)
		if room == 0 {
			return
		}
		for i := start; i < len(order); i++ {
			// The best we can still add from here is the next `room` values in order.
			end := min(i+room, len(order))
			if sum+prefix[end]-prefix[i] < goal-tol {
				return
			}
			v := mag(order[i])
			if sum+v > goal+tol {
				continue
			}
			chosen = append(chosen, order[i])
			walk(i+1, sum+v)
			chosen = chosen[:len(chosen)-1]
			if len(out) >= limit {
				return
			}
		}
	}
	walk(0, 0)
	return out
}
