"""Position snapshots for one as-of date, with injected breaks and their truth.

Prices are per unit of quantity for every asset class, so bond prices sit near 1.00 per unit of
face value rather than being quoted per 100.
"""

from dataclasses import dataclass, replace
from datetime import date

from .amounts import market_value, price_e8
from .refdata import ASSET_CLASS_WEIGHTS, ISIN_PREFIX, make_isin, weighted

E6 = 1_000_000

PRICE_TOLERANCE_BPS = 5

BREAK_SCENARIOS = {
    "PRICE_WITHIN_TOLERANCE": 0.05,
    "PRICE_BREAK": 0.03,
    "SPLIT_UNBOOKED": 0.01,
    "DRIP_FRACTION": 0.01,
    "QUANTITY_BREAK": 0.02,
    "MISSING_IN_CUSTODIAN": 0.015,
    "MISSING_IN_LEDGER": 0.015,
}

# scenario -> (expected, category)
OUTCOMES = {
    "MATCH": ("MATCH", ""),
    "PRICE_WITHIN_TOLERANCE": ("MATCH", ""),
    "PRICE_BREAK": ("BREAK", "PRICE"),
    "SPLIT_UNBOOKED": ("BREAK", "CORPORATE_ACTION"),
    "DRIP_FRACTION": ("BREAK", "QUANTITY"),
    "QUANTITY_BREAK": ("BREAK", "QUANTITY"),
    "MISSING_IN_CUSTODIAN": ("BREAK", "MISSING_IN_CUSTODIAN"),
    "MISSING_IN_LEDGER": ("BREAK", "MISSING_IN_LEDGER"),
}

EQUITY_ONLY = {"SPLIT_UNBOOKED", "DRIP_FRACTION"}


@dataclass
class Position:
    account: str
    as_of: date
    security_id: str
    asset_class: str
    currency: str
    quantity_e6: int
    price_e8: int
    market_value: int = 0


@dataclass
class PositionTruth:
    account: str
    security_id: str
    scenario: str
    expected: str
    category: str


def scenario_weights(break_scale: float) -> dict[str, float]:
    weights = {k: v * break_scale for k, v in BREAK_SCENARIOS.items()}
    clean = 1.0 - sum(weights.values())
    if clean <= 0:
        raise ValueError(f"break_scale {break_scale} leaves no room for clean positions")
    return {"MATCH": clean, **weights}


def _with_value(p: Position) -> Position:
    return replace(p, market_value=market_value(p.quantity_e6, p.price_e8, p.currency))


def _random_position(rng, account, as_of, ccy, asset_class, isin) -> Position:
    if asset_class == "EQUITY":
        qty = rng.randint(10, 100_000) * E6
        if rng.random() < 0.1:
            qty += rng.randint(1, E6 - 1)
        price = price_e8(rng.randint(5_00, 500_00), ccy)
    elif asset_class == "FUND":
        qty = rng.randint(100, 1_000_000) * E6 + rng.randint(0, E6 - 1)
        price = price_e8(rng.randint(10_00, 200_00), ccy)
    else:  # BOND, LOAN: face value in thousands, priced per unit of face
        qty = rng.randint(1, 5_000) * 1_000 * E6
        price = rng.randint(85_000_000, 110_000_000)
    return Position(account, as_of, isin, asset_class, ccy, qty, price)


def _shift_bps(price: int, bps: int) -> int:
    return price * (10_000 + bps) // 10_000


def generate_positions(rng, accounts, as_of: date, break_scale: float = 1.0, per_account=(5, 30)):
    weights = scenario_weights(break_scale)
    ledger, custodian, truth = [], [], []
    for acct in accounts:
        seen: set[str] = set()
        for _ in range(rng.randint(*per_account)):
            asset_class = weighted(rng, ASSET_CLASS_WEIGHTS)
            isin = make_isin(rng, ISIN_PREFIX[asset_class])
            if isin in seen:
                continue
            seen.add(isin)
            ccy = rng.choice(acct.currencies)
            base = _random_position(rng, acct.id, as_of, ccy, asset_class, isin)

            scenario = weighted(rng, weights)
            if scenario in EQUITY_ONLY and asset_class != "EQUITY":
                scenario = "QUANTITY_BREAK"
            mirror = replace(base)

            if scenario == "PRICE_WITHIN_TOLERANCE":
                # Stay a full basis point inside the limit; flooring can add a fraction of one.
                bps = rng.randint(1, PRICE_TOLERANCE_BPS - 1)
                mirror.price_e8 = _shift_bps(base.price_e8, rng.choice((-1, 1)) * bps)
            elif scenario == "PRICE_BREAK":
                mirror.price_e8 = _shift_bps(base.price_e8, rng.choice((-1, 1)) * rng.randint(50, 300))
            elif scenario == "SPLIT_UNBOOKED":
                ratio = rng.choice((2, 3))
                mirror.quantity_e6 = base.quantity_e6 * ratio
                mirror.price_e8 = base.price_e8 // ratio
            elif scenario == "DRIP_FRACTION":
                mirror.quantity_e6 = base.quantity_e6 + rng.randint(1, E6 - 1)
            elif scenario == "QUANTITY_BREAK":
                delta = rng.randint(1, max(1, base.quantity_e6 // E6 // 10)) * E6
                # Never take the custodian quantity to zero or below.
                sign = rng.choice((-1, 1)) if delta < base.quantity_e6 else 1
                mirror.quantity_e6 = base.quantity_e6 + sign * delta

            if scenario != "MISSING_IN_LEDGER":
                ledger.append(_with_value(base))
            if scenario != "MISSING_IN_CUSTODIAN":
                custodian.append(_with_value(mirror))
            expected, category = OUTCOMES[scenario]
            truth.append(PositionTruth(acct.id, isin, scenario, expected, category))

    rng.shuffle(custodian)
    return ledger, custodian, truth
