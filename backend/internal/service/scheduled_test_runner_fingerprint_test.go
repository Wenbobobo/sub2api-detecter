package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type fakeRunnerPlanRepo struct {
	due                 []*ScheduledTestPlan
	updateAfterRunCalls int
}

func (f *fakeRunnerPlanRepo) Create(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return plan, nil
}

func (f *fakeRunnerPlanRepo) GetByID(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	for _, plan := range f.due {
		if plan.ID == id {
			return plan, nil
		}
	}
	return nil, nil
}

func (f *fakeRunnerPlanRepo) ListByAccountID(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return f.due, nil
}

func (f *fakeRunnerPlanRepo) ListDue(ctx context.Context, now time.Time) ([]*ScheduledTestPlan, error) {
	return f.due, nil
}

func (f *fakeRunnerPlanRepo) Update(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return plan, nil
}

func (f *fakeRunnerPlanRepo) Delete(ctx context.Context, id int64) error { return nil }

func (f *fakeRunnerPlanRepo) UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	f.updateAfterRunCalls++
	return nil
}

type fakeRunnerResultRepo struct {
	created []*ScheduledTestResult
}

func (f *fakeRunnerResultRepo) Create(ctx context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error) {
	result.ID = int64(len(f.created) + 1)
	f.created = append(f.created, result)
	return result, nil
}

func (f *fakeRunnerResultRepo) ListByPlanID(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	return f.created, nil
}

func (f *fakeRunnerResultRepo) PruneOldResults(ctx context.Context, planID int64, keepCount int) error {
	return nil
}

type fakeRunnerAccountScheduler struct {
	account      *Account
	getErr       error
	setErr       error
	extraErr     error
	setCalls     []bool
	extraUpdates []map[string]any
}

func (f *fakeRunnerAccountScheduler) GetByID(ctx context.Context, id int64) (*Account, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.account, nil
}

func (f *fakeRunnerAccountScheduler) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	f.setCalls = append(f.setCalls, schedulable)
	if f.account != nil {
		f.account.Schedulable = schedulable
	}
	return f.setErr
}

func (f *fakeRunnerAccountScheduler) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	f.extraUpdates = append(f.extraUpdates, updates)
	return f.extraErr
}

func newRunnerFingerprintTest(plan *ScheduledTestPlan, account *Account, analyzer *fakeFingerprintAnalyzer, upstream *fakeFingerprintUpstream, scheduler *fakeRunnerAccountScheduler) (*ScheduledTestRunnerService, *fakeRunnerResultRepo) {
	resultRepo := &fakeRunnerResultRepo{}
	planRepo := &fakeRunnerPlanRepo{due: []*ScheduledTestPlan{plan}}
	scheduledSvc := NewScheduledTestService(planRepo, resultRepo)
	fingerprintSvc := NewModelFingerprintService(&fakeFingerprintAccountSource{account: account}, upstream, nil, nil)
	fingerprintSvc.detector = analyzer
	runner := &ScheduledTestRunnerService{
		planRepo:         planRepo,
		scheduledSvc:     scheduledSvc,
		fingerprintSvc:   fingerprintSvc,
		accountScheduler: scheduler,
		cfg:              &config.Config{},
	}
	return runner, resultRepo
}

func TestRunnerFingerprintMismatchAutoDisable(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = true
	account.Extra = map[string]any{}
	upstream := &fakeFingerprintUpstream{bodies: [][]byte{
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
	}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("gpt-5.6-sol", "gpt", 0.9), builtAt: "test"}
	scheduler := &fakeRunnerAccountScheduler{account: account}
	plan := &ScheduledTestPlan{
		ID:             10,
		AccountID:      11,
		ModelID:        "claude-sonnet-4-5",
		CronExpression: "* * * * *",
		MaxResults:     10,
		TestMode:       ScheduledTestModeFingerprint,
		AutoDisable:    true,
		Enabled:        true,
	}
	runner, resultRepo := newRunnerFingerprintTest(plan, account, analyzer, upstream, scheduler)

	runner.runOnePlan(context.Background(), plan)

	if len(resultRepo.created) != 1 {
		t.Fatalf("results saved = %d", len(resultRepo.created))
	}
	result := resultRepo.created[0]
	if result.Verdict != FingerprintVerdictMismatch || result.Status != "failed" {
		t.Fatalf("unexpected result: verdict=%s status=%s", result.Verdict, result.Status)
	}
	if result.AccountAction != ScheduledAccountActionDisabled {
		t.Fatalf("account action = %q", result.AccountAction)
	}
	if len(scheduler.setCalls) != 1 || scheduler.setCalls[0] != false {
		t.Fatalf("set schedulable calls = %v", scheduler.setCalls)
	}
	if len(scheduler.extraUpdates) != 1 {
		t.Fatalf("extra updates = %d", len(scheduler.extraUpdates))
	}
	marker, ok := scheduler.extraUpdates[0][scheduledTestAutoDisableExtraKey].(map[string]any)
	if !ok || marker["reason"] != "fingerprint_mismatch" {
		t.Fatalf("marker = %#v", scheduler.extraUpdates[0][scheduledTestAutoDisableExtraKey])
	}
}

func TestRunnerFingerprintMatchAutoRecover(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = false
	account.Extra = map[string]any{scheduledTestAutoDisableExtraKey: map[string]any{"plan_id": int64(1), "reason": "fingerprint_mismatch"}}
	upstream := &fakeFingerprintUpstream{bodies: [][]byte{
		fingerprintStreamBody("claude-sonnet-4.6"),
		fingerprintStreamBody("claude-sonnet-4.6"),
		fingerprintStreamBody("claude-sonnet-4.6"),
	}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.95), builtAt: "test"}
	scheduler := &fakeRunnerAccountScheduler{account: account}
	plan := &ScheduledTestPlan{
		ID:             11,
		AccountID:      11,
		ModelID:        "claude-sonnet-4-5",
		CronExpression: "* * * * *",
		MaxResults:     10,
		TestMode:       ScheduledTestModeFingerprint,
		AutoRecover:    true,
		Enabled:        true,
	}
	runner, resultRepo := newRunnerFingerprintTest(plan, account, analyzer, upstream, scheduler)

	runner.runOnePlan(context.Background(), plan)

	result := resultRepo.created[0]
	if result.Verdict != FingerprintVerdictMatch || result.AccountAction != ScheduledAccountActionRestored {
		t.Fatalf("unexpected result: verdict=%s action=%s", result.Verdict, result.AccountAction)
	}
	if len(scheduler.setCalls) != 1 || scheduler.setCalls[0] != true {
		t.Fatalf("set schedulable calls = %v", scheduler.setCalls)
	}
	if len(scheduler.extraUpdates) != 1 {
		t.Fatalf("extra updates = %d", len(scheduler.extraUpdates))
	}
	if value, exists := scheduler.extraUpdates[0][scheduledTestAutoDisableExtraKey]; !exists || value != nil {
		t.Fatalf("marker must be cleared with nil, got %#v", value)
	}
}

func TestRunnerMismatchWithoutAutoDisable(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = true
	upstream := &fakeFingerprintUpstream{bodies: [][]byte{
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
	}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("gpt-5.6-sol", "gpt", 0.9)}
	scheduler := &fakeRunnerAccountScheduler{account: account}
	plan := &ScheduledTestPlan{ID: 12, AccountID: 11, ModelID: "claude-sonnet-4-5", CronExpression: "* * * * *", MaxResults: 10, TestMode: ScheduledTestModeFingerprint, Enabled: true}
	runner, resultRepo := newRunnerFingerprintTest(plan, account, analyzer, upstream, scheduler)

	runner.runOnePlan(context.Background(), plan)

	if len(scheduler.setCalls) != 0 || len(scheduler.extraUpdates) != 0 {
		t.Fatalf("auto_disable=false must not touch scheduling: %v %v", scheduler.setCalls, scheduler.extraUpdates)
	}
	if resultRepo.created[0].AccountAction != "" {
		t.Fatalf("account action = %q", resultRepo.created[0].AccountAction)
	}
}

func TestRunnerUnsupportedAccountNotDisabled(t *testing.T) {
	oauth := &Account{ID: 13, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Schedulable: true}
	upstream := &fakeFingerprintUpstream{}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.9)}
	scheduler := &fakeRunnerAccountScheduler{account: oauth}
	plan := &ScheduledTestPlan{ID: 13, AccountID: 13, ModelID: "claude-sonnet-4-5", CronExpression: "* * * * *", MaxResults: 10, TestMode: ScheduledTestModeFingerprint, AutoDisable: true, Enabled: true}
	runner, resultRepo := newRunnerFingerprintTest(plan, oauth, analyzer, upstream, scheduler)

	runner.runOnePlan(context.Background(), plan)

	if len(scheduler.setCalls) != 0 {
		t.Fatalf("unsupported account must not be disabled, calls=%v", scheduler.setCalls)
	}
	if resultRepo.created[0].Verdict != FingerprintVerdictUnsupported {
		t.Fatalf("verdict = %s", resultRepo.created[0].Verdict)
	}
}

func TestRunnerAutoRecoverWithoutMarkerKeepsManualState(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = false
	account.Extra = map[string]any{}
	upstream := &fakeFingerprintUpstream{bodies: [][]byte{
		fingerprintStreamBody("claude-sonnet-4.6"),
		fingerprintStreamBody("claude-sonnet-4.6"),
		fingerprintStreamBody("claude-sonnet-4.6"),
	}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.95)}
	scheduler := &fakeRunnerAccountScheduler{account: account}
	plan := &ScheduledTestPlan{ID: 14, AccountID: 11, ModelID: "claude-sonnet-4-5", CronExpression: "* * * * *", MaxResults: 10, TestMode: ScheduledTestModeFingerprint, AutoRecover: true, Enabled: true}
	runner, resultRepo := newRunnerFingerprintTest(plan, account, analyzer, upstream, scheduler)

	runner.runOnePlan(context.Background(), plan)

	if len(scheduler.setCalls) != 0 {
		t.Fatalf("manual disable must not be overridden, calls=%v", scheduler.setCalls)
	}
	if resultRepo.created[0].AccountAction != "" {
		t.Fatalf("account action = %q", resultRepo.created[0].AccountAction)
	}
}

func TestRunnerAutoDisableLivenessFailure(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = true
	account.Extra = map[string]any{}
	scheduler := &fakeRunnerAccountScheduler{account: account}
	runner := &ScheduledTestRunnerService{accountScheduler: scheduler}
	plan := &ScheduledTestPlan{ID: 15, AccountID: 11, AutoDisable: true}
	result := &ScheduledTestResult{Status: "failed", Verdict: ""}

	runner.applyAutoDisable(context.Background(), plan, result)

	if len(scheduler.setCalls) != 1 || scheduler.setCalls[0] != false {
		t.Fatalf("set schedulable calls = %v", scheduler.setCalls)
	}
	if result.AccountAction != ScheduledAccountActionDisabled {
		t.Fatalf("account action = %q", result.AccountAction)
	}
	marker, ok := scheduler.extraUpdates[0][scheduledTestAutoDisableExtraKey].(map[string]any)
	if !ok || marker["reason"] != "test_failed" {
		t.Fatalf("marker = %#v", scheduler.extraUpdates[0])
	}
}

func TestRunnerAutoDisableSkipsAlreadyDisabledAccount(t *testing.T) {
	account := anthropicAPIKeyAccount()
	account.Schedulable = false
	scheduler := &fakeRunnerAccountScheduler{account: account}
	runner := &ScheduledTestRunnerService{accountScheduler: scheduler}
	result := &ScheduledTestResult{Status: "failed"}
	runner.applyAutoDisable(context.Background(), &ScheduledTestPlan{ID: 16, AccountID: 11}, result)
	if len(scheduler.setCalls) != 0 || len(scheduler.extraUpdates) != 0 {
		t.Fatalf("already disabled account must be untouched: %v %v", scheduler.setCalls, scheduler.extraUpdates)
	}
}

func TestRunnerFingerprintServiceMissing(t *testing.T) {
	resultRepo := &fakeRunnerResultRepo{}
	planRepo := &fakeRunnerPlanRepo{}
	runner := &ScheduledTestRunnerService{
		planRepo:     planRepo,
		scheduledSvc: NewScheduledTestService(planRepo, resultRepo),
		cfg:          &config.Config{},
	}
	plan := &ScheduledTestPlan{ID: 17, AccountID: 11, ModelID: "claude-sonnet-4-5", CronExpression: "* * * * *", MaxResults: 10, TestMode: ScheduledTestModeFingerprint, AutoDisable: true}
	runner.runOnePlan(context.Background(), plan)
	if len(resultRepo.created) != 1 {
		t.Fatalf("results = %d", len(resultRepo.created))
	}
	if resultRepo.created[0].Verdict != FingerprintVerdictUnsupported || resultRepo.created[0].Status != "failed" {
		t.Fatalf("unexpected result: %+v", resultRepo.created[0])
	}
}

func TestScheduledTestDisableReason(t *testing.T) {
	cases := map[string]string{
		FingerprintVerdictMismatch:     "fingerprint_mismatch",
		FingerprintVerdictInvalidReply: "invalid_reply",
		FingerprintVerdictError:        "fingerprint_error",
		"":                             "test_failed",
	}
	for verdict, want := range cases {
		if got := scheduledTestDisableReason(&ScheduledTestResult{Verdict: verdict}); got != want {
			t.Fatalf("reason(%q) = %q, want %q", verdict, got, want)
		}
	}
}

func TestRunnerAccountLocksArePerAccount(t *testing.T) {
	runner := &ScheduledTestRunnerService{}
	unlockA := runner.lockAccount(1)
	defer unlockA()
	unlocked := make(chan struct{})
	go func() {
		unlockB := runner.lockAccount(2)
		unlockB()
		close(unlocked)
	}()
	select {
	case <-unlocked:
	case <-time.After(time.Second):
		t.Fatal("locking account 2 must not block on account 1")
	}
}
