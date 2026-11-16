// Package officesetup keeps the state of the office setup wizard (issue
// #273): which steps are done or skipped, the chosen accounting program and
// the taxpayer import in progress. The wizard can be left at any step and
// continued later. The state lives in
// organizations.setup_state (migration 000085), apart from the settings
// object that PATCH /v1/tenant/settings replaces.
package officesetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Steps of the wizard, in order.
var Steps = []string{"office", "team", "taxpayers", "accounting", "integrations", "invites"}

// Step statuses.
const (
	StatusPending = "pending"
	StatusDone    = "done"
	StatusSkipped = "skipped"
)

// Accounting programs (export drivers, 02-muhasebe-cikti-surucu-modeli).
var programs = map[string]bool{"luca": true, "zirve": true}

// StepState is one step.
type StepState struct {
	Status    string     `json:"status"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// State is the wizard state.
type State struct {
	Steps             map[string]StepState `json:"steps"`
	AccountingProgram string               `json:"accounting_program,omitempty"`
	ImportUUID        *uuid.UUID           `json:"import_uuid,omitempty"`
	CompletedAt       *time.Time           `json:"completed_at,omitempty"`
}

// ErrInvalid is a malformed state.
var ErrInvalid = errors.New("officesetup: invalid state")

// normalize fills every step and checks the values.
func (s *State) normalize() error {
	if s.Steps == nil {
		s.Steps = map[string]StepState{}
	}
	known := map[string]bool{}
	for _, k := range Steps {
		known[k] = true
		if _, ok := s.Steps[k]; !ok {
			s.Steps[k] = StepState{Status: StatusPending}
		}
	}
	for k, v := range s.Steps {
		if !known[k] {
			return fmt.Errorf("%w: unknown step %q", ErrInvalid, k)
		}
		switch v.Status {
		case StatusPending, StatusDone, StatusSkipped:
		default:
			return fmt.Errorf("%w: step %q status %q", ErrInvalid, k, v.Status)
		}
	}
	if s.AccountingProgram != "" && !programs[s.AccountingProgram] {
		return fmt.Errorf("%w: accounting_program must be luca or zirve", ErrInvalid)
	}
	return nil
}

// Service reads and writes the state.
type Service struct{ pool database.Pool }

// New builds the service.
func New(pool database.Pool) *Service { return &Service{pool: pool} }

// Get returns the state of the office (every step pending when new).
func (s *Service) Get(ctx context.Context, orgID int64) (State, error) {
	raw, err := database.QueriesFor(s.pool).GetOrganizationSetup(ctx, orgID)
	if err != nil {
		return State{}, err
	}
	var st State
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &st); err != nil {
			return State{}, err
		}
	}
	_ = st.normalize() // stored state was valid when written
	return st, nil
}

// Put replaces the state. Steps that change status get the current time.
func (s *Service) Put(ctx context.Context, orgID int64, in State, now time.Time) (State, error) {
	if err := in.normalize(); err != nil {
		return State{}, err
	}
	prev, err := s.Get(ctx, orgID)
	if err != nil {
		return State{}, err
	}
	for k, v := range in.Steps {
		if old, ok := prev.Steps[k]; ok && old.Status == v.Status {
			v.UpdatedAt = old.UpdatedAt
		} else if v.Status != StatusPending {
			t := now.UTC()
			v.UpdatedAt = &t
		} else {
			v.UpdatedAt = nil
		}
		in.Steps[k] = v
	}
	allSettled := true
	for _, k := range Steps {
		if in.Steps[k].Status == StatusPending {
			allSettled = false
		}
	}
	switch {
	case allSettled && prev.CompletedAt != nil:
		in.CompletedAt = prev.CompletedAt
	case allSettled:
		t := now.UTC()
		in.CompletedAt = &t
	default:
		in.CompletedAt = nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return State{}, err
	}
	if _, err := database.QueriesFor(s.pool).SetOrganizationSetup(ctx, db.SetOrganizationSetupParams{OrganizationID: orgID, Setup: raw}); err != nil {
		return State{}, err
	}
	return in, nil
}

// Handler serves GET / PUT /v1/tenant/setup.
type Handler struct {
	svc *Service
	now func() time.Time
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc, now: time.Now} }

const maxStateBytes = 8 << 10

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the setup state")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	var in State
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxStateBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "body must be a setup state object up to 8 KiB")
		return
	}
	st, err := h.svc.Put(r.Context(), orgctx.Must(r.Context()).ID, in, h.now())
	if errors.Is(err, ErrInvalid) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not save the setup state")
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// Module mounts the routes.
type Module struct{ H *Handler }

// Name implements module.Module.
func (Module) Name() string { return "officesetup" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/setup", rbac.PermTenantSettingsRead, h.get)
	r.Tenant("PUT /v1/tenant/setup", rbac.PermTenantSettingsWrite, h.put)
}
