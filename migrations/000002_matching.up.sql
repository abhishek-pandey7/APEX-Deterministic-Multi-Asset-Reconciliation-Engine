-- A run pins everything needed to reproduce it: engine and rule versions, the full
-- configuration and the exact input files. output_sha256 lets two runs be compared.

CREATE TABLE match_runs (
    id              bigserial PRIMARY KEY,
    engine_version  text NOT NULL,
    rule_version    text NOT NULL,
    config          jsonb NOT NULL,
    input_file_ids  bigint[] NOT NULL,
    status          text NOT NULL DEFAULT 'RUNNING' CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED')),
    stats           jsonb,
    output_sha256   bytea CHECK (output_sha256 IS NULL OR octet_length(output_sha256) = 32),
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);

-- Scores are stored in integer millibits (log2 likelihood ratio × 1000). Integer scores
-- keep results identical across CPU architectures, where float fusing can differ.
CREATE TABLE matches (
    id              bigserial PRIMARY KEY,
    run_id          bigint NOT NULL REFERENCES match_runs (id),
    phase           text NOT NULL CHECK (phase IN ('EXACT', 'TOLERANCE', 'N_TO_M', 'FUZZY')),
    disposition     text NOT NULL CHECK (disposition IN ('AUTO', 'PROPOSED')),
    score_millibits integer,
    rule            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, run_id, disposition)
);

-- run_id and disposition are repeated here so the database itself guarantees that an
-- entry is auto-matched at most once per run. Proposals may overlap; they are alternatives.
CREATE TABLE match_legs (
    match_id     bigint NOT NULL,
    run_id       bigint NOT NULL,
    disposition  text NOT NULL,
    entry_id     bigint NOT NULL REFERENCES cash_entries (id),
    PRIMARY KEY (match_id, entry_id),
    FOREIGN KEY (match_id, run_id, disposition) REFERENCES matches (id, run_id, disposition)
);

CREATE UNIQUE INDEX match_legs_one_auto_per_run ON match_legs (run_id, entry_id)
    WHERE disposition = 'AUTO';
CREATE INDEX match_legs_entry_idx ON match_legs (entry_id);

-- category is the engine's deterministic classification. AI suggestions are stored
-- separately and never overwrite it.
CREATE TABLE breaks (
    id            bigserial PRIMARY KEY,
    run_id        bigint NOT NULL REFERENCES match_runs (id),
    kind          text NOT NULL CHECK (kind IN ('CASH', 'POSITION')),
    category      text NOT NULL CHECK (category IN (
                      'TIMING', 'AMOUNT', 'FX', 'DUPLICATE', 'MISSING_IN_LEDGER',
                      'MISSING_IN_CUSTODIAN', 'QUANTITY', 'PRICE', 'CORPORATE_ACTION',
                      'UNCLASSIFIED')),
    account_id    text NOT NULL,
    currency      char(3) NOT NULL,
    amount_minor  bigint NOT NULL,
    opened_on     date NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX breaks_run_idx ON breaks (run_id);

CREATE TABLE break_legs (
    break_id       bigint NOT NULL REFERENCES breaks (id),
    cash_entry_id  bigint REFERENCES cash_entries (id),
    position_id    bigint REFERENCES positions (id),
    CHECK (num_nonnulls(cash_entry_id, position_id) = 1)
);

CREATE INDEX break_legs_break_idx ON break_legs (break_id);
