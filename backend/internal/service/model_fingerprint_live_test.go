package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

// TestModelFingerprintLiveDetector exercises the real lm-detector reference bank
// end to end: recorded conformance outputs are replayed through a fake upstream
// and scored by the real detector (no fake analyzer involved).
//
// It only runs when the reference bank and the conformance fixture are supplied:
//
//	FINGERPRINT_LIVE_DATA_DIR=<lm-detector/data> \
//	FINGERPRINT_LIVE_ANALYZE_FIXTURE=<lm-detector/sdk/conformance/analyze.json> \
//	go test ./internal/service/ -run TestModelFingerprintLiveDetector -v
func TestModelFingerprintLiveDetector(t *testing.T) {
	dataDir := strings.TrimSpace(os.Getenv("FINGERPRINT_LIVE_DATA_DIR"))
	fixturePath := strings.TrimSpace(os.Getenv("FINGERPRINT_LIVE_ANALYZE_FIXTURE"))
	if dataDir == "" || fixturePath == "" {
		t.Skip("FINGERPRINT_LIVE_DATA_DIR / FINGERPRINT_LIVE_ANALYZE_FIXTURE not set")
	}

	rawFixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Cases []struct {
			Name    string `json:"name"`
			Outputs []struct {
				Text          string `json:"text"`
				ExpectedCount int    `json:"expected_count"`
			} `json:"outputs"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(rawFixture, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	var scoredCase *struct {
		Name    string `json:"name"`
		Outputs []struct {
			Text          string `json:"text"`
			ExpectedCount int    `json:"expected_count"`
		} `json:"outputs"`
	}
	for index := range fixture.Cases {
		if fixture.Cases[index].Name == "scored-claude-sonnet-4.6" {
			scoredCase = &fixture.Cases[index]
			break
		}
	}
	if scoredCase == nil {
		t.Fatal("fixture case scored-claude-sonnet-4.6 not found")
	}

	detector, err := lmfpd.Load(context.Background(), lmfpd.LoadOptions{Sources: []string{dataDir}})
	if err != nil {
		t.Fatalf("load reference bank: %v", err)
	}
	t.Logf("reference bank built_at=%s", detector.BuiltAt())

	bodies := make([][]byte, 0, len(scoredCase.Outputs))
	for _, output := range scoredCase.Outputs {
		bodies = append(bodies, messagesSSEBody("claude-sonnet-4.6", output.Text))
	}
	upstream := &fakeFingerprintUpstream{bodies: bodies}
	svc := NewModelFingerprintService(&fakeFingerprintAccountSource{account: anthropicAPIKeyAccount()}, upstream, nil, nil)
	svc.detector = detector

	plan := &ScheduledTestPlan{
		ID:             900,
		AccountID:      11,
		ModelID:        "claude-sonnet-4-6",
		TestMode:       ScheduledTestModeFingerprint,
		DetectorConfig: DetectorConfig{Count: 3, Parallel: 3, TimeoutSec: 30, Rounds: 1, ServiceTier: "default"},
	}
	result, err := svc.TestPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("TestPlan: %v", err)
	}
	if result.Verdict != FingerprintVerdictMatch {
		t.Fatalf("expected match, verdict=%s detected=%s error=%s", result.Verdict, result.DetectedModel, result.ErrorMessage)
	}
	if result.DetectedModel != "claude-sonnet-4.6" {
		t.Fatalf("detected model = %q, want claude-sonnet-4.6", result.DetectedModel)
	}
	if result.Probability == nil || *result.Probability < 0.5 {
		t.Fatalf("unexpected probability: %v", result.Probability)
	}
	t.Logf("live detection: status=%s verdict=%s detected=%s probability=%.4f latency=%dms", result.Status, result.Verdict, result.DetectedModel, *result.Probability, result.LatencyMs)

	// A different plan model must be reported as a mismatch by the real detector.
	mismatchPlan := *plan
	mismatchPlan.ModelID = "gpt-5.1"
	upstream2 := &fakeFingerprintUpstream{bodies: bodies}
	svc2 := NewModelFingerprintService(&fakeFingerprintAccountSource{account: anthropicAPIKeyAccount()}, upstream2, nil, nil)
	svc2.detector = detector
	mismatchResult, err := svc2.TestPlan(context.Background(), &mismatchPlan)
	if err != nil {
		t.Fatalf("TestPlan mismatch: %v", err)
	}
	if mismatchResult.Verdict != FingerprintVerdictMismatch {
		t.Fatalf("expected mismatch, got %s (detected=%s)", mismatchResult.Verdict, mismatchResult.DetectedModel)
	}
}

// messagesSSEBody wraps recorded answer text into an Anthropic Messages SSE body.
func messagesSSEBody(model, text string) []byte {
	encoded, err := json.Marshal(map[string]any{"type": "content_block_delta", "delta": map[string]any{"text": text}})
	if err != nil {
		return nil
	}
	head, err := json.Marshal(map[string]any{"type": "message_start", "message": map[string]any{"model": model}})
	if err != nil {
		return nil
	}
	return []byte(strings.Join([]string{
		"data: " + string(head),
		"",
		"data: " + string(encoded),
		"",
		"data: {\"type\":\"message_stop\"}",
		"",
	}, "\n"))
}
