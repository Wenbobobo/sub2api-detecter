package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// fakeScheduledTestPlanStore is a minimal in-memory plan repository for handler tests.
type fakeScheduledTestPlanStore struct {
	mu     sync.Mutex
	nextID int64
	plans  map[int64]*service.ScheduledTestPlan
}

func newFakeScheduledTestPlanStore() *fakeScheduledTestPlanStore {
	return &fakeScheduledTestPlanStore{nextID: 1, plans: map[int64]*service.ScheduledTestPlan{}}
}

func (f *fakeScheduledTestPlanStore) Create(_ context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *plan
	cp.ID = f.nextID
	f.nextID++
	now := time.Now()
	cp.CreatedAt = now
	cp.UpdatedAt = now
	f.plans[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeScheduledTestPlanStore) GetByID(_ context.Context, id int64) (*service.ScheduledTestPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan, ok := f.plans[id]
	if !ok {
		return nil, fmt.Errorf("plan %d not found", id)
	}
	cp := *plan
	return &cp, nil
}

func (f *fakeScheduledTestPlanStore) ListByAccountID(_ context.Context, accountID int64) ([]*service.ScheduledTestPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*service.ScheduledTestPlan
	for _, plan := range f.plans {
		if plan.AccountID == accountID {
			cp := *plan
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeScheduledTestPlanStore) ListDue(_ context.Context, _ time.Time) ([]*service.ScheduledTestPlan, error) {
	return nil, nil
}

func (f *fakeScheduledTestPlanStore) Update(_ context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *plan
	cp.UpdatedAt = time.Now()
	f.plans[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeScheduledTestPlanStore) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.plans, id)
	return nil
}

func (f *fakeScheduledTestPlanStore) UpdateAfterRun(_ context.Context, _ int64, _ time.Time, _ time.Time) error {
	return nil
}

type fakeScheduledTestResultStore struct{}

func (f *fakeScheduledTestResultStore) Create(_ context.Context, _ *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	return nil, nil
}

func (f *fakeScheduledTestResultStore) ListByPlanID(_ context.Context, _ int64, _ int) ([]*service.ScheduledTestResult, error) {
	return nil, nil
}

func (f *fakeScheduledTestResultStore) PruneOldResults(_ context.Context, _ int64, _ int) error {
	return nil
}

func newScheduledTestTestRouter(t *testing.T) (*gin.Engine, *fakeScheduledTestPlanStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store := newFakeScheduledTestPlanStore()
	svc := service.NewScheduledTestService(store, &fakeScheduledTestResultStore{})
	handler := NewScheduledTestHandler(svc)

	router := gin.New()
	router.GET("/admin/accounts/:id/scheduled-test-plans", handler.ListByAccount)
	router.POST("/admin/scheduled-test-plans", handler.Create)
	router.PUT("/admin/scheduled-test-plans/:id", handler.Update)
	router.DELETE("/admin/scheduled-test-plans/:id", handler.Delete)
	router.GET("/admin/scheduled-test-plans/:id/results", handler.ListResults)
	return router, store
}

func doScheduledTestJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestScheduledTestHandlerCreateDefaults(t *testing.T) {
	router, _ := newScheduledTestTestRouter(t)

	rec := doScheduledTestJSON(t, router, http.MethodPost, "/admin/scheduled-test-plans",
		`{"account_id":5,"model_id":"claude-sonnet-4-5","cron_expression":"*/30 * * * *"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var plan service.ScheduledTestPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if plan.TestMode != service.ScheduledTestModeLiveness {
		t.Fatalf("test_mode = %q, want liveness", plan.TestMode)
	}
	if plan.AutoDisable {
		t.Fatal("auto_disable = true, want false")
	}
	if plan.DetectorConfig != service.DefaultDetectorConfig() {
		t.Fatalf("detector_config = %+v, want defaults", plan.DetectorConfig)
	}
}

func TestScheduledTestHandlerCreateFingerprintPlan(t *testing.T) {
	router, _ := newScheduledTestTestRouter(t)

	rec := doScheduledTestJSON(t, router, http.MethodPost, "/admin/scheduled-test-plans",
		`{"account_id":5,"model_id":"gpt-5.1","cron_expression":"*/30 * * * *","test_mode":"fingerprint","auto_disable":true,"detector_config":{"api":"responses","count":1,"parallel":3,"strict":true}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var plan service.ScheduledTestPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if plan.TestMode != service.ScheduledTestModeFingerprint || !plan.AutoDisable {
		t.Fatalf("plan mode/auto_disable = %q/%v", plan.TestMode, plan.AutoDisable)
	}
	// strict forces count=3 and parallel stays 3; omitting fields keeps defaults.
	if plan.DetectorConfig.API != service.DetectorAPIResponses ||
		plan.DetectorConfig.Count != 3 ||
		plan.DetectorConfig.Parallel != 3 ||
		!plan.DetectorConfig.Strict {
		t.Fatalf("detector_config = %+v", plan.DetectorConfig)
	}
}

func TestScheduledTestHandlerCreateRejectsInvalidFields(t *testing.T) {
	router, _ := newScheduledTestTestRouter(t)

	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "unknown test mode",
			body: `{"account_id":5,"cron_expression":"*/30 * * * *","test_mode":"probe"}`,
		},
		{
			name: "detector count out of range",
			body: `{"account_id":5,"cron_expression":"*/30 * * * *","detector_config":{"count":5}}`,
		},
		{
			name: "invalid effort",
			body: `{"account_id":5,"cron_expression":"*/30 * * * *","detector_config":{"effort":"high effort"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doScheduledTestJSON(t, router, http.MethodPost, "/admin/scheduled-test-plans", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestScheduledTestHandlerUpdatePartialSemantics(t *testing.T) {
	router, store := newScheduledTestTestRouter(t)

	rec := doScheduledTestJSON(t, router, http.MethodPost, "/admin/scheduled-test-plans",
		`{"account_id":5,"cron_expression":"*/30 * * * *","test_mode":"fingerprint","auto_disable":true,"detector_config":{"api":"responses","count":1,"timeout_sec":300}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Omitted fields keep their stored values.
	rec = doScheduledTestJSON(t, router, http.MethodPut, "/admin/scheduled-test-plans/1", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var plan service.ScheduledTestPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if plan.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if plan.TestMode != service.ScheduledTestModeFingerprint || !plan.AutoDisable {
		t.Fatalf("omitted mode fields were reset: %+v", plan)
	}
	if plan.DetectorConfig.API != service.DetectorAPIResponses || plan.DetectorConfig.Count != 1 || plan.DetectorConfig.TimeoutSec != 300 {
		t.Fatalf("omitted detector_config was reset: %+v", plan.DetectorConfig)
	}

	// An explicit empty object resets the detector config to defaults.
	rec = doScheduledTestJSON(t, router, http.MethodPut, "/admin/scheduled-test-plans/1", `{"detector_config":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if plan.DetectorConfig != service.DefaultDetectorConfig() {
		t.Fatalf("detector_config = %+v, want defaults", plan.DetectorConfig)
	}

	// Explicit false / liveness must be applied, not treated as "unset".
	rec = doScheduledTestJSON(t, router, http.MethodPut, "/admin/scheduled-test-plans/1", `{"auto_disable":false,"test_mode":"liveness"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if plan.AutoDisable || plan.TestMode != service.ScheduledTestModeLiveness {
		t.Fatalf("partial update ignored explicit false/empty mode: %+v", plan)
	}

	// Invalid values are rejected on update as well and leave the stored plan unchanged.
	rec = doScheduledTestJSON(t, router, http.MethodPut, "/admin/scheduled-test-plans/1", `{"test_mode":"probe"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid test_mode status = %d, want 400", rec.Code)
	}
	rec = doScheduledTestJSON(t, router, http.MethodPut, "/admin/scheduled-test-plans/1", `{"detector_config":{"rounds":9}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid detector_config status = %d, want 400", rec.Code)
	}

	stored, err := store.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.TestMode != service.ScheduledTestModeLiveness || stored.DetectorConfig != service.DefaultDetectorConfig() {
		t.Fatalf("rejected update mutated stored plan: %+v", stored)
	}
}
