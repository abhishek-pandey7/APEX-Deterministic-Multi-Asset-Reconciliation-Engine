"""Business-day arithmetic. Weekends only; exchange holidays are out of scope for synthetic data."""

from datetime import date, timedelta


def is_business_day(d: date) -> bool:
    return d.weekday() < 5


def add_business_days(d: date, n: int) -> date:
    step = 1 if n >= 0 else -1
    remaining = abs(n)
    while remaining:
        d += timedelta(days=step)
        if is_business_day(d):
            remaining -= 1
    return d


def business_days(start: date, count: int) -> list[date]:
    d = start
    while not is_business_day(d):
        d += timedelta(days=1)
    days = [d]
    while len(days) < count:
        days.append(add_business_days(days[-1], 1))
    return days
