package service

import (
	"fmt"
	"regexp"
	"strings"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

// Fingerprint verdict values persisted in scheduled_test_results.verdict.
const (
	FingerprintVerdictMatch        = "match"
	FingerprintVerdictMismatch     = "mismatch"
	FingerprintVerdictInvalidReply = "invalid_reply"
	FingerprintVerdictError        = "error"
	FingerprintVerdictUnsupported  = "unsupported"
)

// Account actions recorded on scheduled_test_results.account_action.
const (
	ScheduledAccountActionDisabled = "disabled"
	ScheduledAccountActionRestored = "restored"
)

// scheduledTestAutoDisableExtraKey is the accounts.extra marker written when a
// scheduled test auto-disabled the account. A nil value means "no marker".
const scheduledTestAutoDisableExtraKey = "scheduled_test_auto_disable"

var (
	fingerprintDateSuffixPattern = regexp.MustCompile(`-20\d{2}-?\d{2}-?\d{2}$`)
	fingerprintDashesPattern     = regexp.MustCompile(`-{2,}`)
)

// expectedModelFamilyPrefixes maps a model name prefix to the lm-detector
// reference-bank family label. Longest prefixes must come first.
var expectedModelFamilyPrefixes = []struct {
	prefix string
	family string
}{
	{"chatgpt", "gpt"},
	{"deepseek", "deepseek"},
	{"minimax", "minimax"},
	{"hunyuan", "hunyuan"},
	{"moonshot", "kimi"},
	{"chatglm", "glm"},
	{"claude", "claude"},
	{"gemini", "gemini"},
	{"doubao", "seed"},
	{"gpt", "gpt"},
	{"grok", "grok"},
	{"qwen", "qwen"},
	{"kimi", "kimi"},
	{"mimo", "mimo"},
	{"muse", "muse"},
	{"seed", "seed"},
	{"step", "step"},
	{"glm", "glm"},
	{"hy", "hunyuan"},
	{"o1", "gpt"},
	{"o3", "gpt"},
	{"o4", "gpt"},
}

// normalizeModelLabel makes an API model name and an lm-detector reference
// label comparable: lowercase, strip provider prefixes, dots/underscores to
// dashes and drop the trailing release date (e.g. -20250929).
func normalizeModelLabel(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = fingerprintDateSuffixPattern.ReplaceAllString(s, "")
	s = fingerprintDigitLetterBoundary.ReplaceAllString(s, "$1-$2")
	s = fingerprintDashesPattern.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// modelLabelsEquivalent reports whether two model labels denote the same model
// after normalisation (claude-opus-4.5 == claude-opus-4-5-20250929).
func modelLabelsEquivalent(a, b string) bool {
	na, nb := normalizeModelLabel(a), normalizeModelLabel(b)
	return na != "" && nb != "" && na == nb
}

// expectedModelFamily returns the reference-bank family of an expected model
// name, or "" when the family cannot be inferred.
func expectedModelFamily(raw string) string {
	n := normalizeModelLabel(raw)
	if n == "" {
		return ""
	}
	for _, prefix := range expectedModelFamilyPrefixes {
		if strings.HasPrefix(n, prefix.prefix) {
			return prefix.family
		}
	}
	return ""
}

// fingerprintDetection holds the model identification of one scoring round.
type fingerprintDetection struct {
	DetectedModel     string
	DetectedModelName string
	DetectedFamily    string
	Probability       *float64
	// MatchedVia is "exact", "alias" or "family" when the round matched.
	MatchedVia string
}

// judgeFingerprintAnalysis maps one lm-detector analysis to a verdict.
// allowFamilyMatch treats a different version of the same model family as a
// match (opt-in because it weakens the "same model" guarantee). referenceIndex
// may be nil, in which case only exact normalised comparison is used.
func judgeFingerprintAnalysis(expected string, analysis *lmfpd.Analysis, allowFamilyMatch bool, referenceIndex *fingerprintReferenceIndex) (string, string, fingerprintDetection) {
	var det fingerprintDetection
	if analysis == nil {
		return FingerprintVerdictError, "检测器没有返回分析结果", det
	}
	det.DetectedModel = strings.TrimSpace(analysis.Prediction)
	det.DetectedModelName = strings.TrimSpace(analysis.PredictionName)
	det.DetectedFamily = strings.TrimSpace(analysis.FamilyPrediction)
	if analysis.Probability != nil && analysis.ProbabilityStatus == "reference_calibrated" {
		p := *analysis.Probability
		det.Probability = &p
	}

	switch analysis.Decision {
	case "not_model":
		reason := strings.TrimSpace(analysis.Evidence.Reason)
		if reason == "" {
			reason = "回复的数值序列不像语言模型生成"
		}
		return FingerprintVerdictInvalidReply, "回复不像模型输出：" + reason, det
	case "unscorable":
		reason := strings.TrimSpace(analysis.Evidence.Reason)
		if reason == "" {
			reason = "有效样本不足，无法评分"
		}
		return FingerprintVerdictError, "指纹样本无法评分：" + reason, det
	}

	if det.DetectedModel == "" {
		return FingerprintVerdictError, "检测器没有给出候选模型", det
	}
	matched, via, ambiguity := referenceIndex.matches(expected, det)
	if ambiguity != "" {
		return FingerprintVerdictError, ambiguity, det
	}
	if matched {
		det.MatchedVia = via
		if via == "alias" {
			return FingerprintVerdictMatch, fmt.Sprintf("别名匹配：期望 %s，参考库识别为 %s（%s）", expected, det.DetectedModel, det.DetectedModelName), det
		}
		return FingerprintVerdictMatch, "", det
	}
	if allowFamilyMatch {
		expectedFamily := expectedModelFamily(expected)
		if referenceIndex != nil {
			if resolved := referenceIndex.resolve(expected); resolved.Family != "" {
				expectedFamily = resolved.Family
			}
		}
		if expectedFamily != "" && expectedFamily == analysis.FamilyPrediction {
			det.MatchedVia = "family"
			return FingerprintVerdictMatch, fmt.Sprintf("同家族（%s）不同版本：期望 %s，检测为 %s", expectedFamily, expected, det.DetectedModel), det
		}
	}
	return FingerprintVerdictMismatch, fmt.Sprintf("期望 %s，检测为 %s（%s）", expected, det.DetectedModel, det.DetectedModelName), det
}

// aggregateFingerprintVerdicts combines the verdicts of multiple detection
// rounds. Any mismatch wins; only an all-match run is a match.
func aggregateFingerprintVerdicts(verdicts []string) string {
	if len(verdicts) == 0 {
		return FingerprintVerdictError
	}
	sawInvalid := false
	sawError := false
	allMatch := true
	for _, v := range verdicts {
		switch v {
		case FingerprintVerdictMatch:
		case FingerprintVerdictMismatch:
			return FingerprintVerdictMismatch
		case FingerprintVerdictInvalidReply:
			allMatch = false
			sawInvalid = true
		default:
			allMatch = false
			sawError = true
		}
	}
	if allMatch {
		return FingerprintVerdictMatch
	}
	if sawInvalid {
		return FingerprintVerdictInvalidReply
	}
	if sawError {
		return FingerprintVerdictError
	}
	return FingerprintVerdictError
}

// bestFingerprintDetection picks the most confident detection across rounds.
func bestFingerprintDetection(detections []fingerprintDetection) fingerprintDetection {
	best := fingerprintDetection{}
	bestProbability := -1.0
	for _, det := range detections {
		probability := 0.0
		if det.Probability != nil {
			probability = *det.Probability
		}
		if det.DetectedModel != "" && probability > bestProbability {
			best = det
			bestProbability = probability
		}
	}
	return best
}

// fingerprintFailureReason returns the persisted error message of a fingerprint
// run, or "" when the account matched.
func fingerprintFailureReason(verdict, expected, detected, baseReason string) string {
	switch verdict {
	case FingerprintVerdictMatch:
		return ""
	case FingerprintVerdictUnsupported:
		return baseReason
	case FingerprintVerdictMismatch:
		return "模型指纹不一致（疑似降智/换模）：" + baseReason
	case FingerprintVerdictInvalidReply:
		return "模型回复异常（疑似非模型/被劫持）：" + baseReason
	default:
		if baseReason == "" {
			return "模型指纹检测失败"
		}
		return "模型指纹检测失败：" + baseReason
	}
}
