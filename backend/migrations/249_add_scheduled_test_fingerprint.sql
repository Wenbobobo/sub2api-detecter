-- 249_add_scheduled_test_fingerprint.sql
-- Fingerprint (lm-detector) test mode: plan-level parameters and result verdict fields.
-- 幂等：所有 ADD COLUMN 使用 IF NOT EXISTS，重复执行安全。

ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS test_mode VARCHAR(20) NOT NULL DEFAULT 'liveness',
    ADD COLUMN IF NOT EXISTS auto_disable BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS detector_config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scheduled_test_results
    ADD COLUMN IF NOT EXISTS verdict VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS expected_model VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS detected_model VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS detected_model_name VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS detected_family VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS probability DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS account_action VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS detail_json JSONB;

-- 说明：scheduled_test_results(plan_id, created_at DESC) 索引已存在（迁移 066），无需新增。
