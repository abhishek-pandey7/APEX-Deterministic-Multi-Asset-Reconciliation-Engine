-- Work queue shared by the Go engine and the Python AI worker. Workers claim jobs with
-- FOR UPDATE SKIP LOCKED, so many workers can poll at once without blocking each other:
--
--   UPDATE jobs SET status = 'RUNNING', attempts = attempts + 1,
--                   locked_by = $1, locked_at = now()
--   WHERE id = (
--       SELECT id FROM jobs
--       WHERE status = 'QUEUED' AND kind = $2 AND run_after <= now()
--       ORDER BY run_after, id
--       FOR UPDATE SKIP LOCKED
--       LIMIT 1)
--   RETURNING id, payload;

CREATE TABLE jobs (
    id            bigserial PRIMARY KEY,
    kind          text NOT NULL,
    payload       jsonb NOT NULL,
    status        text NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED', 'RUNNING', 'DONE', 'FAILED')),
    attempts      integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts  integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    run_after     timestamptz NOT NULL DEFAULT now(),
    locked_by     text,
    locked_at     timestamptz,
    last_error    text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_ready_idx ON jobs (kind, run_after, id) WHERE status = 'QUEUED';
