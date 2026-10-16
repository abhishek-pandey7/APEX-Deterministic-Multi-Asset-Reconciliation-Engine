"""Deterministic feature vectors for finding similar breaks.

A break is described by what an analyst would compare: its category, how big the difference is,
which way it goes, the shape of the evidence (how many entries on each side), and the vocabulary
of its references (SETTLEMENT, CHARGES, the TRD/FEE/DIV id prefixes, counterparty names).
Specific id digits are left out on purpose: two fee breaks are alike whatever their fee numbers.

Features are placed with the signed hashing trick, using SHA-256 rather than Python's hash(),
which is randomised per process. The same break always gets the same vector, on any machine.
"""

import hashlib
import math
import re

DIM = 64

# ISO 4217 minor-unit exponents, as in internal/money/currency.go.
_EXPONENTS = {"JPY": 0, "KRW": 0, "CLP": 0, "ISK": 0, "VND": 0,
              "BHD": 3, "JOD": 3, "KWD": 3, "OMR": 3, "TND": 3}

_TOKEN = re.compile(r"[A-Z0-9]+")


def _slot(name: str) -> tuple[int, float]:
    digest = hashlib.sha256(name.encode()).digest()
    return int.from_bytes(digest[:4], "big") % DIM, (1.0 if digest[4] & 1 else -1.0)


def _words(text: str) -> set[str]:
    """Reference vocabulary: plain words, and the alphabetic prefix of ids (TRD2026... -> TRD)."""
    out = set()
    for tok in _TOKEN.findall(text.upper()):
        if tok.isdigit():
            continue
        prefix = re.match(r"[A-Z]+", tok)
        word = prefix.group(0) if prefix else ""
        if len(word) >= 2 and word != "REF":
            out.add(word)
    return out


def magnitude(amount_minor: int, currency: str) -> int:
    """Order of magnitude of the amount in major units: 0 for under 10, 1 for under 100, ..."""
    major = abs(amount_minor) // 10 ** _EXPONENTS.get(currency, 2)
    return len(str(major)) - 1 if major else -1


def vectorize(brk: dict) -> list[float]:
    """brk: category, currency, amount_minor, and legs as [{"side": ..., "reference": ...}]."""
    weights: dict[str, float] = {}

    def add(name: str, w: float) -> None:
        weights[name] = weights.get(name, 0.0) + w

    add(f"category:{brk['category']}", 3.0)
    add(f"currency:{brk['currency']}", 0.5)
    add(f"magnitude:{magnitude(brk['amount_minor'], brk['currency'])}", 1.0)
    amount = brk["amount_minor"]
    add(f"direction:{'ledger-higher' if amount > 0 else 'custodian-higher' if amount < 0 else 'equal'}", 0.5)
    sides = [leg["side"] for leg in brk["legs"]]
    add(f"shape:{sides.count('LEDGER')}x{sides.count('CUSTODIAN')}", 1.0)
    for leg in brk["legs"]:
        for word in _words(leg.get("reference") or ""):
            add(f"word:{leg['side']}:{word}", 0.5)

    vec = [0.0] * DIM
    for name in sorted(weights):
        i, sign = _slot(name)
        vec[i] += sign * weights[name]
    norm = math.sqrt(sum(v * v for v in vec)) or 1.0
    return [round(v / norm, 6) for v in vec]


def format_amount(amount_minor: int, currency: str) -> str:
    """-418744424 EUR minor units -> "-4,187,444.24 EUR". Language models misread raw minor units
    as major units, so amounts only ever reach a prompt in this form."""
    exp = _EXPONENTS.get(currency, 2)
    sign = "-" if amount_minor < 0 else ""
    whole, frac = divmod(abs(amount_minor), 10**exp)
    text = f"{whole:,}" + (f".{frac:0{exp}d}" if exp else "")
    return f"{sign}{text} {currency}"


def to_pgvector(vec: list[float]) -> str:
    return "[" + ",".join(f"{v:.6f}" for v in vec) + "]"
