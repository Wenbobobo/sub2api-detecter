package service

import (
	"os"
	"path/filepath"
	"testing"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

func aliasTestIndex() *fingerprintReferenceIndex {
	return newFingerprintReferenceIndexFromEntries([]fingerprintReferenceModel{
		{ID: "gpt-6-sol", DisplayName: "gpt-6-sol", Family: "gpt", FamilyName: "GPT"},
		{ID: "gpt-6-astra", DisplayName: "gpt-6-astra/6.1-sol", Family: "gpt", FamilyName: "GPT"},
		{ID: "gpt-5.6-sol", DisplayName: "gpt-5.6-sol", Family: "gpt", FamilyName: "GPT"},
		{ID: "claude-sonnet-4.6", DisplayName: "claude-sonnet-4.6", Family: "claude", FamilyName: "Claude"},
	})
}

func aliasScored(model, name, family string) *lmfpd.Analysis {
	return &lmfpd.Analysis{
		Decision:         "scored",
		Prediction:       model,
		PredictionName:   name,
		FamilyPrediction: family,
	}
}

func TestFingerprintNormalizeDigitLetterBoundary(t *testing.T) {
	cases := map[string]string{
		"6.1sol":                    "6-1-sol",
		"gpt-6.1-sol":               "gpt-6-1-sol",
		"gpt-6-sol":                 "gpt-6-sol",
		"gpt-5.6-sol":               "gpt-5-6-sol",
		"claude-opus-4.5":           "claude-opus-4-5",
		"claude-haiku-4-5-20251001": "claude-haiku-4-5",
	}
	for raw, want := range cases {
		if got := normalizeModelLabel(raw); got != want {
			t.Fatalf("normalizeModelLabel(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFingerprintReferenceIndexResolveAlias(t *testing.T) {
	index := aliasTestIndex()
	candidate := index.resolve("6.1sol")
	if candidate.Ambiguous {
		t.Fatalf("unexpected ambiguity: %v", candidate.Ambiguity)
	}
	if candidate.CanonicalID != "gpt-6-astra" {
		t.Fatalf("canonical = %q, want gpt-6-astra", candidate.CanonicalID)
	}
	if candidate.Family != "gpt" {
		t.Fatalf("family = %q", candidate.Family)
	}
	if got := index.resolve("gpt-6-sol").CanonicalID; got != "gpt-6-sol" {
		t.Fatalf("gpt-6-sol canonical = %q", got)
	}
	if got := index.resolve("unknown-model").CanonicalID; got != "" {
		t.Fatalf("unknown canonical = %q, want empty", got)
	}
}

func TestFingerprintReferenceIndexAmbiguousAlias(t *testing.T) {
	index := newFingerprintReferenceIndexFromEntries([]fingerprintReferenceModel{
		{ID: "gpt-6-astra", DisplayName: "gpt-6-astra/6.1-sol", Family: "gpt"},
		{ID: "gpt-6-nova", DisplayName: "gpt-6-nova/6.1-sol", Family: "gpt"},
	})
	candidate := index.resolve("6.1sol")
	if !candidate.Ambiguous {
		t.Fatalf("expected ambiguity, got %+v", candidate)
	}
	if len(candidate.Ambiguity) != 2 {
		t.Fatalf("ambiguity candidates = %v", candidate.Ambiguity)
	}
	if matched, _, ambiguity := index.matches("6.1sol", fingerprintDetection{DetectedModel: "gpt-6-astra", DetectedModelName: "gpt-6-astra/6.1-sol"}); matched || ambiguity == "" {
		t.Fatalf("ambiguous expectation must not match (matched=%v ambiguity=%q)", matched, ambiguity)
	}
}

func TestFingerprintReferenceIndexMatchesAliases(t *testing.T) {
	index := aliasTestIndex()
	cases := []struct {
		name     string
		expected string
		det      fingerprintDetection
		want     bool
		via      string
	}{
		{
			name:     "6.1sol maps to gpt-6-astra display alias",
			expected: "6.1sol",
			det:      fingerprintDetection{DetectedModel: "gpt-6-astra", DetectedModelName: "gpt-6-astra/6.1-sol"},
			want:     true,
			via:      "alias",
		},
		{
			name:     "different reference model stays mismatch",
			expected: "6.1sol",
			det:      fingerprintDetection{DetectedModel: "gpt-6-sol", DetectedModelName: "gpt-6-sol"},
			want:     false,
		},
		{
			name:     "exact id",
			expected: "gpt-6-sol",
			det:      fingerprintDetection{DetectedModel: "gpt-6-sol"},
			want:     true,
			via:      "exact",
		},
		{
			name:     "prediction and name disagree with expectation",
			expected: "gpt-6-sol",
			det:      fingerprintDetection{DetectedModel: "gpt-6-astra", DetectedModelName: "gpt-6-astra/6.1-sol"},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, via, ambiguity := index.matches(tc.expected, tc.det)
			if ambiguity != "" {
				t.Fatalf("unexpected ambiguity: %s", ambiguity)
			}
			if matched != tc.want || via != tc.via {
				t.Fatalf("matches = (%v, %q), want (%v, %q)", matched, via, tc.want, tc.via)
			}
		})
	}
}

func TestJudgeFingerprintAnalysisAliasMatch(t *testing.T) {
	index := aliasTestIndex()
	// The user's reported case: API model id "6.1sol", bank prediction gpt-6-astra.
	verdict, _, detection := judgeFingerprintAnalysis("6.1sol", aliasScored("gpt-6-astra", "gpt-6-astra/6.1-sol", "gpt"), false, index)
	if verdict != FingerprintVerdictMatch {
		t.Fatalf("alias verdict = %q, want match", verdict)
	}
	if detection.MatchedVia != "alias" {
		t.Fatalf("matched_via = %q, want alias", detection.MatchedVia)
	}

	// Without the reference index the same run stays a legacy mismatch.
	verdict, _, _ = judgeFingerprintAnalysis("6.1sol", aliasScored("gpt-6-astra", "gpt-6-astra/6.1-sol", "gpt"), false, nil)
	if verdict != FingerprintVerdictMismatch {
		t.Fatalf("legacy verdict = %q, want mismatch", verdict)
	}

	// Different bank model must not become a match just because of the prefix.
	verdict, _, _ = judgeFingerprintAnalysis("6.1sol", aliasScored("gpt-6-sol", "gpt-6-sol", "gpt"), false, index)
	if verdict != FingerprintVerdictMismatch {
		t.Fatalf("different model verdict = %q, want mismatch", verdict)
	}
}

func TestJudgeFingerprintAnalysisAmbiguousExpectation(t *testing.T) {
	index := newFingerprintReferenceIndexFromEntries([]fingerprintReferenceModel{
		{ID: "gpt-6-astra", DisplayName: "gpt-6-astra/6.1-sol", Family: "gpt"},
		{ID: "gpt-6-nova", DisplayName: "gpt-6-nova/6.1-sol", Family: "gpt"},
	})
	verdict, reason, _ := judgeFingerprintAnalysis("6.1sol", aliasScored("gpt-6-astra", "gpt-6-astra/6.1-sol", "gpt"), false, index)
	if verdict != FingerprintVerdictError {
		t.Fatalf("ambiguous verdict = %q, want error", verdict)
	}
	if reason == "" {
		t.Fatal("ambiguous verdict must carry a reason")
	}
}
func TestFingerprintReferenceIndexRealBankAlias(t *testing.T) {
	dataDir := os.Getenv("FINGERPRINT_LIVE_DATA_DIR")
	if dataDir == "" {
		t.Skip("FINGERPRINT_LIVE_DATA_DIR not set")
	}
	index := readFingerprintReferenceIndex(filepath.Join(dataDir, "unified_bank.json"))
	if index == nil {
		t.Fatalf("cannot load reference bank from %s", dataDir)
	}
	candidate := index.resolve("6.1sol")
	if candidate.Ambiguous || candidate.CanonicalID != "gpt-6-astra" {
		t.Fatalf("6.1sol resolved to %+v, want gpt-6-astra", candidate)
	}
	matched, via, ambiguity := index.matches("6.1sol", fingerprintDetection{
		DetectedModel:     "gpt-6-astra",
		DetectedModelName: "gpt-6-astra/6.1-sol",
	})
	if ambiguity != "" || !matched || via != "alias" {
		t.Fatalf("real-bank alias match = (%v, %q, %q)", matched, via, ambiguity)
	}
	// gpt-6-sol is a different reference model and must not be merged.
	if matched, _, _ := index.matches("6.1sol", fingerprintDetection{DetectedModel: "gpt-6-sol", DetectedModelName: "gpt-6-sol"}); matched {
		t.Fatal("6.1sol must not match gpt-6-sol")
	}
	if index.bankSHA256 == "" {
		t.Fatal("bank sha256 must be recorded")
	}
}
func TestFingerprintReferenceIndexDetectionConflictIsAmbiguous(t *testing.T) {
	index := aliasTestIndex()
	matched, _, ambiguity := index.matches("gpt-6-sol", fingerprintDetection{
		DetectedModel:     "gpt-6-sol",
		DetectedModelName: "gpt-6-astra/6.1-sol",
	})
	if matched || ambiguity == "" {
		t.Fatalf("conflicting detection must be ambiguous (matched=%v ambiguity=%q)", matched, ambiguity)
	}
}
