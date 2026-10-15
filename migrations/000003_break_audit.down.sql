DROP VIEW break_status;
DROP TRIGGER break_events_transition ON break_events;
DROP FUNCTION check_break_transition();
DROP TRIGGER match_legs_no_update_delete ON match_legs;
DROP TRIGGER matches_no_update_delete ON matches;
DROP TABLE break_events;
DROP FUNCTION forbid_mutation();
