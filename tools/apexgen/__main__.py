"""python -m apexgen --out ../data/small --seed 42"""

import argparse
import hashlib
import json
import random
from collections import Counter
from datetime import date
from pathlib import Path

from . import VERSION, cash, formats, positions
from .bizdays import business_days
from .refdata import make_accounts


def generate(out: Path, seed: int, accounts: int, days: int, events_per_day: int,
             start: date, break_scale: float) -> dict:
    # One stream per component, so changing e.g. the position settings never changes the cash data.
    def stream(name: str) -> random.Random:
        return random.Random(f"{seed}/{name}")

    out.mkdir(parents=True, exist_ok=True)
    accts = make_accounts(stream("accounts"), accounts)
    calendar = business_days(start, days)

    groups = cash.CashGenerator(stream("cash"), break_scale).run(accts, calendar, events_per_day)
    ledger = [e for g in groups for e in g.ledger]
    custodian = [c for g in groups for c in g.custodian]
    # Real feeds aren't aligned with the ledger; ids are assigned after shuffling so they leak nothing.
    stream("custodian-order").shuffle(custodian)
    for i, c in enumerate(custodian, 1):
        c.id = f"C{i:09d}"

    pos_ledger, pos_custodian, pos_truth = positions.generate_positions(
        stream("positions"), accts, calendar[-1], break_scale
    )

    formats.write_ledger_cash(out / "ledger_cash.csv", ledger)
    formats.write_custodian_cash(out / "custodian_cash.csv", stream("narratives"), custodian)
    formats.write_cash_truth(out / "truth_cash.csv", groups)
    formats.write_ledger_positions(out / "ledger_positions.csv", pos_ledger)
    formats.write_custodian_positions(out / "custodian_positions.csv", pos_custodian)
    formats.write_position_truth(out / "truth_positions.csv", pos_truth)

    files = {}
    for path in sorted(out.glob("*.csv")):
        data = path.read_bytes()
        files[path.name] = {
            "rows": data.count(b"\n") - 1,
            "sha256": hashlib.sha256(data).hexdigest(),
        }

    manifest = {
        "generator_version": VERSION,
        "seed": seed,
        "params": {
            "accounts": accounts,
            "business_days": days,
            "events_per_day": events_per_day,
            "start": start.isoformat(),
            "as_of": calendar[-1].isoformat(),
            "break_scale": break_scale,
        },
        "engine_hints": {
            "date_window_business_days": cash.DATE_WINDOW_BUSINESS_DAYS,
            "amount_tolerance_minor": cash.AMOUNT_TOLERANCE_MINOR,
            "price_tolerance_bps": positions.PRICE_TOLERANCE_BPS,
        },
        "scenarios": {
            "cash": dict(sorted(Counter(g.scenario for g in groups).items())),
            "positions": dict(sorted(Counter(t.scenario for t in pos_truth).items())),
        },
        "files": files,
    }
    (out / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8", newline="\n")
    return manifest


def main() -> None:
    p = argparse.ArgumentParser(prog="apexgen", description=__doc__)
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--accounts", type=int, default=20)
    p.add_argument("--days", type=int, default=20, help="business days of cash activity")
    p.add_argument("--events-per-day", type=int, default=25, help="per account and currency")
    p.add_argument("--start", type=date.fromisoformat, default=date(2026, 9, 1))
    p.add_argument("--break-scale", type=float, default=1.0, help="multiplies every break rate")
    a = p.parse_args()

    m = generate(a.out, a.seed, a.accounts, a.days, a.events_per_day, a.start, a.break_scale)
    for name, info in m["files"].items():
        print(f"{name:26} {info['rows']:>10,} rows")


if __name__ == "__main__":
    main()
