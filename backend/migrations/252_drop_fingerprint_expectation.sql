-- 252_drop_fingerprint_expectation.sql
-- Remove the "expected model override" experiment. Reference-bank alias matching
-- (display_name aliases) now handles upstream-id vs bank-label differences
-- automatically, so no manual expectation override is needed.
-- 251 already added these columns on existing instances; drop them here.
-- 幂等：DROP COLUMN IF EXISTS.

ALTER TABLE scheduled_test_plans DROP COLUMN IF EXISTS expected_model;
ALTER TABLE account_model_fingerprint_runs DROP COLUMN IF EXISTS expected_models;
