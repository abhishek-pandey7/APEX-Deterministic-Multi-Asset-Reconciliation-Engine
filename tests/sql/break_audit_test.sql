-- Run with: psql -v ON_ERROR_STOP=1 -f tests/sql/break_audit_test.sql
-- Everything happens inside one transaction that is rolled back.

BEGIN;

CREATE FUNCTION pg_temp.assert_fails(stmt text, pattern text) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE stmt;
    RAISE EXCEPTION 'statement succeeded but should fail: %', stmt USING ERRCODE = 'AS001';
EXCEPTION
    WHEN SQLSTATE 'AS001' THEN
        RAISE;
    WHEN OTHERS THEN
        IF SQLERRM NOT LIKE pattern THEN
            RAISE EXCEPTION 'wrong error "%" for: %', SQLERRM, stmt USING ERRCODE = 'AS002';
        END IF;
END;
$$;

INSERT INTO match_runs (id, engine_version, rule_version, config, input_file_ids)
VALUES (900001, 'test', 'test', '{}', '{}');
INSERT INTO breaks (id, run_id, kind, category, account_id, currency, amount_minor, opened_on)
VALUES (900001, 900001, 'CASH', 'AMOUNT', 'ACC1', 'USD', 1250, '2026-10-01');

-- A break must start OPEN.
SELECT pg_temp.assert_fails(
    $$INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'ASSIGNED', 'ana')$$,
    '%invalid transition (none) -> ASSIGNED%');

INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'OPEN', 'engine');

-- No skipping straight to approval.
SELECT pg_temp.assert_fails(
    $$INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'APPROVED', 'ana')$$,
    '%invalid transition OPEN -> APPROVED%');

INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'ASSIGNED', 'ana');
INSERT INTO break_events (break_id, status, actor, reason) VALUES (900001, 'PROPOSED', 'ana', 'write off rounding');

-- The proposer can't approve their own proposal.
SELECT pg_temp.assert_fails(
    $$INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'APPROVED', 'ana')$$,
    '%approver must differ from proposer%');

INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'APPROVED', 'ben');
INSERT INTO break_events (break_id, status, actor) VALUES (900001, 'CLOSED', 'ben');

-- History is immutable.
SELECT pg_temp.assert_fails(
    $$UPDATE break_events SET actor = 'mallory' WHERE break_id = 900001$$,
    '%append-only%');
SELECT pg_temp.assert_fails(
    $$DELETE FROM break_events WHERE break_id = 900001$$,
    '%append-only%');

-- TRUNCATE skips row triggers, so it needs its own guard, including via CASCADE.
SELECT pg_temp.assert_fails($$TRUNCATE break_events CASCADE$$, '%append-only%');
SELECT pg_temp.assert_fails($$TRUNCATE matches CASCADE$$, '%append-only%');
SELECT pg_temp.assert_fails($$TRUNCATE match_legs$$, '%append-only%');
SELECT pg_temp.assert_fails($$TRUNCATE cash_entries CASCADE$$, '%append-only%');

-- The view reports the latest state.
DO $$
BEGIN
    IF (SELECT status FROM break_status WHERE break_id = 900001) <> 'CLOSED' THEN
        RAISE EXCEPTION 'break_status should be CLOSED';
    END IF;
END;
$$;

\echo break audit tests passed
ROLLBACK;
