package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// DetectorConfig holds the lm-detector fingerprint parameters of one scheduled test plan.
// Only used when the plan test_mode is "fingerprint"; it is always stored normalised.
type DetectorConfig struct {
	API              string `json:"api"`                // auto|responses|chat_completions|messages
	Count            int    `json:"count"`              // 1-3, challenges per round
	Parallel         int    `json:"parallel"`           // 1-3, never greater than Count
	TimeoutSec       int    `json:"timeout_sec"`        // 1-600
	Rounds           int    `json:"rounds"`             // 1-5
	NoStream         bool   `json:"no_stream"`          // use JSON response instead of SSE
	Strict           bool   `json:"strict"`             // require a complete 3/3 sample set
	Effort           string `json:"effort"`             // empty = do not send reasoning effort
	ServiceTier      string `json:"service_tier"`       // flex|default|fast|ultrafast
	AllowFamilyMatch bool   `json:"allow_family_match"` // same family counts as a match
}

// Allowed values for DetectorConfig.API / DetectorConfig.ServiceTier.
const (
	DetectorAPIAuto            = "auto"
	DetectorAPIResponses       = "responses"
	DetectorAPIChatCompletions = "chat_completions"
	DetectorAPIMessages        = "messages"

	DetectorServiceTierFlex      = "flex"
	DetectorServiceTierDefault   = "default"
	DetectorServiceTierFast      = "fast"
	DetectorServiceTierUltrafast = "ultrafast"
)

const (
	detectorConfigDefaultCount      = 3
	detectorConfigDefaultParallel   = 3
	detectorConfigDefaultTimeoutSec = 120
	detectorConfigDefaultRounds     = 1

	detectorConfigMinCount      = 1
	detectorConfigMaxCount      = 3
	detectorConfigMinParallel   = 1
	detectorConfigMaxParallel   = 3
	detectorConfigMinTimeoutSec = 1
	detectorConfigMaxTimeoutSec = 600
	detectorConfigMinRounds     = 1
	detectorConfigMaxRounds     = 5
	detectorConfigMaxEffortLen  = 32
)

// detectorEffortPattern restricts effort to the same charset accepted by the upstream API.
var detectorEffortPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// DefaultDetectorConfig returns the parameters used when a plan does not specify them.
// It is safe for the runner to use directly as a fallback.
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		API:              DetectorAPIAuto,
		Count:            detectorConfigDefaultCount,
		Parallel:         detectorConfigDefaultParallel,
		TimeoutSec:       detectorConfigDefaultTimeoutSec,
		Rounds:           detectorConfigDefaultRounds,
		NoStream:         false,
		Strict:           false,
		Effort:           "",
		ServiceTier:      DetectorServiceTierDefault,
		AllowFamilyMatch: false,
	}
}

// NormalizeDetectorConfig fills defaults for empty fields, lowercases enumerations
// and validates every bounded field. Invalid values return an error (handler -> 400).
func NormalizeDetectorConfig(cfg DetectorConfig) (DetectorConfig, error) {
	defaults := DefaultDetectorConfig()
	out := cfg

	out.API = strings.ToLower(strings.TrimSpace(cfg.API))
	if out.API == "" {
		out.API = defaults.API
	}
	switch out.API {
	case DetectorAPIAuto, DetectorAPIResponses, DetectorAPIChatCompletions, DetectorAPIMessages:
	default:
		return DetectorConfig{}, fmt.Errorf("detector_config.api %q is invalid (auto|responses|chat_completions|messages)", cfg.API)
	}

	switch {
	case cfg.Count == 0:
		out.Count = defaults.Count
	case cfg.Count < detectorConfigMinCount || cfg.Count > detectorConfigMaxCount:
		return DetectorConfig{}, fmt.Errorf("detector_config.count must be between %d and %d", detectorConfigMinCount, detectorConfigMaxCount)
	}
	if out.Strict {
		// Strict mode compares a complete 3/3 sample set, so count is forced to 3.
		out.Count = detectorConfigMaxCount
	}

	switch {
	case cfg.Parallel == 0:
		out.Parallel = defaults.Parallel
	case cfg.Parallel < detectorConfigMinParallel || cfg.Parallel > detectorConfigMaxParallel:
		return DetectorConfig{}, fmt.Errorf("detector_config.parallel must be between %d and %d", detectorConfigMinParallel, detectorConfigMaxParallel)
	}
	if out.Parallel > out.Count {
		// Never ask for more concurrent requests than samples in a round.
		out.Parallel = out.Count
	}

	switch {
	case cfg.TimeoutSec == 0:
		out.TimeoutSec = defaults.TimeoutSec
	case cfg.TimeoutSec < detectorConfigMinTimeoutSec || cfg.TimeoutSec > detectorConfigMaxTimeoutSec:
		return DetectorConfig{}, fmt.Errorf("detector_config.timeout_sec must be between %d and %d", detectorConfigMinTimeoutSec, detectorConfigMaxTimeoutSec)
	}

	switch {
	case cfg.Rounds == 0:
		out.Rounds = defaults.Rounds
	case cfg.Rounds < detectorConfigMinRounds || cfg.Rounds > detectorConfigMaxRounds:
		return DetectorConfig{}, fmt.Errorf("detector_config.rounds must be between %d and %d", detectorConfigMinRounds, detectorConfigMaxRounds)
	}

	out.Effort = strings.TrimSpace(cfg.Effort)
	if out.Effort != "" && !detectorEffortPattern.MatchString(out.Effort) {
		return DetectorConfig{}, fmt.Errorf("detector_config.effort must match [A-Za-z0-9_-]{1,%d}", detectorConfigMaxEffortLen)
	}

	out.ServiceTier = strings.ToLower(strings.TrimSpace(cfg.ServiceTier))
	if out.ServiceTier == "" {
		out.ServiceTier = defaults.ServiceTier
	}
	switch out.ServiceTier {
	case DetectorServiceTierFlex, DetectorServiceTierDefault, DetectorServiceTierFast, DetectorServiceTierUltrafast:
	default:
		return DetectorConfig{}, fmt.Errorf("detector_config.service_tier %q is invalid (flex|default|fast|ultrafast)", cfg.ServiceTier)
	}

	return out, nil
}

// ToJSON normalises and marshals the config for a JSONB column. The value is
// sent as a string parameter and cast with ::jsonb by the repository.
func (c DetectorConfig) ToJSON() (string, error) {
	normalized, err := NormalizeDetectorConfig(c)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal detector_config: %w", err)
	}
	return string(raw), nil
}

// DetectorConfigFromJSON parses a JSONB column value (or an API payload) and
// normalises it. Empty / NULL input yields the default config.
func DetectorConfigFromJSON(raw []byte) (DetectorConfig, error) {
	cfg := DefaultDetectorConfig()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return cfg, nil
	}
	if err := json.Unmarshal(trimmed, &cfg); err != nil {
		return DetectorConfig{}, fmt.Errorf("unmarshal detector_config: %w", err)
	}
	return NormalizeDetectorConfig(cfg)
}
