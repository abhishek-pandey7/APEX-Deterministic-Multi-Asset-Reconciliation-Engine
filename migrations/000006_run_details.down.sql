DROP INDEX matches_run_idx;
DROP INDEX breaks_run_category_idx;
ALTER TABLE breaks DROP COLUMN reason;
ALTER TABLE matches DROP CONSTRAINT matches_ref_sim_range;
ALTER TABLE matches RENAME COLUMN ref_sim TO score_millibits;
