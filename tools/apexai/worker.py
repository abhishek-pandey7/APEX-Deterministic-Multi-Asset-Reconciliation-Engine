"""The worker loop: claim a job, handle it, record the outcome."""

import os
import socket
import sys
import time

from . import db
from .advisor import GroqAdvisor, RulesAdvisor, screen
from .features import format_amount

KINDS = ["vectorize_run", "suggest_break"]


def log(msg: str) -> None:
    print(f"{time.strftime('%H:%M:%S')} {msg}", file=sys.stderr, flush=True)


def make_advisor(settings):
    if not settings.groq_api_key:
        log("GROQ_API_KEY not set: suggestions will come from rules only")
        return RulesAdvisor()
    from groq import Groq  # imported lazily so the worker runs without the package when unused

    log(f"suggestions from {settings.model} on Groq, rules as fallback")
    return GroqAdvisor(Groq(api_key=settings.groq_api_key, max_retries=2, timeout=30), settings.model, log=log)


def suggest(conn, advisor, break_id: int):
    ctx = db.break_context(conn, break_id)
    if ctx is None:
        raise LookupError(f"break {break_id} does not exist")
    near = db.similar(conn, break_id)
    # The model sees what the past breaks were and how they ended, not their database ids.
    shown = [{"category": r["category"], "difference": format_amount(r["amount_minor"], r["currency"]),
              "reason": r["reason"], "status": r["status"], "resolution": r["resolution"]} for r in near]
    s = screen(ctx, advisor.suggest(ctx, shown))
    s.similar_break_ids = [r["id"] for r in near]
    return db.insert_suggestion(conn, break_id, s), s


def handle(conn, advisor, job: dict) -> str:
    payload = job["payload"]
    if job["kind"] == "vectorize_run":
        n = db.vectorize_run(conn, int(payload["run_id"]))
        return f"vectorized {n} breaks of run {payload['run_id']}"
    if job["kind"] == "suggest_break":
        sid, s = suggest(conn, advisor, int(payload["break_id"]))
        return f"suggestion {sid} for break {payload['break_id']} from {s.model}: {s.next_action}"
    raise ValueError(f"unknown job kind {job['kind']}")


def run(settings, once: bool = False, poll_seconds: float = 1.0) -> None:
    advisor = make_advisor(settings)
    name = f"{socket.gethostname()}:{os.getpid()}"
    with db.connect(settings.db_url) as conn:
        log(f"worker {name} polling for {', '.join(KINDS)}")
        while True:
            job = db.claim(conn, KINDS, name)
            if job is None:
                if once:
                    return
                time.sleep(poll_seconds)
                continue
            try:
                log(f"job {job['id']}: {handle(conn, advisor, job)}")
                db.finish(conn, job["id"])
            except Exception as exc:
                log(f"job {job['id']} failed (attempt {job['attempts']}): {exc}")
                db.fail(conn, job, f"{type(exc).__name__}: {exc}")
