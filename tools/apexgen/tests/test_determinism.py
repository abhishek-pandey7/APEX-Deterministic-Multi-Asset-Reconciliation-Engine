import csv
import tempfile
import unittest
from datetime import date
from pathlib import Path

from apexgen.__main__ import generate

PARAMS = dict(accounts=5, days=5, events_per_day=10, start=date(2026, 9, 1), break_scale=1.5)


class DeterminismTest(unittest.TestCase):
    def test_same_seed_same_bytes(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            ma = generate(Path(a), seed=7, **PARAMS)
            mb = generate(Path(b), seed=7, **PARAMS)
            self.assertEqual(ma, mb)
            for name in ma["files"]:
                self.assertEqual((Path(a) / name).read_bytes(), (Path(b) / name).read_bytes(), name)

    def test_different_seed_different_data(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            ma = generate(Path(a), seed=7, **PARAMS)
            mb = generate(Path(b), seed=8, **PARAMS)
            self.assertNotEqual(ma["files"]["ledger_cash.csv"], mb["files"]["ledger_cash.csv"])


class TruthCoverageTest(unittest.TestCase):
    def test_every_entry_appears_in_exactly_one_group(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            generate(out, seed=3, **PARAMS)

            def ids(name, column):
                with open(out / name, newline="", encoding="utf-8") as f:
                    return [row[column] for row in csv.DictReader(f)]

            ledger = ids("ledger_cash.csv", "entry_id")
            custodian = ids("custodian_cash.csv", "Stmt Ref")
            truth_ledger, truth_custodian = [], []
            with open(out / "truth_cash.csv", newline="", encoding="utf-8") as f:
                for row in csv.DictReader(f):
                    truth_ledger += [i for i in row["ledger_ids"].split(";") if i]
                    truth_custodian += [i for i in row["custodian_ids"].split(";") if i]

            self.assertEqual(sorted(ledger), sorted(truth_ledger))
            self.assertEqual(sorted(custodian), sorted(truth_custodian))
            self.assertEqual(len(truth_ledger), len(set(truth_ledger)))
            self.assertEqual(len(truth_custodian), len(set(truth_custodian)))


if __name__ == "__main__":
    unittest.main()
