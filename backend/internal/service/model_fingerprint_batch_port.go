package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Batch model fingerprint run statuses (account_model_fingerprint_runs.status).
const (
	ModelFingerprintRunPending    = "pending"
	ModelFingerprintRunRunning    = "running"
	ModelFingerprintRunCancelling = "cancelling"
	ModelFingerprintRunCompleted  = "completed"
	ModelFingerprintRunCancelled  = "cancelled"
	ModelFingerprintRunFailed     = "failed"
)

// Batch model fingerprint result statuses (account_model_fingerprint_results.status).
const (
	ModelFingerprintResultPending   = "pending"
	ModelFingerprintResultRunning   = "running"
	ModelFingerprintResultSuccess   = "success"
	ModelFingerprintResultFailed    = "failed"
	ModelFingerprintResultCancelled = "cancelled"
)

// Batch fingerprint limits.
const (
	ModelFingerprintBatchMaxModels         = 100
	ModelFingerprintBatchMaxConc           = 3
	ModelFingerprintBatchDefaultConc       = 2
	ModelFingerprintBatchMaxEstimatedReqs  = 600
	ModelFingerprintBatchRunTimeout        = 30 * time.Minute
	ModelFingerprintBatchKeepRuns          = 20
	ModelFingerprintBatchMaxModelLength    = 200
	ModelFingerprintBatchMaxIdempotencyLen = 200
	ModelFingerprintBatchMaxDetailBytes    = 16 * 1024
)

// Sentinel errors surfaced by the repository so the service/handler can map them
// to HTTP 409 without depending on PostgreSQL error codes.
var (
	ErrModelFingerprintBatchActive      = errors.New("another batch fingerprint run is already active for this account")
	ErrModelFingerprintBatchIdempotency = errors.New("duplicate batch fingerprint idempotency key")
)

// ModelFingerprintRun represents one batch fingerprint test job.
type ModelFingerprintRun struct {
	ID                int64          `json:"id"`
	AccountID         int64          `json:"account_id"`
	Status            string         `json:"status"`
	Models            []string       `json:"models"`
	DetectorConfig    DetectorConfig `json:"detector_config"`
	Concurrency       int            `json:"concurrency"`
	TotalModels       int            `json:"total_models"`
	CompletedModels   int            `json:"completed_models"`
	MatchedModels     int            `json:"matched_models"`
	MismatchedModels  int            `json:"mismatched_models"`
	FailedModels      int            `json:"failed_models"`
	UnsupportedModels int            `json:"unsupported_models"`
	CancelledModels   int            `json:"cancelled_models"`
	EstimatedRequests int            `json:"estimated_requests"`
	ErrorMessage      string         `json:"error_message"`
	TerminalReason    string         `json:"terminal_reason"`
	IdempotencyKey    string         `json:"idempotency_key,omitempty"`
	StartedAt         *time.Time     `json:"started_at"`
	FinishedAt        *time.Time     `json:"finished_at"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// ModelFingerprintRunResult is the per-model outcome of a batch run.
type ModelFingerprintRunResult struct {
	ID                int64           `json:"id"`
	RunID             int64           `json:"run_id"`
	Position          int             `json:"position"`
	ModelID           string          `json:"model_id"`
	Status            string          `json:"status"`
	Verdict           string          `json:"verdict"`
	ExpectedModel     string          `json:"expected_model"`
	DetectedModel     string          `json:"detected_model"`
	DetectedModelName string          `json:"detected_model_name"`
	DetectedFamily    string          `json:"detected_family"`
	Probability       *float64        `json:"probability"`
	LatencyMs         int64           `json:"latency_ms"`
	ErrorMessage      string          `json:"error_message"`
	Detail            json.RawMessage `json:"detail,omitempty"`
	DetailTruncated   bool            `json:"detail_truncated"`
	StartedAt         *time.Time      `json:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ModelFingerprintBatchRepository stores batch runs and their per-model results.
// All state transitions are conditional single statements so that cancel,
// timeout and late results cannot overwrite a terminal run.
type ModelFingerprintBatchRepository interface {
	CreateRun(ctx context.Context, run *ModelFingerprintRun, results []*ModelFingerprintRunResult) (*ModelFingerprintRun, error)
	GetRunForAccount(ctx context.Context, accountID, runID int64) (*ModelFingerprintRun, error)
	ListRunsByAccount(ctx context.Context, accountID int64, limit int) ([]*ModelFingerprintRun, error)
	GetActiveRunByAccount(ctx context.Context, accountID int64) (*ModelFingerprintRun, error)
	GetRunByIdempotencyKey(ctx context.Context, accountID int64, key string) (*ModelFingerprintRun, error)
	GetResults(ctx context.Context, runID int64) ([]*ModelFingerprintRunResult, error)
	MarkRunning(ctx context.Context, runID int64) (*ModelFingerprintRun, error)
	GetRunStatus(ctx context.Context, runID int64) (string, error)
	FinishResult(ctx context.Context, result *ModelFingerprintRunResult) (bool, error)
	FinalizeRun(ctx context.Context, runID int64, status, errorMessage, terminalReason string) (*ModelFingerprintRun, error)
	RequestCancel(ctx context.Context, runID int64) (*ModelFingerprintRun, bool, error)
	MarkUnfinishedRunsFailed(ctx context.Context, reason string) (int64, error)
	PruneRuns(ctx context.Context, accountID int64, keep int) error
}

// NormalizeBatchFingerprintModels trims, de-duplicates and validates the model
// list of a batch run. Order of first appearance is preserved.
func NormalizeBatchFingerprintModels(models []string) ([]string, error) {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		model := strings.TrimSpace(raw)
		if model == "" {
			continue
		}
		if len(model) > ModelFingerprintBatchMaxModelLength {
			return nil, fmt.Errorf("model id is longer than %d characters", ModelFingerprintBatchMaxModelLength)
		}
		if strings.Contains(model, "*") {
			return nil, fmt.Errorf("wildcard model %q cannot be fingerprint tested; use an exact model id", model)
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("models must not be empty")
	}
	if len(out) > ModelFingerprintBatchMaxModels {
		return nil, fmt.Errorf("too many models: %d (max %d)", len(out), ModelFingerprintBatchMaxModels)
	}
	return out, nil
}

// NormalizeBatchFingerprintConcurrency clamps/validates the batch concurrency.
func NormalizeBatchFingerprintConcurrency(concurrency int) (int, error) {
	if concurrency == 0 {
		return ModelFingerprintBatchDefaultConc, nil
	}
	if concurrency < 1 || concurrency > ModelFingerprintBatchMaxConc {
		return 0, fmt.Errorf("concurrency must be between 1 and %d", ModelFingerprintBatchMaxConc)
	}
	return concurrency, nil
}

// EstimateBatchFingerprintRequests returns the upstream request budget of a run.
func EstimateBatchFingerprintRequests(modelCount int, cfg DetectorConfig) int {
	if modelCount <= 0 {
		return 0
	}
	count := cfg.Count
	if count <= 0 {
		count = detectorConfigDefaultCount
	}
	rounds := cfg.Rounds
	if rounds <= 0 {
		rounds = detectorConfigDefaultRounds
	}
	return modelCount * count * rounds
}

// ValidateBatchFingerprintBudget enforces the per-run request budget.
func ValidateBatchFingerprintBudget(modelCount int, cfg DetectorConfig) (int, error) {
	estimated := EstimateBatchFingerprintRequests(modelCount, cfg)
	if estimated > ModelFingerprintBatchMaxEstimatedReqs {
		return 0, fmt.Errorf("estimated upstream requests %d exceed the limit %d; reduce models, count or rounds",
			estimated, ModelFingerprintBatchMaxEstimatedReqs)
	}
	return estimated, nil
}
