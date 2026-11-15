package taxpayers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Rate limits for the invitation endpoints (per 15 minutes).
const (
	limitWindow             = 15 * time.Minute
	limitPreviewPerIP       = 60
	limitAcceptPerIP        = 20
	limitInvitesPerOrg      = 50
)

// MemberHandler serves mükellef invitations and the mükellef shell context.
type MemberHandler struct {
	members *Members
	limiter *ratelimit.Limiter
}

// NewMemberHandler builds the handler. limiter may be nil.
func NewMemberHandler(members *Members, limiter *ratelimit.Limiter) *MemberHandler {
	return &MemberHandler{members: members, limiter: limiter}
}

type invitationList struct {
	Items []Invitation `json:"items"`
}

func (h *MemberHandler) listInvitations(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	items, err := h.members.ListInvitations(r.Context(), e.OrganizationID, e.ID)
	if err != nil {
		writeMemberError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, invitationList{Items: items})
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *MemberHandler) createInvitation(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	e := engagementctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	var in inviteRequest
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, "taxpayer_invite_org", strconv.FormatInt(s.ID, 10), limitInvitesPerOrg) {
		return
	}
	out, err := h.members.Invite(r.Context(), InviteInput{
		OrgID: s.ID, OrgName: s.Name, EngagementID: e.ID, TaxpayerID: e.TaxpayerID,
		Email: in.Email, Role: in.Role, InviterUserID: p.UserInternal,
	})
	if err != nil {
		writeMemberError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *MemberHandler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	e := engagementctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.members.RevokeInvitation(r.Context(), e.OrganizationID, e.ID, id); err != nil {
		writeMemberError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type tokenRequest struct {
	Token string `json:"token"`
}

func (h *MemberHandler) previewInvitation(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "taxpayer_invite_preview", clientip.From(r), limitPreviewPerIP) {
		return
	}
	var in tokenRequest
	if !decode(w, r, &in) {
		return
	}
	out, err := h.members.Preview(r.Context(), in.Token)
	if err != nil {
		writeMemberError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *MemberHandler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "taxpayer_invite_accept", clientip.From(r), limitAcceptPerIP) {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	var in tokenRequest
	if !decode(w, r, &in) {
		return
	}
	out, err := h.members.Accept(r.Context(), in.Token, p.UserInternal)
	if err != nil {
		writeMemberError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type taxpayerContextDTO struct {
	Taxpayer struct {
		UUID      string `json:"uuid"`
		Kind      string `json:"kind"`
		TaxID     string `json:"tax_id"`
		TaxIDType string `json:"tax_id_type"`
		Title     string `json:"title"`
	} `json:"taxpayer"`
	Role string `json:"role"`
}

// taxpayerContext returns the resolved mükellef scope of the request.
func (h *MemberHandler) taxpayerContext(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	var out taxpayerContextDTO
	out.Taxpayer.UUID = s.UUID.String()
	out.Taxpayer.Kind = s.Kind
	out.Taxpayer.TaxID = s.TaxID
	out.Taxpayer.TaxIDType = "vkn"
	if len(s.TaxID) == 11 {
		out.Taxpayer.TaxIDType = "tckn"
	}
	out.Taxpayer.Title = s.Title
	out.Role = s.Role
	response.JSON(w, r, http.StatusOK, out)
}

func (h *MemberHandler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil || strings.TrimSpace(subject) == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, limitWindow)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "")
	return false
}

func writeMemberError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrInvitationNotFound), errors.Is(err, ErrInvitationsDisabled):
		response.Error(w, r, http.StatusNotFound, response.CodeInvitationNotFound, "Invitation was not found")
	case errors.Is(err, ErrInvitationUnavailable):
		response.Error(w, r, http.StatusGone, response.CodeInvitationUnavailable, "Invitation was already used, revoked or has expired")
	case errors.Is(err, ErrInvitationEmailMismatch):
		response.Error(w, r, http.StatusForbidden, response.CodeInvitationEmailMismatch, "Invitation was sent to another email address")
	case errors.Is(err, ErrAlreadyMember):
		response.Conflict(w, r, response.CodeAlreadyMember, "Already a member of this taxpayer")
	default:
		writeError(w, r, err)
	}
}
