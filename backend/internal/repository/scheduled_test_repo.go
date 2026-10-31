package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Column lists are kept identical across SELECT / INSERT ... RETURNING / UPDATE
// ... RETURNING so that scanPlan and scanResult stay in sync.
const scheduledTestPlanColumns = "id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, test_mode, auto_disable, detector_config, last_run_at, next_run_at, created_at, updated_at"

const scheduledTestResultColumns = "id, plan_id, status, response_text, error_message, latency_ms, verdict, expected_model, detected_model, detected_model_name, detected_family, probability, account_action, detail_json, started_at, finished_at, created_at"

// --- Plan Repository ---

type scheduledTestPlanRepository struct {
	db *sql.DB
}

func NewScheduledTestPlanRepository(db *sql.DB) service.ScheduledTestPlanRepository {
	return &scheduledTestPlanRepository{db: db}
}

func (r *scheduledTestPlanRepository) Create(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	testMode, detectorConfigJSON, err := normalizePlanTestOptions(plan)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_plans (account_id, model_id, cron_expression, enabled, max_results, auto_recover, test_mode, auto_disable, detector_config, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, NOW(), NOW())
		RETURNING `+scheduledTestPlanColumns+`
	`, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, testMode, plan.AutoDisable, detectorConfigJSON, plan.NextRunAt)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) GetByID(ctx context.Context, id int64) (*service.ScheduledTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+scheduledTestPlanColumns+`
		FROM scheduled_test_plans WHERE id = $1
	`, id)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) ListByAccountID(ctx context.Context, accountID int64) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+scheduledTestPlanColumns+`
		FROM scheduled_test_plans WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

func (r *scheduledTestPlanRepository) ListDue(ctx context.Context, now time.Time) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+scheduledTestPlanColumns+`
		FROM scheduled_test_plans
		WHERE enabled = true AND next_run_at <= $1
		ORDER BY next_run_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

func (r *scheduledTestPlanRepository) Update(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	testMode, detectorConfigJSON, err := normalizePlanTestOptions(plan)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRowContext(ctx, `
		UPDATE scheduled_test_plans
		SET model_id = $2, cron_expression = $3, enabled = $4, max_results = $5, auto_recover = $6,
			test_mode = $7, auto_disable = $8, detector_config = $9::jsonb, next_run_at = $10, updated_at = NOW()
		WHERE id = $1
		RETURNING `+scheduledTestPlanColumns+`
	`, plan.ID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, testMode, plan.AutoDisable, detectorConfigJSON, plan.NextRunAt)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_test_plans WHERE id = $1`, id)
	return err
}

func (r *scheduledTestPlanRepository) UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_test_plans SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1
	`, id, lastRunAt, nextRunAt)
	return err
}

// normalizePlanTestOptions defensively normalizes the plan level fingerprint
// options at the storage boundary and returns the values to persist.
func normalizePlanTestOptions(plan *service.ScheduledTestPlan) (testMode string, detectorConfigJSON string, err error) {
	testMode, err = service.NormalizeTestMode(plan.TestMode)
	if err != nil {
		return "", "", err
	}

	cfg, err := service.NormalizeDetectorConfig(plan.DetectorConfig)
	if err != nil {
		return "", "", err
	}

	detectorConfigJSON, err = cfg.ToJSON()
	if err != nil {
		return "", "", err
	}

	return testMode, detectorConfigJSON, nil
}

// --- Result Repository ---

type scheduledTestResultRepository struct {
	db *sql.DB
}

func NewScheduledTestResultRepository(db *sql.DB) service.ScheduledTestResultRepository {
	return &scheduledTestResultRepository{db: db}
}

func (r *scheduledTestResultRepository) Create(ctx context.Context, result *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_results (plan_id, status, response_text, error_message, latency_ms, verdict, expected_model, detected_model, detected_model_name, detected_family, probability, account_action, detail_json, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, $15, NOW())
		RETURNING `+scheduledTestResultColumns+`
	`, result.PlanID, result.Status, result.ResponseText, result.ErrorMessage, result.LatencyMs,
		result.Verdict, result.ExpectedModel, result.DetectedModel, result.DetectedModelName, result.DetectedFamily,
		result.Probability, result.AccountAction, detailJSONParam(result.Detail), result.StartedAt, result.FinishedAt)
	return scanResult(row)
}

func (r *scheduledTestResultRepository) ListByPlanID(ctx context.Context, planID int64, limit int) ([]*service.ScheduledTestResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+scheduledTestResultColumns+`
		FROM scheduled_test_results
		WHERE plan_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*service.ScheduledTestResult
	for rows.Next() {
		result, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (r *scheduledTestResultRepository) PruneOldResults(ctx context.Context, planID int64, keepCount int) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduled_test_results
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY created_at DESC) AS rn
				FROM scheduled_test_results
				WHERE plan_id = $1
			) ranked
			WHERE rn > $2
		)
	`, planID, keepCount)
	return err
}

// --- scan helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanPlan(row scannable) (*service.ScheduledTestPlan, error) {
	p := &service.ScheduledTestPlan{}
	var detectorConfig []byte
	if err := row.Scan(
		&p.ID, &p.AccountID, &p.ModelID, &p.CronExpression, &p.Enabled, &p.MaxResults, &p.AutoRecover,
		&p.TestMode, &p.AutoDisable, &detectorConfig,
		&p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}

	cfg, err := service.DetectorConfigFromJSON(detectorConfig)
	if err != nil {
		return nil, err
	}
	p.DetectorConfig = cfg

	return p, nil
}

func scanPlans(rows *sql.Rows) ([]*service.ScheduledTestPlan, error) {
	var plans []*service.ScheduledTestPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func scanResult(row scannable) (*service.ScheduledTestResult, error) {
	result := &service.ScheduledTestResult{}
	var detail []byte
	if err := row.Scan(
		&result.ID, &result.PlanID, &result.Status, &result.ResponseText, &result.ErrorMessage,
		&result.LatencyMs, &result.Verdict, &result.ExpectedModel, &result.DetectedModel,
		&result.DetectedModelName, &result.DetectedFamily, &result.Probability, &result.AccountAction,
		&detail, &result.StartedAt, &result.FinishedAt, &result.CreatedAt,
	); err != nil {
		return nil, err
	}

	// A SQL NULL detail_json is exposed as JSON null (the field uses omitempty,
	// so an empty RawMessage would disappear from the response entirely).
	if len(detail) == 0 {
		result.Detail = json.RawMessage("null")
	} else {
		result.Detail = json.RawMessage(detail)
	}

	return result, nil
}

// detailJSONParam converts a result detail into a nullable ::jsonb parameter.
func detailJSONParam(detail json.RawMessage) any {
	if len(detail) == 0 {
		return nil
	}
	return string(detail)
}
