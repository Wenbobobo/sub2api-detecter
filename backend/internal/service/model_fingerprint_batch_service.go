package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// Batch run terminal reasons.
const (
	modelFingerprintTerminalCompleted = "completed"
	modelFingerprintTerminalCancelled = "cancelled"
	modelFingerprintTerminalTimeout   = "timeout"
)

// batchFingerprintAccountSource is the narrow account lookup used by batch runs.
type batchFingerprintAccountSource interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
}

// batchFingerprintTester runs one fingerprint test for one account+model.
type batchFingerprintTester interface {
	DetectModel(ctx context.Context, account *Account, modelID string, detectorConfig DetectorConfig) ModelFingerprintDetection
}

// ModelFingerprintBatchService runs asynchronous batch fingerprint tests for the
// models of an account model whitelist.
//
// Consistency model (v1, node-local execution):
//   - A partial unique index ensures at most one active run per account, so two
//     concurrent creates (even from different processes) cannot both succeed.
//   - Results and counters are updated by one conditional SQL statement; late
//     responses after a cancel/finish are dropped.
//   - Cancel and finish are conditional transitions (pending/running ->
//     cancelling -> terminal); an in-process cancel map only accelerates the
//     local worker, the database status is the source of truth.
type ModelFingerprintBatchService struct {
	accountRepo batchFingerprintAccountSource
	tester      batchFingerprintTester
	repo        ModelFingerprintBatchRepository

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	wg      sync.WaitGroup
}

// NewModelFingerprintBatchService creates the batch fingerprint service.
func NewModelFingerprintBatchService(
	accountRepo batchFingerprintAccountSource,
	tester batchFingerprintTester,
	repo ModelFingerprintBatchRepository,
) *ModelFingerprintBatchService {
	return &ModelFingerprintBatchService{
		accountRepo: accountRepo,
		tester:      tester,
		repo:        repo,
		cancels:     make(map[int64]context.CancelFunc),
	}
}

// Start marks runs left pending/running by a previous process as failed.
func (s *ModelFingerprintBatchService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	count, err := s.repo.MarkUnfinishedRunsFailed(context.Background(), "服务重启，任务中断")
	if err != nil {
		logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] startup cleanup failed: %v", err)
		return
	}
	if count > 0 {
		logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] marked %d unfinished run(s) as failed after restart", count)
	}
}

// Stop cancels every local running job and waits for the workers to exit.
func (s *ModelFingerprintBatchService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	s.wg.Wait()
}

// StartRun validates the request, persists the run with pending result rows and
// starts the background worker. A non-empty idempotencyKey reuses the existing run.
func (s *ModelFingerprintBatchService) StartRun(
	ctx context.Context,
	accountID int64,
	models []string,
	detectorConfig DetectorConfig,
	concurrency int,
	idempotencyKey string,
) (*ModelFingerprintRun, []*ModelFingerprintRunResult, error) {
	if s == nil || s.repo == nil || s.accountRepo == nil || s.tester == nil {
		return nil, nil, infraerrors.InternalServer("MODEL_FINGERPRINT_BATCH_UNAVAILABLE", "batch fingerprint service is not initialized")
	}

	normalizedModels, err := NormalizeBatchFingerprintModels(models)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("MODEL_FINGERPRINT_INVALID_MODELS", err.Error())
	}
	normalizedCfg, err := NormalizeDetectorConfig(detectorConfig)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("MODEL_FINGERPRINT_INVALID_CONFIG", err.Error())
	}
	normalizedConcurrency, err := NormalizeBatchFingerprintConcurrency(concurrency)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("MODEL_FINGERPRINT_INVALID_CONCURRENCY", err.Error())
	}
	estimatedRequests, err := ValidateBatchFingerprintBudget(len(normalizedModels), normalizedCfg)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("MODEL_FINGERPRINT_BUDGET_EXCEEDED", err.Error())
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > ModelFingerprintBatchMaxIdempotencyLen {
		return nil, nil, infraerrors.BadRequest("MODEL_FINGERPRINT_INVALID_IDEMPOTENCY_KEY", "idempotency key is too long")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, nil, infraerrors.NotFound("ACCOUNT_NOT_FOUND", "account not found")
	}

	if idempotencyKey != "" {
		existing, lookupErr := s.repo.GetRunByIdempotencyKey(ctx, accountID, idempotencyKey)
		if lookupErr != nil {
			return nil, nil, lookupErr
		}
		if existing != nil {
			results, resultsErr := s.repo.GetResults(ctx, existing.ID)
			if resultsErr != nil {
				return nil, nil, resultsErr
			}
			return existing, results, nil
		}
	}

	active, err := s.repo.GetActiveRunByAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	if active != nil {
		return nil, nil, infraerrors.Conflict("MODEL_FINGERPRINT_RUN_ACTIVE",
			"another batch fingerprint run is already active for this account")
	}

	run := &ModelFingerprintRun{
		AccountID:         accountID,
		Status:            ModelFingerprintRunPending,
		Models:            normalizedModels,
		DetectorConfig:    normalizedCfg,
		Concurrency:       normalizedConcurrency,
		TotalModels:       len(normalizedModels),
		EstimatedRequests: estimatedRequests,
		IdempotencyKey:    idempotencyKey,
	}
	results := make([]*ModelFingerprintRunResult, 0, len(normalizedModels))
	for _, model := range normalizedModels {
		results = append(results, &ModelFingerprintRunResult{
			ModelID: model,
			Status:  ModelFingerprintResultPending,
		})
	}

	created, err := s.repo.CreateRun(ctx, run, results)
	if err != nil {
		switch {
		case errors.Is(err, ErrModelFingerprintBatchActive):
			return nil, nil, infraerrors.Conflict("MODEL_FINGERPRINT_RUN_ACTIVE",
				"another batch fingerprint run is already active for this account")
		case errors.Is(err, ErrModelFingerprintBatchIdempotency) && idempotencyKey != "":
			existing, lookupErr := s.repo.GetRunByIdempotencyKey(ctx, accountID, idempotencyKey)
			if lookupErr == nil && existing != nil {
				existingResults, resultsErr := s.repo.GetResults(ctx, existing.ID)
				if resultsErr != nil {
					return nil, nil, resultsErr
				}
				return existing, existingResults, nil
			}
			return nil, nil, err
		default:
			return nil, nil, err
		}
	}
	if pruneErr := s.repo.PruneRuns(ctx, accountID, ModelFingerprintBatchKeepRuns); pruneErr != nil {
		logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] prune runs account=%d failed: %v", accountID, pruneErr)
	}

	runCtx, cancel := context.WithTimeout(context.Background(), ModelFingerprintBatchRunTimeout)
	s.mu.Lock()
	s.cancels[created.ID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer s.unregisterCancel(created.ID)
		s.executeRun(runCtx, created.ID, results, account, normalizedCfg, normalizedConcurrency)
	}()

	return created, results, nil
}

// ListRuns returns the latest runs of an account (without per-model results).
func (s *ModelFingerprintBatchService) ListRuns(ctx context.Context, accountID int64, limit int) ([]*ModelFingerprintRun, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("MODEL_FINGERPRINT_BATCH_UNAVAILABLE", "batch fingerprint service is not initialized")
	}
	return s.repo.ListRunsByAccount(ctx, accountID, limit)
}

// GetRun returns one run and its per-model results (owner scoped).
func (s *ModelFingerprintBatchService) GetRun(ctx context.Context, accountID, runID int64) (*ModelFingerprintRun, []*ModelFingerprintRunResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, infraerrors.InternalServer("MODEL_FINGERPRINT_BATCH_UNAVAILABLE", "batch fingerprint service is not initialized")
	}
	run, err := s.repo.GetRunForAccount(ctx, accountID, runID)
	if errors.Is(err, sql.ErrNoRows) || run == nil {
		return nil, nil, infraerrors.NotFound("MODEL_FINGERPRINT_RUN_NOT_FOUND", "batch fingerprint run not found")
	}
	if err != nil {
		return nil, nil, err
	}
	results, err := s.repo.GetResults(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	return run, results, nil
}

// CancelRun moves a pending/running run to cancelling. Finished runs return an
// error; an already cancelled/cancelling run is returned idempotently.
func (s *ModelFingerprintBatchService) CancelRun(ctx context.Context, accountID, runID int64) (*ModelFingerprintRun, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("MODEL_FINGERPRINT_BATCH_UNAVAILABLE", "batch fingerprint service is not initialized")
	}
	run, err := s.repo.GetRunForAccount(ctx, accountID, runID)
	if errors.Is(err, sql.ErrNoRows) || run == nil {
		return nil, infraerrors.NotFound("MODEL_FINGERPRINT_RUN_NOT_FOUND", "batch fingerprint run not found")
	}
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case ModelFingerprintRunCancelling, ModelFingerprintRunCancelled:
		return run, nil
	case ModelFingerprintRunCompleted, ModelFingerprintRunFailed:
		return nil, infraerrors.Conflict("MODEL_FINGERPRINT_RUN_NOT_CANCELABLE", "run is already finished")
	}

	cancelledRun, updated, err := s.repo.RequestCancel(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("MODEL_FINGERPRINT_RUN_NOT_FOUND", "batch fingerprint run not found")
	}
	if err != nil {
		return nil, err
	}
	if !updated {
		switch cancelledRun.Status {
		case ModelFingerprintRunCancelling, ModelFingerprintRunCancelled:
			return cancelledRun, nil
		default:
			return nil, infraerrors.Conflict("MODEL_FINGERPRINT_RUN_NOT_CANCELABLE", "run is already finished")
		}
	}
	s.mu.Lock()
	cancel := s.cancels[runID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return cancelledRun, nil
}

func (s *ModelFingerprintBatchService) unregisterCancel(runID int64) {
	s.mu.Lock()
	delete(s.cancels, runID)
	s.mu.Unlock()
}

// executeRun schedules every model with the configured concurrency, persists each
// finished result through FinishResult and finalises the run exactly once.
func (s *ModelFingerprintBatchService) executeRun(
	ctx context.Context,
	runID int64,
	results []*ModelFingerprintRunResult,
	account *Account,
	cfg DetectorConfig,
	concurrency int,
) {
	if run, err := s.repo.MarkRunning(ctx, runID); err != nil || run == nil {
		logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] run=%d cannot start: %v", runID, err)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			_, _ = s.repo.FinalizeRun(context.Background(), runID, ModelFingerprintRunFailed, "启动任务失败："+err.Error(), "start_failed")
		}
		return
	}

	workers := concurrency
	if workers > len(results) {
		workers = len(results)
	}
	if workers < 1 {
		workers = 1
	}

	type finished struct {
		index     int
		detection ModelFingerprintDetection
	}

	jobs := make(chan int)
	finishedCh := make(chan finished, workers)
	var workersWG sync.WaitGroup

	for i := 0; i < workers; i++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					detection := s.tester.DetectModel(ctx, account, results[index].ModelID, cfg)
					finishedCh <- finished{index: index, detection: detection}
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for index := range results {
			if ctx.Err() != nil {
				return
			}
			status, err := s.repo.GetRunStatus(ctx, runID)
			if err != nil || status != ModelFingerprintRunRunning {
				return
			}
			select {
			case <-ctx.Done():
				return
			case jobs <- index:
			}
		}
	}()
	go func() {
		workersWG.Wait()
		close(finishedCh)
	}()

	for item := range finishedCh {
		result := applyModelFingerprintDetection(results[item.index], item.detection)
		accepted, err := s.repo.FinishResult(context.Background(), result)
		if err != nil {
			logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] run=%d result=%d save failed: %v", runID, result.ID, err)
			continue
		}
		if !accepted {
			logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] run=%d result=%d dropped (run is not active)", runID, result.ID)
		}
	}

	status := ModelFingerprintRunCompleted
	message := ""
	reason := modelFingerprintTerminalCompleted
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = ModelFingerprintRunFailed
			message = "任务超时"
			reason = modelFingerprintTerminalTimeout
		} else {
			status = ModelFingerprintRunCancelled
			message = "任务已取消"
			reason = modelFingerprintTerminalCancelled
		}
	} else if current, err := s.repo.GetRunStatus(context.Background(), runID); err == nil && current == ModelFingerprintRunCancelling {
		status = ModelFingerprintRunCancelled
		message = "任务已取消"
		reason = modelFingerprintTerminalCancelled
	}

	if _, err := s.repo.FinalizeRun(context.Background(), runID, status, message, reason); err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] run=%d finalize failed: %v", runID, err)
	}
	logger.LegacyPrintf("service.model_fingerprint_batch", "[ModelFingerprintBatch] run=%d finished status=%s reason=%s", runID, status, reason)
}

// applyModelFingerprintDetection copies one detection into a result row,
// trimming oversized detail JSON.
func applyModelFingerprintDetection(result *ModelFingerprintRunResult, detection ModelFingerprintDetection) *ModelFingerprintRunResult {
	result.Status = detection.Status
	result.Verdict = detection.Verdict
	result.ExpectedModel = detection.ExpectedModel
	result.DetectedModel = detection.DetectedModel
	result.DetectedModelName = detection.DetectedModelName
	result.DetectedFamily = detection.DetectedFamily
	result.Probability = detection.Probability
	result.LatencyMs = detection.LatencyMs
	result.ErrorMessage = detection.Reason
	result.Detail, result.DetailTruncated = trimBatchFingerprintDetail(detection.Detail)
	finishedAt := time.Now()
	result.FinishedAt = &finishedAt
	result.UpdatedAt = finishedAt
	return result
}

// trimBatchFingerprintDetail bounds one model's detail JSON. When the full detail
// is too large it progressively drops the heavy per-sample data.
func trimBatchFingerprintDetail(detail map[string]any) (json.RawMessage, bool) {
	if detail == nil {
		return nil, false
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil, false
	}
	if len(raw) <= ModelFingerprintBatchMaxDetailBytes {
		return raw, false
	}
	for _, dropKeys := range [][]string{{"rounds"}, {"rounds", "samples"}, {"rounds", "samples", "target"}, {"rounds", "samples", "target", "top"}} {
		trimmed := make(map[string]any, len(detail))
		for key, value := range detail {
			trimmed[key] = value
		}
		for _, key := range dropKeys {
			delete(trimmed, key)
		}
		if candidate, err := json.Marshal(trimmed); err == nil && len(candidate) <= ModelFingerprintBatchMaxDetailBytes {
			return candidate, true
		}
	}
	fallback, _ := json.Marshal(map[string]any{
		"truncated":      true,
		"original_bytes": len(raw),
		"note":           "detail exceeded the size limit",
	})
	return fallback, true
}
