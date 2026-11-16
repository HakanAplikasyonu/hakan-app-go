// Package privacy serves KVKK data subject requests (issue #193, KVKK md. 11):
// a machine-readable export (JSON in a ZIP) of a taxpayer's or a taxpayer
// user's personal data, and erasure by anonymisation of the data the law
// does not require the office to keep.
//
// Flow: an office user opens a request on an engagement; an office
// owner/admin approves or rejects it. Approval runs the request in the same
// transaction. Accounting records (invoices, bank movements, expenses,
// journals) and the taxpayer's legal identity are never deleted: they fall
// under the statutory retention period (VUK md. 253). Consent records stay as
// evidence. What was done is kept on the request (summary) and in the audit
// log. See docs/kvkk.md §7.
package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Kinds and subjects.
const (
	KindExport  = "export"
	KindErasure = "erasure"

	SubjectTaxpayer = "taxpayer"
	SubjectUser     = "user"

	StatusPending   = "pending"
	StatusRejected  = "rejected"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var (
	ErrNotFound       = errors.New("privacy: request not found")
	ErrNotPending     = errors.New("privacy: request already decided")
	ErrInvalid        = errors.New("privacy: invalid request")
	ErrSubjectUnknown = errors.New("privacy: subject is not a user of this taxpayer")
	ErrNotExport      = errors.New("privacy: not a completed export")
	ErrExpired        = errors.New("privacy: export expired")
)

// Config holds the configurable retention settings (docs/kvkk.md §7).
type Config struct {
	// AccountingRetentionYears: accounting records and the taxpayer identity
	// are kept this many full years after the year the engagement ended (VUK
	// md. 253: 5). Reported on erasure; nothing deletes them before.
	AccountingRetentionYears int
	// ExportTTL: how long a completed export stays downloadable.
	ExportTTL time.Duration
}

func (c Config) withDefaults() Config {
	if c.AccountingRetentionYears <= 0 {
		c.AccountingRetentionYears = 5
	}
	if c.ExportTTL <= 0 {
		c.ExportTTL = 7 * 24 * time.Hour
	}
	return c
}

// Service runs data subject requests.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	cfg  Config
	now  func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{pool: pool, q: db.New(pool), cfg: cfg.withDefaults(), now: time.Now}
}

// CreateInput opens a request.
type CreateInput struct {
	Kind        string
	SubjectType string
	// SubjectUser is the taxpayer user's uuid (subject "user").
	SubjectUser uuid.UUID
	Reason      string
}

// Create opens a pending request on the engagement.
func (s *Service) Create(ctx context.Context, eng engagementctx.Scope, actor int64, in CreateInput) (db.PrivacyRequest, error) {
	if in.Kind != KindExport && in.Kind != KindErasure {
		return db.PrivacyRequest{}, fmt.Errorf("%w: kind", ErrInvalid)
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) > 2000 {
		return db.PrivacyRequest{}, fmt.Errorf("%w: reason too long", ErrInvalid)
	}
	p := db.InsertPrivacyRequestParams{
		OrganizationID: eng.OrganizationID, EngagementID: eng.ID, Kind: in.Kind, Reason: reason,
		RequestedBy: actorID(actor),
	}
	switch in.SubjectType {
	case SubjectTaxpayer:
		p.SubjectType = SubjectTaxpayer
	case SubjectUser:
		u, err := s.q.GetPrivacyUserByUUID(ctx, in.SubjectUser)
		if err != nil {
			return db.PrivacyRequest{}, ErrSubjectUnknown
		}
		ties, err := s.q.CountPrivacyUserTies(ctx, db.CountPrivacyUserTiesParams{UserID: u.ID, TaxpayerID: eng.TaxpayerID})
		if err != nil {
			return db.PrivacyRequest{}, err
		}
		if ties.ThisTaxpayer == 0 {
			return db.PrivacyRequest{}, ErrSubjectUnknown
		}
		p.SubjectType, p.SubjectUserID = SubjectUser, pgtype.Int8{Int64: u.ID, Valid: true}
	default:
		return db.PrivacyRequest{}, fmt.Errorf("%w: subject_type", ErrInvalid)
	}
	return s.q.InsertPrivacyRequest(ctx, p)
}

// List returns the engagement's requests, newest first.
func (s *Service) List(ctx context.Context, eng engagementctx.Scope, limit, offset int32) ([]db.PrivacyRequest, error) {
	return s.q.ListPrivacyRequests(ctx, db.ListPrivacyRequestsParams{
		OrganizationID: eng.OrganizationID, EngagementID: eng.ID, Lim: limit, Off: offset,
	})
}

// Get returns one request.
func (s *Service) Get(ctx context.Context, eng engagementctx.Scope, id uuid.UUID) (db.PrivacyRequest, error) {
	r, err := s.q.GetPrivacyRequest(ctx, db.GetPrivacyRequestParams{OrganizationID: eng.OrganizationID, EngagementID: eng.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PrivacyRequest{}, ErrNotFound
	}
	return r, err
}

// Reject closes a pending request without running it.
func (s *Service) Reject(ctx context.Context, eng engagementctx.Scope, actor int64, id uuid.UUID, note string) (db.PrivacyRequest, error) {
	return s.decide(ctx, s.q, eng, actor, id, StatusRejected, note, map[string]any{}, pgtype.Timestamptz{})
}

// Approve runs a pending request. An export becomes downloadable until
// ExportTTL; an erasure anonymises in the same transaction as the decision,
// so a failure leaves the request pending and nothing half done.
func (s *Service) Approve(ctx context.Context, eng engagementctx.Scope, actor int64, id uuid.UUID, note string) (db.PrivacyRequest, error) {
	req, err := s.Get(ctx, eng, id)
	if err != nil {
		return db.PrivacyRequest{}, err
	}
	if req.Status != StatusPending {
		return db.PrivacyRequest{}, ErrNotPending
	}
	if req.Kind == KindExport {
		exp := pgtype.Timestamptz{Time: s.now().Add(s.cfg.ExportTTL), Valid: true}
		return s.decide(ctx, s.q, eng, actor, id, StatusCompleted, note, map[string]any{"export_expires_at": exp.Time.UTC()}, exp)
	}
	var out db.PrivacyRequest
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		summary, err := s.erase(ctx, q, eng, req)
		if err != nil {
			return err
		}
		out, err = s.decide(ctx, q, eng, actor, id, StatusCompleted, note, summary, pgtype.Timestamptz{})
		return err
	})
	return out, err
}

func (s *Service) decide(ctx context.Context, q *db.Queries, eng engagementctx.Scope, actor int64, id uuid.UUID, status, note string,
	summary map[string]any, expires pgtype.Timestamptz,
) (db.PrivacyRequest, error) {
	body, err := json.Marshal(summary)
	if err != nil {
		return db.PrivacyRequest{}, err
	}
	r, err := q.DecidePrivacyRequest(ctx, db.DecidePrivacyRequestParams{
		Status: status, DecidedBy: actorID(actor), DecisionNote: strings.TrimSpace(note),
		Summary: body, ExpiresAt: expires,
		OrganizationID: eng.OrganizationID, EngagementID: eng.ID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.Get(ctx, eng, id); gerr != nil {
			return db.PrivacyRequest{}, gerr
		}
		return db.PrivacyRequest{}, ErrNotPending
	}
	return r, err
}

// erase anonymises what the office may erase and reports what it kept.
func (s *Service) erase(ctx context.Context, q *db.Queries, eng engagementctx.Scope, req db.PrivacyRequest) (map[string]any, error) {
	summary := map[string]any{"retained": s.retained()}
	var users []int64
	switch req.SubjectType {
	case SubjectUser:
		if !req.SubjectUserID.Valid {
			return nil, ErrSubjectUnknown
		}
		users = []int64{req.SubjectUserID.Int64}
	default:
		members, err := q.ListPrivacyTaxpayerMembers(ctx, eng.TaxpayerID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			users = append(users, m.UserID)
		}
		masked := maskedEmail(req.Uuid)
		inv, err := q.MaskPrivacyInvitations(ctx, db.MaskPrivacyInvitationsParams{Email: masked, OrganizationID: eng.OrganizationID, EngagementID: eng.ID})
		if err != nil {
			return nil, err
		}
		onb, err := q.MaskPrivacyOnboardingContacts(ctx, db.MaskPrivacyOnboardingContactsParams{Email: masked, OrganizationID: eng.OrganizationID, TaxpayerID: int8v(eng.TaxpayerID)})
		if err != nil {
			return nil, err
		}
		summary["invitations_masked"], summary["onboarding_contacts_masked"] = inv, onb
	}
	var anonymised, unlinked, shared int
	for _, uid := range users {
		ties, err := q.CountPrivacyUserTies(ctx, db.CountPrivacyUserTiesParams{UserID: uid, TaxpayerID: eng.TaxpayerID})
		if err != nil {
			return nil, err
		}
		if ties.ThisTaxpayer > 0 {
			if _, err := q.DeletePrivacyTaxpayerMember(ctx, db.DeletePrivacyTaxpayerMemberParams{TaxpayerID: eng.TaxpayerID, UserID: uid}); err != nil {
				return nil, err
			}
			unlinked++
		}
		// A person who also works for an office or another taxpayer keeps the
		// account: this office only removes its own link.
		if ties.Organizations > 0 || ties.OtherTaxpayers > 0 {
			shared++
			continue
		}
		if err := anonymiseUser(ctx, q, uid); err != nil {
			return nil, err
		}
		anonymised++
	}
	summary["users_anonymised"], summary["users_unlinked"], summary["users_kept_shared"] = anonymised, unlinked, shared
	return summary, nil
}

func anonymiseUser(ctx context.Context, q *db.Queries, userID int64) error {
	u, err := q.GetPrivacyUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, del := range []func(context.Context, int64) (int64, error){
		q.DeletePrivacyUserSessions, q.DeletePrivacyUserDevices, q.DeletePrivacyUserPushSubscriptions,
		q.DeletePrivacyUserPasskeys, q.DeletePrivacyUserOAuth,
	} {
		if _, err := del(ctx, userID); err != nil {
			return err
		}
	}
	if _, err := q.DeletePrivacyUserNotifications(ctx, int8v(userID)); err != nil {
		return err
	}
	if _, err := q.DeletePrivacyUserOTPs(ctx, db.DeletePrivacyUserOTPsParams{UserID: pgtype.Int8{Int64: userID, Valid: true}, Email: u.Email}); err != nil {
		return err
	}
	return q.AnonymizePrivacyUser(ctx, db.AnonymizePrivacyUserParams{ID: userID, Email: "deleted-" + u.Uuid.String() + "@anonymized.invalid"})
}

// maskedEmail replaces contact addresses; tied to the request for traceability.
func maskedEmail(requestID uuid.UUID) string {
	return "kvkk-" + requestID.String()[:8] + "@anonymized.invalid"
}

// retained reports what erasure leaves in place and until when.
func (s *Service) retained() map[string]any {
	end := s.now()
	until := time.Date(end.Year()+1+s.cfg.AccountingRetentionYears, 1, 1, 0, 0, 0, 0, time.UTC)
	return map[string]any{
		"accounting_records": "kept: invoices, bank movements, expenses, retail sales, journals (VUK md. 253)",
		"taxpayer_identity":  "kept: tax id, title, tax office",
		"legal_acceptances":  "kept as evidence of consent",
		"audit_log":          "kept",
		"retention_years":    s.cfg.AccountingRetentionYears,
		"retention_until":    until.Format(time.DateOnly),
	}
}

func actorID(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
