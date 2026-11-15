package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/controls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// ControlRunner runs the accountant controls (issue #31).
type ControlRunner interface {
	Run(ctx context.Context, s engine.Subject, period string) (controls.Result, error)
}

// WithControls attaches the control runner.
func (h *Handler) WithControls(r ControlRunner) *Handler { h.controls = r; return h }

// RunControls: POST /v1/tenant/engagements/{engagementUuid}/controls/run {"period":"YYYY-MM"}.
func (h *Handler) RunControls(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Period string `json:"period"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	e := engagementctx.Must(r.Context())
	res, err := h.controls.Run(r.Context(), engine.Subject{OrganizationID: e.OrganizationID, EngagementID: e.ID, TaxpayerID: e.TaxpayerID}, in.Period)
	if errors.Is(err, controls.ErrInvalidPeriod) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}
