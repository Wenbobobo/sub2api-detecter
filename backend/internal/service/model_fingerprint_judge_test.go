package service

import (
	"strings"
	"testing"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

func TestNormalizeModelLabel(t *testing.T) {
	cases := map[string]string{
		"claude-sonnet-4-5-20250929":    "claude-sonnet-4-5",
		"claude-opus-4.5":               "claude-opus-4-5",
		" anthropic/Claude-Sonnet-4-5 ": "claude-sonnet-4-5",
		"gpt-5.1-2025-08-07":            "gpt-5-1",
		"GLM-5.3-FlashX":                "glm-5-3-flashx",
		"qwen3.8-max-0902":              "qwen3-8-max-0902",
		"":                              "",
	}
	for raw, want := range cases {
		if got := normalizeModelLabel(raw); got != want {
			t.Fatalf("normalizeModelLabel(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestModelLabelsEquivalent(t *testing.T) {
	if !modelLabelsEquivalent("claude-haiku-4-5-20251001", "claude-haiku-4.5") {
		t.Fatal("dated API name should equal dotted reference label")
	}
	if !modelLabelsEquivalent("gpt-5.1", "gpt-5-1") {
		t.Fatal("dotted and dashed gpt versions should be equivalent")
	}
	if modelLabelsEquivalent("claude-sonnet-4-5", "claude-opus-4-5") {
		t.Fatal("different model tiers must not be equivalent")
	}
	if modelLabelsEquivalent("", "gpt-5") {
		t.Fatal("empty label must not match")
	}
}

func TestExpectedModelFamily(t *testing.T) {
	cases := map[string]string{
		"claude-sonnet-4-5":  "claude",
		"gpt-5.1":            "gpt",
		"o3-mini":            "gpt",
		"gemini-2.5-pro":     "gemini",
		"deepseek-v4-pro":    "deepseek",
		"kimi-k2.8-preview":  "kimi",
		"glm-5.3":            "glm",
		"qwen3.8-max":        "qwen",
		"made-up-model-1234": "",
	}
	for raw, want := range cases {
		if got := expectedModelFamily(raw); got != want {
			t.Fatalf("expectedModelFamily(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestJudgeFingerprintAnalysis(t *testing.T) {
	scored := func(prediction, family string, probability float64) *lmfpd.Analysis {
		p := probability
		return &lmfpd.Analysis{
			Decision:          "scored",
			Prediction:        prediction,
			PredictionName:    prediction,
			FamilyPrediction:  family,
			Probability:       &p,
			ProbabilityStatus: "reference_calibrated",
			Results: []lmfpd.AnalysisResult{
				{Model: prediction, Family: family, Score: 0.91, Probability: &p},
			},
		}
	}

	verdict, reason, detection := judgeFingerprintAnalysis("claude-sonnet-4-5", scored("claude-sonnet-4.5", "claude", 0.93), false, nil)
	if verdict != FingerprintVerdictMatch {
		t.Fatalf("expected match, got %s (%s)", verdict, reason)
	}
	if detection.DetectedModel != "claude-sonnet-4.5" || detection.Probability == nil || *detection.Probability != 0.93 {
		t.Fatalf("unexpected detection: %+v", detection)
	}

	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", scored("gpt-5.1", "gpt", 0.8), false, nil)
	if verdict != FingerprintVerdictMismatch {
		t.Fatalf("expected mismatch, got %s", verdict)
	}

	// Family match is opt-in.
	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", scored("claude-opus-4.6", "claude", 0.7), true, nil)
	if verdict != FingerprintVerdictMatch {
		t.Fatalf("expected family match, got %s", verdict)
	}
	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", scored("claude-opus-4.6", "claude", 0.7), false, nil)
	if verdict != FingerprintVerdictMismatch {
		t.Fatalf("expected strict mismatch, got %s", verdict)
	}

	notModel := &lmfpd.Analysis{Decision: "not_model", Evidence: lmfpd.Evidence{Reason: "sequence looks uniform"}}
	verdict, reason, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", notModel, false, nil)
	if verdict != FingerprintVerdictInvalidReply || !strings.Contains(reason, "uniform") {
		t.Fatalf("expected invalid_reply, got %s (%s)", verdict, reason)
	}

	unscorable := &lmfpd.Analysis{Decision: "unscorable", Evidence: lmfpd.Evidence{Reason: "not enough numbers"}}
	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", unscorable, false, nil)
	if verdict != FingerprintVerdictError {
		t.Fatalf("expected error for unscorable, got %s", verdict)
	}

	noPrediction := &lmfpd.Analysis{Decision: "scored"}
	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", noPrediction, false, nil)
	if verdict != FingerprintVerdictError {
		t.Fatalf("expected error without prediction, got %s", verdict)
	}

	verdict, _, _ = judgeFingerprintAnalysis("claude-sonnet-4-5", nil, false, nil)
	if verdict != FingerprintVerdictError {
		t.Fatalf("expected error for nil analysis, got %s", verdict)
	}
}

func TestAggregateFingerprintVerdicts(t *testing.T) {
	cases := []struct {
		name     string
		verdicts []string
		want     string
	}{
		{"all match", []string{FingerprintVerdictMatch, FingerprintVerdictMatch}, FingerprintVerdictMatch},
		{"any mismatch", []string{FingerprintVerdictMatch, FingerprintVerdictMismatch}, FingerprintVerdictMismatch},
		{"match plus error", []string{FingerprintVerdictMatch, FingerprintVerdictError}, FingerprintVerdictError},
		{"invalid beats error", []string{FingerprintVerdictInvalidReply, FingerprintVerdictError}, FingerprintVerdictInvalidReply},
		{"empty", nil, FingerprintVerdictError},
	}
	for _, tc := range cases {
		if got := aggregateFingerprintVerdicts(tc.verdicts); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestFingerprintFailureReason(t *testing.T) {
	if got := fingerprintFailureReason(FingerprintVerdictMatch, "a", "a", ""); got != "" {
		t.Fatalf("match must have no error, got %q", got)
	}
	if got := fingerprintFailureReason(FingerprintVerdictUnsupported, "a", "", "账号不支持"); got != "账号不支持" {
		t.Fatalf("unsupported reason = %q", got)
	}
	if got := fingerprintFailureReason(FingerprintVerdictMismatch, "a", "b", "期望 a，检测为 b"); !strings.Contains(got, "不一致") {
		t.Fatalf("mismatch reason = %q", got)
	}
	if got := fingerprintFailureReason(FingerprintVerdictError, "a", "", "超时"); !strings.Contains(got, "超时") {
		t.Fatalf("error reason = %q", got)
	}
}
