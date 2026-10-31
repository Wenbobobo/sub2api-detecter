package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Scheduled test modes stored in scheduled_test_plans.test_mode.
const (
	ScheduledTestModeLiveness    = "liveness"
	ScheduledTestModeFingerprint = "fingerprint"
)

// NormalizeTestMode lowercases and validates a plan test mode. An empty value
// means the legacy liveness behaviour.
func NormalizeTestMode(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "":
		return ScheduledTestModeLiveness, nil
	case ScheduledTestModeLiveness, ScheduledTestModeFingerprint:
		return m, nil
	default:
		return "", fmt.Errorf("test_mode %q is invalid (liveness|fingerprint)", mode)
	}
}

// ScheduledTestPlan represents a scheduled test plan domain model.
type ScheduledTestPlan struct {
	ID             int64          `json:"id"`
	AccountID      int64          `json:"account_id"`
	ModelID        string         `json:"model_id"`
	CronExpression string         `json:"cron_expression"`
	Enabled        bool           `json:"enabled"`
	MaxResults     int            `json:"max_results"`
	AutoRecover    bool           `json:"auto_recover"`
	TestMode       string         `json:"test_mode"`
	AutoDisable    bool           `json:"auto_disable"`
	DetectorConfig DetectorConfig `json:"detector_config"`
	LastRunAt      *time.Time     `json:"last_run_at"`
	NextRunAt      *time.Time     `json:"next_run_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ScheduledTestResult represents a single test execution result.
type ScheduledTestResult struct {
	ID                int64           `json:"id"`
	PlanID            int64           `json:"plan_id"`
	Status            string          `json:"status"`
	ResponseText      string          `json:"response_text"`
	ErrorMessage      string          `json:"error_message"`
	LatencyMs         int64           `json:"latency_ms"`
	Verdict           string          `json:"verdict"`
	ExpectedModel     string          `json:"expected_model"`
	DetectedModel     string          `json:"detected_model"`
	DetectedModelName string          `json:"detected_model_name"`
	DetectedFamily    string          `json:"detected_family"`
	Probability       *float64        `json:"probability"`
	AccountAction     string          `json:"account_action"`
	Detail            json.RawMessage `json:"detail,omitempty"`
	StartedAt         time.Time       `json:"started_at"`
	FinishedAt        time.Time       `json:"finished_at"`
	CreatedAt         time.Time       `json:"created_at"`
}

// ScheduledTestPlanRepository defines the data access interface for test plans.
type ScheduledTestPlanRepository interface {
	Create(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error)
	GetByID(ctx context.Context, id int64) (*ScheduledTestPlan, error)
	ListByAccountID(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error)
	ListDue(ctx context.Context, now time.Time) ([]*ScheduledTestPlan, error)
	Update(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error)
	Delete(ctx context.Context, id int64) error
	UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error
}

// ScheduledTestResultRepository defines the data access interface for test results.
type ScheduledTestResultRepository interface {
	Create(ctx context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error)
	ListByPlanID(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error)
	PruneOldResults(ctx context.Context, planID int64, keepCount int) error
}
