package match

import "strings"

// JaroWinkler returns the Jaro–Winkler similarity of a and b scaled to 0..1000.
//
// It is computed with exact integer fractions rather than float64, so the score is identical
// on every platform (Go may fuse floating-point multiply-adds on some architectures, which can
// change the last bit of a float result and with it a tie-break).
func JaroWinkler(a, b string) int32 {
	if a == b {
		return 1000
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 0
	}
	window := max(la, lb)/2 - 1
	if window < 0 {
		window = 0
	}

	var aMatched, bMatched [64]bool
	if la > 64 || lb > 64 {
		// References are short; truncating keeps the work bounded and the arrays on the stack.
		a, b = a[:min(la, 64)], b[:min(lb, 64)]
		la, lb = len(a), len(b)
	}
	m := 0
	for i := 0; i < la; i++ {
		lo, hi := max(0, i-window), min(lb-1, i+window)
		for j := lo; j <= hi; j++ {
			if !bMatched[j] && a[i] == b[j] {
				aMatched[i], bMatched[j] = true, true
				m++
				break
			}
		}
	}
	if m == 0 {
		return 0
	}

	// outOfOrder counts matched characters that appear in a different order; Jaro's
	// "transpositions" is half of it.
	outOfOrder, j := 0, 0
	for i := 0; i < la; i++ {
		if !aMatched[i] {
			continue
		}
		for !bMatched[j] {
			j++
		}
		if a[i] != b[j] {
			outOfOrder++
		}
		j++
	}

	prefix := 0
	for prefix < min(4, la, lb) && a[prefix] == b[prefix] {
		prefix++
	}

	// jaro = (m/la + m/lb + (m - outOfOrder/2)/m) / 3 = num/den
	num := int64(2*m*m*lb + 2*m*m*la + (2*m-outOfOrder)*la*lb)
	den := int64(6 * m * la * lb)
	// jw = jaro + prefix/10 * (1 - jaro) = ((10-prefix)*num + prefix*den) / (10*den)
	p := int64(prefix)
	scaledNum := 1000 * ((10-p)*num + p*den)
	scaledDen := 10 * den
	return int32((scaledNum + scaledDen/2) / scaledDen)
}

// compact upper-cases s and keeps only letters and digits: "TRD-2026-0001234" -> "TRD20260001234".
func compact(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 'a' + 'A')
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// tokens splits a free-text narrative on spaces and slashes and compacts each piece, dropping
// pieces too short to carry identifying information.
func tokens(narrative string) []string {
	fields := strings.FieldsFunc(narrative, func(r rune) bool { return r == ' ' || r == '/' })
	out := fields[:0]
	for _, f := range fields {
		if t := compact(f); len(t) >= 4 {
			out = append(out, t)
		}
	}
	return out
}

// RefSim scores how strongly a narrative's tokens point at reference ref (already compacted),
// on 0..1000:
//
//	1000  a token equals the reference
//	 950  one contains the other (truncated, prefix dropped, wrapped in /RFB/)
//	 900  same digits apart from one adjacent swap, possibly after truncation (a keying error)
//	≤500  identifier-like tokens whose digits differ in any other way
//
// Identifiers are compared on their digits rather than by Jaro–Winkler. References are often
// sequential, and JW scores neighbours like FEE0000123 and FEE0000124 around 0.9, as similar
// as a genuine typo. A single changed digit is deliberately not forgiven: that is exactly how
// neighbouring ids differ. Tokens without digits fall back to Jaro–Winkler.
func RefSim(ref string, narrativeTokens []string) int32 {
	if ref == "" {
		return 0
	}
	refDigits := digitsOf(ref)
	var best int32
	for _, t := range narrativeTokens {
		if t == ref {
			return 1000
		}
		var s int32
		td := digitsOf(t)
		switch {
		case len(t) >= 8 && (strings.Contains(ref, t) || strings.Contains(t, ref)):
			s = 950
		case len(refDigits) >= 4 && len(td) >= 4:
			if sameOrSwapped(refDigits, td) ||
				(len(td) >= 6 && len(td) < len(refDigits) && sameOrSwapped(refDigits[:len(td)], td)) {
				s = 900
			} else {
				s = min(JaroWinkler(ref, t), 500)
			}
		default:
			s = JaroWinkler(ref, t)
		}
		best = max(best, s)
	}
	return best
}

func digitsOf(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// sameOrSwapped reports whether a and b are equal or differ by one swap of adjacent characters.
func sameOrSwapped(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	i := 0
	for i < len(a) && a[i] == b[i] {
		i++
	}
	if i == len(a) {
		return true
	}
	return i+1 < len(a) && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:]
}
