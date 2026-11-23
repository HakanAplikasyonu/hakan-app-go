package taxpayers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	legalusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/usecase"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Per-window limit of add-mükellef requests per office.
// RATE_LIMIT_TAXPAYER_REQUEST_LIMIT overrides it: the e2e suite adds a new
// mükellef to the demo office in almost every scenario, well over 50 in one
// run (scripts/e2e-up.sh), like the consent preview and accept limits.
var limitRequestsPerOrg = envLimit("RATE_LIMIT_TAXPAYER_REQUEST_LIMIT", 50)

// OnboardingHandler serves the add-mükellef flow.
type OnboardingHandler struct {
	flow    *Onboarding
	limiter *ratelimit.Limiter
	notice  NoticeRecorder
}

// NoticeRecorder records that the person approving was shown the privacy
// notice (KVKK md. 10, issue #57). It is legal usecase.Service.
type NoticeRecorder interface {
	Current(ctx context.Context, kind, language string) (legalusecase.Document, error)
	Accept(ctx context.Context, in legalusecase.AcceptInput) (legalusecase.Acceptance, error)
}

// NewOnboardingHandler builds the handler. limiter may be nil.
func NewOnboardingHandler(flow *Onboarding, limiter *ratelimit.Limiter) *OnboardingHandler {
	return &OnboardingHandler{flow: flow, limiter: limiter}
}

// WithNotice makes approval require the current privacy notice version.
func (h *OnboardingHandler) WithNotice(n NoticeRecorder) *OnboardingHandler {
	h.notice = n
	return h
}

// approveRequest: the consent link token (owner approval has none) and the
// privacy notice version the person was shown.
type approveRequest struct {
	Token                 string `json:"token,omitempty"`
	PrivacyNoticeVersion  int    `json:"privacy_notice_version"`
	PrivacyNoticeLanguage string `json:"privacy_notice_language"`
}

// checkNotice refuses an approval made against an outdated (or missing)
// privacy notice version, before anything changes.
func (h *OnboardingHandler) checkNotice(w http.ResponseWriter, r *http.Request, in approveRequest) bool {
	if h.notice == nil {
		return true
	}
	if in.PrivacyNoticeVersion < 1 {
		response.BadRequest(w, r, response.CodeValidationError, "privacy_notice_version is required (show the privacy notice)")
		return false
	}
	cur, err := h.notice.Current(r.Context(), legalusecase.KindPrivacyNotice, in.PrivacyNoticeLanguage)
	if err != nil {
		response.InternalErr(w, r, err, "Privacy notice is not available")
		return false
	}
	if cur.Version != in.PrivacyNoticeVersion {
		response.Conflict(w, r, "LEGAL_VERSION_OUTDATED", "The privacy notice changed; review the current version")
		return false
	}
	return true
}

// recordNotice stores the evidence after the approval. A failure is logged
// by the failure recorder but does not undo the approval.
func (h *OnboardingHandler) recordNotice(w http.ResponseWriter, r *http.Request, userID int64, in approveRequest, subject *uuid.UUID) {
	if h.notice == nil {
		return
	}
	if _, err := h.notice.Accept(r.Context(), legalusecase.AcceptInput{
		Kind: legalusecase.KindPrivacyNotice, Version: in.PrivacyNoticeVersion, Language: in.PrivacyNoticeLanguage,
		UserID: userID, Context: legalusecase.ContextOnboarding, SubjectUUID: subject,
		IP: clientip.From(r), UserAgent: r.UserAgent(),
	}); err != nil {
		response.RecordFailure(w, err)
	}
}

type requestBody struct {
	Kind         string `json:"kind"`
	TaxID        string `json:"tax_id"`
	Title        string `json:"title"`
	ContactEmail string `json:"contact_email"`
	TaxOffice    string `json:"tax_office"`
	Address      string `json:"address"`
	ActivityCode string `json:"activity_code"`
}

type requestAccepted struct {
	Status  string            `json:"status"`
	Request OnboardingRequest `json:"request"`
}

// createRequest answers 202 consent_requested in every case that passes
// input validation (D1b): nothing here depends on stored taxpayers.
func (h *OnboardingHandler) createRequest(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	var in requestBody
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, "taxpayer_request_org", strconv.FormatInt(s.ID, 10), limitRequestsPerOrg) {
		return
	}
	out, err := h.flow.Request(r.Context(), RequestInput{
		OrgID: s.ID, OrgName: s.Name, UserID: p.UserInternal,
		Kind: in.Kind, TaxID: in.TaxID, Title: in.Title, ContactEmail: in.ContactEmail,
		TaxOffice: in.TaxOffice, Address: in.Address, ActivityCode: in.ActivityCode,
	})
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, requestAccepted{Status: "consent_requested", Request: out})
}

type requestPage struct {
	Items  []OnboardingRequest `json:"items"`
	Total  int64               `json:"total"`
	Limit  int32               `json:"limit"`
	Offset int32               `json:"offset"`
}

func (h *OnboardingHandler) listRequests(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	limit, offset := pageParams(r)
	items, total, err := h.flow.List(r.Context(), s.ID, limit, offset)
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, requestPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *OnboardingHandler) cancelRequest(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.flow.Cancel(r.Context(), s.ID, id); err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OnboardingHandler) previewConsent(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "taxpayer_consent_preview", clientip.From(r), limitPreviewPerIP) {
		return
	}
	var in tokenRequest
	if !decode(w, r, &in) {
		return
	}
	out, err := h.flow.Preview(r.Context(), in.Token)
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *OnboardingHandler) approveConsent(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "taxpayer_consent_decide", clientip.From(r), limitAcceptPerIP) {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	var in approveRequest
	if !decode(w, r, &in) || !h.checkNotice(w, r, in) {
		return
	}
	out, err := h.flow.ApproveByLink(r.Context(), in.Token, p.UserInternal)
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	h.recordNotice(w, r, p.UserInternal, in, out.TaxpayerUUID)
	response.JSON(w, r, http.StatusOK, out)
}

func (h *OnboardingHandler) rejectConsent(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "taxpayer_consent_decide", clientip.From(r), limitAcceptPerIP) {
		return
	}
	p, _ := authctx.PrincipalFrom(r.Context())
	var in tokenRequest
	if !decode(w, r, &in) {
		return
	}
	if err := h.flow.RejectByLink(r.Context(), in.Token, p.UserInternal); err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pendingList struct {
	Items []PendingRequest `json:"items"`
}

func (h *OnboardingHandler) ownerPending(w http.ResponseWriter, r *http.Request) {
	items, err := h.flow.PendingForTaxpayer(r.Context(), taxpayerctx.Must(r.Context()))
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, pendingList{Items: items})
}

func (h *OnboardingHandler) ownerApprove(w http.ResponseWriter, r *http.Request) {
	p, _ := authctx.PrincipalFrom(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in approveRequest
	if h.notice != nil && (!decode(w, r, &in) || !h.checkNotice(w, r, in)) {
		return
	}
	out, err := h.flow.ApproveAsOwner(r.Context(), taxpayerctx.Must(r.Context()), id, p.UserInternal)
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	h.recordNotice(w, r, p.UserInternal, in, &id)
	response.JSON(w, r, http.StatusOK, out)
}

func (h *OnboardingHandler) ownerReject(w http.ResponseWriter, r *http.Request) {
	p, _ := authctx.PrincipalFrom(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.flow.RejectAsOwner(r.Context(), taxpayerctx.Must(r.Context()), id, p.UserInternal); err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OnboardingHandler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	return (&MemberHandler{limiter: h.limiter}).allow(w, r, action, subject, limit)
}

func writeOnboardingError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotOwner) {
		response.RecordFailure(w, err)
		response.Forbidden(w, r, "Only owners of the company can decide")
		return
	}
	writeMemberError(w, r, err)
}
