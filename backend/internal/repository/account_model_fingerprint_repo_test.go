package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func batchRunRowColumns() []string {
	return []string{"id", "account_id", "status", "models", "detector_config", "concurrency",
		"total_models", "completed_models", "matched_models", "mismatched_models",
		"failed_models", "unsupported_models", "cancelled_models", "estimated_requests",
		"error_message", "terminal_reason", "idempotency_key", "started_at", "finished_at",
		"created_at", "updated_at"}
}

func batchResultRowColumns() []string {
	return []string{"id", "run_id", "position", "model_id", "status", "verdict", "expected_model",
		"detected_model", "detected_model_name", "detected_family", "probability",
		"latency_ms", "error_message", "detail_json", "detail_truncated", "started_at",
		"finished_at", "created_at", "updated_at"}
}

func TestAccountModelFingerprintRepositoryCreateRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cfg, err := service.NormalizeDetectorConfig(service.DetectorConfig{Count: 2, Parallel: 2})
	require.NoError(t, err)
	cfgJSON, err := cfg.ToJSON()
	require.NoError(t, err)
	modelsJSON, err := json.Marshal([]string{"claude-haiku-4-5", "gpt-5.1"})
	require.NoError(t, err)
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO account_model_fingerprint_runs")).
		WithArgs(int64(7), service.ModelFingerprintRunPending, string(modelsJSON), cfgJSON, 2, 2, 12, "idem-1").
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(11), int64(7), service.ModelFingerprintRunPending, []byte(modelsJSON), []byte(cfgJSON), 2,
			2, 0, 0, 0, 0, 0, 0, 12, "", "", "idem-1", nil, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO account_model_fingerprint_results")).
		WithArgs(int64(11), 0, "claude-haiku-4-5", service.ModelFingerprintResultPending).
		WillReturnRows(sqlmock.NewRows(batchResultRowColumns()).AddRow(
			int64(1), int64(11), 0, "claude-haiku-4-5", service.ModelFingerprintResultPending, "",
			"", "", "", "", nil, int64(0), "", nil, false, nil, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO account_model_fingerprint_results")).
		WithArgs(int64(11), 1, "gpt-5.1", service.ModelFingerprintResultPending).
		WillReturnRows(sqlmock.NewRows(batchResultRowColumns()).AddRow(
			int64(2), int64(11), 1, "gpt-5.1", service.ModelFingerprintResultPending, "",
			"", "", "", "", nil, int64(0), "", nil, false, nil, nil, now, now))
	mock.ExpectCommit()

	repo := NewAccountModelFingerprintRepository(db)
	run := &service.ModelFingerprintRun{
		AccountID:         7,
		Status:            service.ModelFingerprintRunPending,
		Models:            []string{"claude-haiku-4-5", "gpt-5.1"},
		DetectorConfig:    cfg,
		Concurrency:       2,
		TotalModels:       2,
		EstimatedRequests: 12,
		IdempotencyKey:    "idem-1",
	}
	results := []*service.ModelFingerprintRunResult{
		{ModelID: "claude-haiku-4-5", Status: service.ModelFingerprintResultPending},
		{ModelID: "gpt-5.1", Status: service.ModelFingerprintResultPending},
	}
	created, err := repo.CreateRun(context.Background(), run, results)
	require.NoError(t, err)
	require.Equal(t, int64(11), created.ID)
	require.Equal(t, 12, created.EstimatedRequests)
	require.Equal(t, "idem-1", created.IdempotencyKey)
	require.Len(t, results, 2)
	require.Equal(t, int64(1), results[0].ID)
	require.Equal(t, 0, results[0].Position)
	require.Equal(t, 1, results[1].Position)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryGetRunForAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("FROM account_model_fingerprint_runs WHERE id = $1 AND account_id = $2")).
		WithArgs(int64(11), int64(7)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(11), int64(7), service.ModelFingerprintRunRunning, []byte("[]"), []byte("{}"), 2,
			3, 2, 1, 1, 0, 0, 0, 30, "", "", "", now, nil, now, now))

	repo := NewAccountModelFingerprintRepository(db)
	run, err := repo.GetRunForAccount(context.Background(), 7, 11)
	require.NoError(t, err)
	require.Equal(t, int64(11), run.ID)
	require.Equal(t, service.ModelFingerprintRunRunning, run.Status)
	require.Equal(t, 2, run.CompletedModels)

	mock.ExpectQuery(regexp.QuoteMeta("FROM account_model_fingerprint_runs WHERE id = $1 AND account_id = $2")).
		WithArgs(int64(12), int64(7)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()))
	_, err = repo.GetRunForAccount(context.Background(), 7, 12)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryActiveRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("WHERE account_id = $1 AND status IN ('pending', 'running', 'cancelling')")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(12), int64(7), service.ModelFingerprintRunPending, []byte("[]"), []byte("{}"), 2,
			1, 0, 0, 0, 0, 0, 0, 3, "", "", "", nil, nil, now, now))
	repo := NewAccountModelFingerprintRepository(db)
	active, err := repo.GetActiveRunByAccount(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, int64(12), active.ID)

	mock.ExpectQuery(regexp.QuoteMeta("WHERE account_id = $1 AND status IN ('pending', 'running', 'cancelling')")).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()))
	none, err := repo.GetActiveRunByAccount(context.Background(), 8)
	require.NoError(t, err)
	require.Nil(t, none)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryMarkRunningAndStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE account_model_fingerprint_runs")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(11), int64(7), service.ModelFingerprintRunRunning, []byte("[]"), []byte("{}"), 2,
			2, 0, 0, 0, 0, 0, 0, 6, "", "", "", now, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM account_model_fingerprint_runs WHERE id = $1")).
		WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.ModelFingerprintRunRunning))

	repo := NewAccountModelFingerprintRepository(db)
	run, err := repo.MarkRunning(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, service.ModelFingerprintRunRunning, run.Status)
	status, err := repo.GetRunStatus(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, service.ModelFingerprintRunRunning, status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryFinishResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta("WITH finished AS (")).
		WithArgs(int64(1), service.ModelFingerprintResultSuccess, service.FingerprintVerdictMatch,
			"claude-haiku-4-5", "claude-haiku-4.5", "claude-haiku-4.5", "claude", 0.98, int64(420), "",
			nil, false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	repo := NewAccountModelFingerprintRepository(db)
	accepted, err := repo.FinishResult(context.Background(), &service.ModelFingerprintRunResult{
		ID: 1, RunID: 11, Status: service.ModelFingerprintResultSuccess, Verdict: service.FingerprintVerdictMatch,
		ExpectedModel: "claude-haiku-4-5", DetectedModel: "claude-haiku-4.5",
		DetectedModelName: "claude-haiku-4.5", DetectedFamily: "claude", Probability: floatPtr(0.98),
		LatencyMs: 420,
	})
	require.NoError(t, err)
	require.True(t, accepted)

	mock.ExpectQuery(regexp.QuoteMeta("WITH finished AS (")).
		WithArgs(int64(2), service.ModelFingerprintResultSuccess, service.FingerprintVerdictMatch,
			"", "", "", "", nil, int64(0), "", nil, false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	accepted, err = repo.FinishResult(context.Background(), &service.ModelFingerprintRunResult{
		ID: 2, RunID: 11, Status: service.ModelFingerprintResultSuccess, Verdict: service.FingerprintVerdictMatch,
	})
	require.NoError(t, err)
	require.False(t, accepted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryFinalizeRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("WITH cancelled AS (")).
		WithArgs(int64(11), service.ModelFingerprintRunCancelled, "任务已取消", "cancelled").
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(11), int64(7), service.ModelFingerprintRunCancelled, []byte("[]"), []byte("{}"), 2,
			2, 1, 1, 0, 0, 0, 1, 6, "任务已取消", "cancelled", "", now, now, now, now))
	repo := NewAccountModelFingerprintRepository(db)
	run, err := repo.FinalizeRun(context.Background(), 11, service.ModelFingerprintRunCancelled, "任务已取消", "cancelled")
	require.NoError(t, err)
	require.Equal(t, service.ModelFingerprintRunCancelled, run.Status)
	require.Equal(t, 1, run.CancelledModels)
	require.Equal(t, "cancelled", run.TerminalReason)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryRequestCancel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE account_model_fingerprint_runs")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(11), int64(7), service.ModelFingerprintRunCancelling, []byte("[]"), []byte("{}"), 2,
			2, 0, 0, 0, 0, 0, 0, 6, "", "", "", now, nil, now, now))
	repo := NewAccountModelFingerprintRepository(db)
	run, updated, err := repo.RequestCancel(context.Background(), 11)
	require.NoError(t, err)
	require.True(t, updated)
	require.Equal(t, service.ModelFingerprintRunCancelling, run.Status)

	// Already terminal: UPDATE returns no row, fallback SELECT returns the run.
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE account_model_fingerprint_runs")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()))
	mock.ExpectQuery(regexp.QuoteMeta("FROM account_model_fingerprint_runs WHERE id = $1")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows(batchRunRowColumns()).AddRow(
			int64(12), int64(7), service.ModelFingerprintRunCompleted, []byte("[]"), []byte("{}"), 2,
			1, 1, 1, 0, 0, 0, 0, 3, "", "completed", "", now, now, now, now))
	run, updated, err = repo.RequestCancel(context.Background(), 12)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, service.ModelFingerprintRunCompleted, run.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountModelFingerprintRepositoryGetResultsAndPrune(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	detail := []byte(`{"decision":"scored"}`)
	mock.ExpectQuery(regexp.QuoteMeta("FROM account_model_fingerprint_results")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows(batchResultRowColumns()).AddRow(
			int64(1), int64(11), 0, "claude-haiku-4-5", service.ModelFingerprintResultSuccess,
			service.FingerprintVerdictMatch, "claude-haiku-4-5", "claude-haiku-4.5", "claude-haiku-4.5",
			"claude", 0.98, int64(420), "", detail, false, now, now, now, now))
	repo := NewAccountModelFingerprintRepository(db)
	results, err := repo.GetResults(context.Background(), 11)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 0, results[0].Position)
	require.Equal(t, service.FingerprintVerdictMatch, results[0].Verdict)
	require.NotNil(t, results[0].Probability)
	require.JSONEq(t, string(detail), string(results[0].Detail))

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM account_model_fingerprint_runs")).
		WithArgs(int64(7), 20).
		WillReturnResult(sqlmock.NewResult(0, 3))
	require.NoError(t, repo.PruneRuns(context.Background(), 7, 20))
	require.NoError(t, mock.ExpectationsWereMet())
}

func floatPtr(value float64) *float64 {
	return &value
}
