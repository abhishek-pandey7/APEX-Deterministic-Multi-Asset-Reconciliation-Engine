"""File layouts. The ledger and the custodian deliberately disagree on format, the way real feeds do:

    ledger     signed decimal amount, ISO dates, clean reference and counterparty
    custodian  absolute amount with a D/C flag, YYYYMMDD dates, free-text narrative
"""

import csv
import re

from .amounts import format_fixed, format_trimmed
from .refdata import EXPONENTS

LEDGER_CASH_HEADER = [
    "entry_id", "account_id", "currency", "amount", "value_date", "trade_date",
    "txn_type", "reference", "counterparty",
]
CUSTODIAN_CASH_HEADER = ["Stmt Ref", "Acct No", "Ccy", "Value Dt", "Amount", "D/C", "Narrative"]
CASH_TRUTH_HEADER = ["group_id", "scenario", "expected", "category", "ledger_ids", "custodian_ids"]
LEDGER_POSITION_HEADER = [
    "account_id", "as_of_date", "security_id", "asset_class", "currency",
    "quantity", "price", "market_value",
]
CUSTODIAN_POSITION_HEADER = ["Acct No", "Pos Date", "ISIN", "Ccy", "Settled Qty", "Px", "Mkt Val"]
POSITION_TRUTH_HEADER = ["account_id", "security_id", "scenario", "expected", "category"]

NARRATIVE_PREFIX = {
    "TRADE_SETTLEMENT": "SETTLEMENT",
    "FEE": "CHARGES",
    "DIVIDEND": "DIVIDEND",
    "COUPON": "COUPON PMT",
    "FX": "FX DEAL",
    "CAPITAL_CALL": "CAPITAL CALL",
    "DISTRIBUTION": "DISTRIBUTION",
}

_LEGAL_SUFFIX = re.compile(r"\b(LLC|INC|LTD|LP|PLC|CORP|CO|SA)\b")
_ABBREVIATIONS = {
    "SECURITIES": "SEC",
    "CAPITAL": "CAP",
    "MARKETS": "MKTS",
    "MANAGEMENT": "MGT",
    "ADMINISTRATION": "ADMIN",
    "BROKERAGE": "BRKG",
    "SERVICES": "SVCS",
}


def short_counterparty(name: str) -> str:
    """'Harbor Point Capital Markets Inc' -> 'HARBOR POINT CAP MKTS'."""
    words = _LEGAL_SUFFIX.sub("", name.upper()).split()
    return " ".join(_ABBREVIATIONS.get(w, w) for w in words)


def mangle_reference(rng, ref: str) -> str:
    """Corrupt a reference the way custodian systems and manual keying do."""
    compact = ref.replace("-", "")
    ops = rng.sample(("truncate", "transpose", "drop_prefix", "wrap"), rng.randint(1, 2))
    out = compact
    for op in ops:
        if op == "truncate" and len(out) > 12:
            out = out[:12]
        elif op == "transpose":
            # Only swap differing digits; swapping "00" would leave the reference unchanged.
            digits = [
                i for i in range(len(out) - 1)
                if out[i].isdigit() and out[i + 1].isdigit() and out[i] != out[i + 1]
            ]
            if digits:
                i = rng.choice(digits)
                out = out[:i] + out[i + 1] + out[i] + out[i + 2:]
        elif op == "drop_prefix":
            out = out.lstrip("ABCDEFGHIJKLMNOPQRSTUVWXYZ") or out
        elif op == "wrap":
            out = f"/RFB/{out}"
    return out


def narrative(rng, entry) -> str:
    if entry.noisy:
        ref = mangle_reference(rng, entry.reference)
    else:
        ref = entry.reference.replace("-", "") if rng.random() < 0.5 else entry.reference
    parts = [NARRATIVE_PREFIX[entry.txn_type], "REF", ref]
    if entry.counterparty:
        parts.append(short_counterparty(entry.counterparty))
    return " ".join(parts)


def _writer(f):
    # Fixed "\n" line endings keep files byte-identical across operating systems.
    return csv.writer(f, lineterminator="\n")


def write_ledger_cash(path, entries):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(LEDGER_CASH_HEADER)
        for e in entries:
            w.writerow([
                e.id, e.account, e.currency, format_fixed(e.amount, EXPONENTS[e.currency]),
                e.value_date.isoformat(), e.trade_date.isoformat(), e.txn_type, e.reference,
                e.counterparty,
            ])


def write_custodian_cash(path, rng, entries):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(CUSTODIAN_CASH_HEADER)
        for c in entries:
            w.writerow([
                c.id, c.account, c.currency, f"{c.value_date:%Y%m%d}",
                format_fixed(abs(c.amount), EXPONENTS[c.currency]),
                "C" if c.amount > 0 else "D", narrative(rng, c),
            ])


def write_cash_truth(path, groups):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(CASH_TRUTH_HEADER)
        for i, g in enumerate(groups, 1):
            w.writerow([
                f"G{i:08d}", g.scenario, g.expected, g.category,
                ";".join(e.id for e in g.ledger), ";".join(c.id for c in g.custodian),
            ])



def write_ledger_positions(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(LEDGER_POSITION_HEADER)
        for p in rows:
            w.writerow([
                p.account, p.as_of.isoformat(), p.security_id, p.asset_class, p.currency,
                format_fixed(p.quantity_e6, 6), format_fixed(p.price_e8, 8),
                format_fixed(p.market_value, EXPONENTS[p.currency]),
            ])


def write_custodian_positions(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(CUSTODIAN_POSITION_HEADER)
        for p in rows:
            w.writerow([
                p.account, f"{p.as_of:%Y%m%d}", p.security_id, p.currency,
                format_trimmed(p.quantity_e6, 6), format_trimmed(p.price_e8, 8),
                format_fixed(p.market_value, EXPONENTS[p.currency]),
            ])


def write_position_truth(path, truth):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = _writer(f)
        w.writerow(POSITION_TRUTH_HEADER)
        for t in truth:
            w.writerow([t.account, t.security_id, t.scenario, t.expected, t.category])
