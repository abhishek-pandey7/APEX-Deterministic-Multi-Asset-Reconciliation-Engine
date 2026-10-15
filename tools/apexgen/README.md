# apexgen

Seeded synthetic data for APEX. Every break is injected on purpose and recorded in a truth file,
so the engine's precision and recall can be measured rather than described.

```
cd tools
python -m apexgen --out ../data/small --seed 42
python -m unittest discover -s apexgen/tests -t .
```

Standard library only. The same seed and parameters produce byte-identical files on any platform:
amounts use integer arithmetic only, and each component draws from its own seeded stream.

## Output

| File | Contents |
|---|---|
| `ledger_cash.csv` | Internal books: signed amounts, ISO dates, clean references |
| `custodian_cash.csv` | Custodian statement: absolute amount + `D/C`, `YYYYMMDD` dates, free-text narrative, shuffled order |
| `truth_cash.csv` | One row per group: scenario, expected outcome, break category, ledger ids, custodian ids |
| `ledger_positions.csv` / `custodian_positions.csv` | Positions as of the last business day; custodian trims trailing zeros |
| `truth_positions.csv` | Scenario, expected outcome and category per `(account, security)` |
| `manifest.json` | Seed, parameters, engine hints (tolerances), scenario counts, row counts and SHA-256 per file |

## Cash scenarios

| Scenario | What differs at the custodian | Expected |
|---|---|---|
| `EXACT` | Nothing; round fees repeat on the same day, so only the reference separates some pairs | `AUTO_MATCH` |
| `TIMING` | Value date 1–2 business days later | `AUTO_MATCH` |
| `ROUNDING` | Amount off by 1–99 minor units | `AUTO_MATCH` |
| `SPLIT` | One ledger entry settled as 2–4 custodian lines | `AUTO_MATCH` |
| `AGGREGATE` | 2–4 ledger fees posted as one custodian batch | `AUTO_MATCH` |
| `MANY_TO_MANY` | 2 ledger trades netted and settled as 3 custodian lines | `AUTO_MATCH` |
| `FUZZY_REF` | Settles 3–6 business days later with a mangled reference | `PROPOSE` |
| `DUPLICATE` | Custodian line appears twice; the two copies are interchangeable | one `AUTO_MATCH`, one `BREAK` |
| `AMOUNT_MISMATCH` | Amount off by 1–10% | `BREAK` / `AMOUNT` |
| `MISSING_IN_CUSTODIAN` | No custodian line | `BREAK` |
| `MISSING_IN_LEDGER` | Custodian-only dividend or fee | `BREAK` / `CORPORATE_ACTION` or `MISSING_IN_LEDGER` |

## Position scenarios

| Scenario | Difference | Expected |
|---|---|---|
| `MATCH` | None | `MATCH` |
| `PRICE_WITHIN_TOLERANCE` | Price off by under 5bp | `MATCH` |
| `PRICE_BREAK` | Price off by 50–300bp | `BREAK` / `PRICE` |
| `SPLIT_UNBOOKED` | Quantity ×2 or ×3, price divided to match | `BREAK` / `CORPORATE_ACTION` |
| `DRIP_FRACTION` | Fractional extra quantity | `BREAK` / `QUANTITY` |
| `QUANTITY_BREAK` | Whole-unit quantity difference | `BREAK` / `QUANTITY` |
| `MISSING_IN_*` | Position on one side only | `BREAK` |

Prices are per unit for every asset class, so bonds are priced near 1.00 per unit of face value.
