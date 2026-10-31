-- 251_add_fingerprint_expectation.sql
-- Explicit "expected model" overrides for fingerprint tests:
--   * scheduled_test_plans.expected_model: per-plan reference label/alias override.
--   * account_model_fingerprint_runs.expected_models: immutable per-model override
--     snapshot of a batch run (model id -> expected label).
-- Idempotent: ADD COLUMN IF NOT EXISTS.

ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS expected_model VARCHAR(200) NOT NULL DEFAULT '';

ALTER TABLE account_model_fingerprint_runs
    ADD COLUMN IF NOT EXISTS expected_models JSONB NOT NULL DEFAULT '{}'::jsonb;
