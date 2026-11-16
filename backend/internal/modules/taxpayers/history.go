package taxpayers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	legalusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoActiveOffice: the company has no active engagement to share with.
var ErrNoActiveOffice = errors.New("taxpayers: no active office to share history with")

// History is the office-change side of issue #8 (D2): the mükellef shares
// (and can take back) data from before the current office, and every office
// sees documents through engagement_document_visible.
type History struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	consents ConsentRecorder
}

// ConsentRecorder ties a share to the explicit-consent text the owner
// accepted (KVKK, issue #57). It is legal usecase.Service.
type ConsentRecorder interface {
	AcceptWith(ctx context.Context, q legalusecase.Queries, in legalusecase.AcceptInput) (legalusecase.Acceptance, error)
	WithdrawWith(ctx context.Context, q legalusecase.Queries, subject uuid.UUID) error
}

// NewHistory builds the service.
func NewHistory(pool *pgxpool.Pool) *History { return &History{pool: pool, q: db.New(pool)} }

// WithConsents requires every share to carry the accepted version of the
// explicit-consent text; the share and its evidence are written together and
// withdrawn together.
func (h *History) WithConsents(c ConsentRecorder) *History {
	h.consents = c
	return h
}

// HistoryShare is a stored consent record.
type HistoryShare struct {
	UUID         uuid.UUID  `json:"uuid"`
	Organization string     `json:"organization"`
	FromDate     *string    `json:"from_date"`
	ToDate       *string    `json:"to_date"`
	GrantedAt    time.Time  `json:"granted_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
	Active       bool       `json:"active"`
}

// ShareInput is a date range; both ends optional (YYYY-MM-DD).
// ConsentVersion is the version of the explicit-consent text
// (history_share_consent) the owner was shown and accepted.
type ShareInput struct {
	FromDate        string `json:"from_date"`
	ToDate          string `json:"to_date"`
	ConsentVersion  int    `json:"consent_version"`
	ConsentLanguage string `json:"consent_language"`
	// Request metadata for the consent evidence (set by the handler).
	IP        string `json:"-"`
	UserAgent string `json:"-"`
}

// List returns the company's consent records (owners only).
func (h *History) List(ctx context.Context, s taxpayerctx.Scope) ([]HistoryShare, error) {
	if s.Role != taxpayerctx.RoleOwner {
		return nil, ErrNotOwner
	}
	rows, err := h.q.ListHistorySharesForTaxpayer(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryShare, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryShare{
			UUID: r.Uuid, Organization: r.OrganizationName, FromDate: datePtr(r.FromDate), ToDate: datePtr(r.ToDate),
			GrantedAt: r.GrantedAt.Time, RevokedAt: tsPtr(r.RevokedAt),
			Active: !r.RevokedAt.Valid && r.EngagementActive,
		})
	}
	return out, nil
}

// Share opens data from before the current office's engagement to that
// office, within the given range (owners only).
func (h *History) Share(ctx context.Context, s taxpayerctx.Scope, userID int64, in ShareInput) (HistoryShare, error) {
	if s.Role != taxpayerctx.RoleOwner {
		return HistoryShare{}, ErrNotOwner
	}
	from, err := parseDate(in.FromDate, "from_date")
	if err != nil {
		return HistoryShare{}, err
	}
	to, err := parseDate(in.ToDate, "to_date")
	if err != nil {
		return HistoryShare{}, err
	}
	if from.Valid && to.Valid && to.Time.Before(from.Time) {
		return HistoryShare{}, fmt.Errorf("%w: to_date is before from_date", ErrInvalidRequest)
	}
	if h.consents != nil && in.ConsentVersion < 1 {
		return HistoryShare{}, fmt.Errorf("%w: consent_version is required (accept the explicit-consent text)", ErrInvalidRequest)
	}
	eng, err := h.q.GetActiveEngagementOfTaxpayer(ctx, s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return HistoryShare{}, ErrNoActiveOffice
	}
	if err != nil {
		return HistoryShare{}, err
	}
	var row db.CreateHistoryShareRow
	err = h.inTx(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.CreateHistoryShare(ctx, db.CreateHistoryShareParams{
			TaxpayerID: s.ID, OrganizationID: eng.OrganizationID, EngagementID: eng.ID,
			FromDate: from, ToDate: to, GrantedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
		})
		if err != nil || h.consents == nil {
			return err
		}
		_, err = h.consents.AcceptWith(ctx, q, legalusecase.AcceptInput{
			Kind: legalusecase.KindHistoryConsent, Version: in.ConsentVersion, Language: in.ConsentLanguage,
			UserID: userID, Context: legalusecase.ContextHistory, TaxpayerID: s.ID, OrganizationID: eng.OrganizationID,
			SubjectUUID: &row.Uuid, IP: in.IP, UserAgent: in.UserAgent,
		})
		return err
	})
	if err != nil {
		return HistoryShare{}, err
	}
	return HistoryShare{UUID: row.Uuid, FromDate: datePtr(from), ToDate: datePtr(to), GrantedAt: row.GrantedAt.Time, Active: true}, nil
}

// Revoke takes a share back (owners only). The office loses the shared
// period at once.
func (h *History) Revoke(ctx context.Context, s taxpayerctx.Scope, userID int64, id uuid.UUID) error {
	if s.Role != taxpayerctx.RoleOwner {
		return ErrNotOwner
	}
	return h.inTx(ctx, func(q *db.Queries) error {
		n, err := q.RevokeHistoryShare(ctx, db.RevokeHistoryShareParams{
			TaxpayerID: s.ID, Uuid: id, RevokedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if h.consents != nil {
			// Açık rıza geri alındı (KVKK md. 5): kanıt kaydı da işaretlenir.
			return h.consents.WithdrawWith(ctx, q, id)
		}
		return nil
	})
}

func (h *History) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if h.pool == nil {
		return fn(h.q)
	}
	return pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error { return fn(db.New(tx)) })
}

// Document is a document as an office sees it.
type Document struct {
	UUID          uuid.UUID `json:"uuid"`
	InvoiceNumber string    `json:"invoice_number"`
	IssueDate     string    `json:"issue_date"`
	Direction     string    `json:"direction"`
	PayableAmount string    `json:"payable_amount"`
	Currency      string    `json:"currency"`
}

// OfficeDocuments lists the invoices the engagement may see, under RLS:
// its own period, history the mükellef shared, nothing after it ended.
func (h *History) OfficeDocuments(ctx context.Context, e engagementctx.Scope, limit, offset int32) ([]Document, error) {
	var rows []db.ListEngagementVisibleInvoicesRow
	err := rls.WithScope(ctx, h.pool, rls.FromRequest(ctx), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = h.q.WithTx(tx).ListEngagementVisibleInvoices(ctx, db.ListEngagementVisibleInvoicesParams{
			TaxpayerID: e.TaxpayerID, EngagementID: e.ID, LimitCount: limit, OffsetCount: offset,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		amount := ""
		if v, err := r.PayableAmount.Value(); err == nil && v != nil {
			amount = fmt.Sprint(v)
		}
		out = append(out, Document{
			UUID: r.Uuid, InvoiceNumber: r.InvoiceNumber, IssueDate: r.IssueDate.Time.Format(time.DateOnly),
			Direction: r.Direction, PayableAmount: amount, Currency: r.Currency,
		})
	}
	return out, nil
}

// OfficeShares lists the history shares the mükellef granted to this
// engagement, so the office knows which past period it can see.
func (h *History) OfficeShares(ctx context.Context, e engagementctx.Scope) ([]HistoryShare, error) {
	rows, err := h.q.ListHistorySharesForEngagement(ctx, db.ListHistorySharesForEngagementParams{
		TaxpayerID: e.TaxpayerID, OrganizationID: e.OrganizationID, EngagementID: e.ID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]HistoryShare, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryShare{
			UUID: r.Uuid, FromDate: datePtr(r.FromDate), ToDate: datePtr(r.ToDate),
			GrantedAt: r.GrantedAt.Time, RevokedAt: tsPtr(r.RevokedAt), Active: !r.RevokedAt.Valid,
		})
	}
	return out, nil
}

func parseDate(v, field string) (pgtype.Date, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrInvalidRequest, field)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// HistoryHandler serves the history routes.
type HistoryHandler struct{ h *History }

// NewHistoryHandler builds the handler.
func NewHistoryHandler(h *History) *HistoryHandler { return &HistoryHandler{h: h} }

type shareList struct {
	Items []HistoryShare `json:"items"`
}

type documentList struct {
	Items  []Document `json:"items"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

func (x *HistoryHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := x.h.List(r.Context(), taxpayerctx.Must(r.Context()))
	if err != nil {
		writeHistoryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, shareList{Items: items})
}

func (x *HistoryHandler) share(w http.ResponseWriter, r *http.Request) {
	p, _ := authctx.PrincipalFrom(r.Context())
	var in ShareInput
	if !decode(w, r, &in) {
		return
	}
	in.IP, in.UserAgent = clientip.From(r), r.UserAgent()
	out, err := x.h.Share(r.Context(), taxpayerctx.Must(r.Context()), p.UserInternal, in)
	if err != nil {
		writeHistoryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (x *HistoryHandler) revoke(w http.ResponseWriter, r *http.Request) {
	p, _ := authctx.PrincipalFrom(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := x.h.Revoke(r.Context(), taxpayerctx.Must(r.Context()), p.UserInternal, id); err != nil {
		writeHistoryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (x *HistoryHandler) officeDocuments(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	items, err := x.h.OfficeDocuments(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		writeHistoryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, documentList{Items: items, Limit: limit, Offset: offset})
}

func (x *HistoryHandler) officeShares(w http.ResponseWriter, r *http.Request) {
	items, err := x.h.OfficeShares(r.Context(), engagementctx.Must(r.Context()))
	if err != nil {
		writeHistoryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, shareList{Items: items})
}

func writeHistoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, legalusecase.ErrOutdated):
		response.RecordFailure(w, err)
		response.Conflict(w, r, "LEGAL_VERSION_OUTDATED", "The consent text changed; review and accept the current version")
		return
	case errors.Is(err, legalusecase.ErrNotFound):
		response.RecordFailure(w, err)
		response.InternalErr(w, r, err, "Consent text is not available")
		return
	}
	if errors.Is(err, ErrNoActiveOffice) {
		response.RecordFailure(w, err)
		response.Conflict(w, r, response.CodeConflict, "The company has no active office")
		return
	}
	writeOnboardingError(w, r, err)
}
