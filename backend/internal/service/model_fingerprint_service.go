package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	// fingerprintDetectorRetryInterval throttles retries after a failed reference
	// bank load so a broken mirror cannot hammer the network on every plan tick.
	fingerprintDetectorRetryInterval = 30 * time.Second
	// fingerprintMaxResponseBytes caps one upstream answer body.
	fingerprintMaxResponseBytes = 8 << 20
)

// fingerprintAnalyzer is the subset of *lmfpd.Detector used by this service.
// Keeping it narrow lets unit tests avoid loading the ~18 MB reference bank.
type fingerprintAnalyzer interface {
	Analyze(outputs []lmfpd.Output, strict bool) (*lmfpd.Analysis, error)
	BuiltAt() string
}

// fingerprintAccountSource is the account lookup used by the fingerprint tester.
type fingerprintAccountSource interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
}

// fingerprintTarget is a resolved upstream endpoint for one detection run.
type fingerprintTarget struct {
	BaseURL string
	APIKey  string
	Model   string
	API     lmfpd.API
}

// fingerprintOutcome is the result of one fingerprint test run.
type fingerprintOutcome struct {
	Verdict           string
	Reason            string
	Summary           string
	ExpectedModel     string
	MatchingMode      string
	MatchedVia        string
	DetectedModel     string
	DetectedModelName string
	DetectedFamily    string
	Probability       *float64
	Detail            map[string]any
}

// ModelFingerprintDetection is the outcome of one fingerprint test for a single
// account model. Scheduled test plans and the batch fingerprint service share it.
type ModelFingerprintDetection struct {
	Status            string
	Verdict           string
	Reason            string
	Summary           string
	ExpectedModel     string
	DetectedModel     string
	DetectedModelName string
	DetectedFamily    string
	Probability       *float64
	LatencyMs         int64
	Detail            map[string]any
}

// fingerprintRunSample is one executed challenge within a detection round.
type fingerprintRunSample struct {
	Index         int
	ChallengeID   string
	ExpectedCount int
	State         string
	Numbers       int
	ResponseModel string
	Text          string
	Error         string
}

// ModelFingerprintService runs lm-detector fingerprint tests for scheduled test
// plans against a single API-key upstream account.
//
// The lm-detector Go SDK is used only for challenge generation, request body
// construction and offline scoring. Requests themselves go through sub2api's
// HTTPUpstream so per-account proxies, TLS fingerprint profiles, header
// overrides and upstream SSRF guards keep applying.
type ModelFingerprintService struct {
	accountRepo         fingerprintAccountSource
	httpUpstream        HTTPUpstream
	tlsFPProfileService *TLSFingerprintProfileService
	cfg                 *config.Config

	detectorMu        sync.Mutex
	detector          fingerprintAnalyzer
	detectorErr       error
	detectorAttemptAt time.Time

	referenceMu     sync.Mutex
	referenceIndex  *fingerprintReferenceIndex
	referenceLoaded bool
}

// NewModelFingerprintService creates a ModelFingerprintService. Any dependency
// may be nil in tests; production wiring provides all of them.
func NewModelFingerprintService(
	accountRepo fingerprintAccountSource,
	httpUpstream HTTPUpstream,
	tlsFPProfileService *TLSFingerprintProfileService,
	cfg *config.Config,
) *ModelFingerprintService {
	return &ModelFingerprintService{
		accountRepo:         accountRepo,
		httpUpstream:        httpUpstream,
		tlsFPProfileService: tlsFPProfileService,
		cfg:                 cfg,
	}
}

// TestPlan runs one fingerprint test for a scheduled plan and converts the
// outcome into a ScheduledTestResult ready to be persisted.
func (s *ModelFingerprintService) TestPlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestResult, error) {
	startedAt := time.Now()
	if plan == nil {
		return nil, errors.New("plan is nil")
	}
	outcome := s.detect(ctx, plan)
	finishedAt := time.Now()

	result := &ScheduledTestResult{
		Status:            fingerprintStatusForVerdict(outcome.Verdict),
		Verdict:           outcome.Verdict,
		ExpectedModel:     outcome.ExpectedModel,
		DetectedModel:     outcome.DetectedModel,
		DetectedModelName: outcome.DetectedModelName,
		DetectedFamily:    outcome.DetectedFamily,
		Probability:       outcome.Probability,
		LatencyMs:         finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
	}
	if outcome.Verdict == FingerprintVerdictMatch {
		result.ResponseText = outcome.Summary
	}
	result.ErrorMessage = outcome.Reason
	if outcome.Detail != nil {
		if raw, err := json.Marshal(outcome.Detail); err == nil {
			result.Detail = raw
		}
	}
	return result, nil
}

// DetectModel runs one fingerprint test for an already loaded account and model.
// It never returns an error: configuration/upstream failures are reported in the
// verdict and reason fields so that batch jobs can persist every model outcome.
func (s *ModelFingerprintService) DetectModel(ctx context.Context, account *Account, modelID string, detectorConfig DetectorConfig) ModelFingerprintDetection {
	startedAt := time.Now()
	outcome := s.detectAccountModel(ctx, account, modelID, detectorConfig)
	return ModelFingerprintDetection{
		Status:            fingerprintStatusForVerdict(outcome.Verdict),
		Verdict:           outcome.Verdict,
		Reason:            outcome.Reason,
		Summary:           outcome.Summary,
		ExpectedModel:     outcome.ExpectedModel,
		DetectedModel:     outcome.DetectedModel,
		DetectedModelName: outcome.DetectedModelName,
		DetectedFamily:    outcome.DetectedFamily,
		Probability:       outcome.Probability,
		LatencyMs:         time.Since(startedAt).Milliseconds(),
		Detail:            outcome.Detail,
	}
}

// detect loads the plan account and runs one fingerprint test for the plan model.
func (s *ModelFingerprintService) detect(ctx context.Context, plan *ScheduledTestPlan) *fingerprintOutcome {
	outcome := &fingerprintOutcome{
		Verdict:       FingerprintVerdictError,
		ExpectedModel: strings.TrimSpace(plan.ModelID),
		Detail:        map[string]any{},
	}
	if s.accountRepo == nil {
		outcome.Verdict = FingerprintVerdictUnsupported
		outcome.Reason = "指纹检测服务未初始化（缺少账号仓库）"
		return outcome
	}
	account, err := s.accountRepo.GetByID(ctx, plan.AccountID)
	if err != nil || account == nil {
		outcome.Verdict = FingerprintVerdictUnsupported
		outcome.Reason = "读取账号失败：" + errorString(err)
		return outcome
	}
	return s.detectAccountModel(ctx, account, plan.ModelID, plan.DetectorConfig)
}

// detectAccountModel executes the configured number of rounds for one account+model
// and aggregates their verdicts.
func (s *ModelFingerprintService) detectAccountModel(ctx context.Context, account *Account, modelID string, detectorConfig DetectorConfig) *fingerprintOutcome {
	outcome := &fingerprintOutcome{
		Verdict:       FingerprintVerdictError,
		ExpectedModel: strings.TrimSpace(modelID),
		Detail:        map[string]any{},
	}

	cfg, err := NormalizeDetectorConfig(detectorConfig)
	if err != nil {
		outcome.Reason = "检测参数无效：" + err.Error()
		return outcome
	}
	outcome.Detail["policy"] = cfg

	if account == nil {
		outcome.Verdict = FingerprintVerdictUnsupported
		outcome.Reason = "账号不存在或已被删除"
		return outcome
	}
	target, unsupportedReason := s.resolveTarget(account, modelID, cfg)
	if unsupportedReason != "" {
		outcome.Verdict = FingerprintVerdictUnsupported
		outcome.Reason = unsupportedReason
		return outcome
	}
	outcome.ExpectedModel = target.Model
	outcome.Detail["target"] = map[string]any{
		"base_url": target.BaseURL,
		"model":    target.Model,
		"api":      string(target.API),
	}

	detector, err := s.getDetector(ctx)
	if err != nil {
		outcome.Verdict = FingerprintVerdictUnsupported
		outcome.Reason = err.Error()
		return outcome
	}

	verdicts := make([]string, 0, cfg.Rounds)
	detections := make([]fingerprintDetection, 0, cfg.Rounds)
	roundDetails := make([]any, 0, cfg.Rounds)
	var lastReason string
	var totalSamples int

	for round := 0; round < cfg.Rounds; round++ {
		if ctx.Err() != nil {
			break
		}
		challenges, challengeErr := lmfpd.GenerateChallenges(cfg.Count)
		if challengeErr != nil {
			verdicts = append(verdicts, FingerprintVerdictError)
			lastReason = challengeErr.Error()
			roundDetails = append(roundDetails, map[string]any{"round": round + 1, "verdict": FingerprintVerdictError, "error": challengeErr.Error()})
			continue
		}
		samples := s.runRound(ctx, account, target, cfg, challenges)
		outputs := make([]lmfpd.Output, 0, len(samples))
		sampleDetails := make([]any, 0, len(samples))
		for _, sample := range samples {
			sampleDetails = append(sampleDetails, map[string]any{
				"index":          sample.Index,
				"state":          sample.State,
				"numbers":        sample.Numbers,
				"expected":       sample.ExpectedCount,
				"response_model": sample.ResponseModel,
				"error":          sample.Error,
			})
			if sample.State == fingerprintSampleComplete && sample.Numbers > 0 {
				outputs = append(outputs, lmfpd.Output{Text: sample.Text, ExpectedCount: sample.ExpectedCount})
			}
		}
		totalSamples += len(samples)

		roundDetail := map[string]any{
			"round":   round + 1,
			"samples": sampleDetails,
		}
		if len(outputs) == 0 {
			verdicts = append(verdicts, FingerprintVerdictError)
			lastReason = "本轮没有任何有效样本"
			roundDetail["verdict"] = FingerprintVerdictError
			roundDetail["error"] = lastReason
			roundDetails = append(roundDetails, roundDetail)
			continue
		}

		analysis, analyzeErr := detector.Analyze(outputs, cfg.Strict)
		if analyzeErr != nil {
			verdicts = append(verdicts, FingerprintVerdictError)
			lastReason = analyzeErr.Error()
			roundDetail["verdict"] = FingerprintVerdictError
			roundDetail["error"] = lastReason
			roundDetails = append(roundDetails, roundDetail)
			continue
		}
		verdict, reason, detection := judgeFingerprintAnalysis(outcome.ExpectedModel, analysis, cfg.AllowFamilyMatch, s.getReferenceIndex())
		verdicts = append(verdicts, verdict)
		detections = append(detections, detection)
		if reason != "" {
			lastReason = reason
		}
		roundDetail["verdict"] = verdict
		roundDetail["analysis"] = fingerprintAnalysisDetail(analysis)
		roundDetails = append(roundDetails, roundDetail)
	}

	outcome.Verdict = aggregateFingerprintVerdicts(verdicts)
	best := bestFingerprintDetection(detections)
	outcome.DetectedModel = best.DetectedModel
	outcome.DetectedModelName = best.DetectedModelName
	outcome.DetectedFamily = best.DetectedFamily
	outcome.Probability = best.Probability
	outcome.Reason = fingerprintFailureReason(outcome.Verdict, outcome.ExpectedModel, best.DetectedModel, lastReason)
	if outcome.Verdict == FingerprintVerdictMatch {
		outcome.Summary = fingerprintMatchSummary(best, cfg.Rounds)
	}
	outcome.Detail["rounds"] = roundDetails
	outcome.Detail["sample_count"] = totalSamples
	outcome.Detail["bank_built_at"] = detector.BuiltAt()
	outcome.MatchedVia = best.MatchedVia
	reference := s.getReferenceIndex()
	outcome.MatchingMode = "legacy"
	if reference != nil {
		outcome.MatchingMode = "alias_index"
	}
	resolved := reference.resolve(outcome.ExpectedModel)
	expectationDetail := map[string]any{
		"expected":       outcome.ExpectedModel,
		"resolved_model": resolved.CanonicalID,
		"matched_via":    outcome.MatchedVia,
		"matching_mode":  outcome.MatchingMode,
	}
	if reference != nil && reference.bankSHA256 != "" {
		expectationDetail["bank_sha256"] = reference.bankSHA256
	}
	outcome.Detail["expectation"] = expectationDetail
	return outcome
}

// runRound executes every challenge of one round with the configured parallelism.
func (s *ModelFingerprintService) runRound(
	ctx context.Context,
	account *Account,
	target fingerprintTarget,
	cfg DetectorConfig,
	challenges []lmfpd.Challenge,
) []fingerprintRunSample {
	samples := make([]fingerprintRunSample, len(challenges))
	parallel := cfg.Parallel
	if parallel < 1 {
		parallel = 1
	}
	if parallel > len(challenges) {
		parallel = len(challenges)
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for index := range challenges {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()
			samples[index] = s.requestSample(ctx, account, target, cfg, index, challenges[index])
		}(index)
	}
	wg.Wait()
	return samples
}

// requestSample sends one challenge through the account upstream and parses its answer.
func (s *ModelFingerprintService) requestSample(
	ctx context.Context,
	account *Account,
	target fingerprintTarget,
	cfg DetectorConfig,
	index int,
	challenge lmfpd.Challenge,
) fingerprintRunSample {
	sample := fingerprintRunSample{
		Index:         index,
		ChallengeID:   challenge.ID,
		ExpectedCount: challenge.ExpectedCount,
		State:         fingerprintSampleFailed,
	}
	if s.httpUpstream == nil {
		sample.Error = "HTTP 上游客户端未初始化"
		return sample
	}

	detectConfig := lmfpd.DetectConfig{
		BaseURL:     target.BaseURL,
		APIKey:      target.APIKey,
		Model:       target.Model,
		API:         target.API,
		Effort:      cfg.Effort,
		ServiceTier: cfg.ServiceTier,
		NoStream:    cfg.NoStream,
	}
	builtRequest, err := lmfpd.BuildRequest(detectConfig, challenge.Prompt)
	if err != nil {
		sample.Error = "构建指纹请求失败：" + err.Error()
		return sample
	}
	body, err := json.Marshal(builtRequest.Body)
	if err != nil {
		sample.Error = "序列化指纹请求失败：" + err.Error()
		return sample
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, builtRequest.URL, bytes.NewReader(body))
	if err != nil {
		sample.Error = "构建 HTTP 请求失败：" + err.Error()
		return sample
	}
	for key, value := range builtRequest.Headers {
		httpRequest.Header.Set(key, value)
	}
	account.ApplyHeaderOverrides(httpRequest.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(httpRequest, proxyURL, account.ID, account.Concurrency, s.resolveTLSProfile(account))
	if err != nil {
		sample.Error = "上游请求失败：" + err.Error()
		return sample
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, fingerprintMaxResponseBytes))
	if readErr != nil {
		sample.Error = "读取上游响应失败：" + readErr.Error()
		return sample
	}
	parsed := parseFingerprintResponse(string(target.API), cfg.NoStream, resp.StatusCode, responseBody)
	sample.State = parsed.State
	sample.Text = parsed.Text
	sample.Numbers = parsed.Numbers
	sample.ResponseModel = parsed.ResponseModel
	sample.Error = parsed.Error
	return sample
}

// resolveTarget decides which upstream and protocol one account uses. It
// returns a non-empty reason when the account type is not supported.
func (s *ModelFingerprintService) resolveTarget(account *Account, expectedModel string, cfg DetectorConfig) (fingerprintTarget, string) {
	if account == nil {
		return fingerprintTarget{}, "账号不存在或已被删除"
	}
	if account.IsSyntheticUITest() {
		return fingerprintTarget{}, "合成 UI 测试账号不支持指纹检测"
	}
	if account.IsBedrock() {
		return fingerprintTarget{}, "Bedrock 账号使用 AWS 签名，暂不支持指纹检测"
	}
	if account.IsGemini() {
		return fingerprintTarget{}, "Gemini 账号暂不支持指纹检测（v1 仅支持 OpenAI / Anthropic 兼容 API Key 上游）"
	}
	if account.IsGrok() {
		return fingerprintTarget{}, "Grok 账号暂不支持指纹检测"
	}
	if account.Type != AccountTypeAPIKey {
		return fingerprintTarget{}, "仅支持 API Key 类型账号做指纹检测（当前类型：" + account.Type + "）"
	}

	model := strings.TrimSpace(account.GetMappedModel(expectedModel))
	if model == "" {
		model = strings.TrimSpace(expectedModel)
	}
	if model == "" {
		return fingerprintTarget{}, "计划未配置模型名"
	}

	var api lmfpd.API
	baseURL, apiKey := "", ""
	switch {
	case account.Platform == PlatformAnthropic:
		baseURL = strings.TrimSpace(account.GetBaseURL())
		apiKey = strings.TrimSpace(account.GetCredential("api_key"))
		api = lmfpd.APIMessages
	case account.Platform == PlatformOpenAI:
		baseURL = strings.TrimSpace(account.GetOpenAIBaseURL())
		apiKey = strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
		api = lmfpd.APIResponses
	case IsMultiProtocolAPIKeyProvider(account.Platform):
		apiKey = strings.TrimSpace(account.GetCNAPIKey())
		switch account.GetAPIProtocol() {
		case APIProtocolAnthropic:
			baseURL = strings.TrimSpace(account.GetAnthropicProtocolBaseURL())
			api = lmfpd.APIMessages
		case APIProtocolResponses, APIProtocolAdaptive:
			baseURL = strings.TrimSpace(account.GetCNProtocolBaseURL(APIProtocolResponses))
			if baseURL == "" {
				baseURL = strings.TrimSpace(account.GetOpenAIBaseURL())
			}
			api = lmfpd.APIResponses
		default:
			baseURL = strings.TrimSpace(account.GetOpenAIBaseURL())
			api = lmfpd.APIChatCompletions
		}
	default:
		return fingerprintTarget{}, "账号平台暂不支持指纹检测（platform=" + account.Platform + "）"
	}
	if apiKey == "" {
		return fingerprintTarget{}, "账号缺少可用的 API Key 凭据"
	}
	if baseURL == "" {
		return fingerprintTarget{}, "账号缺少上游 Base URL，无法发起指纹请求"
	}

	switch strings.ToLower(strings.TrimSpace(cfg.API)) {
	case DetectorAPIResponses:
		api = lmfpd.APIResponses
	case DetectorAPIChatCompletions:
		api = lmfpd.APIChatCompletions
	case DetectorAPIMessages:
		api = lmfpd.APIMessages
	}
	return fingerprintTarget{BaseURL: baseURL, APIKey: apiKey, Model: model, API: api}, ""
}

// getDetector lazily loads and caches the lm-detector reference bank.
func (s *ModelFingerprintService) getDetector(ctx context.Context) (fingerprintAnalyzer, error) {
	s.detectorMu.Lock()
	defer s.detectorMu.Unlock()

	if s.detector != nil {
		return s.detector, nil
	}
	if s.detectorErr != nil && time.Since(s.detectorAttemptAt) < fingerprintDetectorRetryInterval {
		return nil, s.detectorErr
	}
	fingerprintConfig := s.fingerprintConfig()
	if !fingerprintConfig.Enabled {
		return nil, errors.New("指纹检测已被服务端配置关闭（FINGERPRINT_ENABLED=false）")
	}
	s.detectorAttemptAt = time.Now()

	options := lmfpd.LoadOptions{}
	if dataDir := strings.TrimSpace(fingerprintConfig.DataDir); dataDir != "" {
		options.Sources = []string{dataDir}
	} else if sources := splitFingerprintSources(fingerprintConfig.DataSources); len(sources) > 0 {
		options.Sources = sources
	}
	if cacheFile := strings.TrimSpace(fingerprintConfig.CacheFile); cacheFile != "" {
		options.Cache = cacheFile
	}

	detector, err := lmfpd.Load(ctx, options)
	if err != nil {
		s.detectorErr = fmt.Errorf("加载 lm-detector 参考库失败: %w", err)
		return nil, s.detectorErr
	}
	s.detector = detector
	s.detectorErr = nil
	return detector, nil
}

// fingerprintConfig returns the fingerprint section of the server config.
func (s *ModelFingerprintService) fingerprintConfig() config.FingerprintConfig {
	if s.cfg == nil {
		return config.FingerprintConfig{Enabled: true}
	}
	return s.cfg.Fingerprint
}

// getReferenceIndex lazily loads the unified-bank alias index used to map
// upstream model ids/aliases to reference-bank labels. It returns nil when the
// data directory is not configured or the bank cannot be read; callers then fall
// back to exact label comparison.
func (s *ModelFingerprintService) getReferenceIndex() *fingerprintReferenceIndex {
	if s == nil {
		return nil
	}
	s.referenceMu.Lock()
	defer s.referenceMu.Unlock()
	if s.referenceLoaded {
		return s.referenceIndex
	}
	s.referenceLoaded = true
	s.referenceIndex = loadFingerprintReferenceIndex(s.fingerprintConfig().DataDir)
	return s.referenceIndex
}

// resolveTLSProfile resolves the optional TLS fingerprint profile of an account.
func (s *ModelFingerprintService) resolveTLSProfile(account *Account) *tlsfingerprint.Profile {
	if s.tlsFPProfileService == nil {
		return nil
	}
	return s.tlsFPProfileService.ResolveTLSProfile(account)
}

// fingerprintAnalysisDetail renders a compact analysis summary for detail_json.
func fingerprintAnalysisDetail(analysis *lmfpd.Analysis) map[string]any {
	if analysis == nil {
		return nil
	}
	results := make([]any, 0, len(analysis.Results))
	for index, candidate := range analysis.Results {
		if index >= 5 {
			break
		}
		results = append(results, map[string]any{
			"model":        candidate.Model,
			"display_name": candidate.DisplayName,
			"family":       candidate.Family,
			"score":        candidate.Score,
			"probability":  candidate.Probability,
		})
	}
	detail := map[string]any{
		"decision":           analysis.Decision,
		"prediction":         analysis.Prediction,
		"prediction_name":    analysis.PredictionName,
		"family_prediction":  analysis.FamilyPrediction,
		"ranking_score":      analysis.RankingScore,
		"probability":        analysis.Probability,
		"probability_status": analysis.ProbabilityStatus,
		"used_outputs":       analysis.UsedOutputs,
		"method":             analysis.Method,
		"evidence": map[string]any{
			"label":  analysis.Evidence.Label,
			"reason": analysis.Evidence.Reason,
		},
		"results": results,
	}
	if analysis.Sequence != nil {
		detail["sequence"] = map[string]any{
			"autocorrelation":  analysis.Sequence.Autocorrelation,
			"small_step_share": analysis.Sequence.SmallStepShare,
			"model_like":       analysis.Sequence.ModelLike,
		}
	}
	return detail
}

// fingerprintStatusForVerdict maps a fingerprint verdict to the legacy status field.
func fingerprintStatusForVerdict(verdict string) string {
	if verdict == FingerprintVerdictMatch {
		return "success"
	}
	return "failed"
}

// fingerprintMatchSummary renders a short human readable match line.
func fingerprintMatchSummary(detection fingerprintDetection, rounds int) string {
	model := detection.DetectedModel
	if model == "" {
		model = "unknown"
	}
	summary := "模型指纹一致：" + model
	if detection.Probability != nil {
		summary += fmt.Sprintf("（置信度 %.0f%%）", *detection.Probability*100)
	}
	if rounds > 1 {
		summary += fmt.Sprintf("，%d 轮全部一致", rounds)
	}
	return summary
}

// splitFingerprintSources splits a comma separated mirror list.
func splitFingerprintSources(raw string) []string {
	parts := strings.Split(raw, ",")
	sources := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			sources = append(sources, trimmed)
		}
	}
	return sources
}

// errorString renders an error for user-facing messages.
func errorString(err error) string {
	if err == nil {
		return "未知错误"
	}
	return err.Error()
}
