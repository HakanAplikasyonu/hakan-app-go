// Package repository persists the review queue in Postgres.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Postgres implements usecase.Store.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the store.
func New(pool *pgxpool.Pool, q *db.Queries) *Postgres { return &Postgres{pool: pool, q: q} }

var _ usecase.Store = (*Postgres)(nil)

type tx struct{ q *db.Queries }

// InTx runs fn in one transaction; appliers get the tx-bound *db.Queries
// through Tx.Handle().
func (p *Postgres) InTx(ctx context.Context, fn func(usecase.Tx) error) error {
	t, err := database.Begin(ctx, p.pool)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = t.Rollback(ctx) }()
	if err := fn(tx{q: p.q.WithTx(t)}); err != nil {
		return err
	}
	return t.Commit(ctx)
}

func (t tx) Handle() any { return t.q }

func (t tx) SupersedePending(ctx context.Context, s usecase.Subject, in usecase.Input) ([]usecase.Superseded, error) {
	rows, err := t.q.SupersedePendingSuggestions(ctx, db.SupersedePendingSuggestionsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Kind: in.Kind,
		TargetType: in.TargetType, TargetID: optInt8(in.TargetID), TargetField: in.TargetField,
	})
	if err != nil {
		return nil, fmt.Errorf("supersede suggestions: %w", err)
	}
	out := make([]usecase.Superseded, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.Superseded{ID: r.ID, Confidence: fromNumeric(r.Confidence)})
	}
	return out, nil
}

func (t tx) Insert(ctx context.Context, s usecase.Subject, in usecase.Input) (usecase.Suggestion, error) {
	inputs, err := json.Marshal(in.Provenance.Inputs)
	if err != nil {
		return usecase.Suggestion{}, fmt.Errorf("%w: inputs", usecase.ErrInvalid)
	}
	p := in.Provenance
	row, err := t.q.InsertSuggestion(ctx, db.InsertSuggestionParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TaxpayerID: s.TaxpayerID,
		Kind: in.Kind, TargetType: in.TargetType, TargetID: optInt8(in.TargetID), TargetField: in.TargetField,
		Period: optText(in.Period), ProposedValue: in.Value, Confidence: toNumeric(in.Confidence),
		Rationale: in.Rationale, Origin: string(p.Origin), SourceEntityType: optText(p.SourceEntityType),
		SourceEntityID: optInt8(p.SourceEntityID), RuleRef: optText(p.RuleRef), Model: optText(p.Model),
		AiUsageEventID: optInt8(p.AIUsageEventID), Inputs: inputs,
	})
	if err != nil {
		return usecase.Suggestion{}, fmt.Errorf("insert suggestion: %w", err)
	}
	return fromRow(row), nil
}

func (t tx) GetForUpdate(ctx context.Context, s usecase.Subject, id uuid.UUID) (usecase.Suggestion, error) {
	row, err := t.q.GetSuggestionForUpdate(ctx, db.GetSuggestionForUpdateParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.Suggestion{}, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.Suggestion{}, fmt.Errorf("get suggestion: %w", err)
	}
	return fromRow(row), nil
}

func (t tx) UpdateDecision(ctx context.Context, sug usecase.Suggestion) (usecase.Suggestion, error) {
	row, err := t.q.UpdateSuggestionDecision(ctx, db.UpdateSuggestionDecisionParams{
		Status: string(sug.Status), AppliedValue: optJSON(sug.AppliedValue), PreviousValue: optJSON(sug.PreviousValue),
		RuleCandidate: sug.RuleCandidate, DecidedBy: optInt8(sug.DecidedBy),
		OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, ID: sug.ID,
	})
	if err != nil {
		return usecase.Suggestion{}, fmt.Errorf("update suggestion: %w", err)
	}
	return fromRow(row), nil
}

func (t tx) InsertDecision(ctx context.Context, s usecase.Subject, suggestionID int64, d usecase.Decision) error {
	thr := pgtype.Numeric{}
	if d.Threshold != nil {
		thr = toNumeric(*d.Threshold)
	}
	if err := t.q.InsertSuggestionDecision(ctx, db.InsertSuggestionDecisionParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, SuggestionID: suggestionID,
		Action: string(d.Action), ActorUserID: optInt8(d.ActorUserID), Reason: d.Reason,
		Value: optJSON(d.Value), Confidence: toNumeric(d.Confidence), Threshold: thr,
	}); err != nil {
		return fmt.Errorf("insert suggestion decision: %w", err)
	}
	return nil
}

// Get implements usecase.Store.
func (p *Postgres) Get(ctx context.Context, s usecase.Subject, id uuid.UUID) (usecase.Suggestion, []usecase.Decision, error) {
	row, err := p.q.GetSuggestion(ctx, db.GetSuggestionParams{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.Suggestion{}, nil, usecase.ErrNotFound
	}
	if err != nil {
		return usecase.Suggestion{}, nil, fmt.Errorf("get suggestion: %w", err)
	}
	logRows, err := p.q.ListSuggestionDecisions(ctx, db.ListSuggestionDecisionsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, SuggestionID: row.ID,
	})
	if err != nil {
		return usecase.Suggestion{}, nil, fmt.Errorf("list suggestion decisions: %w", err)
	}
	log := make([]usecase.Decision, 0, len(logRows))
	for _, l := range logRows {
		d := usecase.Decision{
			Action: usecase.Action(l.Action), Reason: l.Reason, Value: json.RawMessage(l.Value),
			Confidence: fromNumeric(l.Confidence), CreatedAt: l.CreatedAt.Time,
		}
		if l.Threshold.Valid {
			t := fromNumeric(l.Threshold)
			d.Threshold = &t
		}
		if l.ActorUuid.Valid {
			u := uuid.UUID(l.ActorUuid.Bytes)
			d.ActorUUID = &u
		}
		log = append(log, d)
	}
	return fromRow(row), log, nil
}

// List implements usecase.Store.
func (p *Postgres) List(ctx context.Context, f usecase.Filter) ([]usecase.ListRow, int64, error) {
	eng := pgtype.Int8{}
	if f.EngagementID > 0 {
		eng = pgtype.Int8{Int64: f.EngagementID, Valid: true}
	}
	rc := pgtype.Bool{}
	if f.RuleCandidate != nil {
		rc = pgtype.Bool{Bool: *f.RuleCandidate, Valid: true}
	}
	rows, err := p.q.ListTenantSuggestions(ctx, db.ListTenantSuggestionsParams{
		OrganizationID: f.OrganizationID, EngagementID: eng, AllEngagements: f.AllEngagements, UserID: f.UserID,
		Status: optText(f.Status), Kind: optText(f.Kind), RuleCandidate: rc, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list suggestions: %w", err)
	}
	total, err := p.q.CountTenantSuggestions(ctx, db.CountTenantSuggestionsParams{
		OrganizationID: f.OrganizationID, EngagementID: eng, AllEngagements: f.AllEngagements, UserID: f.UserID,
		Status: optText(f.Status), Kind: optText(f.Kind), RuleCandidate: rc,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count suggestions: %w", err)
	}
	out := make([]usecase.ListRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, usecase.ListRow{Suggestion: fromRow(db.Suggestion{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID,
			TaxpayerID: r.TaxpayerID, Kind: r.Kind, TargetType: r.TargetType, TargetID: r.TargetID,
			TargetField: r.TargetField, Period: r.Period, ProposedValue: r.ProposedValue, Confidence: r.Confidence,
			Rationale: r.Rationale, Origin: r.Origin, SourceEntityType: r.SourceEntityType,
			SourceEntityID: r.SourceEntityID, RuleRef: r.RuleRef, Model: r.Model, AiUsageEventID: r.AiUsageEventID,
			Inputs: r.Inputs, Status: r.Status, AppliedValue: r.AppliedValue, PreviousValue: r.PreviousValue,
			RuleCandidate: r.RuleCandidate, DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}), EngagementUUID: r.EngagementUuid})
	}
	return out, total, nil
}

// Settings implements usecase.Store.
func (p *Postgres) Settings(ctx context.Context, organizationID, engagementID int64) ([]usecase.SettingRow, error) {
	eng := pgtype.Int8{}
	if engagementID > 0 {
		eng = pgtype.Int8{Int64: engagementID, Valid: true}
	}
	rows, err := p.q.ListReviewSettings(ctx, db.ListReviewSettingsParams{OrganizationID: organizationID, EngagementID: eng})
	if err != nil {
		return nil, fmt.Errorf("list review settings: %w", err)
	}
	out := make([]usecase.SettingRow, 0, len(rows))
	for _, r := range rows {
		s := usecase.SettingRow{Kind: r.Kind}
		if r.EngagementID.Valid {
			s.EngagementID = r.EngagementID.Int64
		}
		if r.AutoAccept.Valid {
			b := r.AutoAccept.Bool
			s.AutoAccept = &b
		}
		if r.Threshold.Valid {
			t := fromNumeric(r.Threshold)
			s.Threshold = &t
		}
		out = append(out, s)
	}
	return out, nil
}

// UpsertSetting implements usecase.Store.
func (p *Postgres) UpsertSetting(ctx context.Context, organizationID, engagementID int64, row usecase.SettingRow, userID int64) error {
	auto := pgtype.Bool{}
	if row.AutoAccept != nil {
		auto = pgtype.Bool{Bool: *row.AutoAccept, Valid: true}
	}
	thr := pgtype.Numeric{}
	if row.Threshold != nil {
		thr = toNumeric(*row.Threshold)
	}
	by := pgtype.Int8{}
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	var err error
	if engagementID > 0 {
		err = p.q.UpsertEngagementReviewSetting(ctx, db.UpsertEngagementReviewSettingParams{
			OrganizationID: organizationID, EngagementID: pgtype.Int8{Int64: engagementID, Valid: true},
			Kind: row.Kind, AutoAccept: auto, Threshold: thr, UpdatedBy: by,
		})
	} else {
		err = p.q.UpsertOfficeReviewSetting(ctx, db.UpsertOfficeReviewSettingParams{
			OrganizationID: organizationID, Kind: row.Kind, AutoAccept: auto, Threshold: thr, UpdatedBy: by,
		})
	}
	if err != nil {
		return fmt.Errorf("upsert review setting: %w", err)
	}
	return nil
}

// DeleteSetting implements usecase.Store.
func (p *Postgres) DeleteSetting(ctx context.Context, organizationID, engagementID int64, kind string) (bool, error) {
	var n int64
	var err error
	if engagementID > 0 {
		n, err = p.q.DeleteEngagementReviewSetting(ctx, db.DeleteEngagementReviewSettingParams{
			OrganizationID: organizationID, EngagementID: pgtype.Int8{Int64: engagementID, Valid: true}, Kind: kind,
		})
	} else {
		n, err = p.q.DeleteOfficeReviewSetting(ctx, db.DeleteOfficeReviewSettingParams{OrganizationID: organizationID, Kind: kind})
	}
	if err != nil {
		return false, fmt.Errorf("delete review setting: %w", err)
	}
	return n > 0, nil
}

func fromRow(r db.Suggestion) usecase.Suggestion {
	in := usecase.Input{
		Kind: r.Kind, TargetType: r.TargetType, TargetField: r.TargetField, Period: r.Period.String,
		Value: json.RawMessage(r.ProposedValue), Confidence: fromNumeric(r.Confidence), Rationale: r.Rationale,
		Provenance: usecase.Provenance{
			Origin: usecase.Origin(r.Origin), SourceEntityType: r.SourceEntityType.String,
			RuleRef: r.RuleRef.String, Model: r.Model.String, Inputs: map[string]any{},
		},
	}
	in.TargetID = ptrInt8(r.TargetID)
	in.Provenance.SourceEntityID = ptrInt8(r.SourceEntityID)
	in.Provenance.AIUsageEventID = ptrInt8(r.AiUsageEventID)
	_ = json.Unmarshal(r.Inputs, &in.Provenance.Inputs)
	s := usecase.Suggestion{
		ID: r.ID, UUID: r.Uuid, Input: in, Status: usecase.Status(r.Status),
		Subject:      usecase.Subject{OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaxpayerID: r.TaxpayerID},
		AppliedValue: nilIfEmpty(r.AppliedValue), PreviousValue: nilIfEmpty(r.PreviousValue),
		RuleCandidate: r.RuleCandidate, DecidedBy: ptrInt8(r.DecidedBy),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		s.DecidedAt = &t
	}
	return s
}

func toNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func fromNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func ptrInt8(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func optText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optJSON(v json.RawMessage) []byte {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilIfEmpty(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}
