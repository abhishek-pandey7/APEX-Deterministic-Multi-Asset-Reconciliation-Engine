package money

import "math/big"

// MarketValue returns quantity × price in the currency's minor units, rounded half to even.
// qty is scaled by QuantityScale and price by PriceScale; the product can exceed int64 before
// rescaling, so it is computed with big integers.
func MarketValue(qty, price int64, ccy Currency) (int64, error) {
	p := new(big.Int).Mul(big.NewInt(qty), big.NewInt(price))
	return rescale(p, QuantityScale+PriceScale, ccy.Exponent)
}

// rescale converts v from scale `from` to scale `to`, rounding half to even when digits are dropped.
func rescale(v *big.Int, from, to uint8) (int64, error) {
	if to >= from {
		out := new(big.Int).Mul(v, pow10(to-from))
		if !out.IsInt64() {
			return 0, ErrRange
		}
		return out.Int64(), nil
	}

	div := pow10(from - to)
	q, r := new(big.Int).QuoRem(v, div, new(big.Int))
	twice := new(big.Int).Lsh(new(big.Int).Abs(r), 1)
	if c := twice.Cmp(div); c > 0 || c == 0 && q.Bit(0) == 1 {
		if v.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return 0, ErrRange
	}
	return q.Int64(), nil
}

func pow10(n uint8) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
