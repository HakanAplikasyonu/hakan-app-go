package taxpayers

import (
	"context"
	"errors"

	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Request statuses.
const (
	RequestPending   = "pending"
	RequestApproved  = "approved"
	RequestRejected  = "rejected"
	RequestCancelled = "cancelled"
	RequestExpired   = "expired" // derived: pending and past expires_at
)

// Consent outcomes.
const (
	OutcomeApproved  = "approved"
	OutcomeForwarded = "forwarded"
)

// DefaultMinRequestDuration pads the office's add-mükellef answer so its
// timing never depends on what happened inside (D1b).
const DefaultMinRequestDuration = 400 * time.Millisecond

// ConsentTemplate is the email sent to the mükellef.
const ConsentTemplate = "taxpayer.onboarding_consent"

// ErrNotOwner: the caller is not an owner of the company.
var ErrNotOwner = errors.New("taxpayers: only company owners can decide")

// OnboardingConfig controls the consent flow.
type OnboardingConfig struct {
	TTL         time.Duration
	FrontendURL string
	// MinRequestDuration is the fixed minimum duration of an office's
	// add-mükellef call. Zero means DefaultMinRequestDuration.
	MinRequestDuration time.Duration
}

// Onboarding is the add-mükellef flow (issue #7). The office side never reads
// the platform's taxpayer records: whether the tax id exists only matters once
// the mükellef approves (D1b).
type Onboarding struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	notifier Notifier
	inv      orgctx.Invalidator
	cfg      OnboardingConfig
	log      *slog.Logger
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration)
}

// NewOnboarding builds the flow. notifier and inv may be nil.
func NewOnboarding(pool *pgxpool.Pool, notifier Notifier, inv orgctx.Invalidator, cfg OnboardingConfig, log *slog.Logger) *Onboarding {
	if cfg.TTL <= 0 {
		cfg.TTL = 14 * 24 * time.Hour
	}
	if cfg.MinRequestDuration <= 0 {
		cfg.MinRequestDuration = DefaultMinRequestDuration
	}
	if log == nil {
		log = slog.Default()
	}
	return &Onboarding{
		pool: pool, q: db.New(pool), notifier: notifier, inv: inv, cfg: cfg, log: log,
		now: time.Now, sleep: sleepCtx,
	}
}

// OnboardingRequest is a request as the office sees it: only what it entered,
// plus the status.
type OnboardingRequest struct {
	UUID           uuid.UUID  `json:"uuid"`
	Kind           string     `json:"kind"`
	TaxID          string     `json:"tax_id"`
	Title          string     `json:"title"`
	ContactEmail   string     `json:"contact_email"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	EngagementUUID *uuid.UUID `json:"engagement_uuid"`
}

// RequestInput is what the office enters.
type RequestInput struct {
	OrgID        int64
	OrgName      string
	UserID       int64
	Kind         string
	TaxID        string
	Title        string
	ContactEmail string
}

// Request stores an add-mükellef request and emails the consent link. The
// work, the answer and (through padding) the duration are the same whether
// or not the tax id is on the platform: this code path never looks.
// Validation errors are returned without padding: they depend only on the
// input, never on stored data.
func (o *Onboarding) Request(ctx context.Context, in RequestInput) (OnboardingRequest, error) {
	start := o.now()
	v, _, err := Validate(CreateInput{Kind: in.Kind, TaxID: in.TaxID, Title: in.Title})
	if err != nil {
		return OnboardingRequest{}, err
	}
	email, err := normalizeEmail(in.ContactEmail)
	if err != nil {
		return OnboardingRequest{}, err
	}
	defer o.pad(ctx, start)

	token, hash, err := newToken()
	if err != nil {
		return OnboardingRequest{}, err
	}
	row, err := o.q.CreateOnboardingRequest(ctx, db.CreateOnboardingRequestParams{
		OrganizationID: in.OrgID, Kind: v.Kind, TaxID: v.TaxID, Title: v.Title, ContactEmail: email,
		TokenHash: hash, RequestedBy: pgtype.Int8{Int64: in.UserID, Valid: in.UserID > 0},
		ExpiresAt: pgtype.Timestamptz{Time: o.now().Add(o.cfg.TTL), Valid: true},
	})
	if err != nil {
		return OnboardingRequest{}, err
	}
	o.sendConsentEmail(ctx, in.OrgName, row, token)
	return o.view(row, nil), nil
}

// pad waits until start + MinRequestDuration.
func (o *Onboarding) pad(ctx context.Context, start time.Time) {
	if rest := o.cfg.MinRequestDuration - o.now().Sub(start); rest > 0 {
		o.sleep(ctx, rest)
	}
}

// List returns the office's requests.
func (o *Onboarding) List(ctx context.Context, orgID int64, limit, offset int32) ([]OnboardingRequest, int64, error) {
	rows, err := o.q.ListOnboardingRequestsByOrg(ctx, db.ListOnboardingRequestsByOrgParams{
		OrganizationID: orgID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := o.q.CountOnboardingRequestsByOrg(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]OnboardingRequest, 0, len(rows))
	for _, r := range rows {
		var eng *uuid.UUID
		if r.EngagementUuid.Valid {
			u := uuid.UUID(r.EngagementUuid.Bytes)
			eng = &u
		}
		out = append(out, o.view(db.TaxpayerOnboardingRequest{
			Uuid: r.Uuid, Kind: r.Kind, TaxID: r.TaxID, Title: r.Title, ContactEmail: r.ContactEmail,
			Status: r.Status, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
		}, eng))
	}
	return out, total, nil
}

// Cancel withdraws a pending request of the office.
func (o *Onboarding) Cancel(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := o.q.CancelOnboardingRequest(ctx, db.CancelOnboardingRequestParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ConsentPreview is what the holder of a consent link sees.
type ConsentPreview struct {
	Organization struct {
		Name string `json:"name"`
	} `json:"organization"`
	Kind         string    `json:"kind"`
	TaxID        string    `json:"tax_id"`
	Title        string    `json:"title"`
	ContactEmail string    `json:"contact_email"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ConsentOutcome is the result of an approval.
type ConsentOutcome struct {
	Status       string     `json:"status"`
	TaxpayerUUID *uuid.UUID `json:"taxpayer_uuid"`
}

// Preview describes a consent link.
func (o *Onboarding) Preview(ctx context.Context, token string) (ConsentPreview, error) {
	row, err := o.lookup(ctx, token)
	if err != nil {
		return ConsentPreview{}, err
	}
	var out ConsentPreview
	out.Organization.Name = row.OrganizationName
	out.Kind, out.TaxID, out.Title, out.ContactEmail = row.Kind, row.TaxID, row.Title, row.ContactEmail
	out.Status = o.status(row.Status, row.ExpiresAt)
	out.ExpiresAt = row.ExpiresAt.Time
	return out, nil
}

// CheckSignupInvitation lets the consent link's address sign up while
// registration is closed (auth InvitationGate).
func (o *Onboarding) CheckSignupInvitation(ctx context.Context, token, email string) error {
	row, err := o.lookup(ctx, token)
	if err != nil {
		return err
	}
	if o.status(row.Status, row.ExpiresAt) != RequestPending {
		return ErrInvitationUnavailable
	}
	if !strings.EqualFold(strings.TrimSpace(email), row.ContactEmail) {
		return ErrInvitationEmailMismatch
	}
	return nil
}

// ApproveByLink approves through the emailed link. The signed-in account
// must be the request's contact address.
func (o *Onboarding) ApproveByLink(ctx context.Context, token string, userID int64) (ConsentOutcome, error) {
	row, err := o.linkHolder(ctx, token, userID)
	if err != nil {
		return ConsentOutcome{}, err
	}
	return o.approve(ctx, row.ID, userID, false)
}

// RejectByLink declines through the emailed link.
func (o *Onboarding) RejectByLink(ctx context.Context, token string, userID int64) error {
	row, err := o.linkHolder(ctx, token, userID)
	if err != nil {
		return err
	}
	return o.reject(ctx, row.ID, userID)
}

// PendingRequest is an office's request as the company's owner sees it.
type PendingRequest struct {
	UUID         uuid.UUID `json:"uuid"`
	Organization struct {
		Name string `json:"name"`
	} `json:"organization"`
	Title     string    `json:"title"`
	TaxID     string    `json:"tax_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PendingForTaxpayer lists open requests for the company's own tax id
// (owners only).
func (o *Onboarding) PendingForTaxpayer(ctx context.Context, s taxpayerctx.Scope) ([]PendingRequest, error) {
	if s.Role != taxpayerctx.RoleOwner {
		return nil, ErrNotOwner
	}
	rows, err := o.q.ListPendingOnboardingRequestsForTaxID(ctx, s.TaxID)
	if err != nil {
		return nil, err
	}
	out := make([]PendingRequest, 0, len(rows))
	for _, r := range rows {
		p := PendingRequest{UUID: r.Uuid, Title: r.Title, TaxID: r.TaxID, CreatedAt: r.CreatedAt.Time, ExpiresAt: r.ExpiresAt.Time}
		p.Organization.Name = r.OrganizationName
		out = append(out, p)
	}
	return out, nil
}

// ApproveAsOwner approves a request for the company from its mükellef panel.
func (o *Onboarding) ApproveAsOwner(ctx context.Context, s taxpayerctx.Scope, id uuid.UUID, userID int64) (ConsentOutcome, error) {
	row, err := o.ownerRequest(ctx, s, id)
	if err != nil {
		return ConsentOutcome{}, err
	}
	return o.approve(ctx, row.ID, userID, true)
}

// RejectAsOwner declines a request for the company from its mükellef panel.
func (o *Onboarding) RejectAsOwner(ctx context.Context, s taxpayerctx.Scope, id uuid.UUID, userID int64) error {
	row, err := o.ownerRequest(ctx, s, id)
	if err != nil {
		return err
	}
	return o.reject(ctx, row.ID, userID)
}

func (o *Onboarding) ownerRequest(ctx context.Context, s taxpayerctx.Scope, id uuid.UUID) (db.GetPendingOnboardingRequestForTaxIDRow, error) {
	if s.Role != taxpayerctx.RoleOwner {
		return db.GetPendingOnboardingRequestForTaxIDRow{}, ErrNotOwner
	}
	row, err := o.q.GetPendingOnboardingRequestForTaxID(ctx, db.GetPendingOnboardingRequestForTaxIDParams{Uuid: id, TaxID: s.TaxID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func (o *Onboarding) linkHolder(ctx context.Context, token string, userID int64) (db.GetOnboardingRequestByTokenHashRow, error) {
	row, err := o.lookup(ctx, token)
	if err != nil {
		return row, err
	}
	if o.status(row.Status, row.ExpiresAt) != RequestPending {
		return row, ErrInvitationUnavailable
	}
	user, err := o.q.GetUserByID(ctx, userID)
	if err != nil {
		return row, err
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), row.ContactEmail) {
		return row, ErrInvitationEmailMismatch
	}
	return row, nil
}

// approve is the single place where an office's request meets the platform's
// taxpayer records. In one transaction:
//
//   - a new tax id becomes a taxpayer and the approver its owner;
//   - an existing taxpayer with owner users needs one of them (asOwner, or
//     the link holder being an owner); otherwise the outcome is "forwarded"
//     and nothing changes;
//   - an existing taxpayer without owners gets the approver as owner;
//   - an active engagement with another office ends (office change, D2) and
//     the new office's engagement opens.
func (o *Onboarding) approve(ctx context.Context, requestID, userID int64, asOwner bool) (ConsentOutcome, error) {
	var (
		out          ConsentOutcome
		invalidate   []int64
		taxpayerUUID uuid.UUID
	)
	err := o.withTx(ctx, func(q *db.Queries) error {
		req, err := q.LockOnboardingRequest(ctx, requestID)
		if err != nil {
			return mapNoRows(err, ErrInvitationNotFound)
		}
		if o.status(req.Status, req.ExpiresAt) != RequestPending {
			return ErrInvitationUnavailable
		}
		tp, err := q.LockTaxpayerByTaxID(ctx, req.TaxID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			tp, err = q.CreateTaxpayer(ctx, db.CreateTaxpayerParams{Kind: req.Kind, TaxID: req.TaxID, Title: req.Title})
			if err != nil {
				return err
			}
		case err != nil:
			return err
		}
		owners, err := q.ListTaxpayerOwnerUserIDs(ctx, tp.ID)
		if err != nil {
			return err
		}
		isOwner := false
		for _, id := range owners {
			isOwner = isOwner || id == userID
		}
		if len(owners) > 0 && !isOwner {
			if asOwner {
				return ErrNotOwner
			}
			out = ConsentOutcome{Status: OutcomeForwarded}
			return nil
		}
		if !isOwner {
			if _, err := q.GetTaxpayerMember(ctx, db.GetTaxpayerMemberParams{TaxpayerID: tp.ID, UserID: userID}); errors.Is(err, pgx.ErrNoRows) {
				if err := q.AddTaxpayerMember(ctx, db.AddTaxpayerMemberParams{TaxpayerID: tp.ID, UserID: userID, Role: taxpayerctx.RoleOwner}); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		var engagementID int64
		active, err := q.GetActiveEngagementForTaxpayer(ctx, tp.ID)
		switch {
		case err == nil && active.OrganizationID == req.OrganizationID:
			engagementID = active.ID
		case err == nil:
			if err := q.EndEngagementByID(ctx, active.ID); err != nil {
				return err
			}
			invalidate = append(invalidate, active.OrganizationID)
			fallthrough
		case errors.Is(err, pgx.ErrNoRows):
			eng, err := q.CreateEngagement(ctx, db.CreateEngagementParams{OrganizationID: req.OrganizationID, TaxpayerID: tp.ID})
			if err != nil {
				return err
			}
			engagementID = eng.ID
		default:
			return err
		}
		invalidate = append(invalidate, req.OrganizationID)
		if err := q.DecideOnboardingRequest(ctx, db.DecideOnboardingRequestParams{
			Status: RequestApproved, DecidedBy: pgtype.Int8{Int64: userID, Valid: true},
			TaxpayerID: pgtype.Int8{Int64: tp.ID, Valid: true}, EngagementID: pgtype.Int8{Int64: engagementID, Valid: true},
			ID: req.ID,
		}); err != nil {
			return err
		}
		taxpayerUUID = tp.Uuid
		out = ConsentOutcome{Status: OutcomeApproved, TaxpayerUUID: &taxpayerUUID}
		return nil
	})
	if err != nil {
		return ConsentOutcome{}, err
	}
	for _, id := range invalidate {
		_ = orgctx.Invalidate(ctx, o.inv, id)
	}
	return out, nil
}

func (o *Onboarding) reject(ctx context.Context, requestID, userID int64) error {
	return o.withTx(ctx, func(q *db.Queries) error {
		req, err := q.LockOnboardingRequest(ctx, requestID)
		if err != nil {
			return mapNoRows(err, ErrInvitationNotFound)
		}
		if o.status(req.Status, req.ExpiresAt) != RequestPending {
			return ErrInvitationUnavailable
		}
		return q.DecideOnboardingRequest(ctx, db.DecideOnboardingRequestParams{
			Status: RequestRejected, DecidedBy: pgtype.Int8{Int64: userID, Valid: true}, ID: req.ID,
		})
	})
}

func (o *Onboarding) withTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(o.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (o *Onboarding) lookup(ctx context.Context, token string) (db.GetOnboardingRequestByTokenHashRow, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return db.GetOnboardingRequestByTokenHashRow{}, ErrInvitationNotFound
	}
	row, err := o.q.GetOnboardingRequestByTokenHash(ctx, hashToken(token))
	if err != nil {
		return row, mapNoRows(err, ErrInvitationNotFound)
	}
	if row.OrganizationDeleted {
		return row, ErrInvitationNotFound
	}
	return row, nil
}

func (o *Onboarding) status(stored string, expires pgtype.Timestamptz) string {
	if stored == RequestPending && !o.now().Before(expires.Time) {
		return RequestExpired
	}
	return stored
}

func (o *Onboarding) view(r db.TaxpayerOnboardingRequest, eng *uuid.UUID) OnboardingRequest {
	return OnboardingRequest{
		UUID: r.Uuid, Kind: r.Kind, TaxID: r.TaxID, Title: r.Title, ContactEmail: r.ContactEmail,
		Status: o.status(r.Status, r.ExpiresAt), CreatedAt: r.CreatedAt.Time, ExpiresAt: r.ExpiresAt.Time,
		EngagementUUID: eng,
	}
}

func (o *Onboarding) sendConsentEmail(ctx context.Context, orgName string, row db.TaxpayerOnboardingRequest, token string) {
	if o.notifier == nil {
		return
	}
	days := int(o.cfg.TTL.Hours() / 24)
	if days < 1 {
		days = 1
	}
	email := row.ContactEmail
	_, err := o.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		Channels:      []string{notifmodel.ChannelEmail},
		Recipient:     &email,
		TemplateCode:  ConsentTemplate,
		SourceEvent:   ConsentTemplate,
		SecurityEmail: true,
		// The language is not looked up from an account: that lookup would
		// depend on stored data. Turkish is the product language.
		Language: "tr",
		TemplateVars: map[string]string{
			"org_name":     orgName,
			"title":        row.Title,
			"tax_id":       row.TaxID,
			"link":         strings.TrimRight(o.cfg.FrontendURL, "/") + "/consent/" + token,
			"expires_days": strconv.Itoa(days),
		},
	})
	if err != nil {
		o.log.Warn("taxpayer_onboarding_email_enqueue_failed", "error", err, "organization_id", row.OrganizationID)
	}
}

func mapNoRows(err, to error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return to
	}
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
