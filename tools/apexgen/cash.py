"""Cash scenarios: ledger entries, their custodian counterparts, and the truth linking them.

Each scenario emits one or more Groups. A Group records which ledger and custodian entries belong
together and what a correct engine should do with them:

    AUTO_MATCH  match without human review
    PROPOSE     link them, but only as a proposal for a human to confirm
    BREAK       raise a break with the given category
"""

from dataclasses import dataclass, replace
from datetime import date

from .amounts import convert_usd_cents, random_usd_cents
from .bizdays import add_business_days
from .refdata import COUNTERPARTIES, make_isin, weighted

# Engine settings the scenarios are built around. They go into the manifest so the evaluation
# knows which differences were meant to be absorbed by tolerance and which were not.
DATE_WINDOW_BUSINESS_DAYS = 2
AMOUNT_TOLERANCE_MINOR = 99

# Share of events per scenario at break_scale 1.0. EXACT takes whatever is left.
BREAK_SCENARIOS = {
    "TIMING": 0.08,
    "ROUNDING": 0.03,
    "FUZZY_REF": 0.03,
    "SPLIT": 0.03,
    "AGGREGATE": 0.02,
    "MANY_TO_MANY": 0.005,
    "MISSING_IN_CUSTODIAN": 0.02,
    "MISSING_IN_LEDGER": 0.02,
    "DUPLICATE": 0.01,
    "AMOUNT_MISMATCH": 0.02,
}

TXN_TYPE_WEIGHTS = {
    "TRADE_SETTLEMENT": 45,
    "FEE": 20,
    "DIVIDEND": 10,
    "COUPON": 8,
    "FX": 7,
    "CAPITAL_CALL": 5,
    "DISTRIBUTION": 5,
}

# (low USD, high USD, sign). Sign 0 means either direction.
AMOUNT_RANGES = {
    "TRADE_SETTLEMENT": (10_000, 5_000_000, 0),
    "FEE": (10, 5_000, -1),
    "DIVIDEND": (100, 50_000, 1),
    "COUPON": (1_000, 200_000, 1),
    "FX": (10_000, 1_000_000, 0),
    "CAPITAL_CALL": (100_000, 20_000_000, -1),
    "DISTRIBUTION": (50_000, 10_000_000, 1),
}

# Round fee amounts recur within an account and day, so exact-key ties are common.
STANDARD_FEES_USD = (25, 50, 100, 250, 500)

SPLITTABLE_TYPES = ("TRADE_SETTLEMENT", "CAPITAL_CALL", "DISTRIBUTION")


@dataclass
class LedgerEntry:
    id: str
    account: str
    currency: str
    amount: int
    value_date: date
    trade_date: date
    txn_type: str
    reference: str
    counterparty: str


@dataclass
class CustodianEntry:
    account: str
    currency: str
    amount: int
    value_date: date
    txn_type: str
    reference: str
    counterparty: str
    noisy: bool = False
    id: str = ""


@dataclass
class Group:
    scenario: str
    expected: str
    category: str
    ledger: list[LedgerEntry]
    custodian: list[CustodianEntry]


def scenario_weights(break_scale: float) -> dict[str, float]:
    weights = {k: v * break_scale for k, v in BREAK_SCENARIOS.items()}
    exact = 1.0 - sum(weights.values())
    if exact <= 0:
        raise ValueError(f"break_scale {break_scale} leaves no room for clean entries")
    return {"EXACT": exact, **weights}


def split_amount(rng, total: int, parts: int) -> list[int]:
    """Split a signed amount into `parts` non-zero pieces with the same sign."""
    sign = 1 if total > 0 else -1
    mag = abs(total)
    parts = min(parts, mag)
    cuts = sorted(rng.sample(range(1, mag), parts - 1)) if parts > 1 else []
    bounds = [0, *cuts, mag]
    return [sign * (b - a) for a, b in zip(bounds, bounds[1:])]


class CashGenerator:
    def __init__(self, rng, break_scale: float = 1.0):
        self.rng = rng
        self.weights = scenario_weights(break_scale)
        self.groups: list[Group] = []
        self._ledger_seq = 0
        self._ref_seq = 0
        self._handlers = {
            "EXACT": self._exact,
            "TIMING": self._timing,
            "ROUNDING": self._rounding,
            "FUZZY_REF": self._fuzzy_ref,
            "SPLIT": self._split,
            "AGGREGATE": self._aggregate,
            "MANY_TO_MANY": self._many_to_many,
            "MISSING_IN_CUSTODIAN": self._missing_in_custodian,
            "MISSING_IN_LEDGER": self._missing_in_ledger,
            "DUPLICATE": self._duplicate,
            "AMOUNT_MISMATCH": self._amount_mismatch,
        }

    def run(self, accounts, days: list[date], events_per_day: int) -> list[Group]:
        for acct in accounts:
            for ccy in acct.currencies:
                for day in days:
                    for _ in range(events_per_day):
                        scenario = weighted(self.rng, self.weights)
                        self._handlers[scenario](acct.id, ccy, day)
        return self.groups

    # Building blocks

    def _ledger(self, account: str, ccy: str, day: date, txn_type: str | None = None) -> LedgerEntry:
        rng = self.rng
        txn_type = txn_type or weighted(rng, TXN_TYPE_WEIGHTS)
        lo, hi, sign = AMOUNT_RANGES[txn_type]
        if txn_type == "FEE" and rng.random() < 0.5:
            usd_cents = rng.choice(STANDARD_FEES_USD) * 100
        else:
            usd_cents = random_usd_cents(rng, lo, hi)
        if sign == 0:
            sign = rng.choice((-1, 1))
        self._ledger_seq += 1
        lag = 1 if txn_type == "TRADE_SETTLEMENT" else 0
        return LedgerEntry(
            id=f"L{self._ledger_seq:09d}",
            account=account,
            currency=ccy,
            amount=sign * convert_usd_cents(usd_cents, ccy),
            value_date=day,
            trade_date=add_business_days(day, -lag),
            txn_type=txn_type,
            reference=self._reference(txn_type, day),
            counterparty=rng.choice(COUNTERPARTIES),
        )

    def _reference(self, txn_type: str, day: date) -> str:
        self._ref_seq += 1
        n = self._ref_seq
        if txn_type == "TRADE_SETTLEMENT":
            return f"TRD-{day.year}-{n:07d}"
        if txn_type == "FEE":
            return f"FEE-{n:07d}"
        if txn_type == "DIVIDEND":
            return f"DIV-{make_isin(self.rng, 'US')}-{day:%Y%m%d}"
        if txn_type == "COUPON":
            return f"CPN-{make_isin(self.rng, 'XS')}-{day:%Y%m%d}"
        if txn_type == "CAPITAL_CALL":
            return f"CC-FND{n % 997:03d}-{n:07d}"
        if txn_type == "DISTRIBUTION":
            return f"DST-FND{n % 997:03d}-{n:07d}"
        return f"FX-{n:07d}"

    @staticmethod
    def _mirror(entry: LedgerEntry, **changes) -> CustodianEntry:
        c = CustodianEntry(
            account=entry.account,
            currency=entry.currency,
            amount=entry.amount,
            value_date=entry.value_date,
            txn_type=entry.txn_type,
            reference=entry.reference,
            counterparty=entry.counterparty,
        )
        return replace(c, **changes)

    def _group(self, scenario, expected, category, ledger, custodian):
        self.groups.append(Group(scenario, expected, category, ledger, custodian))

    # Scenarios

    def _exact(self, account, ccy, day):
        entry = self._ledger(account, ccy, day)
        self._group("EXACT", "AUTO_MATCH", "", [entry], [self._mirror(entry)])
        # Repeat round fees on the same day so identical exact keys occur; only the
        # reference can tell these apart.
        if entry.txn_type == "FEE" and self.rng.random() < 0.3:
            for _ in range(self.rng.randint(1, 2)):
                twin = replace(
                    self._ledger(account, ccy, day, "FEE"),
                    amount=entry.amount,
                )
                self._group("EXACT", "AUTO_MATCH", "", [twin], [self._mirror(twin)])

    def _timing(self, account, ccy, day):
        entry = self._ledger(account, ccy, day)
        shift = self.rng.randint(1, DATE_WINDOW_BUSINESS_DAYS)
        custodian = self._mirror(entry, value_date=add_business_days(day, shift))
        self._group("TIMING", "AUTO_MATCH", "", [entry], [custodian])

    def _rounding(self, account, ccy, day):
        entry = self._ledger(account, ccy, day)
        delta = self.rng.randint(1, AMOUNT_TOLERANCE_MINOR)
        if abs(entry.amount) <= 2 * delta:
            delta = 1
        delta *= self.rng.choice((-1, 1))
        custodian = self._mirror(entry, amount=entry.amount + delta)
        self._group("ROUNDING", "AUTO_MATCH", "", [entry], [custodian])

    def _fuzzy_ref(self, account, ccy, day):
        # Settles outside the date window with a mangled narrative: only reference evidence links it.
        entry = self._ledger(account, ccy, day)
        shift = self.rng.randint(DATE_WINDOW_BUSINESS_DAYS + 1, DATE_WINDOW_BUSINESS_DAYS + 4)
        custodian = self._mirror(entry, value_date=add_business_days(day, shift), noisy=True)
        self._group("FUZZY_REF", "PROPOSE", "", [entry], [custodian])

    def _split(self, account, ccy, day):
        entry = self._ledger(account, ccy, day, self.rng.choice(SPLITTABLE_TYPES))
        parts = split_amount(self.rng, entry.amount, self.rng.randint(2, 4))
        custodian = [
            self._mirror(entry, amount=p, value_date=add_business_days(day, self.rng.randint(0, 1)))
            for p in parts
        ]
        self._group("SPLIT", "AUTO_MATCH", "", [entry], custodian)

    def _aggregate(self, account, ccy, day):
        fees = [self._ledger(account, ccy, day, "FEE") for _ in range(self.rng.randint(2, 4))]
        batch = CustodianEntry(
            account=account,
            currency=ccy,
            amount=sum(f.amount for f in fees),
            value_date=day,
            txn_type="FEE",
            reference=f"FEE BATCH {day:%Y%m%d}",
            counterparty="",
        )
        self._group("AGGREGATE", "AUTO_MATCH", "", fees, [batch])

    def _many_to_many(self, account, ccy, day):
        first = self._ledger(account, ccy, day, "TRADE_SETTLEMENT")
        second = self._ledger(account, ccy, day, "TRADE_SETTLEMENT")
        sign = 1 if first.amount > 0 else -1
        second.amount = sign * abs(second.amount)
        parts = split_amount(self.rng, first.amount + second.amount, 3)
        custodian = [
            self._mirror(first, amount=p, reference=f"NET-{first.reference}") for p in parts
        ]
        self._group("MANY_TO_MANY", "AUTO_MATCH", "", [first, second], custodian)

    def _missing_in_custodian(self, account, ccy, day):
        entry = self._ledger(account, ccy, day)
        self._group("MISSING_IN_CUSTODIAN", "BREAK", "MISSING_IN_CUSTODIAN", [entry], [])

    def _missing_in_ledger(self, account, ccy, day):
        # Built like a ledger entry, but the ledger never books it.
        txn_type = self.rng.choice(("DIVIDEND", "FEE"))
        unbooked = self._ledger(account, ccy, day, txn_type)
        category = "CORPORATE_ACTION" if txn_type == "DIVIDEND" else "MISSING_IN_LEDGER"
        self._group("MISSING_IN_LEDGER", "BREAK", category, [], [self._mirror(unbooked)])

    def _duplicate(self, account, ccy, day):
        # The two custodian lines are identical, so either one may be the matched copy.
        entry = self._ledger(account, ccy, day)
        self._group("DUPLICATE", "AUTO_MATCH", "", [entry], [self._mirror(entry)])
        self._group("DUPLICATE", "BREAK", "DUPLICATE", [], [self._mirror(entry)])

    def _amount_mismatch(self, account, ccy, day):
        entry = self._ledger(account, ccy, day)
        bps = self.rng.randint(100, 1000)
        diff = max(abs(entry.amount) * bps // 10_000, 10 * (AMOUNT_TOLERANCE_MINOR + 1))
        sign = 1 if entry.amount > 0 else -1
        # Push the amount further from zero so the sign never flips.
        custodian = self._mirror(entry, amount=entry.amount + sign * diff)
        self._group("AMOUNT_MISMATCH", "BREAK", "AMOUNT", [entry], [custodian])
