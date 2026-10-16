"""Database access for the worker: the job queue, break context, vectors and suggestions."""

import json

import psycopg
from psycopg.rows import dict_row

from .features import format_amount, to_pgvector, vectorize

LATEST_EVENT = """JOIN LATERAL (
    SELECT e.status, e.actor, e.created_at FROM break_events e
    WHERE e.break_id = b.id ORDER BY e.id DESC LIMIT 1) s ON true"""


def connect(url: str) -> psycopg.Connection:
    return psycopg.connect(url, row_factory=dict_row, autocommit=True)


# Job queue (see migrations/000004_job_queue.up.sql)

def claim(conn, kinds: list[str], worker: str) -> dict | None:
    """Take the oldest ready job of one of `kinds`. SKIP LOCKED lets many workers poll at once."""
    return conn.execute(
        """UPDATE jobs SET status = 'RUNNING', attempts = attempts + 1, locked_by = %s, locked_at = now()
           WHERE id = (SELECT id FROM jobs
                       WHERE status = 'QUEUED' AND kind = ANY(%s) AND run_after <= now()
                       ORDER BY run_after, id
                       FOR UPDATE SKIP LOCKED LIMIT 1)
           RETURNING id, kind, payload, attempts, max_attempts""",
        (worker, kinds),
    ).fetchone()


def finish(conn, job_id: int) -> None:
    conn.execute("UPDATE jobs SET status = 'DONE', last_error = NULL WHERE id = %s", (job_id,))


def fail(conn, job: dict, error: str) -> None:
    """Retry with exponential backoff until max_attempts, then give up."""
    if job["attempts"] >= job["max_attempts"]:
        conn.execute("UPDATE jobs SET status = 'FAILED', last_error = %s WHERE id = %s", (error, job["id"]))
    else:
        conn.execute(
            """UPDATE jobs SET status = 'QUEUED', last_error = %s,
                      run_after = now() + make_interval(secs => power(2, attempts))
               WHERE id = %s""",
            (error, job["id"]),
        )


def enqueue(conn, kind: str, payload: dict) -> int:
    return conn.execute("INSERT INTO jobs (kind, payload) VALUES (%s, %s) RETURNING id",
                        (kind, json.dumps(payload))).fetchone()["id"]


# Breaks and vectors

def _breaks_with_legs(conn, where: str, params) -> list[dict]:
    return conn.execute(
        f"""SELECT b.id, b.category, b.currency, b.amount_minor,
                   coalesce(json_agg(json_build_object('side', e.side, 'reference', e.reference)
                            ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '[]') AS legs
            FROM breaks b
            LEFT JOIN break_legs l ON l.break_id = b.id
            LEFT JOIN cash_entries e ON e.id = l.cash_entry_id
            WHERE {where}
            GROUP BY b.id ORDER BY b.id""",
        params,
    ).fetchall()


def vectorize_breaks(conn, where: str, params) -> int:
    """Compute and store vectors for breaks matching `where` that don't have one yet."""
    rows = _breaks_with_legs(conn, f"({where}) AND NOT EXISTS (SELECT 1 FROM break_vectors v WHERE v.break_id = b.id)", params)
    if not rows:
        return 0
    with conn.cursor().copy("COPY break_vectors (break_id, features) FROM STDIN") as copy:
        for row in rows:
            copy.write_row((row["id"], to_pgvector(vectorize(row))))
    return len(rows)


def vectorize_run(conn, run_id: int) -> int:
    return vectorize_breaks(conn, "b.run_id = %s", (run_id,))


def similar(conn, break_id: int, k: int = 5) -> list[dict]:
    """The k nearest other breaks, preferring ones an analyst has already resolved, with how."""
    vectorize_breaks(conn, "b.id = %s", (break_id,))
    rows = conn.execute(
        f"""SELECT b.id, b.run_id, b.category, b.amount_minor, b.currency, b.reason,
                   s.status, s.actor AS status_by,
                   (SELECT h.reason FROM break_events h
                     WHERE h.break_id = b.id AND h.actor <> 'engine' AND h.reason <> ''
                     ORDER BY h.id DESC LIMIT 1) AS resolution,
                   v.features <=> (SELECT features FROM break_vectors WHERE break_id = %(id)s) AS distance
            FROM (SELECT break_id, features FROM break_vectors
                  WHERE break_id <> %(id)s
                  ORDER BY features <=> (SELECT features FROM break_vectors WHERE break_id = %(id)s), break_id DESC
                  LIMIT 50) v
            JOIN breaks b ON b.id = v.break_id
            {LATEST_EVENT}
            ORDER BY distance, b.id DESC""",
        {"id": break_id},
    ).fetchall()
    resolved = [r for r in rows if r["resolution"]]
    others = [r for r in rows if not r["resolution"]]
    picked = (resolved + others)[:k]
    for r in picked:
        r["distance"] = round(float(r["distance"]), 4)
    return picked


def break_context(conn, break_id: int) -> dict | None:
    """Everything about a break the advisor may see: no internal ids beyond the break's own."""
    brk = conn.execute(
        f"""SELECT b.id, b.category, b.reason, b.account_id, b.currency, b.amount_minor,
                   b.opened_on, s.status
            FROM breaks b {LATEST_EVENT} WHERE b.id = %s""",
        (break_id,),
    ).fetchone()
    if brk is None:
        return None
    brk["difference_ledger_minus_custodian"] = format_amount(brk.pop("amount_minor"), brk["currency"])
    entries = conn.execute(
        """SELECT e.side, e.amount_minor, e.currency, e.value_date, e.txn_type, e.reference, e.counterparty
           FROM break_legs l JOIN cash_entries e ON e.id = l.cash_entry_id
           WHERE l.break_id = %s ORDER BY e.side DESC, e.value_date, e.id""",
        (break_id,),
    ).fetchall()
    for e in entries:
        e["amount"] = format_amount(e.pop("amount_minor"), e.pop("currency"))
    brk["entries"] = entries
    brk["history"] = conn.execute(
        "SELECT status, actor, reason FROM break_events WHERE break_id = %s ORDER BY id", (break_id,)
    ).fetchall()
    return brk


def insert_suggestion(conn, break_id: int, s) -> int:
    return conn.execute(
        """INSERT INTO break_suggestions (break_id, model, prompt_sha256, category, confidence, explanation,
                                          next_action, action_detail, similar_break_ids, latency_ms)
           VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s) RETURNING id""",
        (break_id, s.model, s.prompt_sha256, s.category, s.confidence, s.explanation,
         s.next_action, s.action_detail, s.similar_break_ids, s.latency_ms),
    ).fetchone()["id"]
