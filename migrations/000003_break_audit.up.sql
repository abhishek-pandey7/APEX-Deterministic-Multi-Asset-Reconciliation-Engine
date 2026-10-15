-- Break history is an append-only event log. The current state of a break is its latest
-- event; nothing is ever updated in place, so the full audit trail is always available.

CREATE TABLE break_events (
    id          bigserial PRIMARY KEY,
    break_id    bigint NOT NULL REFERENCES breaks (id),
    status      text NOT NULL CHECK (status IN (
                    'OPEN', 'ASSIGNED', 'PROPOSED', 'APPROVED', 'REJECTED', 'CLOSED')),
    actor       text NOT NULL CHECK (actor <> ''),
    reason      text NOT NULL DEFAULT '',
    payload     jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX break_events_break_idx ON break_events (break_id, id);

CREATE FUNCTION forbid_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER break_events_no_update_delete
    BEFORE UPDATE OR DELETE ON break_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER break_events_no_truncate
    BEFORE TRUNCATE ON break_events
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

-- Engine output is immutable once written.
CREATE TRIGGER matches_no_update_delete
    BEFORE UPDATE OR DELETE ON matches
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER match_legs_no_update_delete
    BEFORE UPDATE OR DELETE ON match_legs
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Enforces the break state machine and the two-person rule:
--
--   (none) -> OPEN -> ASSIGNED -> PROPOSED -> APPROVED -> CLOSED
--                        ^           |
--                        |           v
--                        +------ REJECTED
--
-- OPEN and ASSIGNED may also go straight to CLOSED when a later run matches the entries.
-- APPROVED must come from a different actor than the PROPOSED event it approves.
CREATE FUNCTION check_break_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    prev_status text;
    prev_actor  text;
BEGIN
    -- Serialise writers per break so two concurrent transitions can't both read the same state.
    PERFORM 1 FROM breaks WHERE id = NEW.break_id FOR UPDATE;

    SELECT status, actor INTO prev_status, prev_actor
    FROM break_events
    WHERE break_id = NEW.break_id
    ORDER BY id DESC
    LIMIT 1;

    -- Without this, comparisons against NULL make the whole check NULL, and IF NULL passes.
    prev_status := coalesce(prev_status, '(none)');

    IF NOT (
        (prev_status = '(none)'    AND NEW.status = 'OPEN') OR
        (prev_status = 'OPEN'      AND NEW.status IN ('ASSIGNED', 'CLOSED')) OR
        (prev_status = 'ASSIGNED'  AND NEW.status IN ('ASSIGNED', 'PROPOSED', 'CLOSED')) OR
        (prev_status = 'PROPOSED'  AND NEW.status IN ('APPROVED', 'REJECTED')) OR
        (prev_status = 'REJECTED'  AND NEW.status = 'ASSIGNED') OR
        (prev_status = 'APPROVED'  AND NEW.status = 'CLOSED')
    ) THEN
        RAISE EXCEPTION 'break %: invalid transition % -> %',
            NEW.break_id, prev_status, NEW.status;
    END IF;

    IF NEW.status = 'APPROVED' AND NEW.actor = prev_actor THEN
        RAISE EXCEPTION 'break %: approver must differ from proposer (%)', NEW.break_id, NEW.actor;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER break_events_transition
    BEFORE INSERT ON break_events
    FOR EACH ROW EXECUTE FUNCTION check_break_transition();

CREATE VIEW break_status AS
SELECT DISTINCT ON (e.break_id)
    e.break_id,
    e.status,
    e.actor,
    e.created_at AS status_since
FROM break_events e
ORDER BY e.break_id, e.id DESC;
