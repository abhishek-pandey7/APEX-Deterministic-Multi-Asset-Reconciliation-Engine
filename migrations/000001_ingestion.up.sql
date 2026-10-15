-- Normalised inputs. Every row points back to the file it came from, and a file
-- can only be loaded once (sha256 is unique), which makes ingestion idempotent.

CREATE TABLE source_files (
    id         bigserial PRIMARY KEY,
    side       text NOT NULL CHECK (side IN ('LEDGER', 'CUSTODIAN')),
    kind       text NOT NULL CHECK (kind IN ('CASH', 'POSITION')),
    filename   text NOT NULL,
    sha256     bytea NOT NULL UNIQUE CHECK (octet_length(sha256) = 32),
    row_count  integer NOT NULL CHECK (row_count >= 0),
    loaded_at  timestamptz NOT NULL DEFAULT now()
);

-- Amounts are signed minor units of `currency`, from the account's point of view.
CREATE TABLE cash_entries (
    id            bigserial PRIMARY KEY,
    file_id       bigint NOT NULL REFERENCES source_files (id),
    side          text NOT NULL CHECK (side IN ('LEDGER', 'CUSTODIAN')),
    external_id   text NOT NULL,
    account_id    text NOT NULL,
    currency      char(3) NOT NULL,
    amount_minor  bigint NOT NULL,
    value_date    date NOT NULL,
    trade_date    date,
    txn_type      text NOT NULL,
    reference     text NOT NULL DEFAULT '',
    counterparty  text NOT NULL DEFAULT '',
    UNIQUE (side, external_id)
);

CREATE INDEX cash_entries_partition_idx ON cash_entries (account_id, currency, value_date);

-- Quantity is scaled by 10^6, price by 10^8, market value is minor units of `currency`.
CREATE TABLE positions (
    id                  bigserial PRIMARY KEY,
    file_id             bigint NOT NULL REFERENCES source_files (id),
    side                text NOT NULL CHECK (side IN ('LEDGER', 'CUSTODIAN')),
    account_id          text NOT NULL,
    as_of_date          date NOT NULL,
    security_id         text NOT NULL,
    asset_class         text NOT NULL,
    currency            char(3) NOT NULL,
    quantity_e6         bigint NOT NULL,
    price_e8            bigint NOT NULL,
    market_value_minor  bigint NOT NULL,
    UNIQUE (side, account_id, as_of_date, security_id)
);
