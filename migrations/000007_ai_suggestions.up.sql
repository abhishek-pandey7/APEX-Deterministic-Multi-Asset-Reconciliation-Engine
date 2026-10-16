CREATE EXTENSION IF NOT EXISTS vector;

-- One feature vector per break, for "similar past breaks". The vector is computed
-- deterministically from the break itself (no external embedding API), so it is reproducible.
CREATE TABLE break_vectors (
    break_id    bigint PRIMARY KEY REFERENCES breaks (id),
    features    vector(64) NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX break_vectors_hnsw ON break_vectors USING hnsw (features vector_cosine_ops);

-- AI output is advisory. It sits next to a break and never changes a match, a break's
-- category or its workflow status. Every row records which model and which prompt produced it.
CREATE TABLE break_suggestions (
    id                 bigserial PRIMARY KEY,
    break_id           bigint NOT NULL REFERENCES breaks (id),
    model              text NOT NULL,     -- 'rules' when no language model was used
    prompt_sha256      bytea CHECK (prompt_sha256 IS NULL OR octet_length(prompt_sha256) = 32),
    category           text,
    confidence         smallint CHECK (confidence BETWEEN 0 AND 100),
    explanation        text NOT NULL,
    next_action        text NOT NULL CHECK (next_action IN (
                           'BOOK_ENTRY', 'QUERY_CUSTODIAN', 'WAIT_FOR_SETTLEMENT',
                           'WRITE_OFF', 'CANCEL_DUPLICATE', 'INVESTIGATE')),
    action_detail      text NOT NULL DEFAULT '',
    similar_break_ids  bigint[] NOT NULL DEFAULT '{}',
    latency_ms         integer,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX break_suggestions_break_idx ON break_suggestions (break_id, id);

CREATE TRIGGER break_suggestions_no_update_delete
    BEFORE UPDATE OR DELETE ON break_suggestions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER break_suggestions_no_truncate
    BEFORE TRUNCATE ON break_suggestions
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

CREATE INDEX jobs_break_idx ON jobs ((payload ->> 'break_id')) WHERE status IN ('QUEUED', 'RUNNING');
