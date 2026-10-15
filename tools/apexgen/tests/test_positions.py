import random
import unittest
from datetime import date

from apexgen.amounts import market_value
from apexgen.positions import E6, PRICE_TOLERANCE_BPS, generate_positions
from apexgen.refdata import make_accounts


class PositionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        rng = random.Random(21)
        accounts = make_accounts(rng, 60)
        cls.ledger, cls.custodian, cls.truth = generate_positions(
            rng, accounts, date(2026, 9, 30), break_scale=2.0
        )
        cls.by_key = {
            "L": {(p.account, p.security_id): p for p in cls.ledger},
            "C": {(p.account, p.security_id): p for p in cls.custodian},
        }

    def pair(self, t):
        key = (t.account, t.security_id)
        return self.by_key["L"].get(key), self.by_key["C"].get(key)

    def test_every_scenario_occurs(self):
        self.assertEqual(
            {t.scenario for t in self.truth},
            {"MATCH", "PRICE_WITHIN_TOLERANCE", "PRICE_BREAK", "SPLIT_UNBOOKED",
             "DRIP_FRACTION", "QUANTITY_BREAK", "MISSING_IN_CUSTODIAN", "MISSING_IN_LEDGER"},
        )

    def test_market_values_are_consistent(self):
        for p in [*self.ledger, *self.custodian]:
            self.assertEqual(p.market_value, market_value(p.quantity_e6, p.price_e8, p.currency))

    def test_scenarios_produce_the_intended_difference(self):
        for t in self.truth:
            led, cus = self.pair(t)
            if t.scenario == "MISSING_IN_CUSTODIAN":
                self.assertIsNotNone(led)
                self.assertIsNone(cus)
                continue
            if t.scenario == "MISSING_IN_LEDGER":
                self.assertIsNone(led)
                self.assertIsNotNone(cus)
                continue
            price_bps = abs(cus.price_e8 - led.price_e8) * 10_000 / led.price_e8
            if t.scenario == "MATCH":
                self.assertEqual((led.quantity_e6, led.price_e8), (cus.quantity_e6, cus.price_e8))
            elif t.scenario == "PRICE_WITHIN_TOLERANCE":
                self.assertEqual(led.quantity_e6, cus.quantity_e6)
                self.assertTrue(0 < price_bps <= PRICE_TOLERANCE_BPS, price_bps)
            elif t.scenario == "PRICE_BREAK":
                self.assertGreater(price_bps, PRICE_TOLERANCE_BPS)
            elif t.scenario == "SPLIT_UNBOOKED":
                self.assertIn(cus.quantity_e6 // led.quantity_e6, (2, 3))
            elif t.scenario == "DRIP_FRACTION":
                self.assertTrue(0 < cus.quantity_e6 - led.quantity_e6 < E6)
            elif t.scenario == "QUANTITY_BREAK":
                self.assertNotEqual(led.quantity_e6, cus.quantity_e6)
                self.assertGreater(cus.quantity_e6, 0)


if __name__ == "__main__":
    unittest.main()
