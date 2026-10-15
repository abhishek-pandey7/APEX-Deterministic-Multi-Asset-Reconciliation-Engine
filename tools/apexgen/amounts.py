"""Integer-only amount helpers.

Nothing here touches floating point, so a given seed produces byte-identical files on every
platform. Rounding and formatting mirror internal/money in the Go engine.
"""

from .refdata import EXPONENTS, USD_RATES

QUANTITY_SCALE = 6
PRICE_SCALE = 8


def div_half_even(n: int, d: int) -> int:
    q, r = divmod(abs(n), d)
    if 2 * r > d or (2 * r == d and q % 2 == 1):
        q += 1
    return q if n >= 0 else -q


def format_fixed(v: int, scale: int) -> str:
    sign = "-" if v < 0 else ""
    digits = str(abs(v)).rjust(scale + 1, "0")
    if scale == 0:
        return sign + digits
    return f"{sign}{digits[:-scale]}.{digits[-scale:]}"


def format_trimmed(v: int, scale: int) -> str:
    """Like format_fixed but drops trailing fractional zeros, as many custodian feeds do."""
    s = format_fixed(v, scale)
    if "." in s:
        s = s.rstrip("0").rstrip(".")
    return s


def market_value(qty_e6: int, price_e8: int, currency: str) -> int:
    drop = QUANTITY_SCALE + PRICE_SCALE - EXPONENTS[currency]
    return div_half_even(qty_e6 * price_e8, 10**drop)


def random_usd_cents(rng, lo_usd: int, hi_usd: int) -> int:
    """Roughly log-uniform: pick a decade between the bounds, then a uniform value inside it."""
    lo, hi = lo_usd * 100, hi_usd * 100
    decades = []
    d = lo
    while d < hi:
        decades.append((d, min(d * 10, hi)))
        d *= 10
    a, b = rng.choice(decades)
    return rng.randint(a, b - 1)


def convert_usd_cents(usd_cents: int, currency: str) -> int:
    """USD cents to minor units of `currency`, never less than one minor unit."""
    num, den = USD_RATES[currency]
    minor = div_half_even(usd_cents * num * 10 ** EXPONENTS[currency], den * 100)
    return max(1, minor)


def price_e8(usd_cents: int, currency: str) -> int:
    return convert_usd_cents(usd_cents, currency) * 10 ** (PRICE_SCALE - EXPONENTS[currency])
