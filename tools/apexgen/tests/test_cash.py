import random
import unittest
from datetime import date

from apexgen.bizdays import business_days
from apexgen.cash import (
    AMOUNT_TOLERANCE_MINOR,
    DATE_WINDOW_BUSINESS_DAYS,
    CashGenerator,
    scenario_weights,
    split_amount,
)
from apexgen.refdata import make_accounts

EXACT_SUM_SCENARIOS = {"EXACT", "TIMING", "SPLIT", "AGGREGATE", "MANY_TO_MANY", "DUPLICATE", "FUZZY_REF"}


def business_gap(a: date, b: date) -> int:
    days = business_days(min(a, b), 30)
    return days.index(max(a, b))


class CashScenarioTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        rng = random.Random(11)
        accounts = make_accounts(rng, 8)
        days = business_days(date(2026, 9, 1), 10)
        cls.groups = CashGenerator(rng, break_scale=2.0).run(accounts, days, 20)

    def test_every_scenario_occurs(self):
        seen = {g.scenario for g in self.groups}
        self.assertEqual(seen, set(scenario_weights(1.0)))

    def test_ledger_ids_unique(self):
        ids = [e.id for g in self.groups for e in g.ledger]
        self.assertEqual(len(ids), len(set(ids)))

    def test_groups_stay_in_one_partition(self):
        for g in self.groups:
            keys = {(e.account, e.currency) for e in [*g.ledger, *g.custodian]}
            self.assertEqual(len(keys), 1, g)

    def test_amounts_agree_where_they_should(self):
        for g in self.groups:
            if g.expected == "BREAK":
                continue
            diff = sum(e.amount for e in g.ledger) - sum(c.amount for c in g.custodian)
            if g.scenario in EXACT_SUM_SCENARIOS:
                self.assertEqual(diff, 0, g)
            else:
                self.assertTrue(0 < abs(diff) <= AMOUNT_TOLERANCE_MINOR, g)

    def test_timing_inside_window_fuzzy_outside(self):
        for g in self.groups:
            if g.scenario not in ("TIMING", "FUZZY_REF"):
                continue
            gap = business_gap(g.ledger[0].value_date, g.custodian[0].value_date)
            if g.scenario == "TIMING":
                self.assertTrue(1 <= gap <= DATE_WINDOW_BUSINESS_DAYS, g)
            else:
                self.assertGreater(gap, DATE_WINDOW_BUSINESS_DAYS, g)

    def test_amount_mismatch_exceeds_tolerance(self):
        for g in self.groups:
            if g.scenario == "AMOUNT_MISMATCH":
                diff = g.custodian[0].amount - g.ledger[0].amount
                self.assertGreater(abs(diff), AMOUNT_TOLERANCE_MINOR)
                self.assertEqual(g.custodian[0].amount > 0, g.ledger[0].amount > 0)

    def test_breaks_have_categories(self):
        for g in self.groups:
            self.assertEqual(g.expected == "BREAK", g.category != "", g)


class SplitAmountTest(unittest.TestCase):
    def test_parts_sum_and_keep_sign(self):
        rng = random.Random(3)
        for total in (-1_000_000, 7, 123_456_789):
            parts = split_amount(rng, total, 4)
            self.assertEqual(sum(parts), total)
            self.assertTrue(all(p != 0 and (p > 0) == (total > 0) for p in parts))

    def test_too_many_breaks_rejected(self):
        with self.assertRaises(ValueError):
            scenario_weights(10.0)


if __name__ == "__main__":
    unittest.main()
