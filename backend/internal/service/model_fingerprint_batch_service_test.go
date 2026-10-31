package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ---------- fakes ----------

type fakeBatchRepo struct {
	mu            sync.Mutex
	nextRunID     int64
	nextResultID  int64
	runs          map[int64]*ModelFingerprintRun
	results       map[int64][]*ModelFingerprintRunResult
	markCleanCall int
	lastCleanMsg  string
}

func newFakeBatchRepo() *fakeBatchRepo {
	return &fakeBatchRepo{
		runs:    make(map[int64]*ModelFingerprintRun),
		results: make(map[int64][]*ModelFingerprintRunResult),
	}
}

func cloneBatchRun(run *ModelFingerprintRun) *ModelFingerprintRun {
	if run == nil {
		return nil
	}
	clone := *run
	clone.Models = append([]string(nil), run.Models...)
	if run.StartedAt != nil {
		t := *run.StartedAt
		clone.StartedAt = &t
	}
	if run.FinishedAt != nil {
		t := *run.FinishedAt
		clone.FinishedAt = &t
	}
	return &clone
}

func cloneBatchResult(result *ModelFingerprintRunResult) *ModelFingerprintRunResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Detail = append([]byte(nil), result.Detail...)
	if result.StartedAt != nil {
		t := *result.StartedAt
		clone.StartedAt = &t
	}
	if result.FinishedAt != nil {
		t := *result.FinishedAt
		clone.FinishedAt = &t
	}
	return &clone
}

func (f *fakeBatchRepo) isActive(run *ModelFingerprintRun) bool {
	return run.Status == ModelFingerprintRunPending || run.Status == ModelFingerprintRunRunning || run.Status == ModelFingerprintRunCancelling
}

func (f *fakeBatchRepo) CreateRun(ctx context.Context, run *ModelFingerprintRun, results []*ModelFingerprintRunResult) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.runs {
		if existing.AccountID == run.AccountID && f.isActive(existing) {
			return nil, ErrModelFingerprintBatchActive
		}
		if run.IdempotencyKey != "" && existing.AccountID == run.AccountID && existing.IdempotencyKey == run.IdempotencyKey {
			return nil, ErrModelFingerprintBatchIdempotency
		}
	}
	f.nextRunID++
	run.ID = f.nextRunID
	now := time.Now()
	run.CreatedAt = now
	run.UpdatedAt = now
	f.runs[run.ID] = cloneBatchRun(run)
	stored := make([]*ModelFingerprintRunResult, 0, len(results))
	for index, result := range results {
		f.nextResultID++
		result.ID = f.nextResultID
		result.RunID = run.ID
		result.Position = index
		result.CreatedAt = now
		result.UpdatedAt = now
		stored = append(stored, cloneBatchResult(result))
	}
	f.results[run.ID] = stored
	return cloneBatchRun(run), nil
}

func (f *fakeBatchRepo) GetRunForAccount(ctx context.Context, accountID, runID int64) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[runID]
	if !ok || run.AccountID != accountID {
		return nil, sql.ErrNoRows
	}
	return cloneBatchRun(run), nil
}

func (f *fakeBatchRepo) ListRunsByAccount(ctx context.Context, accountID int64, limit int) ([]*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*ModelFingerprintRun, 0)
	for id := f.nextRunID; id >= 1; id-- {
		run, ok := f.runs[id]
		if !ok || run.AccountID != accountID {
			continue
		}
		out = append(out, cloneBatchRun(run))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeBatchRepo) GetActiveRunByAccount(ctx context.Context, accountID int64) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id := f.nextRunID; id >= 1; id-- {
		run, ok := f.runs[id]
		if ok && run.AccountID == accountID && f.isActive(run) {
			return cloneBatchRun(run), nil
		}
	}
	return nil, nil
}

func (f *fakeBatchRepo) GetRunByIdempotencyKey(ctx context.Context, accountID int64, key string) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key == "" {
		return nil, nil
	}
	for _, run := range f.runs {
		if run.AccountID == accountID && run.IdempotencyKey == key {
			return cloneBatchRun(run), nil
		}
	}
	return nil, nil
}

func (f *fakeBatchRepo) GetResults(ctx context.Context, runID int64) ([]*ModelFingerprintRunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := f.results[runID]
	out := make([]*ModelFingerprintRunResult, 0, len(stored))
	for _, result := range stored {
		out = append(out, cloneBatchResult(result))
	}
	return out, nil
}

func (f *fakeBatchRepo) MarkRunning(ctx context.Context, runID int64) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[runID]
	if !ok || run.Status != ModelFingerprintRunPending {
		return nil, sql.ErrNoRows
	}
	now := time.Now()
	run.Status = ModelFingerprintRunRunning
	run.StartedAt = &now
	run.UpdatedAt = now
	return cloneBatchRun(run), nil
}

func (f *fakeBatchRepo) GetRunStatus(ctx context.Context, runID int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[runID]
	if !ok {
		return "", sql.ErrNoRows
	}
	return run.Status, nil
}

func (f *fakeBatchRepo) FinishResult(ctx context.Context, result *ModelFingerprintRunResult) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[result.RunID]
	if !ok || (run.Status != ModelFingerprintRunRunning && run.Status != ModelFingerprintRunCancelling) {
		return false, nil
	}
	stored := f.results[result.RunID]
	for i := range stored {
		if stored[i].ID != result.ID {
			continue
		}
		if stored[i].Status != ModelFingerprintResultPending && stored[i].Status != ModelFingerprintResultRunning {
			return false, nil
		}
		*result = *cloneBatchResult(result)
		stored[i] = cloneBatchResult(result)
		run.CompletedModels++
		switch result.Verdict {
		case FingerprintVerdictMatch:
			run.MatchedModels++
		case FingerprintVerdictMismatch:
			run.MismatchedModels++
		case FingerprintVerdictUnsupported:
			run.UnsupportedModels++
		default:
			run.FailedModels++
		}
		run.UpdatedAt = time.Now()
		return true, nil
	}
	return false, nil
}

func (f *fakeBatchRepo) FinalizeRun(ctx context.Context, runID int64, status, errorMessage, terminalReason string) (*ModelFingerprintRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[runID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	if run.Status != ModelFingerprintRunRunning && run.Status != ModelFingerprintRunCancelling {
		return cloneBatchRun(run), nil
	}
	now := time.Now()
	run.Status = status
	run.ErrorMessage = errorMessage
	run.TerminalReason = terminalReason
	run.FinishedAt = &now
	run.UpdatedAt = now
	for _, result := range f.results[runID] {
		if result.Status == ModelFingerprintResultPending || result.Status == ModelFingerprintResultRunning {
			result.Status = ModelFingerprintResultCancelled
			result.FinishedAt = &now
			run.CancelledModels++
		}
	}
	return cloneBatchRun(run), nil
}

func (f *fakeBatchRepo) RequestCancel(ctx context.Context, runID int64) (*ModelFingerprintRun, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[runID]
	if !ok {
		return nil, false, sql.ErrNoRows
	}
	if run.Status == ModelFingerprintRunPending || run.Status == ModelFingerprintRunRunning {
		run.Status = ModelFingerprintRunCancelling
		run.UpdatedAt = time.Now()
		return cloneBatchRun(run), true, nil
	}
	return cloneBatchRun(run), false, nil
}

func (f *fakeBatchRepo) MarkUnfinishedRunsFailed(ctx context.Context, reason string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCleanCall++
	f.lastCleanMsg = reason
	var count int64
	now := time.Now()
	for _, run := range f.runs {
		if !f.isActive(run) {
			continue
		}
		run.Status = ModelFingerprintRunFailed
		run.ErrorMessage = reason
		run.TerminalReason = "restart"
		run.FinishedAt = &now
		count++
		for _, result := range f.results[run.ID] {
			if result.Status == ModelFingerprintResultPending || result.Status == ModelFingerprintResultRunning {
				result.Status = ModelFingerprintResultCancelled
			}
		}
	}
	return count, nil
}

func (f *fakeBatchRepo) PruneRuns(ctx context.Context, accountID int64, keep int) error { return nil }

type fakeBatchAccountSource struct {
	mu      sync.Mutex
	account *Account
}

func (f *fakeBatchAccountSource) GetByID(ctx context.Context, id int64) (*Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.account, nil
}

type fakeBatchTester struct {
	mu            sync.Mutex
	detections    map[string]ModelFingerprintDetection
	active        int
	maxActive     int
	delay         time.Duration
	blockOnCancel bool
	started       chan struct{}
}

func (f *fakeBatchTester) DetectModel(ctx context.Context, account *Account, modelID string, cfg DetectorConfig) ModelFingerprintDetection {
	return f.detect(ctx, account, modelID, cfg)
}

func (f *fakeBatchTester) detect(ctx context.Context, account *Account, modelID string, cfg DetectorConfig) ModelFingerprintDetection {
	f.mu.Lock()
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	detection, ok := f.detections[modelID]
	started := f.started
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()

	if f.blockOnCancel {
		<-ctx.Done()
		return ModelFingerprintDetection{Status: ModelFingerprintResultFailed, Verdict: FingerprintVerdictError, Reason: "cancelled"}
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if ok {
		return detection
	}
	return ModelFingerprintDetection{Status: ModelFingerprintResultFailed, Verdict: FingerprintVerdictError, Reason: "no fake detection"}
}

func (f *fakeBatchTester) peakConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxActive
}

// ---------- helpers ----------

func newBatchTestService(t *testing.T, tester *fakeBatchTester, repo *fakeBatchRepo) *ModelFingerprintBatchService {
	t.Helper()
	return NewModelFingerprintBatchService(&fakeBatchAccountSource{account: &Account{ID: 1}}, tester, repo)
}

func waitBatchRun(t *testing.T, repo *fakeBatchRepo, accountID, runID int64, statuses ...string) *ModelFingerprintRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := repo.GetRunForAccount(context.Background(), accountID, runID)
		if err == nil && run != nil {
			for _, status := range statuses {
				if run.Status == status {
					return run
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	run, _ := repo.GetRunForAccount(context.Background(), accountID, runID)
	t.Fatalf("run %d did not reach %v, last=%+v", runID, statuses, run)
	return nil
}

// ---------- tests ----------

func TestModelFingerprintBatchStartRunCounts(t *testing.T) {
	repo := newFakeBatchRepo()
	tester := &fakeBatchTester{detections: map[string]ModelFingerprintDetection{
		"claude-haiku-4-5":  {Status: ModelFingerprintResultSuccess, Verdict: FingerprintVerdictMatch, ExpectedModel: "claude-haiku-4-5", DetectedModel: "claude-haiku-4.5"},
		"claude-sonnet-4-6": {Status: ModelFingerprintResultFailed, Verdict: FingerprintVerdictMismatch, ExpectedModel: "claude-sonnet-4-6", DetectedModel: "gpt-5.1"},
		"gpt-5.1":           {Status: ModelFingerprintResultFailed, Verdict: FingerprintVerdictUnsupported, Reason: "account type unsupported"},
	}}
	svc := newBatchTestService(t, tester, repo)

	run, results, err := svc.StartRun(context.Background(), 1, []string{"claude-haiku-4-5", "claude-sonnet-4-6", "gpt-5.1", "claude-haiku-4-5"}, DetectorConfig{}, 2, "")
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if len(results) != 3 || run.TotalModels != 3 {
		t.Fatalf("dedupe failed: results=%d total=%d", len(results), run.TotalModels)
	}
	if run.EstimatedRequests == 0 {
		t.Fatalf("estimated requests not set")
	}

	finished := waitBatchRun(t, repo, 1, run.ID, ModelFingerprintRunCompleted)
	if finished.CompletedModels != 3 || finished.MatchedModels != 1 || finished.MismatchedModels != 1 || finished.UnsupportedModels != 1 || finished.FailedModels != 0 {
		t.Fatalf("unexpected counters: %+v", finished)
	}
	if finished.TerminalReason != "completed" {
		t.Fatalf("terminal reason = %q", finished.TerminalReason)
	}
	persisted, _ := repo.GetResults(context.Background(), run.ID)
	if len(persisted) != 3 {
		t.Fatalf("persisted results = %d", len(persisted))
	}
	for index, result := range persisted {
		if result.Position != index {
			t.Fatalf("result[%d].Position = %d", index, result.Position)
		}
	}
	if persisted[0].Status != ModelFingerprintResultSuccess || persisted[0].Verdict != FingerprintVerdictMatch {
		t.Fatalf("result[0] = %+v", persisted[0])
	}
}

func TestModelFingerprintBatchIdempotency(t *testing.T) {
	repo := newFakeBatchRepo()
	svc := newBatchTestService(t, &fakeBatchTester{detections: map[string]ModelFingerprintDetection{}}, repo)

	first, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 1, "key-1")
	if err != nil {
		t.Fatalf("first StartRun: %v", err)
	}
	second, secondResults, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 1, "key-1")
	if err != nil {
		t.Fatalf("second StartRun with same key: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotent run id = %d, want %d", second.ID, first.ID)
	}
	if len(secondResults) != 1 {
		t.Fatalf("idempotent results = %d", len(secondResults))
	}
}

func TestModelFingerprintBatchRejectsActiveRun(t *testing.T) {
	repo := newFakeBatchRepo()
	existing := &ModelFingerprintRun{AccountID: 1, Status: ModelFingerprintRunRunning, TotalModels: 1}
	if _, err := repo.CreateRun(context.Background(), existing, []*ModelFingerprintRunResult{{ModelID: "m1", Status: ModelFingerprintResultPending}}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	svc := newBatchTestService(t, &fakeBatchTester{detections: map[string]ModelFingerprintDetection{}}, repo)

	_, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 2, "")
	if err == nil {
		t.Fatal("expected active run conflict")
	}
	if code := infraerrors.Code(err); code != 409 {
		t.Fatalf("code = %d, want 409 (%v)", code, err)
	}
	if reason := infraerrors.Reason(err); reason != "MODEL_FINGERPRINT_RUN_ACTIVE" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestModelFingerprintBatchCancel(t *testing.T) {
	repo := newFakeBatchRepo()
	tester := &fakeBatchTester{
		detections:    map[string]ModelFingerprintDetection{},
		blockOnCancel: true,
		started:       make(chan struct{}, 8),
	}
	svc := newBatchTestService(t, tester, repo)

	run, _, err := svc.StartRun(context.Background(), 1, []string{"m1", "m2", "m3"}, DetectorConfig{}, 1, "")
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	select {
	case <-tester.started:
	case <-time.After(2 * time.Second):
		t.Fatal("tester never started")
	}

	cancelled, err := svc.CancelRun(context.Background(), 1, run.ID)
	if err != nil {
		t.Fatalf("CancelRun error: %v", err)
	}
	if cancelled == nil || cancelled.Status != ModelFingerprintRunCancelling {
		t.Fatalf("cancel returned %+v", cancelled)
	}
	finished := waitBatchRun(t, repo, 1, run.ID, ModelFingerprintRunCancelled, ModelFingerprintRunFailed)
	if finished.Status != ModelFingerprintRunCancelled || finished.TerminalReason != "cancelled" {
		t.Fatalf("status=%s reason=%s", finished.Status, finished.TerminalReason)
	}
	results, _ := repo.GetResults(context.Background(), run.ID)
	for _, result := range results {
		if result.Status == ModelFingerprintResultPending || result.Status == ModelFingerprintResultRunning {
			t.Fatalf("result %s left in %s after cancel", result.ModelID, result.Status)
		}
	}
	if finished.CancelledModels == 0 {
		t.Fatalf("cancelled_models not recorded: %+v", finished)
	}
}

func TestModelFingerprintBatchConcurrencyLimit(t *testing.T) {
	repo := newFakeBatchRepo()
	tester := &fakeBatchTester{
		detections: map[string]ModelFingerprintDetection{},
		delay:      30 * time.Millisecond,
	}
	for _, model := range []string{"m1", "m2", "m3", "m4", "m5", "m6"} {
		tester.detections[model] = ModelFingerprintDetection{Status: ModelFingerprintResultSuccess, Verdict: FingerprintVerdictMatch}
	}
	svc := newBatchTestService(t, tester, repo)

	run, _, err := svc.StartRun(context.Background(), 1, []string{"m1", "m2", "m3", "m4", "m5", "m6"}, DetectorConfig{}, 2, "")
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	waitBatchRun(t, repo, 1, run.ID, ModelFingerprintRunCompleted)
	peak := tester.peakConcurrency()
	if peak > 2 || peak < 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak)
	}
}

func TestModelFingerprintBatchValidation(t *testing.T) {
	svc := newBatchTestService(t, &fakeBatchTester{detections: map[string]ModelFingerprintDetection{}}, newFakeBatchRepo())

	if _, _, err := svc.StartRun(context.Background(), 1, nil, DetectorConfig{}, 2, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("empty models: code=%d err=%v", infraerrors.Code(err), err)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, []string{"claude-opus-*"}, DetectorConfig{}, 2, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("wildcard model: code=%d err=%v", infraerrors.Code(err), err)
	}
	tooMany := make([]string, ModelFingerprintBatchMaxModels+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("model-%d", i)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, tooMany, DetectorConfig{}, 2, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("too many models: code=%d err=%v", infraerrors.Code(err), err)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 9, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("bad concurrency: code=%d err=%v", infraerrors.Code(err), err)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{API: "bad-api"}, 2, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("bad detector config: code=%d err=%v", infraerrors.Code(err), err)
	}
	models := make([]string, 50)
	for i := range models {
		models[i] = fmt.Sprintf("budget-%d", i)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, models, DetectorConfig{Count: 3, Parallel: 2, Rounds: 5}, 2, ""); infraerrors.Code(err) != 400 {
		t.Fatalf("budget: code=%d err=%v", infraerrors.Code(err), err)
	}
	if _, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 2, string(make([]byte, ModelFingerprintBatchMaxIdempotencyLen+1))); infraerrors.Code(err) != 400 {
		t.Fatalf("idempotency key: code=%d err=%v", infraerrors.Code(err), err)
	}
}

func TestModelFingerprintBatchStartCleanup(t *testing.T) {
	repo := newFakeBatchRepo()
	run := &ModelFingerprintRun{AccountID: 1, Status: ModelFingerprintRunRunning, TotalModels: 1}
	if _, err := repo.CreateRun(context.Background(), run, []*ModelFingerprintRunResult{{ModelID: "m1", Status: ModelFingerprintResultRunning}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newBatchTestService(t, &fakeBatchTester{detections: map[string]ModelFingerprintDetection{}}, repo)
	svc.Start()

	if repo.markCleanCall != 1 {
		t.Fatalf("MarkUnfinishedRunsFailed calls = %d", repo.markCleanCall)
	}
	cleaned, _ := repo.GetRunForAccount(context.Background(), 1, run.ID)
	if cleaned.Status != ModelFingerprintRunFailed {
		t.Fatalf("run status = %s", cleaned.Status)
	}
}

func TestModelFingerprintBatchGetRunWrongAccount(t *testing.T) {
	repo := newFakeBatchRepo()
	svc := newBatchTestService(t, &fakeBatchTester{detections: map[string]ModelFingerprintDetection{}}, repo)
	run, _, err := svc.StartRun(context.Background(), 1, []string{"m1"}, DetectorConfig{}, 1, "")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitBatchRun(t, repo, 1, run.ID, ModelFingerprintRunCompleted)
	if _, _, err := svc.GetRun(context.Background(), 999, run.ID); infraerrors.Code(err) != 404 {
		t.Fatalf("wrong account code=%d err=%v", infraerrors.Code(err), err)
	}
}

func TestModelFingerprintBatchLateResultDropped(t *testing.T) {
	repo := newFakeBatchRepo()
	run := &ModelFingerprintRun{AccountID: 1, Status: ModelFingerprintRunRunning, TotalModels: 1}
	results := []*ModelFingerprintRunResult{{ModelID: "m1", Status: ModelFingerprintResultPending}}
	if _, err := repo.CreateRun(context.Background(), run, results); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.FinalizeRun(context.Background(), run.ID, ModelFingerprintRunCancelled, "cancelled", "cancelled"); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	accepted, err := repo.FinishResult(context.Background(), &ModelFingerprintRunResult{
		ID: results[0].ID, RunID: run.ID, Status: ModelFingerprintResultSuccess, Verdict: FingerprintVerdictMatch,
	})
	if err != nil {
		t.Fatalf("FinishResult: %v", err)
	}
	if accepted {
		t.Fatal("late result must be dropped after finalize")
	}
	var _ = errors.Is
}
