// Package repository persists warning configuration (warning_codes,
// warning_rule_configs) and, once the shared data model lands (#9), the
// data_warning rows.
package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConfigQueries is the subset of db.Querier used here.
type ConfigQueries interface {
	ListWarningCodes(ctx context.Context, codes []string) ([]db.ListWarningCodesRow, error)
	ListWarningRuleConfigs(ctx context.Context, arg db.ListWarningRuleConfigsParams) ([]db.ListWarningRuleConfigsRow, error)
	UpsertOfficeWarningRule(ctx context.Context, arg db.UpsertOfficeWarningRuleParams) error
	UpsertEngagementWarningRule(ctx context.Context, arg db.UpsertEngagementWarningRuleParams) error
	DeleteOfficeWarningRule(ctx context.Context, arg db.DeleteOfficeWarningRuleParams) (int64, error)
	DeleteEngagementWarningRule(ctx context.Context, arg db.DeleteEngagementWarningRuleParams) (int64, error)
}

// Config is the Postgres configuration store. It implements
// engine.ConfigResolver.
type Config struct{ q ConfigQueries }

// NewConfig builds the store.
func NewConfig(q ConfigQueries) *Config { return &Config{q: q} }

var _ engine.ConfigResolver = (*Config)(nil)

// CodeLayers is every configuration level of one code.
type CodeLayers struct {
	Code       string
	Section    string
	Default    engine.Layer
	Office     *engine.Layer
	Engagement *engine.Layer
}

// Effective merges the layers.
func (c CodeLayers) Effective() engine.Effective {
	var upper []engine.Layer
	if c.Office != nil {
		upper = append(upper, *c.Office)
	}
	if c.Engagement != nil {
		upper = append(upper, *c.Engagement)
	}
	return engine.Merge(c.Code, c.Default, upper...)
}

// Layers loads the layers of codes (all codes when empty) for an office
// and, when engagementID > 0, one of its engagements.
func (p *Config) Layers(ctx context.Context, organizationID, engagementID int64, codes []string) ([]CodeLayers, error) {
	if codes == nil {
		codes = []string{}
	}
	defs, err := p.q.ListWarningCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("list warning codes: %w", err)
	}
	eng := pgtype.Int8{}
	if engagementID > 0 {
		eng = pgtype.Int8{Int64: engagementID, Valid: true}
	}
	rows, err := p.q.ListWarningRuleConfigs(ctx, db.ListWarningRuleConfigsParams{
		OrganizationID: organizationID, EngagementID: eng, Codes: codes,
	})
	if err != nil {
		return nil, fmt.Errorf("list warning rule configs: %w", err)
	}
	out := make([]CodeLayers, 0, len(defs))
	idx := map[string]int{}
	for _, d := range defs {
		sev := catalog.Severity(d.DefaultSeverity)
		idx[d.Code] = len(out)
		out = append(out, CodeLayers{
			Code: d.Code, Section: d.Section,
			Default: engine.Layer{Severity: &sev, Params: json.RawMessage(d.DefaultParams)},
		})
	}
	for _, r := range rows {
		i, ok := idx[r.Code]
		if !ok {
			continue
		}
		l := layerFrom(r)
		if r.EngagementID.Valid {
			out[i].Engagement = &l
		} else {
			out[i].Office = &l
		}
	}
	return out, nil
}

// Resolve implements engine.ConfigResolver.
func (p *Config) Resolve(ctx context.Context, s engine.Subject, codes []string) (map[string]engine.Effective, error) {
	layers, err := p.Layers(ctx, s.OrganizationID, s.EngagementID, codes)
	if err != nil {
		return nil, err
	}
	out := make(map[string]engine.Effective, len(layers))
	for _, l := range layers {
		out[l.Code] = l.Effective()
	}
	return out, nil
}

// Upsert writes an office (engagementID 0) or engagement override.
func (p *Config) Upsert(ctx context.Context, organizationID, engagementID int64, code string, l engine.Layer, userID int64) error {
	sev := pgtype.Text{}
	if l.Severity != nil {
		sev = pgtype.Text{String: string(*l.Severity), Valid: true}
	}
	en := pgtype.Bool{}
	if l.Enabled != nil {
		en = pgtype.Bool{Bool: *l.Enabled, Valid: true}
	}
	params := []byte(l.Params)
	if len(params) == 0 {
		params = []byte("{}")
	}
	by := pgtype.Int8{}
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	var err error
	if engagementID > 0 {
		err = p.q.UpsertEngagementWarningRule(ctx, db.UpsertEngagementWarningRuleParams{
			OrganizationID: organizationID, EngagementID: pgtype.Int8{Int64: engagementID, Valid: true},
			Code: code, Severity: sev, Enabled: en, Params: params, UpdatedBy: by,
		})
	} else {
		err = p.q.UpsertOfficeWarningRule(ctx, db.UpsertOfficeWarningRuleParams{
			OrganizationID: organizationID, Code: code, Severity: sev, Enabled: en, Params: params, UpdatedBy: by,
		})
	}
	if err != nil {
		return fmt.Errorf("upsert warning rule: %w", err)
	}
	return nil
}

// Delete removes an override; false when there was none.
func (p *Config) Delete(ctx context.Context, organizationID, engagementID int64, code string) (bool, error) {
	var n int64
	var err error
	if engagementID > 0 {
		n, err = p.q.DeleteEngagementWarningRule(ctx, db.DeleteEngagementWarningRuleParams{
			OrganizationID: organizationID, EngagementID: pgtype.Int8{Int64: engagementID, Valid: true}, Code: code,
		})
	} else {
		n, err = p.q.DeleteOfficeWarningRule(ctx, db.DeleteOfficeWarningRuleParams{OrganizationID: organizationID, Code: code})
	}
	if err != nil {
		return false, fmt.Errorf("delete warning rule: %w", err)
	}
	return n > 0, nil
}

func layerFrom(r db.ListWarningRuleConfigsRow) engine.Layer {
	l := engine.Layer{Params: json.RawMessage(r.Params)}
	if r.Severity.Valid {
		s := catalog.Severity(r.Severity.String)
		l.Severity = &s
	}
	if r.Enabled.Valid {
		b := r.Enabled.Bool
		l.Enabled = &b
	}
	return l
}
