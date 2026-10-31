package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultDetectorConfig(t *testing.T) {
	got := DefaultDetectorConfig()
	want := DetectorConfig{
		API:         DetectorAPIAuto,
		Count:       3,
		Parallel:    3,
		TimeoutSec:  120,
		Rounds:      1,
		ServiceTier: DetectorServiceTierDefault,
	}
	if got != want {
		t.Fatalf("DefaultDetectorConfig() = %+v, want %+v", got, want)
	}
}

func TestNormalizeDetectorConfig(t *testing.T) {
	defaults := DefaultDetectorConfig()

	full := DetectorConfig{
		API:              DetectorAPIResponses,
		Count:            2,
		Parallel:         2,
		TimeoutSec:       300,
		Rounds:           2,
		NoStream:         true,
		Strict:           false,
		Effort:           "high",
		ServiceTier:      DetectorServiceTierFast,
		AllowFamilyMatch: true,
	}

	tests := []struct {
		name    string
		in      DetectorConfig
		want    DetectorConfig
		wantErr bool
	}{
		{name: "empty fills defaults", in: DetectorConfig{}, want: defaults},
		{name: "explicit values preserved", in: full, want: full},
		{name: "whitespace and case normalised", in: DetectorConfig{API: " Responses ", ServiceTier: "FAST"}, want: func() DetectorConfig {
			c := defaults
			c.API = DetectorAPIResponses
			c.ServiceTier = DetectorServiceTierFast
			return c
		}()},
		{name: "count too high", in: DetectorConfig{Count: 4}, wantErr: true},
		{name: "count negative", in: DetectorConfig{Count: -1}, wantErr: true},
		{name: "parallel too high", in: DetectorConfig{Parallel: 4}, wantErr: true},
		{name: "parallel negative", in: DetectorConfig{Parallel: -1}, wantErr: true},
		{name: "timeout too high", in: DetectorConfig{TimeoutSec: 601}, wantErr: true},
		{name: "timeout negative", in: DetectorConfig{TimeoutSec: -1}, wantErr: true},
		{name: "rounds too high", in: DetectorConfig{Rounds: 6}, wantErr: true},
		{name: "rounds negative", in: DetectorConfig{Rounds: -1}, wantErr: true},
		{name: "unknown api", in: DetectorConfig{API: "grpc"}, wantErr: true},
		{name: "unknown service tier", in: DetectorConfig{ServiceTier: "turbo"}, wantErr: true},
		{name: "effort with space", in: DetectorConfig{Effort: "high effort"}, wantErr: true},
		{name: "effort too long", in: DetectorConfig{Effort: strings.Repeat("a", 33)}, wantErr: true},
		{name: "effort max length ok", in: DetectorConfig{Effort: strings.Repeat("a", 32)}, want: func() DetectorConfig {
			c := defaults
			c.Effort = strings.Repeat("a", 32)
			return c
		}()},
		{name: "strict forces count to 3", in: DetectorConfig{Strict: true, Count: 1, Parallel: 1}, want: func() DetectorConfig {
			c := defaults
			c.Strict = true
			// count is raised to 3; explicit parallel stays 1 (still <= count).
			c.Parallel = 1
			return c
		}()},
		{name: "parallel clamped to count", in: DetectorConfig{Count: 1, Parallel: 3}, want: func() DetectorConfig {
			c := defaults
			c.Count = 1
			c.Parallel = 1
			return c
		}()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDetectorConfig(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeDetectorConfig(%+v) = %+v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDetectorConfig(%+v) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeDetectorConfig(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDetectorConfigJSONRoundTrip(t *testing.T) {
	normalized, err := NormalizeDetectorConfig(DetectorConfig{API: DetectorAPIResponses, Count: 2, Parallel: 2})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	raw, err := normalized.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	parsed, err := DetectorConfigFromJSON([]byte(raw))
	if err != nil {
		t.Fatalf("DetectorConfigFromJSON: %v", err)
	}
	if parsed != normalized {
		t.Fatalf("round trip = %+v, want %+v", parsed, normalized)
	}

	// Legacy rows store '{}' (or NULL); both must resolve to defaults.
	for _, legacy := range [][]byte{nil, {}, []byte("null"), []byte("{}")} {
		got, err := DetectorConfigFromJSON(legacy)
		if err != nil {
			t.Fatalf("DetectorConfigFromJSON(%q) error: %v", legacy, err)
		}
		if got != DefaultDetectorConfig() {
			t.Fatalf("DetectorConfigFromJSON(%q) = %+v, want defaults", legacy, got)
		}
	}

	if _, err := DetectorConfigFromJSON([]byte("{")); err == nil {
		t.Fatal("DetectorConfigFromJSON malformed JSON: want error")
	}
}

func TestNormalizeScheduledTestMode(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ScheduledTestModeLiveness},
		{in: "liveness", want: ScheduledTestModeLiveness},
		{in: "FINGERPRINT", want: ScheduledTestModeFingerprint},
		{in: " Fingerprint ", want: ScheduledTestModeFingerprint},
		{in: "probe", wantErr: true},
	}
	for _, tc := range tests {
		got, err := NormalizeTestMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeTestMode(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeTestMode(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeTestMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// fakeScheduledTestPlanRepo captures the plan passed to Create/Update.
type fakeScheduledTestPlanRepo struct {
	created *ScheduledTestPlan
	updated *ScheduledTestPlan
}

func (f *fakeScheduledTestPlanRepo) Create(_ context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	cp := *plan
	f.created = &cp
	return &cp, nil
}

func (f *fakeScheduledTestPlanRepo) GetByID(_ context.Context, _ int64) (*ScheduledTestPlan, error) {
	return &ScheduledTestPlan{}, nil
}

func (f *fakeScheduledTestPlanRepo) ListByAccountID(_ context.Context, _ int64) ([]*ScheduledTestPlan, error) {
	return nil, nil
}

func (f *fakeScheduledTestPlanRepo) ListDue(_ context.Context, _ time.Time) ([]*ScheduledTestPlan, error) {
	return nil, nil
}

func (f *fakeScheduledTestPlanRepo) Update(_ context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	cp := *plan
	f.updated = &cp
	return &cp, nil
}

func (f *fakeScheduledTestPlanRepo) Delete(_ context.Context, _ int64) error { return nil }

func (f *fakeScheduledTestPlanRepo) UpdateAfterRun(_ context.Context, _ int64, _ time.Time, _ time.Time) error {
	return nil
}

func TestScheduledTestServiceNormalizesPlanOptions(t *testing.T) {
	repo := &fakeScheduledTestPlanRepo{}
	svc := NewScheduledTestService(repo, nil)
	ctx := context.Background()

	created, err := svc.CreatePlan(ctx, &ScheduledTestPlan{
		AccountID:      1,
		CronExpression: "*/30 * * * *",
		TestMode:       "FINGERPRINT",
		DetectorConfig: DetectorConfig{Strict: true},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if created.TestMode != ScheduledTestModeFingerprint {
		t.Fatalf("test_mode = %q, want fingerprint", created.TestMode)
	}
	if created.DetectorConfig != (func() DetectorConfig {
		c := DefaultDetectorConfig()
		c.Strict = true
		return c
	}()) {
		t.Fatalf("detector_config = %+v, want strict defaults", created.DetectorConfig)
	}

	if _, err := svc.CreatePlan(ctx, &ScheduledTestPlan{
		AccountID:      1,
		CronExpression: "*/30 * * * *",
		TestMode:       "probe",
	}); err == nil {
		t.Fatal("CreatePlan with invalid test_mode: want error")
	}

	if _, err := svc.UpdatePlan(ctx, &ScheduledTestPlan{
		ID:             1,
		CronExpression: "*/30 * * * *",
		DetectorConfig: DetectorConfig{TimeoutSec: 700},
	}); err == nil {
		t.Fatal("UpdatePlan with invalid detector_config: want error")
	}
}
