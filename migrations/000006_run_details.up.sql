-- The engine scores matches by reference similarity on 0..1000, not log-likelihood millibits.
ALTER TABLE matches RENAME COLUMN score_millibits TO ref_sim;
ALTER TABLE matches ADD CONSTRAINT matches_ref_sim_range CHECK (ref_sim IS NULL OR ref_sim BETWEEN 0 AND 1000);

-- Each break carries the one-line reason its classification rule produced.
ALTER TABLE breaks ADD COLUMN reason text NOT NULL DEFAULT '';

CREATE INDEX breaks_run_category_idx ON breaks (run_id, category);
CREATE INDEX matches_run_idx ON matches (run_id);
