package agenttasks

// Office consent and the runner computer (luca-ajan-sozlesme.md 1.3). There is
// no partnership with Luca: the helper signs in to the office's Luca account
// and transfers on its behalf only after someone in the office approves the
// consent screen on a desktop. Who, when, which text version and from which
// computer are stored. The Luca password never reaches the server; it stays
// in that computer's OS credential store. Without consent no agent can claim
// a task (409 CONSENT_REQUIRED).

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// RunnerOnlineWindow: the runner counts as online when it polled within this
// window (the helper polls every 10–15 s).
const RunnerOnlineWindow = 2 * time.Minute

var consentVersionRe = regexp.MustCompile(`^[0-9A-Za-z._-]{1,16}$`)

// Consent is the office's Luca helper consent.
type Consent struct {
	GivenAt     time.Time `json:"given_at"`
	GivenByName string    `json:"given_by_name"`
	AgentName   string    `json:"agent_name,omitempty"`
	TextVersion string    `json:"text_version"`
}

// Runner is the computer that polled for helper tasks last.
type Runner struct {
	AgentUUID  uuid.UUID  `json:"agent_uuid"`
	Name       string     `json:"name"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	Online     bool       `json:"online"`
}

// ConsentInput is POST /v1/tenant/desktop-helper-settings/luca-consent.
type ConsentInput struct {
	// AgentUUID is the desktop where the consent screen was approved
	// (optional).
	AgentUUID   uuid.UUID `json:"agent_uuid"`
	TextVersion string    `json:"text_version"`
}

func (s *Service) consentOf(ctx context.Context, sc Scope, r db.DesktopHelperSetting) (*Consent, error) {
	if !r.LucaConsentAt.Valid {
		return nil, nil
	}
	names, err := s.q.GetLucaConsentNames(ctx, sc.OrganizationID)
	if err != nil {
		return nil, err
	}
	return &Consent{
		GivenAt: r.LucaConsentAt.Time, GivenByName: names.GivenByName, AgentName: names.AgentName,
		TextVersion: r.LucaConsentTextVersion.String,
	}, nil
}

func (s *Service) lucaRunner(ctx context.Context, sc Scope) (*Runner, error) {
	r, err := s.q.GetLucaRunner(ctx, sc.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &Runner{AgentUUID: r.Uuid, Name: r.Name, LastSeenAt: tsPtr(r.HelperSeenAt)}
	if out.LastSeenAt != nil {
		out.Online = time.Since(*out.LastSeenAt) < RunnerOnlineWindow
	}
	return out, nil
}

// GiveConsent records the office's consent: the caller, now, the text
// version and the desktop. Giving it again updates who and when.
func (s *Service) GiveConsent(ctx context.Context, sc Scope, in ConsentInput) (Settings, error) {
	if !consentVersionRe.MatchString(in.TextVersion) {
		return Settings{}, fmt.Errorf("%w: text_version is required", ErrInvalid)
	}
	var agentID int64
	if in.AgentUUID != uuid.Nil {
		id, err := s.agentID(ctx, sc, in.AgentUUID)
		if err != nil {
			return Settings{}, err
		}
		agentID = id
	}
	if _, err := s.q.SetLucaConsent(ctx, db.SetLucaConsentParams{
		OrganizationID: sc.OrganizationID, ConsentBy: optInt8(sc.UserID), ConsentAgentID: optInt8(agentID),
		TextVersion: pgtype.Text{String: in.TextVersion, Valid: true},
	}); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(ctx, sc)
}

// WithdrawConsent clears the consent; agents stop claiming tasks.
func (s *Service) WithdrawConsent(ctx context.Context, sc Scope) (Settings, error) {
	if err := s.q.ClearLucaConsent(ctx, sc.OrganizationID); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(ctx, sc)
}

// TouchHelper records that the agent polled for helper tasks (the "runner
// computer: online" indicator). An unknown or revoked agent is ignored.
func (s *Service) TouchHelper(ctx context.Context, sc Scope, agent uuid.UUID) error {
	return s.q.TouchHelperAgent(ctx, db.TouchHelperAgentParams{OrganizationID: sc.OrganizationID, Uuid: agent})
}

func (s *Service) lucaConsented(ctx context.Context, sc Scope) error {
	r, err := s.q.GetDesktopHelperSettings(ctx, sc.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConsentRequired
	}
	if err != nil {
		return err
	}
	if !r.LucaConsentAt.Valid {
		return ErrConsentRequired
	}
	return nil
}
