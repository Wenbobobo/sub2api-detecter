package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// fingerprintReferenceModel is one entry of lm-detector's unified bank.
// DisplayName may carry aliases separated by '/', ',' or ';'
// (for example "gpt-6-astra/6.1-sol").
type fingerprintReferenceModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Family      string `json:"family"`
	FamilyName  string `json:"family_name"`
}

// fingerprintReferenceIndex resolves model ids/aliases to canonical bank ids.
// An alias that maps to more than one canonical model is ambiguous and is never
// silently resolved to one of them.
type fingerprintReferenceIndex struct {
	byAlias    map[string][]string                  // normalised alias -> canonical ids
	models     map[string]fingerprintReferenceModel // canonical id -> entry
	bankSHA256 string
}

// fingerprintReferenceCandidate is the outcome of resolving an expectation.
type fingerprintReferenceCandidate struct {
	Input       string
	CanonicalID string
	Aliases     []string
	Family      string
	FamilyName  string
	Ambiguous   bool
	Ambiguity   []string
}

var (
	// fingerprintDigitLetterBoundary splits "6.1sol" into "6-1-sol" without
	// changing labels that have no digit/letter boundary (gpt-5.6-sol etc.).
	fingerprintDigitLetterBoundary = regexp.MustCompile(`([0-9])([a-z])`)
)

// newFingerprintReferenceIndexFromEntries builds an index from raw bank entries.
func newFingerprintReferenceIndexFromEntries(entries []fingerprintReferenceModel) *fingerprintReferenceIndex {
	index := &fingerprintReferenceIndex{
		byAlias: make(map[string][]string, len(entries)*4),
		models:  make(map[string]fingerprintReferenceModel, len(entries)),
	}
	for _, entry := range entries {
		canonical := normalizeModelLabel(entry.ID)
		if canonical == "" {
			continue
		}
		entry.ID = strings.TrimSpace(entry.ID)
		index.models[canonical] = entry
		for _, alias := range fingerprintModelAliasKeys(entry) {
			if alias == "" {
				continue
			}
			ids := index.byAlias[alias]
			already := false
			for _, id := range ids {
				if id == canonical {
					already = true
					break
				}
			}
			if !already {
				index.byAlias[alias] = append(ids, canonical)
			}
		}
	}
	if len(index.models) == 0 {
		return nil
	}
	return index
}

// fingerprintModelAliasKeys lists every normalised alias key of one bank entry.
// Only whole labels and whole display-name segments are used; no substring or
// prefix-stripping heuristics.
func fingerprintModelAliasKeys(entry fingerprintReferenceModel) []string {
	keys := make([]string, 0, 8)
	add := func(raw string) {
		if normalised := normalizeModelLabel(raw); normalised != "" {
			keys = append(keys, normalised)
		}
	}
	add(entry.ID)
	add(entry.DisplayName)
	for _, segment := range splitFingerprintDisplayName(entry.DisplayName) {
		add(segment)
	}
	return dedupeFingerprintStrings(keys)
}

// splitFingerprintDisplayName splits "gpt-6-astra/6.1-sol" into alias segments.
func splitFingerprintDisplayName(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '/', ',', ';', '|', '(', ')':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func dedupeFingerprintStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// resolve maps a model id/alias to a canonical bank model. Ambiguous aliases are
// reported instead of choosing one of the candidates.
func (idx *fingerprintReferenceIndex) resolve(expected string) fingerprintReferenceCandidate {
	candidate := fingerprintReferenceCandidate{Input: strings.TrimSpace(expected)}
	normalised := normalizeModelLabel(candidate.Input)
	if normalised == "" {
		return candidate
	}
	candidate.Aliases = []string{normalised}
	if idx == nil {
		return candidate
	}
	ids := idx.byAlias[normalised]
	switch len(ids) {
	case 0:
		return candidate
	case 1:
		model := idx.models[ids[0]]
		candidate.CanonicalID = model.ID
		candidate.Family = model.Family
		candidate.FamilyName = model.FamilyName
		candidate.Aliases = dedupeFingerprintStrings(append(candidate.Aliases, fingerprintModelAliasKeys(model)...))
	default:
		candidate.Ambiguous = true
		candidate.Ambiguity = append([]string(nil), ids...)
	}
	return candidate
}

// detectionCanonicalIDs resolves every identity hint of a detection to canonical
// ids. Any ambiguous hint makes the whole detection ambiguous.
func (idx *fingerprintReferenceIndex) detectionCanonicalIDs(det fingerprintDetection) (ids []string, ambiguity []string) {
	if idx == nil {
		return nil, nil
	}
	add := func(raw string) {
		resolved := idx.resolve(raw)
		if resolved.Ambiguous {
			ambiguity = append(ambiguity, resolved.Ambiguity...)
			return
		}
		if resolved.CanonicalID != "" {
			ids = append(ids, resolved.CanonicalID)
		}
	}
	add(det.DetectedModel)
	add(det.DetectedModelName)
	for _, segment := range splitFingerprintDisplayName(det.DetectedModelName) {
		add(segment)
	}
	return dedupeFingerprintStrings(ids), dedupeFingerprintStrings(ambiguity)
}

// matches reports whether an expectation accepts a detection, how it matched
// (exact/alias) and an explanation when the inputs are ambiguous. It never uses
// substring/fuzzy matching.
func (idx *fingerprintReferenceIndex) matches(expected string, det fingerprintDetection) (matched bool, via string, ambiguity string) {
	expectedNormalised := normalizeModelLabel(expected)
	detectedNormalised := normalizeModelLabel(det.DetectedModel)
	if idx == nil {
		if expectedNormalised != "" && expectedNormalised == detectedNormalised {
			return true, "exact", ""
		}
		return false, "", ""
	}
	expectation := idx.resolve(expected)
	if expectation.Ambiguous {
		return false, "", "期望模型别名有歧义：" + strings.Join(expectation.Ambiguity, ", ")
	}
	detectedIDs, detectedAmbiguity := idx.detectionCanonicalIDs(det)
	if len(detectedAmbiguity) > 0 {
		return false, "", "检测结果别名有歧义：" + strings.Join(detectedAmbiguity, ", ")
	}
	if len(detectedIDs) > 1 {
		// prediction and display name resolve to different reference models: the
		// detection itself is ambiguous and must not be auto-disabled as mismatch.
		return false, "", "检测结果包含多个参考模型：" + strings.Join(detectedIDs, ", ")
	}
	if expectedNormalised != "" && expectedNormalised == detectedNormalised {
		return true, "exact", ""
	}
	if expectation.CanonicalID == "" || len(detectedIDs) == 0 {
		return false, "", ""
	}
	if detectedIDs[0] != expectation.CanonicalID {
		return false, "", ""
	}
	return true, "alias", ""
}

// referenceIndexLoadState guards the lazy load of the reference index.
var (
	fingerprintReferenceLoadMu sync.Mutex
	fingerprintReferenceCache  = make(map[string]*fingerprintReferenceIndex)
)

// loadFingerprintReferenceIndex loads unified_bank.json from dataDir (cached).
func loadFingerprintReferenceIndex(dataDir string) *fingerprintReferenceIndex {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil
	}
	fingerprintReferenceLoadMu.Lock()
	defer fingerprintReferenceLoadMu.Unlock()
	if cached, ok := fingerprintReferenceCache[dataDir]; ok {
		return cached
	}
	index := readFingerprintReferenceIndex(filepath.Join(dataDir, "unified_bank.json"))
	fingerprintReferenceCache[dataDir] = index
	return index
}

func readFingerprintReferenceIndex(path string) *fingerprintReferenceIndex {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var bank struct {
		Models []fingerprintReferenceModel `json:"models"`
	}
	if err := json.Unmarshal(raw, &bank); err != nil {
		return nil
	}
	index := newFingerprintReferenceIndexFromEntries(bank.Models)
	if index != nil {
		sum := sha256.Sum256(raw)
		index.bankSHA256 = hex.EncodeToString(sum[:])
	}
	return index
}
