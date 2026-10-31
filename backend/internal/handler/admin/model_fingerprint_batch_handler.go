package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelFingerprintBatchHandler handles batch model fingerprint tests for an
// account model whitelist (async runs + progress polling).
type ModelFingerprintBatchHandler struct {
	batchSvc *service.ModelFingerprintBatchService
}

// NewModelFingerprintBatchHandler creates a batch fingerprint handler.
func NewModelFingerprintBatchHandler(batchSvc *service.ModelFingerprintBatchService) *ModelFingerprintBatchHandler {
	return &ModelFingerprintBatchHandler{batchSvc: batchSvc}
}

type createModelFingerprintRunRequest struct {
	Models         []string                `json:"models" binding:"required"`
	Concurrency    int                     `json:"concurrency"`
	DetectorConfig *service.DetectorConfig `json:"detector_config"`
}

// Create POST /admin/accounts/:id/model-fingerprint-runs
func (h *ModelFingerprintBatchHandler) Create(c *gin.Context) {
	accountID, ok := parseModelFingerprintAccountID(c)
	if !ok {
		return
	}
	var req createModelFingerprintRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	detectorConfig := service.DefaultDetectorConfig()
	if req.DetectorConfig != nil {
		detectorConfig = *req.DetectorConfig
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")
	run, results, err := h.batchSvc.StartRun(c.Request.Context(), accountID, req.Models, detectorConfig, req.Concurrency, idempotencyKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"run": run, "results": results})
}

// List GET /admin/accounts/:id/model-fingerprint-runs
func (h *ModelFingerprintBatchHandler) List(c *gin.Context) {
	accountID, ok := parseModelFingerprintAccountID(c)
	if !ok {
		return
	}
	limit := 10
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	runs, err := h.batchSvc.ListRuns(c.Request.Context(), accountID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, runs)
}

// Get GET /admin/accounts/:id/model-fingerprint-runs/:runId
func (h *ModelFingerprintBatchHandler) Get(c *gin.Context) {
	accountID, ok := parseModelFingerprintAccountID(c)
	if !ok {
		return
	}
	runID, ok := parseModelFingerprintRunID(c)
	if !ok {
		return
	}
	run, results, err := h.batchSvc.GetRun(c.Request.Context(), accountID, runID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"run": run, "results": results})
}

// Cancel POST /admin/accounts/:id/model-fingerprint-runs/:runId/cancel
func (h *ModelFingerprintBatchHandler) Cancel(c *gin.Context) {
	accountID, ok := parseModelFingerprintAccountID(c)
	if !ok {
		return
	}
	runID, ok := parseModelFingerprintRunID(c)
	if !ok {
		return
	}
	run, err := h.batchSvc.CancelRun(c.Request.Context(), accountID, runID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func parseModelFingerprintAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid account id")
		return 0, false
	}
	return id, true
}

func parseModelFingerprintRunID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("runId"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid run id")
		return 0, false
	}
	return id, true
}
