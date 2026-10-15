import random
import unittest
from datetime import date

from apexgen.cash import CustodianEntry
from apexgen.formats import mangle_reference, narrative, short_counterparty


def custodian(reference="TRD-2026-0001234", noisy=False):
    return CustodianEntry(
        account="ACC00001", currency="USD", amount=-150000, value_date=date(2026, 9, 1),
        txn_type="TRADE_SETTLEMENT", reference=reference,
        counterparty="Harbor Point Capital Markets Inc", noisy=noisy,
    )


class FormatTest(unittest.TestCase):
    def test_short_counterparty(self):
        self.assertEqual(short_counterparty("Harbor Point Capital Markets Inc"), "HARBOR POINT CAP MKTS")
        self.assertEqual(short_counterparty("Meridian Fund Administration SA"), "MERIDIAN FUND ADMIN")

    def test_clean_narrative_keeps_reference(self):
        rng = random.Random(5)
        for _ in range(50):
            text = narrative(rng, custodian())
            self.assertTrue("TRD-2026-0001234" in text or "TRD20260001234" in text, text)
            self.assertTrue(text.startswith("SETTLEMENT REF "))

    def test_mangled_reference_differs(self):
        rng = random.Random(9)
        for _ in range(200):
            out = mangle_reference(rng, "TRD-2026-0001234")
            self.assertNotIn(out, ("TRD-2026-0001234", "TRD20260001234"))


if __name__ == "__main__":
    unittest.main()
