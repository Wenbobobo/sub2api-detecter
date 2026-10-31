-- 250_add_account_model_fingerprint_runs.sql
-- Batch model-fingerprint tests for the models in an account model whitelist.
-- 幂等：CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS account_model_fingerprint_runs (
    id                  BIGSERIAL PRIMARY KEY,
    account_id          BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    models              JSONB NOT NULL DEFAULT '[]'::jsonb,
    detector_config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    concurrency         INT NOT NULL DEFAULT 2,
    total_models        INT NOT NULL DEFAULT 0,
    completed_models    INT NOT NULL DEFAULT 0,
    matched_models      INT NOT NULL DEFAULT 0,
    mismatched_models   INT NOT NULL DEFAULT 0,
    failed_models       INT NOT NULL DEFAULT 0,
    unsupported_models  INT NOT NULL DEFAULT 0,
    cancelled_models    INT NOT NULL DEFAULT 0,
    estimated_requests  INT NOT NULL DEFAULT 0,
    error_message       TEXT NOT NULL DEFAULT '',
    terminal_reason     VARCHAR(40) NOT NULL DEFAULT '',
    idempotency_key     VARCHAR(200) NOT NULL DEFAULT '',
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_model_fingerprint_runs_status_check
        CHECK (status IN ('pending', 'running', 'cancelling', 'completed', 'cancelled', 'failed')),
    CONSTRAINT account_model_fingerprint_runs_conc_check CHECK (concurrency BETWEEN 1 AND 3)
);

CREATE INDEX IF NOT EXISTS idx_amfr_account_created
    ON account_model_fingerprint_runs(account_id, created_at DESC);

-- At most one active (pending/running/cancelling) batch run per account.
-- This is the cross-process protection for concurrent create requests.
CREATE UNIQUE INDEX IF NOT EXISTS idx_amfr_active_account
    ON account_model_fingerprint_runs(account_id)
    WHERE status IN ('pending', 'running', 'cancelling');

-- Optional client retry key to make "double click" idempotent.
CREATE UNIQUE INDEX IF NOT EXISTS idx_amfr_idempotency
    ON account_model_fingerprint_runs(account_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS account_model_fingerprint_results (
    id                  BIGSERIAL PRIMARY KEY,
    run_id              BIGINT NOT NULL REFERENCES account_model_fingerprint_runs(id) ON DELETE CASCADE,
    position            INT NOT NULL DEFAULT 0,
    model_id            VARCHAR(200) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    verdict             VARCHAR(32) NOT NULL DEFAULT '',
    expected_model      VARCHAR(200) NOT NULL DEFAULT '',
    detected_model      VARCHAR(120) NOT NULL DEFAULT '',
    detected_model_name VARCHAR(120) NOT NULL DEFAULT '',
    detected_family     VARCHAR(120) NOT NULL DEFAULT '',
    probability         DOUBLE PRECISION,
    latency_ms          BIGINT NOT NULL DEFAULT 0,
    error_message       TEXT NOT NULL DEFAULT '',
    detail_json         JSONB,
    detail_truncated    BOOLEAN NOT NULL DEFAULT FALSE,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_model_fingerprint_results_status_check
        CHECK (status IN ('pending', 'running', 'success', 'failed', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_amfr_results_run_position
    ON account_model_fingerprint_results(run_id, position);
CREATE INDEX IF NOT EXISTS idx_amfr_results_run ON account_model_fingerprint_results(run_id, id);
