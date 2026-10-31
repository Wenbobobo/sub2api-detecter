package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const scheduledTestPlanRowColumns = "id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, test_mode, auto_disable, detector_config, last_run_at, next_run_at, created_at, updated_at"

const scheduledTestResultRowColumns = "id, plan_id, status, response_text, error_message, latency_ms, verdict, expected_model, detected_model, detected_model_name, detected_family, probability, account_action, detail_json, started_at, finished_at, created_at"

func TestScheduledTestPlanRepositoryCreateAndScanFingerprintFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cfg, err := service.NormalizeDetectorConfig(service.DetectorConfig{
		API:   service.DetectorAPIResponses,
		Count: 2, Parallel: 2,
	})
	require.NoError(t, err)
	cfgJSON, err := cfg.ToJSON()
	require.NoError(t, err)

	now := time.Now().UTC()
	next := now.Add(30 * time.Minute)

	mock.ExpectQuery("(?s)INSERT INTO scheduled_test_plans .*test_mode.*auto_disable.*detector_config.*RETURNING "+scheduledTestPlanRowColumns).
		WithArgs(int64(5), "claude-sonnet-4-5", "*/30 * * * *", true, 50, true, service.ScheduledTestModeFingerprint, true, cfgJSON, next).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "model_id", "cron_expression", "enabled", "max_results", "auto_recover",
			"test_mode", "auto_disable", "detector_config", "last_run_at", "next_run_at", "created_at", "updated_at",
		}).AddRow(int64(1), int64(5), "claude-sonnet-4-5", "*/30 * * * *", true, 50, true,
			service.ScheduledTestModeFingerprint, true, []byte(cfgJSON), nil, next, now, now))

	repo := NewScheduledTestPlanRepository(db)
	got, err := repo.Create(context.Background(), &service.ScheduledTestPlan{
		AccountID:      5,
		ModelID:        "claude-sonnet-4-5",
		CronExpression: "*/30 * * * *",
		Enabled:        true,
		MaxResults:     50,
		AutoRecover:    true,
		TestMode:       service.ScheduledTestModeFingerprint,
		AutoDisable:    true,
		DetectorConfig: cfg,
		NextRunAt:      &next,
	})
	require.NoError(t, err)
	require.Equal(t, service.ScheduledTestModeFingerprint, got.TestMode)
	require.True(t, got.AutoDisable)
	require.Equal(t, cfg, got.DetectorConfig)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScheduledTestPlanRepositoryScanLegacyPlanDefaults(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery("(?s)SELECT " + scheduledTestPlanRowColumns + "\\s+FROM scheduled_test_plans WHERE id = \\$1").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "model_id", "cron_expression", "enabled", "max_results", "auto_recover",
			"test_mode", "auto_disable", "detector_config", "last_run_at", "next_run_at", "created_at", "updated_at",
		}).AddRow(int64(7), int64(5), "", "*/30 * * * *", true, 50, false,
			service.ScheduledTestModeLiveness, false, []byte("{}"), nil, now, now, now))

	repo := NewScheduledTestPlanRepository(db)
	got, err := repo.GetByID(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, service.ScheduledTestModeLiveness, got.TestMode)
	require.False(t, got.AutoDisable)
	require.Equal(t, service.DefaultDetectorConfig(), got.DetectorConfig)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScheduledTestResultRepositoryCreateAndScanDetail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	started := time.Now().UTC()
	finished := started.Add(8 * time.Second)
	probability := 0.93
	detail := `{"rounds":[{"verdict":"match"}],"bank_built_at":"2026-10-01T00:00:00Z"}`

	mock.ExpectQuery("(?s)INSERT INTO scheduled_test_results .*verdict.*detected_model.*account_action.*detail_json.*RETURNING "+scheduledTestResultRowColumns).
		WithArgs(int64(1), "failed", "", "fingerprint mismatch", int64(8421), "mismatch", "claude-sonnet-4-5", "gpt-5.1", "GPT-5.1", "gpt", probability, "disabled", detail, started, finished).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "plan_id", "status", "response_text", "error_message", "latency_ms", "verdict",
			"expected_model", "detected_model", "detected_model_name", "detected_family", "probability",
			"account_action", "detail_json", "started_at", "finished_at", "created_at",
		}).AddRow(int64(9), int64(1), "failed", "", "fingerprint mismatch", int64(8421), "mismatch",
			"claude-sonnet-4-5", "gpt-5.1", "GPT-5.1", "gpt", probability, "disabled", []byte(detail), started, finished, finished))

	repo := NewScheduledTestResultRepository(db)
	got, err := repo.Create(context.Background(), &service.ScheduledTestResult{
		PlanID:            1,
		Status:            "failed",
		ErrorMessage:      "fingerprint mismatch",
		LatencyMs:         8421,
		Verdict:           "mismatch",
		ExpectedModel:     "claude-sonnet-4-5",
		DetectedModel:     "gpt-5.1",
		DetectedModelName: "GPT-5.1",
		DetectedFamily:    "gpt",
		Probability:       &probability,
		AccountAction:     "disabled",
		Detail:            json.RawMessage(detail),
		StartedAt:         started,
		FinishedAt:        finished,
	})
	require.NoError(t, err)
	require.Equal(t, "mismatch", got.Verdict)
	require.InDelta(t, 0.93, *got.Probability, 0.0001)
	require.JSONEq(t, detail, string(got.Detail))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScheduledTestResultRepositoryListNullDetail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectQuery("(?s)SELECT "+scheduledTestResultRowColumns+"\\s+FROM scheduled_test_results WHERE plan_id = \\$1").
		WithArgs(int64(1), 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "plan_id", "status", "response_text", "error_message", "latency_ms", "verdict",
			"expected_model", "detected_model", "detected_model_name", "detected_family", "probability",
			"account_action", "detail_json", "started_at", "finished_at", "created_at",
		}).AddRow(int64(9), int64(1), "success", "ok", "", int64(120), "", "", "", "", "", nil, "", nil, now, now, now))

	repo := NewScheduledTestResultRepository(db)
	results, err := repo.ListByPlanID(context.Background(), 1, 50)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Nil(t, results[0].Probability)
	require.Equal(t, "null", string(results[0].Detail))
	require.NoError(t, mock.ExpectationsWereMet())
}
