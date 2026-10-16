"""python -m apexai worker | vectorize --run N | suggest --break N"""

import argparse
import json
from dataclasses import asdict

from . import db, worker
from .config import Settings


def main() -> None:
    p = argparse.ArgumentParser(prog="apexai", description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)
    w = sub.add_parser("worker", help="process jobs from the queue")
    w.add_argument("--once", action="store_true", help="stop when the queue is empty")
    v = sub.add_parser("vectorize", help="compute similarity vectors for a run's breaks")
    v.add_argument("--run", type=int, required=True)
    s = sub.add_parser("suggest", help="write a suggestion for one break now and print it")
    s.add_argument("--break", dest="break_id", type=int, required=True)
    args = p.parse_args()

    settings = Settings.from_env()
    if args.cmd == "worker":
        worker.run(settings, once=args.once)
        return
    with db.connect(settings.db_url) as conn:
        if args.cmd == "vectorize":
            print(f"vectorized {db.vectorize_run(conn, args.run)} breaks")
        else:
            sid, sug = worker.suggest(conn, worker.make_advisor(settings), args.break_id)
            out = asdict(sug)
            out["prompt_sha256"] = sug.prompt_sha256.hex() if sug.prompt_sha256 else None
            print(json.dumps({"suggestion_id": sid, **out}, indent=2))


if __name__ == "__main__":
    main()
