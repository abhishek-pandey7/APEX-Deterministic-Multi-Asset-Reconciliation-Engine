package match

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/money"
)

// Category is a break's root cause as far as deterministic rules can tell. The values match
// the breaks.category check constraint in the database.
type Category string

const (
	CategoryTiming             Category = "TIMING"
	CategoryAmount             Category = "AMOUNT"
	CategoryDuplicate          Category = "DUPLICATE"
	CategoryMissingInLedger    Category = "MISSING_IN_LEDGER"
	CategoryMissingInCustodian Category = "MISSING_IN_CUSTODIAN"
	CategoryCorporateAction    Category = "CORPORATE_ACTION"
)

// Break is a difference an analyst has to resolve: entries the engine could not match,
// grouped where the evidence links them, with a category and a one-line reason.
type Break struct {
	Category  Category
	Ledger    []int32
	Custodian []int32
	AccountID string
	Currency  string
	// AmountMinor is the ledger total minus the custodian total.
	AmountMinor int64
	// OpenedOn is the earliest value date among the break's entries.
	OpenedOn domain.Date
	Reason   string
}

// Classify turns everything a Result left unmatched into breaks. It is deterministic: the same
// entries and result always give the same breaks in the same order.
//
// Rules, applied in this order within each (account, currency):
//
//	DUPLICATE             a custodian line identical to one that was matched
//	TIMING                a ledger entry and a custodian line with the same reference and
//	                      amount, but further apart than the matching window
//	AMOUNT                a ledger entry and a custodian line with the same reference but
//	                      different amounts
//	CORPORATE_ACTION      a custodian-only dividend or coupon
//	MISSING_IN_LEDGER     any other custodian-only line
//	MISSING_IN_CUSTODIAN  any other ledger-only entry
func Classify(ledger, custodian []domain.CashEntry, res Result, cfg Config) []Break {
	matchedLines := map[string]bool{}
	for _, m := range res.Matches {
		for _, i := range m.Custodian {
			matchedLines[lineKey(&custodian[i])] = true
		}
	}

	open := map[partKey]*part{}
	get := func(e *domain.CashEntry) *part {
		k := partKey{e.AccountID, e.Currency}
		if open[k] == nil {
			open[k] = &part{key: k}
		}
		return open[k]
	}
	for _, i := range res.UnmatchedLedger {
		p := get(&ledger[i])
		p.ledger = append(p.ledger, i)
	}
	for _, i := range res.UnmatchedCustodian {
		p := get(&custodian[i])
		p.custodian = append(p.custodian, i)
	}
	parts := make([]*part, 0, len(open))
	for _, p := range open {
		parts = append(parts, p)
	}
	slices.SortFunc(parts, func(a, b *part) int {
		return cmp.Or(strings.Compare(a.key.account, b.key.account), strings.Compare(a.key.currency, b.key.currency))
	})

	var out []Break
	for _, p := range parts {
		out = append(out, classifyPartition(ledger, custodian, p, matchedLines, cfg)...)
	}
	return out
}

func classifyPartition(ledger, custodian []domain.CashEntry, p *part, matchedLines map[string]bool, cfg Config) []Break {
	l := newSide(ledger, p.ledger, true)
	c := newSide(custodian, p.custodian, false)
	ccy, _ := money.LookupCurrency(p.key.currency)
	amt := func(v int64) string { return money.FormatFixed(v, ccy.Exponent) + " " + ccy.Code }

	var out []Break
	add := func(cat Category, lis, cis []int, reason string) {
		b := Break{Category: cat, AccountID: p.key.account, Currency: p.key.currency, Reason: reason}
		first := true
		for _, i := range lis {
			e := l.entries[i]
			l.used[i] = true
			b.Ledger = append(b.Ledger, l.index[i])
			b.AmountMinor += e.AmountMinor
			if first || e.ValueDate < b.OpenedOn {
				b.OpenedOn, first = e.ValueDate, false
			}
		}
		for _, i := range cis {
			e := c.entries[i]
			c.used[i] = true
			b.Custodian = append(b.Custodian, c.index[i])
			b.AmountMinor -= e.AmountMinor
			if first || e.ValueDate < b.OpenedOn {
				b.OpenedOn, first = e.ValueDate, false
			}
		}
		out = append(out, b)
	}

	for ci, e := range c.entries {
		if matchedLines[lineKey(e)] {
			add(CategoryDuplicate, nil, []int{ci}, fmt.Sprintf(
				"identical to a custodian line already matched (%s on %s); likely reported twice", amt(e.AmountMinor), e.ValueDate))
		}
	}

	// Pair ledger and custodian entries whose references agree, strongest evidence first. With
	// matching amounts, keying-error evidence (900) is enough to call it late settlement; to
	// claim the amounts differ, the reference itself must agree (950), because a swapped digit
	// on a busy day often lands on an unrelated sequential id.
	byDigits := map[string][]int{}
	for ci, ts := range c.tokens {
		if c.used[ci] {
			continue
		}
		for _, t := range ts {
			if d := digitsOf(t); len(d) >= 4 {
				byDigits[d] = append(byDigits[d], ci)
			}
		}
	}
	var edges []edge
	for li, ref := range l.ref {
		seen := map[int]bool{}
		for _, key := range digitKeys(digitsOf(ref)) {
			for _, ci := range byDigits[key] {
				if seen[ci] {
					continue
				}
				seen[ci] = true
				sim := RefSim(ref, c.tokens[ci])
				diff := c.entries[ci].AmountMinor - l.entries[li].AmountMinor
				diff = max(diff, -diff)
				if sim >= 950 || (sim >= 900 && diff <= cfg.AmountTolerance) {
					edges = append(edges, edge{li, ci, sim, diff, BizGap(l.entries[li].ValueDate, c.entries[ci].ValueDate)})
				}
			}
		}
	}
	slices.SortFunc(edges, better(l, c))
	for _, ed := range edges {
		if l.used[ed.li] || c.used[ed.ci] {
			continue
		}
		le, ce := l.entries[ed.li], c.entries[ed.ci]
		if ed.diff <= cfg.AmountTolerance {
			add(CategoryTiming, []int{ed.li}, []int{ed.ci}, fmt.Sprintf(
				"custodian settled %s %d business days from the ledger value date %s, outside the %d-day window",
				ce.ValueDate, ed.gap, le.ValueDate, cfg.DateWindow))
		} else {
			add(CategoryAmount, []int{ed.li}, []int{ed.ci}, fmt.Sprintf(
				"same reference %s, but the custodian amount %s differs from the ledger %s by %s",
				le.Reference, amt(ce.AmountMinor), amt(le.AmountMinor), amt(ce.AmountMinor-le.AmountMinor)))
		}
	}

	for ci, e := range c.entries {
		if c.used[ci] {
			continue
		}
		if slices.Contains(c.tokens[ci], "DIVIDEND") || slices.Contains(c.tokens[ci], "COUPON") {
			add(CategoryCorporateAction, nil, []int{ci}, fmt.Sprintf(
				"custodian received %s income on %s that the ledger has not booked", amt(e.AmountMinor), e.ValueDate))
		} else {
			add(CategoryMissingInLedger, nil, []int{ci}, fmt.Sprintf(
				"custodian line of %s on %s has no ledger entry", amt(e.AmountMinor), e.ValueDate))
		}
	}
	for li, e := range l.entries {
		if !l.used[li] {
			add(CategoryMissingInCustodian, []int{li}, nil, fmt.Sprintf(
				"ledger entry %s of %s on %s has not settled at the custodian", e.Reference, amt(e.AmountMinor), e.ValueDate))
		}
	}
	return out
}

// digitKeys lists every digit string a token could have and still score at least 900 against
// a reference with digits d (see RefSim): d itself, each single adjacent swap, and truncations
// of at least six digits, plus the swaps of those truncations.
func digitKeys(d string) []string {
	if len(d) < 4 {
		return nil
	}
	var keys []string
	for n := len(d); n >= 6 || n == len(d); n-- {
		p := d[:n]
		keys = append(keys, p)
		for i := 0; i+1 < len(p); i++ {
			if p[i] != p[i+1] {
				keys = append(keys, p[:i]+string(p[i+1])+string(p[i])+p[i+2:])
			}
		}
	}
	return keys
}

// lineKey identifies a custodian line by everything the custodian reported, so two identical
// lines share a key.
func lineKey(e *domain.CashEntry) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s", e.AccountID, e.Currency, e.AmountMinor, e.ValueDate, compact(e.Reference))
}
