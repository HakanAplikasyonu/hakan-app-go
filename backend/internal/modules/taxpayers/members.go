package taxpayers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Invitation statuses (same words as office invitations).
const (
	InvitationPending  = "pending"
	InvitationAccepted = "accepted"
	InvitationRevoked  = "revoked"
	InvitationExpired  = "expired"
)

// Invitation errors reuse the organizations sentinels, so the auth module's
// invitation sign-up (InvitationGate) and its error mapping work unchanged.
var (
	ErrInvitationNotFound      = orgusecase.ErrInvitationNotFound
	ErrInvitationUnavailable   = orgusecase.ErrInvitationUnavailable
	ErrInvitationEmailMismatch = orgusecase.ErrInvitationEmailMismatch
	ErrAlreadyMember           = errors.New("taxpayers: already a member of this taxpayer")
	ErrInvitationsDisabled     = orgusecase.ErrInvitationsDisabled
)

// MemberStore is the persistence of memberships and invitations.
type MemberStore interface {
	ResolveMembership(ctx context.Context, taxpayerUUID uuid.UUID, userID int64) (db.ResolveTaxpayerMembershipRow, error)
	ListMembershipsByUser(ctx context.Context, userID int64) ([]db.ListTaxpayerMembershipsByUserRow, error)
	GetMember(ctx context.Context, taxpayerID, userID int64) (db.TaxpayerMember, error)
	CreateInvitation(ctx context.Context, p db.CreateTaxpayerInvitationParams) (db.TaxpayerInvitation, error)
	ListInvitations(ctx context.Context, orgID, engagementID int64) ([]db.ListTaxpayerInvitationsByEngagementRow, error)
	RevokeInvitation(ctx context.Context, orgID, engagementID int64, id uuid.UUID) (int64, error)
	GetInvitationByTokenHash(ctx context.Context, hash string) (db.GetTaxpayerInvitationByTokenHashRow, error)
	// ResendInvitation gives an open invitation a new token and expiry.
	ResendInvitation(ctx context.Context, p db.ResendTaxpayerInvitationParams) (db.TaxpayerInvitation, error)
	// AcceptInvitation consumes the invitation and adds the member in one
	// transaction.
	AcceptInvitation(ctx context.Context, invitationID, taxpayerID, userID int64, role string) error
	GetUser(ctx context.Context, id int64) (db.User, error)
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
}

// Notifier enqueues outbound notifications.
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// InvitationConfig controls invitation links.
type InvitationConfig struct {
	TTL         time.Duration
	FrontendURL string
	// MailUnavailable, when set, is the reason code (EmailError*) why
	// outbound mail is known not to work (see MailUnavailableReason). The
	// email is still queued, but the response tells the office to share the
	// link itself.
	MailUnavailable string
}

// Reason codes of InviteResult.EmailError: why the invitation email may not
// reach the invitee. The invitation exists and its link works either way.
const (
	// EmailErrorDisabled: the server runs without a notifier (no email).
	EmailErrorDisabled = "mail_disabled"
	// EmailErrorNotConfigured: outbound mail (SMTP_HOST) is not set up.
	EmailErrorNotConfigured = "mail_not_configured"
	// EmailErrorEnqueueFailed: the email could not be queued.
	EmailErrorEnqueueFailed = "enqueue_failed"
)

// MailUnavailableReason reports EmailErrorNotConfigured when production
// sends mail to a loopback SMTP host: SMTP_HOST is unset there (the default
// 127.0.0.1 is the development MailHog), so every email fails in the worker.
func MailUnavailableReason(env, smtpHost string) string {
	if env != "production" {
		return ""
	}
	switch strings.ToLower(strings.Trim(strings.TrimSpace(smtpHost), "[]")) {
	case "", "127.0.0.1", "localhost", "::1", "0.0.0.0":
		return EmailErrorNotConfigured
	}
	return ""
}

// Members implements taxpayerctx.Resolver, the /me taxpayer list and the
// office -> mükellef invitation flow (D15: mükellef only joins by an office
// invitation).
type Members struct {
	store    MemberStore
	notifier Notifier
	cfg      InvitationConfig
	log      *slog.Logger
	now      func() time.Time
}

var _ taxpayerctx.Resolver = (*Members)(nil)

// NewMembers builds the service. notifier may be nil (no email).
func NewMembers(store MemberStore, notifier Notifier, cfg InvitationConfig, log *slog.Logger) *Members {
	if log == nil {
		log = slog.Default()
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 7 * 24 * time.Hour
	}
	return &Members{store: store, notifier: notifier, cfg: cfg, log: log, now: time.Now}
}

// ResolveTaxpayer implements taxpayerctx.Resolver.
func (m *Members) ResolveTaxpayer(ctx context.Context, taxpayerUUID uuid.UUID, userID int64) (taxpayerctx.Scope, error) {
	row, err := m.store.ResolveMembership(ctx, taxpayerUUID, userID)
	if err != nil {
		if errors.Is(err, errNoRows) {
			return taxpayerctx.Scope{}, taxpayerctx.ErrNotMember
		}
		return taxpayerctx.Scope{}, err
	}
	return taxpayerctx.Scope{
		ID: row.ID, UUID: row.Uuid, Kind: row.Kind, TaxID: row.TaxID, Title: row.Title, Role: row.Role,
	}, nil
}

// ListForUser returns the caller's taxpayer memberships for /auth/me.
func (m *Members) ListForUser(ctx context.Context, userID int64) ([]authmodel.MeTaxpayer, error) {
	rows, err := m.store.ListMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]authmodel.MeTaxpayer, 0, len(rows))
	for _, r := range rows {
		out = append(out, authmodel.MeTaxpayer{UUID: r.Uuid, Kind: r.Kind, Title: r.Title, Role: r.Role})
	}
	return out, nil
}

// Invitation is an office -> mükellef invitation as the office sees it.
type Invitation struct {
	UUID      uuid.UUID `json:"uuid"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	InvitedBy *string   `json:"invited_by"`
}

// InviteResult is a new (or resent) invitation as its sender sees it: the
// link itself, so the office can share it when the email does not arrive.
type InviteResult struct {
	Invitation
	// InviteURL is the same link the email carries (/invite/m/{token}). It
	// is returned once, to the sender; only the token hash is stored.
	InviteURL string `json:"invite_url"`
	// EmailSent is true when the email was queued for delivery and mail is
	// configured. False: share InviteURL yourself (EmailError says why).
	EmailSent  bool    `json:"email_sent"`
	EmailError *string `json:"email_error"`
}

// InviteInput is a new invitation from an engagement.
type InviteInput struct {
	OrgID         int64
	OrgName       string
	EngagementID  int64
	TaxpayerID    int64
	Email         string
	Role          string
	InviterUserID int64
}

// Invite stores an invitation for the engagement's taxpayer and enqueues the
// email. Older open invitations for the same address on this engagement are
// replaced. The caller has passed RequireEngagement (active engagement).
func (m *Members) Invite(ctx context.Context, in InviteInput) (InviteResult, error) {
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return InviteResult{}, err
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role == "" {
		role = taxpayerctx.RoleOwner
	}
	if role != taxpayerctx.RoleOwner && role != taxpayerctx.RoleEmployee {
		return InviteResult{}, fmt.Errorf("%w: role must be owner or employee", ErrInvalidRequest)
	}
	token, hash, err := newToken()
	if err != nil {
		return InviteResult{}, err
	}
	row, err := m.store.CreateInvitation(ctx, db.CreateTaxpayerInvitationParams{
		OrganizationID: in.OrgID, EngagementID: in.EngagementID, Email: email, Role: role,
		TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: m.now().Add(m.cfg.TTL), Valid: true},
		InvitedBy: pgtype.Int8{Int64: in.InviterUserID, Valid: in.InviterUserID > 0},
	})
	if err != nil {
		return InviteResult{}, err
	}
	return m.deliver(ctx, in, row, token), nil
}

// ResendInput resends an open invitation of an engagement.
type ResendInput struct {
	OrgID         int64
	OrgName       string
	EngagementID  int64
	UUID          uuid.UUID
	InviterUserID int64
}

// Resend gives an open (or expired) invitation a new link and expiry and
// emails it again. The previous link stops working: the link is never
// stored, so a new one is the only way to show it to the office again.
func (m *Members) Resend(ctx context.Context, in ResendInput) (InviteResult, error) {
	token, hash, err := newToken()
	if err != nil {
		return InviteResult{}, err
	}
	row, err := m.store.ResendInvitation(ctx, db.ResendTaxpayerInvitationParams{
		OrganizationID: in.OrgID, EngagementID: in.EngagementID, Uuid: in.UUID,
		TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: m.now().Add(m.cfg.TTL), Valid: true},
	})
	if err != nil {
		if errors.Is(err, errNoRows) {
			return InviteResult{}, ErrInvitationNotFound
		}
		return InviteResult{}, err
	}
	return m.deliver(ctx, InviteInput{
		OrgID: in.OrgID, OrgName: in.OrgName, EngagementID: in.EngagementID,
		Email: row.Email, Role: row.Role, InviterUserID: in.InviterUserID,
	}, row, token), nil
}

// deliver emails a stored invitation and builds the sender's view of it.
// A failed email never fails the invitation: the link works and the
// response says to share it.
func (m *Members) deliver(ctx context.Context, in InviteInput, row db.TaxpayerInvitation, token string) InviteResult {
	var inviter *string
	if in.InviterUserID > 0 {
		if u, err := m.store.GetUser(ctx, in.InviterUserID); err == nil {
			inviter = fullName(pgtype.Text{String: u.Name, Valid: true}, pgtype.Text{String: u.Surname, Valid: true})
		}
	}
	out := InviteResult{
		Invitation: Invitation{
			UUID: row.Uuid, Email: row.Email, Role: row.Role, Status: InvitationPending,
			ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time, InvitedBy: inviter,
		},
		InviteURL: m.inviteURL(token),
	}
	if reason := m.sendEmail(ctx, in, row, token, inviter); reason != "" {
		out.EmailError = &reason
	} else {
		out.EmailSent = true
	}
	return out
}

// inviteURL is the invitation link of token, the one the email carries.
func (m *Members) inviteURL(token string) string {
	return strings.TrimRight(m.cfg.FrontendURL, "/") + "/invite/m/" + token
}

// ListInvitations lists the engagement's invitations (newest first, max 100).
func (m *Members) ListInvitations(ctx context.Context, orgID, engagementID int64) ([]Invitation, error) {
	rows, err := m.store.ListInvitations(ctx, orgID, engagementID)
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := make([]Invitation, 0, len(rows))
	for _, r := range rows {
		out = append(out, Invitation{
			UUID: r.Uuid, Email: r.Email, Role: r.Role,
			Status:    invitationStatus(r.AcceptedAt, r.RevokedAt, r.ExpiresAt, now),
			ExpiresAt: r.ExpiresAt.Time, CreatedAt: r.CreatedAt.Time,
			InvitedBy: fullName(r.InviterName, r.InviterSurname),
		})
	}
	return out, nil
}

// RevokeInvitation cancels an open invitation of the engagement.
func (m *Members) RevokeInvitation(ctx context.Context, orgID, engagementID int64, id uuid.UUID) error {
	n, err := m.store.RevokeInvitation(ctx, orgID, engagementID, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

// InvitationPreview is what the holder of an invitation link sees.
type InvitationPreview struct {
	Taxpayer struct {
		Title string `json:"title"`
	} `json:"taxpayer"`
	Organization struct {
		Name string `json:"name"`
	} `json:"organization"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	InvitedBy *string   `json:"invited_by"`
}

// AcceptedInvitation tells the frontend where to go.
type AcceptedInvitation struct {
	TaxpayerUUID uuid.UUID `json:"taxpayer_uuid"`
	Role         string    `json:"role"`
}

// Preview describes an invitation to the holder of its link. An invitation
// whose engagement has ended is shown as revoked.
func (m *Members) Preview(ctx context.Context, token string) (InvitationPreview, error) {
	row, err := m.lookup(ctx, token)
	if err != nil {
		return InvitationPreview{}, err
	}
	var out InvitationPreview
	out.Taxpayer.Title = row.TaxpayerTitle
	out.Organization.Name = row.OrganizationName
	out.Email = row.Email
	out.Role = row.Role
	out.Status = m.status(row)
	out.ExpiresAt = row.ExpiresAt.Time
	out.InvitedBy = fullName(row.InviterName, row.InviterSurname)
	return out, nil
}

// CheckSignupInvitation implements the auth module's InvitationGate for
// mükellef invitations: the token is open and addressed to email.
func (m *Members) CheckSignupInvitation(ctx context.Context, token, email string) error {
	row, err := m.lookup(ctx, token)
	if err != nil {
		return err
	}
	if m.status(row) != InvitationPending {
		return ErrInvitationUnavailable
	}
	if !strings.EqualFold(strings.TrimSpace(email), row.Email) {
		return ErrInvitationEmailMismatch
	}
	return nil
}

// Accept turns an invitation into a taxpayer membership for the signed-in
// user, whose email must be the invited address. Single use.
func (m *Members) Accept(ctx context.Context, token string, userID int64) (AcceptedInvitation, error) {
	row, err := m.lookup(ctx, token)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	if m.status(row) != InvitationPending {
		return AcceptedInvitation{}, ErrInvitationUnavailable
	}
	user, err := m.store.GetUser(ctx, userID)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), row.Email) {
		return AcceptedInvitation{}, ErrInvitationEmailMismatch
	}
	if _, err := m.store.GetMember(ctx, row.TaxpayerID, user.ID); err == nil {
		return AcceptedInvitation{}, ErrAlreadyMember
	} else if !errors.Is(err, errNoRows) {
		return AcceptedInvitation{}, err
	}
	if err := m.store.AcceptInvitation(ctx, row.ID, row.TaxpayerID, user.ID, row.Role); err != nil {
		if errors.Is(err, errNoRows) {
			return AcceptedInvitation{}, ErrInvitationUnavailable
		}
		if errors.Is(err, errDuplicateMember) {
			return AcceptedInvitation{}, ErrAlreadyMember
		}
		return AcceptedInvitation{}, err
	}
	return AcceptedInvitation{TaxpayerUUID: row.TaxpayerUuid, Role: row.Role}, nil
}

func (m *Members) status(row db.GetTaxpayerInvitationByTokenHashRow) string {
	s := invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, m.now())
	if s == InvitationPending && (row.EngagementEndedAt.Valid || row.OrganizationDeleted) {
		return InvitationRevoked
	}
	return s
}

func (m *Members) lookup(ctx context.Context, token string) (db.GetTaxpayerInvitationByTokenHashRow, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return db.GetTaxpayerInvitationByTokenHashRow{}, ErrInvitationNotFound
	}
	row, err := m.store.GetInvitationByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, errNoRows) {
			return row, ErrInvitationNotFound
		}
		return row, err
	}
	return row, nil
}

// sendEmail queues the invitation email. It returns "" when queued with
// mail configured, otherwise the EmailError* reason (already logged). The
// log never carries the token or the link.
func (m *Members) sendEmail(ctx context.Context, in InviteInput, inv db.TaxpayerInvitation, token string, inviter *string) string {
	email, role := inv.Email, inv.Role
	logArgs := []any{
		"organization_id", in.OrgID, "engagement_id", in.EngagementID,
		"invitation_uuid", inv.Uuid.String(), "recipient_domain", emailDomain(email),
	}
	if m.notifier == nil {
		m.log.WarnContext(ctx, "taxpayer_invitation_email_skipped", append(logArgs, "reason", EmailErrorDisabled)...)
		return EmailErrorDisabled
	}
	title := ""
	if row, err := m.store.GetInvitationByTokenHash(ctx, hashToken(token)); err == nil {
		title = row.TaxpayerTitle
	}
	link := m.inviteURL(token)
	days := int(m.cfg.TTL.Hours() / 24)
	if days < 1 {
		days = 1
	}
	inviterName := ""
	if inviter != nil {
		inviterName = *inviter
	}
	roleName := "sahip"
	if role == taxpayerctx.RoleEmployee {
		roleName = "çalışan"
	}
	enqueue := notifmodel.EnqueueInput{
		Channels:      []string{notifmodel.ChannelEmail},
		Recipient:     &email,
		TemplateCode:  events.TaxpayerInvitation,
		SourceEvent:   events.TaxpayerInvitation,
		SecurityEmail: true,
		TemplateVars: map[string]string{
			"taxpayer_name": title,
			"org_name":      in.OrgName,
			"inviter_name":  inviterName,
			"role_name":     roleName,
			"link":          link,
			"expires_days":  strconv.Itoa(days),
		},
	}
	if u, err := m.store.GetUserByEmail(ctx, email); err == nil {
		enqueue.Language = u.Locale
	}
	if _, err := m.notifier.Enqueue(ctx, enqueue); err != nil {
		m.log.ErrorContext(ctx, "taxpayer_invitation_email_enqueue_failed",
			append(logArgs, "reason", EmailErrorEnqueueFailed, "error", err)...)
		return EmailErrorEnqueueFailed
	}
	if m.cfg.MailUnavailable != "" {
		m.log.WarnContext(ctx, "taxpayer_invitation_email_mail_unavailable",
			append(logArgs, "reason", m.cfg.MailUnavailable)...)
		return m.cfg.MailUnavailable
	}
	return ""
}

// emailDomain is the part of an address after "@": enough to tell which
// provider rejects mail without logging the address.
func emailDomain(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return email[i+1:]
	}
	return ""
}

func invitationStatus(accepted, revoked, expires pgtype.Timestamptz, now time.Time) string {
	switch {
	case accepted.Valid:
		return InvitationAccepted
	case revoked.Valid:
		return InvitationRevoked
	case !now.Before(expires.Time):
		return InvitationExpired
	default:
		return InvitationPending
	}
}

func newToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("invitation token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || len(v) > 254 {
		return "", fmt.Errorf("%w: email is invalid", ErrInvalidRequest)
	}
	return v, nil
}

func fullName(name, surname pgtype.Text) *string {
	full := strings.TrimSpace(strings.TrimSpace(name.String) + " " + strings.TrimSpace(surname.String))
	if full == "" {
		return nil
	}
	return &full
}
