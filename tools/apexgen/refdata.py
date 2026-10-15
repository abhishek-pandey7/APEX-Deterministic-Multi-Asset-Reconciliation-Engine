"""Reference data: currencies, accounts, counterparties and securities. All names are fictional."""

import string
from dataclasses import dataclass

# ISO 4217 minor-unit exponents, mirrored from internal/money/currency.go.
EXPONENTS = {"USD": 2, "EUR": 2, "GBP": 2, "CHF": 2, "JPY": 0, "KWD": 3}

# Units of currency per USD as exact fractions, only to give amounts realistic magnitudes.
USD_RATES = {
    "USD": (1, 1),
    "EUR": (92, 100),
    "GBP": (79, 100),
    "CHF": (88, 100),
    "JPY": (150, 1),
    "KWD": (31, 100),
}

CURRENCY_WEIGHTS = {"USD": 50, "EUR": 20, "GBP": 12, "CHF": 6, "JPY": 8, "KWD": 4}

COUNTERPARTIES = [
    "Northbridge Securities LLC",
    "Harbor Point Capital Markets Inc",
    "Elmstead Brokerage Ltd",
    "Granite Row Partners LP",
    "Bluewater Clearing Corp",
    "Kestrel Prime Services PLC",
    "Sable Island Trust Co",
    "Meridian Fund Administration SA",
    "Ashcombe Asset Management LLC",
    "Copperfield Global Markets Inc",
]

ASSET_CLASS_WEIGHTS = {"EQUITY": 50, "BOND": 25, "FUND": 15, "LOAN": 10}
ISIN_PREFIX = {"EQUITY": "US", "BOND": "XS", "FUND": "LU", "LOAN": "XS"}

_ISIN_CHARS = string.digits + string.ascii_uppercase


@dataclass(frozen=True)
class Account:
    id: str
    currencies: tuple[str, ...]


def weighted(rng, weights: dict[str, float]) -> str:
    keys = list(weights)
    return rng.choices(keys, weights=[weights[k] for k in keys])[0]


def isin_check_digit(payload: str) -> str:
    """ISIN check digit: letters become 10..35, then Luhn over the resulting digit string."""
    digits = "".join(str(int(c, 36)) for c in payload)
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2 == 0:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return str((10 - total % 10) % 10)


def make_isin(rng, prefix: str) -> str:
    payload = prefix + "".join(rng.choice(_ISIN_CHARS) for _ in range(9))
    return payload + isin_check_digit(payload)


def make_accounts(rng, count: int) -> list[Account]:
    accounts = []
    for i in range(1, count + 1):
        n = rng.choice((1, 1, 2, 3))
        picked: list[str] = []
        while len(picked) < n:
            ccy = weighted(rng, CURRENCY_WEIGHTS)
            if ccy not in picked:
                picked.append(ccy)
        accounts.append(Account(id=f"ACC{i:05d}", currencies=tuple(sorted(picked))))
    return accounts
