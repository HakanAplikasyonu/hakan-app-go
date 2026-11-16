package privacy

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ExportFormatVersion is bumped when a section changes shape.
const ExportFormatVersion = 1

// WriteExport writes the ZIP of a completed export request: manifest.json
// plus one JSON file per section. It reads the data at download time.
// Secrets (password hashes, tokens, key material) are never included.
func (s *Service) WriteExport(ctx context.Context, w io.Writer, eng engagementctx.Scope, req db.PrivacyRequest) error {
	if req.Kind != KindExport || req.Status != StatusCompleted {
		return ErrNotExport
	}
	if req.ExpiresAt.Valid && s.now().After(req.ExpiresAt.Time) {
		return ErrExpired
	}
	sections, err := s.collect(ctx, eng, req)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	names := make([]string, 0, len(sections))
	for _, sec := range sections {
		names = append(names, sec.name+".json")
	}
	manifest := map[string]any{
		"format":        "hakan.kvkk-export",
		"version":       ExportFormatVersion,
		"request_uuid":  req.Uuid,
		"subject_type":  req.SubjectType,
		"engagement":    eng.UUID,
		"generated_at":  s.now().UTC().Format(time.RFC3339),
		"files":         names,
		"legal_basis":   "KVKK md. 11/1 (b), (c)",
		"retained_note": "Muhasebe kayıtları yasal saklama süresi boyunca tutulur; bu dosyada sayıları yer alır.",
	}
	if err := writeJSON(zw, "manifest.json", manifest); err != nil {
		return err
	}
	for _, sec := range sections {
		if err := writeJSON(zw, sec.name+".json", sec.data); err != nil {
			return err
		}
	}
	return zw.Close()
}

type section struct {
	name string
	data any
}

func (s *Service) collect(ctx context.Context, eng engagementctx.Scope, req db.PrivacyRequest) ([]section, error) {
	if req.SubjectType == SubjectUser {
		if !req.SubjectUserID.Valid {
			return nil, ErrSubjectUnknown
		}
		return s.collectUser(ctx, req.SubjectUserID.Int64)
	}
	tp, err := s.q.GetTaxpayerByID(ctx, eng.TaxpayerID)
	if err != nil {
		return nil, fmt.Errorf("privacy: taxpayer: %w", err)
	}
	members, err := s.q.ListPrivacyTaxpayerMembers(ctx, eng.TaxpayerID)
	if err != nil {
		return nil, err
	}
	acc, err := s.q.ListPrivacyAcceptancesForTaxpayer(ctx, int8v(eng.TaxpayerID))
	if err != nil {
		return nil, err
	}
	inv, err := s.q.ListPrivacyInvitations(ctx, db.ListPrivacyInvitationsParams{OrganizationID: eng.OrganizationID, EngagementID: eng.ID})
	if err != nil {
		return nil, err
	}
	onb, err := s.q.ListPrivacyOnboardingRequests(ctx, db.ListPrivacyOnboardingRequestsParams{OrganizationID: eng.OrganizationID, TaxpayerID: int8v(eng.TaxpayerID)})
	if err != nil {
		return nil, err
	}
	counts, err := s.q.CountPrivacyAccountingRecords(ctx, eng.TaxpayerID)
	if err != nil {
		return nil, err
	}
	type member struct {
		UUID        any    `json:"uuid"`
		Email       string `json:"email"`
		Name        string `json:"name"`
		Surname     string `json:"surname"`
		Role        string `json:"role"`
		Status      string `json:"status"`
		MemberSince any    `json:"member_since"`
	}
	ms := make([]member, 0, len(members))
	for _, m := range members {
		ms = append(ms, member{m.UserUuid, m.Email, m.Name, m.Surname, m.Role, m.Status, ts(m.MemberSince)})
	}
	return []section{
		{"taxpayer", map[string]any{"uuid": tp.Uuid, "kind": tp.Kind, "tax_id": tp.TaxID, "title": tp.Title, "tax_office": tp.TaxOffice, "created_at": ts(tp.CreatedAt)}},
		{"engagement", map[string]any{"uuid": eng.UUID, "status": eng.Status, "started_at": eng.StartedAt.UTC()}},
		{"users", ms},
		{"legal_acceptances", acc},
		{"invitations", inv},
		{"onboarding_requests", onb},
		{"accounting_record_counts", counts},
	}, nil
}

func (s *Service) collectUser(ctx context.Context, userID int64) ([]section, error) {
	u, err := s.q.GetPrivacyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	acc, err := s.q.ListPrivacyAcceptancesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.q.ListPrivacySessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	devices, err := s.q.ListPrivacyDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	passkeys, err := s.q.ListPrivacyPasskeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	oauth, err := s.q.ListPrivacyOAuthProviders(ctx, userID)
	if err != nil {
		return nil, err
	}
	notif, err := s.q.CountPrivacyNotifications(ctx, int8v(userID))
	if err != nil {
		return nil, err
	}
	return []section{
		{"user", map[string]any{
			"uuid": u.Uuid, "email": u.Email, "name": u.Name, "surname": u.Surname, "status": u.Status,
			"locale": u.Locale, "email_verified_at": ts(u.EmailVerifiedAt), "last_login_at": ts(u.LastLoginAt),
			"created_at": ts(u.CreatedAt), "deactivated_at": ts(u.DeactivatedAt),
		}},
		{"legal_acceptances", acc},
		{"sessions", sessions},
		{"devices", devices},
		{"passkeys", passkeys},
		{"sign_in_providers", oauth},
		{"notifications", map[string]any{"count": notif}},
	}, nil
}

func writeJSON(zw *zip.Writer, name string, v any) error {
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now().UTC()})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func ts(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}

func int8v(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
