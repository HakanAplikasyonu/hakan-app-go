// Package usecase is the desktop agent registry (issue #51, 02 §6): the
// desktop app registers itself with the signed-in office user's session,
// sends a heartbeat every HeartbeatInterval, and a scheduled sweep marks an
// agent silent for OfflineAfter as offline and notifies (AGENT_OFFLINE).
//
// AGENT_OFFLINE is a connection event (05 §4): it is not written to
// data_warnings; the agent row carries the state and the office is notified.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
)

const (
	// HeartbeatInterval is how often the agent reports (02 §6).
	HeartbeatInterval = 5 * time.Minute
	// OfflineAfter is the silence after which the agent is offline (02 §6).
	OfflineAfter = 30 * time.Minute
	// CodeOffline is the connection event code (05 §4).
	CodeOffline = "AGENT_OFFLINE"
	// TemplateOffline is the notification template.
	TemplateOffline = events.AgentsOffline

	sweepBatch        = 500
	maxHeartbeatBytes = 8 << 10
)

var (
	// ErrNotFound: no such agent in the organization (or revoked).
	ErrNotFound = errors.New("agents: not found")
	// ErrInvalid: bad input.
	ErrInvalid = errors.New("agents: invalid input")

	machineIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
	platforms   = map[string]bool{"windows": true, "macos": true, "linux": true}
)

// Queries is the sqlc subset the service uses.
type Queries interface {
	UpsertDesktopAgent(ctx context.Context, arg db.UpsertDesktopAgentParams) (db.DesktopAgent, error)
	HeartbeatDesktopAgent(ctx context.Context, arg db.HeartbeatDesktopAgentParams) (db.HeartbeatDesktopAgentRow, error)
	GetDesktopAgent(ctx context.Context, arg db.GetDesktopAgentParams) (db.DesktopAgent, error)
	ListDesktopAgents(ctx context.Context, organizationID int64) ([]db.DesktopAgent, error)
	RevokeDesktopAgent(ctx context.Context, arg db.RevokeDesktopAgentParams) (int64, error)
	MarkSilentDesktopAgentsOffline(ctx context.Context, arg db.MarkSilentDesktopAgentsOfflineParams) ([]db.MarkSilentDesktopAgentsOfflineRow, error)
}

// Notifier enqueues notifications (notifications usecase.Service).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Service is the agent registry.
type Service struct {
	q           Queries
	notifier    Notifier
	frontendURL string
	log         *slog.Logger
	now         func() time.Time
}

// New builds the service. notifier may be nil (no notifications).
func New(q Queries, notifier Notifier, frontendURL string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: q, notifier: notifier, frontendURL: frontendURL, log: log, now: time.Now}
}

// Agent is the API view.
type Agent struct {
	UUID         uuid.UUID       `json:"uuid"`
	MachineID    string          `json:"machine_id"`
	Name         string          `json:"name"`
	Platform     string          `json:"platform"`
	Version      string          `json:"version"`
	Status       string          `json:"status"`
	LastSeenAt   time.Time       `json:"last_seen_at"`
	OfflineSince *time.Time      `json:"offline_since"`
	Heartbeat    json.RawMessage `json:"heartbeat"`
	CreatedAt    time.Time       `json:"created_at"`
	// LastError is set while offline (05 §4 connection event).
	LastError *EventError `json:"last_error"`
	// HeartbeatSeconds tells the agent how often to report.
	HeartbeatSeconds int `json:"heartbeat_seconds"`
}

// EventError is the connection event shape (05 §4).
type EventError struct {
	Code string    `json:"code"`
	At   time.Time `json:"at"`
}

// RegisterInput is POST /v1/tenant/agents.
type RegisterInput struct {
	MachineID string `json:"machine_id"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Version   string `json:"version"`
}

// Register creates or refreshes the agent of a machine.
func (s *Service) Register(ctx context.Context, organizationID, userID int64, in RegisterInput) (Agent, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case !machineIDRe.MatchString(in.MachineID):
		return Agent{}, fmt.Errorf("%w: machine_id must be 8–128 of [A-Za-z0-9_.:-]", ErrInvalid)
	case in.Name == "" || len([]rune(in.Name)) > 120:
		return Agent{}, fmt.Errorf("%w: name must be 1–120 characters", ErrInvalid)
	case !platforms[in.Platform]:
		return Agent{}, fmt.Errorf("%w: platform must be windows, macos or linux", ErrInvalid)
	case len(in.Version) > 64:
		return Agent{}, fmt.Errorf("%w: version is too long", ErrInvalid)
	}
	var by pgtype.Int8
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	row, err := s.q.UpsertDesktopAgent(ctx, db.UpsertDesktopAgentParams{
		OrganizationID: organizationID, MachineID: in.MachineID, Name: in.Name,
		Platform: in.Platform, Version: in.Version, CreatedBy: by,
	})
	if err != nil {
		return Agent{}, fmt.Errorf("agents: register: %w", err)
	}
	return view(row), nil
}

// HeartbeatInput is POST /v1/tenant/agents/{agentUuid}/heartbeat. Stats is
// free-form agent state (watched folders, last upload, sources); it is kept
// for display only and capped in size.
type HeartbeatInput struct {
	Version string          `json:"version"`
	Stats   json.RawMessage `json:"stats"`
}

// Heartbeat records that the agent is alive. An offline agent comes back
// online (its AGENT_OFFLINE event clears).
func (s *Service) Heartbeat(ctx context.Context, organizationID int64, id uuid.UUID, in HeartbeatInput) (Agent, error) {
	if len(in.Version) > 64 {
		return Agent{}, fmt.Errorf("%w: version is too long", ErrInvalid)
	}
	stats := []byte("{}")
	if len(in.Stats) > 0 && string(in.Stats) != "null" {
		if len(in.Stats) > maxHeartbeatBytes {
			return Agent{}, fmt.Errorf("%w: stats exceed %d bytes", ErrInvalid, maxHeartbeatBytes)
		}
		var obj map[string]any
		if json.Unmarshal(in.Stats, &obj) != nil {
			return Agent{}, fmt.Errorf("%w: stats must be a JSON object", ErrInvalid)
		}
		stats = in.Stats
	}
	row, err := s.q.HeartbeatDesktopAgent(ctx, db.HeartbeatDesktopAgentParams{
		OrganizationID: organizationID, Uuid: id, Version: in.Version, LastHeartbeat: stats,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("agents: heartbeat: %w", err)
	}
	if row.PreviousStatus == "offline" {
		s.log.Info("desktop_agent_back_online", "organization_id", organizationID, "agent", id)
	}
	return view(db.DesktopAgent{
		ID: row.ID, Uuid: row.Uuid, OrganizationID: row.OrganizationID, MachineID: row.MachineID,
		Name: row.Name, Platform: row.Platform, Version: row.Version, CreatedBy: row.CreatedBy,
		Status: row.Status, LastSeenAt: row.LastSeenAt, LastHeartbeat: row.LastHeartbeat,
		OfflineSince: row.OfflineSince, RevokedAt: row.RevokedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

// List returns the organization's active agents.
func (s *Service) List(ctx context.Context, organizationID int64) ([]Agent, error) {
	rows, err := s.q.ListDesktopAgents(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("agents: list: %w", err)
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

// Get returns one agent.
func (s *Service) Get(ctx context.Context, organizationID int64, id uuid.UUID) (Agent, error) {
	row, err := s.q.GetDesktopAgent(ctx, db.GetDesktopAgentParams{OrganizationID: organizationID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.RevokedAt.Valid) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("agents: get: %w", err)
	}
	return view(row), nil
}

// Revoke removes the agent from the list; its next heartbeat gets 404 and the
// desktop app registers again only if a user signs in there.
func (s *Service) Revoke(ctx context.Context, organizationID int64, id uuid.UUID) error {
	n, err := s.q.RevokeDesktopAgent(ctx, db.RevokeDesktopAgentParams{OrganizationID: organizationID, Uuid: id})
	if err != nil {
		return fmt.Errorf("agents: revoke: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SweepReport is what one offline sweep did.
type SweepReport struct {
	Offline  int `json:"offline"`
	Notified int `json:"notified"`
}

// SweepOffline marks agents silent for OfflineAfter as offline and notifies
// the user who registered each (once per transition). It is the scheduled
// job's entry point.
func (s *Service) SweepOffline(ctx context.Context) (SweepReport, error) {
	var rep SweepReport
	rows, err := s.q.MarkSilentDesktopAgentsOffline(ctx, db.MarkSilentDesktopAgentsOfflineParams{
		SilentBefore: pgtype.Timestamptz{Time: s.now().Add(-OfflineAfter), Valid: true},
		Lim:          sweepBatch,
	})
	if err != nil {
		return rep, fmt.Errorf("agents: sweep: %w", err)
	}
	for _, r := range rows {
		rep.Offline++
		s.log.Warn("desktop_agent_offline", "code", CodeOffline, "organization_id", r.OrganizationID,
			"agent", r.Uuid, "last_seen_at", r.LastSeenAt.Time)
		if s.notify(ctx, r) {
			rep.Notified++
		}
	}
	return rep, nil
}

// SweepOfflineCount adapts SweepOffline to the queue handler signature.
func (s *Service) SweepOfflineCount(ctx context.Context) (int, error) {
	rep, err := s.SweepOffline(ctx)
	return rep.Offline, err
}

// notify tells the registering user. A failure is logged: notifications never
// block the sweep (the agent stays offline either way).
func (s *Service) notify(ctx context.Context, r db.MarkSilentDesktopAgentsOfflineRow) bool {
	if s.notifier == nil || !r.CreatedBy.Valid {
		return false
	}
	minutes := int(s.now().Sub(r.LastSeenAt.Time).Minutes())
	link := strings.TrimRight(s.frontendURL, "/")
	org, user := r.OrganizationID, r.CreatedBy.Int64
	_, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		OrganizationID: &org,
		UserID:         &user,
		Channels:       []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
		TemplateCode:   TemplateOffline,
		SourceEvent:    TemplateOffline,
		ActionURL:      &link,
		TemplateVars: map[string]string{
			"agent_name": r.Name,
			"minutes":    strconv.Itoa(minutes),
			"link":       link,
		},
		Payload: map[string]any{"kind": "agents", "code": CodeOffline, "agent_uuid": r.Uuid.String()},
	})
	if err != nil {
		s.log.Warn("desktop_agent_notify_failed", "error", err, "agent", r.Uuid)
		return false
	}
	return true
}

func view(r db.DesktopAgent) Agent {
	a := Agent{
		UUID: r.Uuid, MachineID: r.MachineID, Name: r.Name, Platform: r.Platform, Version: r.Version,
		Status: r.Status, LastSeenAt: r.LastSeenAt.Time, Heartbeat: json.RawMessage(r.LastHeartbeat),
		CreatedAt: r.CreatedAt.Time, HeartbeatSeconds: int(HeartbeatInterval.Seconds()),
	}
	if len(a.Heartbeat) == 0 {
		a.Heartbeat = json.RawMessage("{}")
	}
	if r.OfflineSince.Valid {
		t := r.OfflineSince.Time
		a.OfflineSince = &t
		a.LastError = &EventError{Code: CodeOffline, At: t}
	}
	return a
}
