import random
import unittest
from datetime import date

from apexgen.amounts import (
    QUANTITY_SCALE,
    convert_usd_cents,
    format_fixed,
    format_trimmed,
    market_value,
)
from apexgen.bizdays import add_business_days, business_days
from apexgen.refdata import isin_check_digit, make_accounts, make_isin


class IsinTest(unittest.TestCase):
    def test_known_isins(self):
        self.assertEqual(isin_check_digit("US037833100"), "5")  # US0378331005
        self.assertEqual(isin_check_digit("US594918104"), "5")  # US5949181045
        self.assertEqual(isin_check_digit("GB000263494"), "6")  # GB0002634946

    def test_generated_isins_validate(self):
        rng = random.Random(1)
        for _ in range(200):
            isin = make_isin(rng, "XS")
            self.assertEqual(len(isin), 12)
            self.assertEqual(isin_check_digit(isin[:11]), isin[11])


class BusinessDayTest(unittest.TestCase):
    def test_friday_plus_one_is_monday(self):
        self.assertEqual(add_business_days(date(2026, 10, 16), 1), date(2026, 10, 19))

    def test_monday_minus_one_is_friday(self):
        self.assertEqual(add_business_days(date(2026, 10, 19), -1), date(2026, 10, 16))

    def test_business_days_skip_weekend_start(self):
        days = business_days(date(2026, 10, 17), 3)  # a Saturday
        self.assertEqual(days, [date(2026, 10, 19), date(2026, 10, 20), date(2026, 10, 21)])


class AmountTest(unittest.TestCase):
    # Same cases as internal/money/value_test.go, so both implementations agree.
    def test_market_value_half_even(self):
        e6 = 10**QUANTITY_SCALE
        cases = [
            (100 * e6, 10125000000, "USD", 1012500),
            (1 * e6, 2500000, "USD", 2),
            (1 * e6, 3500000, "USD", 4),
            (-1 * e6, 3500000, "USD", -4),
            (3 * e6, 123450000000, "JPY", 3704),
            (2 * e6, 123450000, "KWD", 2469),
        ]
        for qty, price, ccy, want in cases:
            self.assertEqual(market_value(qty, price, ccy), want, (qty, price, ccy))

    def test_format(self):
        self.assertEqual(format_fixed(-5, 2), "-0.05")
        self.assertEqual(format_fixed(1500, 0), "1500")
        self.assertEqual(format_trimmed(100_000000, 6), "100")
        self.assertEqual(format_trimmed(1_500000, 6), "1.5")

    def test_convert_respects_exponent(self):
        self.assertEqual(convert_usd_cents(2500, "USD"), 2500)
        self.assertEqual(convert_usd_cents(2500, "JPY"), 3750)
        self.assertEqual(convert_usd_cents(2500, "KWD"), 7750)


class AccountTest(unittest.TestCase):
    def test_accounts_have_distinct_sorted_currencies(self):
        for acct in make_accounts(random.Random(7), 50):
            self.assertEqual(list(acct.currencies), sorted(set(acct.currencies)))


if __name__ == "__main__":
    unittest.main()
