-- Row triggers don't fire on TRUNCATE, so matches and match_legs could still be wiped in one
-- statement (directly or via TRUNCATE ... CASCADE from cash_entries). Close that gap.

CREATE TRIGGER matches_no_truncate
    BEFORE TRUNCATE ON matches
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER match_legs_no_truncate
    BEFORE TRUNCATE ON match_legs
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();
