package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/lib/pq"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const modelFingerprintRunColumns = "id, account_id, status, models, detector_config, concurrency, total_models, completed_models, matched_models, mismatched_models, failed_models, unsupported_models, cancelled_models, estimated_requests, error_message, terminal_reason, idempotency_key, started_at, finished_at, created_at, updated_at"

const modelFingerprintResultColumns = "id, run_id, position, model_id, status, verdict, expected_model, detected_model, detected_model_name, detected_family, probability, latency_ms, error_message, detail_json, detail_truncated, started_at, finished_at, created_at, updated_at"

type accountModelFingerprintRepository struct {
	db *sql.DB
}

// NewAccountModelFingerprintRepository creates the batch model fingerprint repo.
func NewAccountModelFingerprintRepository(db *sql.DB) service.ModelFingerprintBatchRepository {
	return &accountModelFingerprintRepository{db: db}
}

// CreateRun inserts the run and its pending result rows in one transaction.
// Concurrent creates for the same account are rejected by the partial unique
// index idx_amfr_active_account, which is mapped to a sentinel error.
func (r *accountModelFingerprintRepository) CreateRun(ctx context.Context, run *service.ModelFingerprintRun, results []*service.ModelFingerprintRunResult) (*service.ModelFingerprintRun, error) {
	modelsJSON, err := json.Marshal(run.Models)
	if err != nil {
		return nil, err
	}
	detectorJSON, err := run.DetectorConfig.ToJSON()
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		INSERT INTO account_model_fingerprint_runs
			(account_id, status, models, detector_config, concurrency, total_models, estimated_requests, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, $7, $8, NOW(), NOW())
		RETURNING `+modelFingerprintRunColumns+`
	`, run.AccountID, run.Status, string(modelsJSON), detectorJSON, run.Concurrency, run.TotalModels,
		run.EstimatedRequests, run.IdempotencyKey)
	created, err := scanModelFingerprintRun(row)
	if err != nil {
		return nil, mapModelFingerprintUniqueViolation(err)
	}

	for index, result := range results {
		result.RunID = created.ID
		result.Position = index
		resultRow := tx.QueryRowContext(ctx, `
			INSERT INTO account_model_fingerprint_results (run_id, position, model_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			RETURNING `+modelFingerprintResultColumns+`
		`, created.ID, index, result.ModelID, result.Status)
		scanned, scanErr := scanModelFingerprintResult(resultRow)
		if scanErr != nil {
			return nil, scanErr
		}
		*result = *scanned
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// mapModelFingerprintUniqueViolation converts the active-run / idempotency
// unique index violations into service sentinel errors.
func mapModelFingerprintUniqueViolation(err error) error {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
		return err
	}
	switch {
	case strings.Contains(pqErr.Constraint, "idx_amfr_active_account"):
		return service.ErrModelFingerprintBatchActive
	case strings.Contains(pqErr.Constraint, "idx_amfr_idempotency"):
		return service.ErrModelFingerprintBatchIdempotency
	default:
		return err
	}
}

func (r *accountModelFingerprintRepository) GetRunForAccount(ctx context.Context, accountID, runID int64) (*service.ModelFingerprintRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+modelFingerprintRunColumns+`
		FROM account_model_fingerprint_runs WHERE id = $1 AND account_id = $2
	`, runID, accountID)
	return scanModelFingerprintRun(row)
}

func (r *accountModelFingerprintRepository) getRunByID(ctx context.Context, runID int64) (*service.ModelFingerprintRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+modelFingerprintRunColumns+`
		FROM account_model_fingerprint_runs WHERE id = $1
	`, runID)
	return scanModelFingerprintRun(row)
}

func (r *accountModelFingerprintRepository) ListRunsByAccount(ctx context.Context, accountID int64, limit int) ([]*service.ModelFingerprintRun, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+modelFingerprintRunColumns+`
		FROM account_model_fingerprint_runs
		WHERE account_id = $1
		ORDER BY id DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]*service.ModelFingerprintRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanModelFingerprintRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *accountModelFingerprintRepository) GetActiveRunByAccount(ctx context.Context, accountID int64) (*service.ModelFingerprintRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+modelFingerprintRunColumns+`
		FROM account_model_fingerprint_runs
		WHERE account_id = $1 AND status IN ('pending', 'running', 'cancelling')
		ORDER BY id DESC
		LIMIT 1
	`, accountID)
	run, err := scanModelFingerprintRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func (r *accountModelFingerprintRepository) GetRunByIdempotencyKey(ctx context.Context, accountID int64, key string) (*service.ModelFingerprintRun, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT `+modelFingerprintRunColumns+`
		FROM account_model_fingerprint_runs
		WHERE account_id = $1 AND idempotency_key = $2
		LIMIT 1
	`, accountID, key)
	run, err := scanModelFingerprintRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func (r *accountModelFingerprintRepository) GetResults(ctx context.Context, runID int64) ([]*service.ModelFingerprintRunResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+modelFingerprintResultColumns+`
		FROM account_model_fingerprint_results
		WHERE run_id = $1
		ORDER BY position ASC, id ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	results := make([]*service.ModelFingerprintRunResult, 0, 16)
	for rows.Next() {
		result, scanErr := scanModelFingerprintResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// MarkRunning moves a pending run into running. It returns sql.ErrNoRows when the
// run was already cancelled/finalised.
func (r *accountModelFingerprintRepository) MarkRunning(ctx context.Context, runID int64) (*service.ModelFingerprintRun, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE account_model_fingerprint_runs
		SET status = 'running', started_at = COALESCE(started_at, NOW()), updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+modelFingerprintRunColumns+`
	`, runID)
	return scanModelFingerprintRun(row)
}

func (r *accountModelFingerprintRepository) GetRunStatus(ctx context.Context, runID int64) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM account_model_fingerprint_runs WHERE id = $1`, runID).Scan(&status)
	return status, err
}

// FinishResult atomically records a finished result and increments the run
// counters. It returns false when the row is already terminal or the run has
// left running/cancelling, so late responses never overwrite a cancelled run.
func (r *accountModelFingerprintRepository) FinishResult(ctx context.Context, result *service.ModelFingerprintRunResult) (bool, error) {
	var runID int64
	err := r.db.QueryRowContext(ctx, `
		WITH finished AS (
			UPDATE account_model_fingerprint_results
			SET status = $2,
				verdict = $3,
				expected_model = $4,
				detected_model = $5,
				detected_model_name = $6,
				detected_family = $7,
				probability = $8,
				latency_ms = $9,
				error_message = $10,
				detail_json = $11::jsonb,
				detail_truncated = $12,
				finished_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
			  AND status IN ('pending', 'running')
			  AND EXISTS (
				SELECT 1 FROM account_model_fingerprint_runs r
				WHERE r.id = account_model_fingerprint_results.run_id
				  AND r.status IN ('running', 'cancelling')
			  )
			RETURNING run_id, verdict
		)
		UPDATE account_model_fingerprint_runs r
		SET completed_models = r.completed_models + 1,
			matched_models = r.matched_models + CASE WHEN f.verdict = 'match' THEN 1 ELSE 0 END,
			mismatched_models = r.mismatched_models + CASE WHEN f.verdict = 'mismatch' THEN 1 ELSE 0 END,
			failed_models = r.failed_models + CASE WHEN f.verdict NOT IN ('match', 'mismatch', 'unsupported') THEN 1 ELSE 0 END,
			unsupported_models = r.unsupported_models + CASE WHEN f.verdict = 'unsupported' THEN 1 ELSE 0 END,
			updated_at = NOW()
		FROM finished f
		WHERE r.id = f.run_id
		RETURNING r.id
	`, result.ID, result.Status, result.Verdict, result.ExpectedModel, result.DetectedModel,
		result.DetectedModelName, result.DetectedFamily, result.Probability, result.LatencyMs,
		result.ErrorMessage, detailJSONParam(result.Detail), result.DetailTruncated).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FinalizeRun moves a running/cancelling run into a terminal status and marks
// every unfinished result as cancelled in the same statement.
func (r *accountModelFingerprintRepository) FinalizeRun(ctx context.Context, runID int64, status, errorMessage, terminalReason string) (*service.ModelFingerprintRun, error) {
	row := r.db.QueryRowContext(ctx, `
		WITH cancelled AS (
			UPDATE account_model_fingerprint_results
			SET status = 'cancelled', finished_at = NOW(), updated_at = NOW()
			WHERE run_id = $1 AND status IN ('pending', 'running')
			RETURNING id
		)
		UPDATE account_model_fingerprint_runs r
		SET status = $2,
			error_message = $3,
			terminal_reason = $4,
			finished_at = NOW(),
			cancelled_models = r.cancelled_models + (SELECT count(*) FROM cancelled),
			updated_at = NOW()
		WHERE r.id = $1 AND r.status IN ('running', 'cancelling')
		RETURNING `+prefixedRunColumns("r")+`
	`, runID, status, errorMessage, terminalReason)
	run, err := scanModelFingerprintRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r.getRunByID(ctx, runID)
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// RequestCancel moves a pending/running run to cancelling and reports whether the
// transition happened. A terminal run is returned unchanged with updated=false.
func (r *accountModelFingerprintRepository) RequestCancel(ctx context.Context, runID int64) (*service.ModelFingerprintRun, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE account_model_fingerprint_runs
		SET status = 'cancelling', updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'running')
		RETURNING `+modelFingerprintRunColumns+`
	`, runID)
	run, err := scanModelFingerprintRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		current, getErr := r.getRunByID(ctx, runID)
		if errors.Is(getErr, sql.ErrNoRows) {
			return nil, false, sql.ErrNoRows
		}
		return current, false, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return run, true, nil
}

// MarkUnfinishedRunsFailed finalises runs left behind by a crashed process and
// cancels only their own results (no cross-run updates).
func (r *accountModelFingerprintRepository) MarkUnfinishedRunsFailed(ctx context.Context, reason string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		UPDATE account_model_fingerprint_runs
		SET status = 'failed', error_message = $1, terminal_reason = 'restart',
			finished_at = NOW(), updated_at = NOW()
		WHERE status IN ('pending', 'running', 'cancelling')
		RETURNING id
	`, reason)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return 0, scanErr
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	_ = rows.Close()
	if len(ids) > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE account_model_fingerprint_results
			SET status = 'cancelled', finished_at = NOW(), updated_at = NOW()
			WHERE run_id = ANY($1) AND status IN ('pending', 'running')
		`, pq.Array(ids)); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func (r *accountModelFingerprintRepository) PruneRuns(ctx context.Context, accountID int64, keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM account_model_fingerprint_runs
		WHERE account_id = $1
		  AND id IN (
			SELECT id FROM account_model_fingerprint_runs
			WHERE account_id = $1
			ORDER BY id DESC
			OFFSET $2
		  )
	`, accountID, keep)
	return err
}

// prefixedRunColumns returns the run column list prefixed with a table alias.
func prefixedRunColumns(alias string) string {
	parts := strings.Split(modelFingerprintRunColumns, ", ")
	for i, part := range parts {
		parts[i] = alias + "." + part
	}
	return strings.Join(parts, ", ")
}

func scanModelFingerprintRun(scanner interface{ Scan(dest ...any) error }) (*service.ModelFingerprintRun, error) {
	run := &service.ModelFingerprintRun{}
	var modelsRaw, detectorRaw []byte
	err := scanner.Scan(
		&run.ID, &run.AccountID, &run.Status, &modelsRaw, &detectorRaw, &run.Concurrency,
		&run.TotalModels, &run.CompletedModels, &run.MatchedModels, &run.MismatchedModels,
		&run.FailedModels, &run.UnsupportedModels, &run.CancelledModels, &run.EstimatedRequests,
		&run.ErrorMessage, &run.TerminalReason, &run.IdempotencyKey,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(modelsRaw) > 0 {
		_ = json.Unmarshal(modelsRaw, &run.Models)
	}
	cfg, cfgErr := service.DetectorConfigFromJSON(detectorRaw)
	if cfgErr == nil {
		run.DetectorConfig = cfg
	}
	return run, nil
}

func scanModelFingerprintResult(scanner interface{ Scan(dest ...any) error }) (*service.ModelFingerprintRunResult, error) {
	result := &service.ModelFingerprintRunResult{}
	var detailRaw []byte
	err := scanner.Scan(
		&result.ID, &result.RunID, &result.Position, &result.ModelID, &result.Status, &result.Verdict,
		&result.ExpectedModel, &result.DetectedModel, &result.DetectedModelName, &result.DetectedFamily,
		&result.Probability, &result.LatencyMs, &result.ErrorMessage, &detailRaw, &result.DetailTruncated,
		&result.StartedAt, &result.FinishedAt, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(detailRaw) > 0 && string(detailRaw) != "null" {
		detail := make([]byte, len(detailRaw))
		copy(detail, detailRaw)
		result.Detail = json.RawMessage(detail)
	}
	return result, nil
}
