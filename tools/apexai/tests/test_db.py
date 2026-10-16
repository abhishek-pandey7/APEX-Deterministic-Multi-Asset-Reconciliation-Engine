"""Integration tests against a migrated database. Set APEX_TEST_DB_URL to run them.

Rows are left behind on purpose: break events and suggestions are append-only by design, and
every fixture uses a fresh account so reruns don't collide.
"""

import os
import secrets
import unittest

from apexai import db, worker
from apexai.advisor import RulesAdvisor

URL = os.environ.get("APEX_TEST_DB_URL", "")


@unittest.skipUnless(URL, "APEX_TEST_DB_URL not set")
class WorkerDatabaseTest(unittest.TestCase):
    def setUp(self):
        self.conn = db.connect(URL)
        self.addCleanup(self.conn.close)
        self.acct = "PY" + secrets.token_hex(6).upper()

    def make_breaks(self, n: int) -> list[int]:
        """n custodian-only fee breaks in one run, like the engine would write them."""
        c = self.conn
        file_id = c.execute(
            "INSERT INTO source_files (side, kind, filename, sha256, row_count) VALUES ('CUSTODIAN', 'CASH', %s, %s, %s) RETURNING id",
            (self.acct + ".csv", secrets.token_bytes(32), n),
        ).fetchone()["id"]
        run_id = c.execute(
            "INSERT INTO match_runs (engine_version, rule_version, config, input_file_ids) VALUES ('test', 'test', '{}', %s) RETURNING id",
            ([file_id],),
        ).fetchone()["id"]
        ids = []
        for i in range(n):
            entry = c.execute(
                """INSERT INTO cash_entries (file_id, side, external_id, account_id, currency, amount_minor, value_date, txn_type, reference)
                   VALUES (%s, 'CUSTODIAN', %s, %s, 'USD', %s, '2026-10-12', 'UNKNOWN', %s) RETURNING id""",
                (file_id, f"{self.acct}C{i}", self.acct, -2500 - i, f"CHARGES REF FEE{i:07d} NORTHBRIDGE SEC"),
            ).fetchone()["id"]
            bid = c.execute(
                """INSERT INTO breaks (run_id, kind, category, account_id, currency, amount_minor, opened_on, reason)
                   VALUES (%s, 'CASH', 'MISSING_IN_LEDGER', %s, 'USD', %s, '2026-10-12', 'custodian line has no ledger entry')
                   RETURNING id""",
                (run_id, self.acct, 2500 + i),
            ).fetchone()["id"]
            c.execute("INSERT INTO break_legs (break_id, cash_entry_id) VALUES (%s, %s)", (bid, entry))
            c.execute("INSERT INTO break_events (break_id, status, actor) VALUES (%s, 'OPEN', 'engine')", (bid,))
            ids.append(bid)
        self.run_id = run_id
        return ids

    def test_suggestion_with_similar_breaks(self):
        first, second, third = self.make_breaks(3)
        # An analyst resolved one of them, which makes it the most useful neighbour to show.
        for status in ("ASSIGNED", "PROPOSED"):
            self.conn.execute("INSERT INTO break_events (break_id, status, actor, reason) VALUES (%s, %s, 'ana', %s)",
                              (third, status, "booked bank charge per fee schedule"))

        self.assertEqual(db.vectorize_run(self.conn, self.run_id), 3)
        self.assertEqual(db.vectorize_run(self.conn, self.run_id), 0)  # idempotent

        sid, s = worker.suggest(self.conn, RulesAdvisor(), first)
        self.assertEqual((s.model, s.next_action), ("rules", "BOOK_ENTRY"))
        self.assertEqual(s.similar_break_ids[0], third, "the resolved neighbour should come first")
        self.assertNotIn(first, s.similar_break_ids)
        # Other data may be in the database; whatever is returned must be the same kind of break.
        cats = self.conn.execute("SELECT DISTINCT category FROM breaks WHERE id = ANY(%s)", (s.similar_break_ids,)).fetchall()
        self.assertEqual([c["category"] for c in cats], ["MISSING_IN_LEDGER"])
        # Among equally similar breaks the newest wins, so this run's other break is included.
        self.assertIn(second, s.similar_break_ids)

        row = self.conn.execute("SELECT similar_break_ids FROM break_suggestions WHERE id = %s", (sid,)).fetchone()
        self.assertEqual(row["similar_break_ids"], s.similar_break_ids)
        with self.assertRaisesRegex(Exception, "append-only"):
            self.conn.execute("UPDATE break_suggestions SET explanation = 'edited' WHERE id = %s", (sid,))

    def test_queue_claims_once_and_backs_off(self):
        kind = "test_" + self.acct.lower()
        job_id = db.enqueue(self.conn, kind, {"break_id": 0})
        job = db.claim(self.conn, [kind], "tester")
        self.assertEqual((job["id"], job["attempts"]), (job_id, 1))
        self.assertIsNone(db.claim(self.conn, [kind], "tester"), "a running job must not be claimed twice")

        db.fail(self.conn, job, "boom")
        row = self.conn.execute("SELECT status, last_error, run_after > now() AS later FROM jobs WHERE id = %s", (job_id,)).fetchone()
        self.assertEqual((row["status"], row["last_error"], row["later"]), ("QUEUED", "boom", True))


if __name__ == "__main__":
    unittest.main()
