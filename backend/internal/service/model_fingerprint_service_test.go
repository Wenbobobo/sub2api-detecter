package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type fakeFingerprintAccountSource struct {
	account *Account
	err     error
}

func (f *fakeFingerprintAccountSource) GetByID(ctx context.Context, id int64) (*Account, error) {
	return f.account, f.err
}

type fakeFingerprintAnalyzer struct {
	analysis *lmfpd.Analysis
	err      error
	builtAt  string
	calls    int
}

func (f *fakeFingerprintAnalyzer) Analyze(outputs []lmfpd.Output, strict bool) (*lmfpd.Analysis, error) {
	f.calls++
	return f.analysis, f.err
}

func (f *fakeFingerprintAnalyzer) BuiltAt() string { return f.builtAt }

type fakeFingerprintUpstream struct {
	mu       sync.Mutex
	statuses []int
	bodies   [][]byte
	errs     []error
	requests []*http.Request
}

func (f *fakeFingerprintUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return f.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (f *fakeFingerprintUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	f.mu.Lock()
	index := len(f.requests)
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if index < len(f.errs) && f.errs[index] != nil {
		return nil, f.errs[index]
	}
	status := http.StatusOK
	if index < len(f.statuses) && f.statuses[index] != 0 {
		status = f.statuses[index]
	}
	var body []byte
	if index < len(f.bodies) {
		body = f.bodies[index]
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

// fingerprintStreamBody returns an Anthropic Messages SSE body (the test
// account is an Anthropic API-key relay).
func fingerprintStreamBody(model string) []byte {
	return []byte(strings.Join([]string{
		"data: {\"type\":\"message_start\",\"message\":{\"model\":\"" + model + "\"}}",
		"",
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"12 45 78 91 133 155 200 244 301 \"}}",
		"",
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"17 29 33 48 59 61 75 82 99\"}}",
		"",
		"data: {\"type\":\"message_stop\"}",
		"",
		"data: [DONE]",
		"",
	}, "\n"))
}

func scoredAnalysis(prediction, family string, probability float64) *lmfpd.Analysis {
	p := probability
	return &lmfpd.Analysis{
		Decision:          "scored",
		Prediction:        prediction,
		PredictionName:    prediction,
		FamilyPrediction:  family,
		Probability:       &p,
		ProbabilityStatus: "reference_calibrated",
		Results:           []lmfpd.AnalysisResult{{Model: prediction, Family: family, Score: 0.8, Probability: &p}},
	}
}

func anthropicAPIKeyAccount() *Account {
	return &Account{
		ID:          11,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-ant-test", "base_url": "https://relay.example.com/v1"},
		Schedulable: true,
	}
}

func TestResolveFingerprintTarget(t *testing.T) {
	svc := NewModelFingerprintService(nil, nil, nil, nil)
	cfg := DetectorConfig{API: DetectorAPIAuto}

	target, reason := svc.resolveTarget(anthropicAPIKeyAccount(), "claude-sonnet-4-5", cfg)
	if reason != "" {
		t.Fatalf("anthropic target unsupported: %s", reason)
	}
	if target.API != lmfpd.APIMessages || target.BaseURL != "https://relay.example.com/v1" || target.APIKey != "sk-ant-test" {
		t.Fatalf("unexpected target: %+v", target)
	}

	openaiAccount := &Account{
		ID:          12,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-openai", "base_url": "https://openai-relay.example.com/v1"},
	}
	target, reason = svc.resolveTarget(openaiAccount, "gpt-5.1", cfg)
	if reason != "" {
		t.Fatalf("openai target unsupported: %s", reason)
	}
	if target.API != lmfpd.APIResponses || target.BaseURL != "https://openai-relay.example.com/v1" {
		t.Fatalf("unexpected openai target: %+v", target)
	}

	chatCfg := DetectorConfig{API: DetectorAPIChatCompletions}
	target, reason = svc.resolveTarget(openaiAccount, "gpt-5.1", chatCfg)
	if reason != "" || target.API != lmfpd.APIChatCompletions {
		t.Fatalf("chat override failed: %+v (%s)", target, reason)
	}

	oauthAccount := &Account{ID: 13, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	if _, reason = svc.resolveTarget(oauthAccount, "claude-sonnet-4-5", cfg); !strings.Contains(reason, "API Key") {
		t.Fatalf("oauth account must be unsupported, got %q", reason)
	}

	geminiAccount := &Account{ID: 14, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	if _, reason = svc.resolveTarget(geminiAccount, "gemini-2.5-pro", cfg); !strings.Contains(reason, "Gemini") {
		t.Fatalf("gemini account must be unsupported, got %q", reason)
	}
}

func newFingerprintTestService(account *Account, upstream *fakeFingerprintUpstream, analyzer *fakeFingerprintAnalyzer) *ModelFingerprintService {
	svc := NewModelFingerprintService(&fakeFingerprintAccountSource{account: account}, upstream, nil, nil)
	svc.detector = analyzer
	return svc
}

func TestModelFingerprintServiceTestPlanMatch(t *testing.T) {
	upstream := &fakeFingerprintUpstream{
		bodies: [][]byte{
			fingerprintStreamBody("claude-sonnet-4.6"),
			fingerprintStreamBody("claude-sonnet-4.6"),
			fingerprintStreamBody("claude-sonnet-4.6"),
		},
	}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.93), builtAt: "2026-10-27"}
	svc := newFingerprintTestService(anthropicAPIKeyAccount(), upstream, analyzer)
	plan := &ScheduledTestPlan{ID: 1, AccountID: 11, ModelID: "claude-sonnet-4-5", TestMode: ScheduledTestModeFingerprint}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if result.Status != "success" || result.Verdict != FingerprintVerdictMatch {
		t.Fatalf("unexpected result: status=%s verdict=%s error=%s", result.Status, result.Verdict, result.ErrorMessage)
	}
	if result.ExpectedModel != "claude-sonnet-4-5" || result.DetectedModel != "claude-sonnet-4.5" {
		t.Fatalf("model fields: expected=%q detected=%q", result.ExpectedModel, result.DetectedModel)
	}
	if result.Probability == nil || *result.Probability != 0.93 {
		t.Fatalf("probability = %v", result.Probability)
	}
	if len(upstream.requests) != 3 {
		t.Fatalf("expected 3 upstream requests, got %d", len(upstream.requests))
	}
	if !strings.Contains(string(result.Detail), "bank_built_at") {
		t.Fatalf("detail missing bank_built_at: %s", string(result.Detail))
	}
	if result.ErrorMessage != "" {
		t.Fatalf("match must not carry an error message: %q", result.ErrorMessage)
	}
	if !strings.Contains(result.ResponseText, "一致") {
		t.Fatalf("match summary = %q", result.ResponseText)
	}
}

func TestModelFingerprintServiceTestPlanMismatch(t *testing.T) {
	upstream := &fakeFingerprintUpstream{bodies: [][]byte{
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
		fingerprintStreamBody("gpt-5.6-sol"),
	}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("gpt-5.6-sol", "gpt", 0.88), builtAt: "2026-10-27"}
	svc := newFingerprintTestService(anthropicAPIKeyAccount(), upstream, analyzer)
	plan := &ScheduledTestPlan{ID: 2, AccountID: 11, ModelID: "claude-sonnet-4-5", TestMode: ScheduledTestModeFingerprint}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if result.Status != "failed" || result.Verdict != FingerprintVerdictMismatch {
		t.Fatalf("unexpected verdict: status=%s verdict=%s", result.Status, result.Verdict)
	}
	if !strings.Contains(result.ErrorMessage, "不一致") || !strings.Contains(result.ErrorMessage, "gpt-5.6-sol") {
		t.Fatalf("error message = %q", result.ErrorMessage)
	}
	if result.DetectedModel != "gpt-5.6-sol" {
		t.Fatalf("detected model = %q", result.DetectedModel)
	}
}

func TestModelFingerprintServiceTestPlanUpstreamFailure(t *testing.T) {
	upstream := &fakeFingerprintUpstream{
		statuses: []int{500, 500, 500},
		bodies:   [][]byte{[]byte("boom"), []byte("boom"), []byte("boom")},
	}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.9)}
	svc := newFingerprintTestService(anthropicAPIKeyAccount(), upstream, analyzer)
	plan := &ScheduledTestPlan{ID: 3, AccountID: 11, ModelID: "claude-sonnet-4-5", TestMode: ScheduledTestModeFingerprint}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if result.Verdict != FingerprintVerdictError || result.Status != "failed" {
		t.Fatalf("unexpected verdict: %+v", result)
	}
	if analyzer.calls != 0 {
		t.Fatalf("analyzer must not score empty samples, calls=%d", analyzer.calls)
	}
	if !strings.Contains(result.ErrorMessage, "指纹") {
		t.Fatalf("error message = %q", result.ErrorMessage)
	}
}

func TestModelFingerprintServiceTestPlanUnsupportedAccount(t *testing.T) {
	upstream := &fakeFingerprintUpstream{}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.9)}
	oauth := &Account{ID: 21, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc := newFingerprintTestService(oauth, upstream, analyzer)
	plan := &ScheduledTestPlan{ID: 4, AccountID: 21, ModelID: "claude-sonnet-4-5", TestMode: ScheduledTestModeFingerprint}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if result.Verdict != FingerprintVerdictUnsupported || result.Status != "failed" {
		t.Fatalf("unexpected verdict: %+v", result)
	}
	if len(upstream.requests) != 0 {
		t.Fatalf("unsupported account must not call upstream, calls=%d", len(upstream.requests))
	}
}

func TestModelFingerprintServiceRounds(t *testing.T) {
	upstream := &fakeFingerprintUpstream{
		bodies: [][]byte{
			fingerprintStreamBody("claude-sonnet-4.6"),
			fingerprintStreamBody("claude-sonnet-4.6"),
		},
	}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.9), builtAt: "test"}
	svc := newFingerprintTestService(anthropicAPIKeyAccount(), upstream, analyzer)
	plan := &ScheduledTestPlan{
		ID:             5,
		AccountID:      11,
		ModelID:        "claude-sonnet-4-5",
		TestMode:       ScheduledTestModeFingerprint,
		DetectorConfig: DetectorConfig{Count: 1, Rounds: 2, Parallel: 1, TimeoutSec: 30, ServiceTier: "default"},
	}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if len(upstream.requests) != 2 || analyzer.calls != 2 {
		t.Fatalf("rounds: requests=%d analyze=%d", len(upstream.requests), analyzer.calls)
	}
	if result.Verdict != FingerprintVerdictMatch {
		t.Fatalf("verdict = %s (%s)", result.Verdict, result.ErrorMessage)
	}
	if !strings.Contains(string(result.Detail), "rounds") {
		t.Fatalf("detail missing rounds: %s", string(result.Detail))
	}
}

func TestModelFingerprintServiceTransportError(t *testing.T) {
	upstream := &fakeFingerprintUpstream{errs: []error{errors.New("dial tcp timeout"), errors.New("dial tcp timeout")}}
	analyzer := &fakeFingerprintAnalyzer{analysis: scoredAnalysis("claude-sonnet-4.5", "claude", 0.9)}
	svc := newFingerprintTestService(anthropicAPIKeyAccount(), upstream, analyzer)
	plan := &ScheduledTestPlan{ID: 6, AccountID: 11, ModelID: "claude-sonnet-4-5", TestMode: ScheduledTestModeFingerprint,
		DetectorConfig: DetectorConfig{Count: 2, Parallel: 1, TimeoutSec: 30, ServiceTier: "default"}}

	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan error: %v", err)
	}
	if result.Verdict != FingerprintVerdictError {
		t.Fatalf("verdict = %s", result.Verdict)
	}
	if !strings.Contains(string(result.Detail), "dial tcp timeout") {
		t.Fatalf("detail should record transport error: %s", string(result.Detail))
	}
}

func TestModelFingerprintServiceConfigDisabled(t *testing.T) {
	svc := NewModelFingerprintService(nil, nil, nil, &config.Config{Fingerprint: config.FingerprintConfig{Enabled: false}})
	if _, err := svc.getDetector(context.Background()); err == nil || !strings.Contains(err.Error(), "关闭") {
		t.Fatalf("expected disabled error, got %v", err)
	}
}
