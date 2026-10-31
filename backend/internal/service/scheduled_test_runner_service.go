package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const scheduledTestDefaultMaxWorkers = 10

// ScheduledTestAccountScheduler is the narrow account-state port used by the
// runner to disable/re-enable account scheduling. *accountRepository satisfies
// it directly (GetByID, SetSchedulable, UpdateExtra).
type ScheduledTestAccountScheduler interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

// ScheduledTestRunnerService periodically scans due test plans and executes them.
type ScheduledTestRunnerService struct {
	planRepo         ScheduledTestPlanRepository
	scheduledSvc     *ScheduledTestService
	accountTestSvc   *AccountTestService
	fingerprintSvc   *ModelFingerprintService
	accountScheduler ScheduledTestAccountScheduler
	rateLimitSvc     *RateLimitService
	cfg              *config.Config

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once

	// accountLocks serialises runs of the same account so two plans cannot race
	// on scheduling state (auto disable / auto recover).
	accountLocks sync.Map // map[int64]*sync.Mutex
}

// NewScheduledTestRunnerService creates a new runner.
func NewScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	fingerprintSvc *ModelFingerprintService,
	accountScheduler ScheduledTestAccountScheduler,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	return &ScheduledTestRunnerService{
		planRepo:         planRepo,
		scheduledSvc:     scheduledSvc,
		accountTestSvc:   accountTestSvc,
		fingerprintSvc:   fingerprintSvc,
		accountScheduler: accountScheduler,
		rateLimitSvc:     rateLimitSvc,
		cfg:              cfg,
	}
}

// Start begins the cron ticker (every minute).
func (s *ScheduledTestRunnerService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
			}
		}

		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc("* * * * *", func() { s.runScheduled() })
		if err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] not started (invalid schedule): %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] started (tick=every minute)")
	})
}

// Stop gracefully shuts down the cron scheduler.
func (s *ScheduledTestRunnerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] cron stop timed out")
			}
		}
	})
}

func (s *ScheduledTestRunnerService) runScheduled() {
	// Delay 10s so execution lands at ~:10 of each minute instead of :00.
	time.Sleep(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	plans, err := s.planRepo.ListDue(ctx, now)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] ListDue error: %v", err)
		return
	}
	if len(plans) == 0 {
		return
	}

	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] found %d due plans", len(plans))

	sem := make(chan struct{}, scheduledTestDefaultMaxWorkers)
	var wg sync.WaitGroup

	for _, plan := range plans {
		sem <- struct{}{}
		wg.Add(1)
		go func(p *ScheduledTestPlan) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOnePlan(ctx, p)
		}(plan)
	}

	wg.Wait()
}

func (s *ScheduledTestRunnerService) runOnePlan(ctx context.Context, plan *ScheduledTestPlan) {
	unlock := s.lockAccount(plan.AccountID)
	defer unlock()

	result, err := s.runScheduledTest(ctx, plan)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d run error: %v", plan.ID, err)
		return
	}
	if result == nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d produced no result", plan.ID)
		return
	}

	// Account scheduling actions run before persisting so account_action is stored.
	// Fingerprint mode only disables on a definitive identity mismatch: upstream
	// errors, invalid replies, unscorable runs and unsupported accounts must not
	// take an account offline (alias/expectation mistakes are not upstream faults).
	if plan.AutoDisable && result.Status == "failed" {
		if plan.TestMode == ScheduledTestModeFingerprint && result.Verdict != FingerprintVerdictMismatch {
			logger.LegacyPrintf("service.scheduled_test_runner",
				"[ScheduledTestRunner] plan=%d fingerprint failure verdict=%q does not auto-disable the account", plan.ID, result.Verdict)
		} else {
			s.applyAutoDisable(ctx, plan, result)
		}
	}
	if plan.AutoRecover && result.Status == "success" {
		s.applyAutoRecover(ctx, plan, result)
	}

	if err := s.scheduledSvc.SaveResult(ctx, plan.ID, plan.MaxResults, result); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d SaveResult error: %v", plan.ID, err)
	}

	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d computeNextRun error: %v", plan.ID, err)
		return
	}

	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
	}
}

// runScheduledTest picks the fingerprint or the legacy liveness test path.
func (s *ScheduledTestRunnerService) runScheduledTest(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestResult, error) {
	if plan.TestMode == ScheduledTestModeFingerprint {
		if s.fingerprintSvc == nil {
			now := time.Now()
			return &ScheduledTestResult{
				Status:       "failed",
				Verdict:      FingerprintVerdictUnsupported,
				ErrorMessage: "服务端未启用模型指纹检测（缺少指纹检测服务）",
				StartedAt:    now,
				FinishedAt:   now,
			}, nil
		}
		return s.fingerprintSvc.TestPlan(ctx, plan)
	}
	if s.accountTestSvc == nil {
		return nil, nil
	}
	return s.accountTestSvc.RunTestBackground(ctx, plan.AccountID, plan.ModelID)
}

// applyAutoDisable closes account scheduling after a failed test.
func (s *ScheduledTestRunnerService) applyAutoDisable(ctx context.Context, plan *ScheduledTestPlan, result *ScheduledTestResult) {
	if s.accountScheduler == nil {
		return
	}
	if result.Verdict == FingerprintVerdictUnsupported {
		// Unsupported account types are not the upstream's fault: never disable.
		return
	}
	account, err := s.accountScheduler.GetByID(ctx, plan.AccountID)
	if err != nil || account == nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-disable: load account failed: %v", plan.ID, err)
		return
	}
	if !account.Schedulable {
		return
	}
	marker := map[string]any{
		"plan_id": plan.ID,
		"at":      time.Now().UTC().Format(time.RFC3339),
		"reason":  scheduledTestDisableReason(result),
	}
	if err := s.accountScheduler.UpdateExtra(ctx, account.ID, map[string]any{scheduledTestAutoDisableExtraKey: marker}); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-disable: write marker failed: %v", plan.ID, err)
		return
	}
	if err := s.accountScheduler.SetSchedulable(ctx, account.ID, false); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-disable: set schedulable failed: %v", plan.ID, err)
		return
	}
	result.AccountAction = ScheduledAccountActionDisabled
	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-disable: account=%d scheduling closed (verdict=%s)", plan.ID, account.ID, result.Verdict)
}

// applyAutoRecover clears runtime blocks after a successful test and re-enables
// scheduling when this runner previously closed it.
func (s *ScheduledTestRunnerService) applyAutoRecover(ctx context.Context, plan *ScheduledTestPlan, result *ScheduledTestResult) {
	if s.rateLimitSvc != nil {
		recovery, err := s.rateLimitSvc.RecoverAccountAfterSuccessfulTest(ctx, plan.AccountID)
		if err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover failed: %v", plan.ID, err)
		} else if recovery != nil {
			if recovery.ClearedError {
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d recovered from error status", plan.ID, plan.AccountID)
			}
			if recovery.ClearedRateLimit {
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d cleared rate-limit/runtime state", plan.ID, plan.AccountID)
			}
		}
	}

	if s.accountScheduler == nil {
		return
	}
	account, err := s.accountScheduler.GetByID(ctx, plan.AccountID)
	if err != nil || account == nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: load account failed: %v", plan.ID, err)
		return
	}
	marker, ok := account.Extra[scheduledTestAutoDisableExtraKey]
	if !ok || marker == nil {
		// The account was not disabled by scheduled tests: never fight an
		// administrator's manual scheduling decision.
		return
	}
	if err := s.accountScheduler.SetSchedulable(ctx, account.ID, true); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: set schedulable failed: %v", plan.ID, err)
		return
	}
	if err := s.accountScheduler.UpdateExtra(ctx, account.ID, map[string]any{scheduledTestAutoDisableExtraKey: nil}); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: clear marker failed: %v", plan.ID, err)
	}
	result.AccountAction = ScheduledAccountActionRestored
	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d scheduling reopened", plan.ID, account.ID)
}

// lockAccount returns the per-account mutex.
func (s *ScheduledTestRunnerService) lockAccount(accountID int64) func() {
	value, _ := s.accountLocks.LoadOrStore(accountID, &sync.Mutex{})
	mutex, ok := value.(*sync.Mutex)
	if !ok {
		return func() {}
	}
	mutex.Lock()
	return mutex.Unlock
}

// scheduledTestDisableReason maps a result to the auto-disable marker reason.
func scheduledTestDisableReason(result *ScheduledTestResult) string {
	switch result.Verdict {
	case FingerprintVerdictMismatch:
		return "fingerprint_mismatch"
	case FingerprintVerdictInvalidReply:
		return "invalid_reply"
	case FingerprintVerdictError:
		return "fingerprint_error"
	}
	return "test_failed"
}
