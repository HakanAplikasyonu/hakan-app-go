// Package usecase serves warning configuration (and, after #9, warning
// listing) to the tenant API.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/shopspring/decimal"
)

// Errors.
var (
	ErrUnknownCode = errors.New("unknown warning code")
	ErrInvalid     = errors.New("invalid warning rule")
	ErrNotFound    = errors.New("warning rule override not found")
)

// ConfigStore is what the service needs from the repository.
type ConfigStore interface {
	Layers(ctx context.Context, organizationID, engagementID int64, codes []string) ([]repository.CodeLayers, error)
	Upsert(ctx context.Context, organizationID, engagementID int64, code string, l engine.Layer, userID int64) error
	Delete(ctx context.Context, organizationID, engagementID int64, code string) (bool, error)
}

// Rules is the warning-rule configuration service.
type Rules struct{ store ConfigStore }

// NewRules builds the service.
func NewRules(store ConfigStore) *Rules { return &Rules{store: store} }

// LayerView is one configuration level in the API.
type LayerView struct {
	Severity *string        `json:"severity"`
	Enabled  *bool          `json:"enabled"`
	Params   map[string]any `json:"params"`
}

// EffectiveView is the resolved configuration.
type EffectiveView struct {
	Enabled  bool           `json:"enabled"`
	Severity string         `json:"severity"`
	Params   map[string]any `json:"params"`
}

// RuleView is one code with its layers.
type RuleView struct {
	Code        string     `json:"code"`
	Section     string     `json:"section"`
	EntityTypes []string   `json:"entity_types"`
	Default     LayerView  `json:"default"`
	Office      *LayerView `json:"office"`
	// Engagement is the engagement override (null when there is none or
	// the office-level list was requested).
	Engagement *LayerView    `json:"engagement"`
	Effective  EffectiveView `json:"effective"`
}

// List returns every code's configuration for the office (engagementID 0)
// or one engagement.
func (s *Rules) List(ctx context.Context, organizationID, engagementID int64) ([]RuleView, error) {
	layers, err := s.store.Layers(ctx, organizationID, engagementID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]RuleView, 0, len(layers))
	for _, l := range layers {
		out = append(out, view(l, engagementID > 0))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// SetInput is a PUT body: nil fields inherit from the level below.
type SetInput struct {
	Severity *string        `json:"severity"`
	Enabled  *bool          `json:"enabled"`
	Params   map[string]any `json:"params"`
}

// Set writes an override and returns the code's new view.
func (s *Rules) Set(ctx context.Context, organizationID, engagementID int64, code string, in SetInput, userID int64) (RuleView, error) {
	cur, err := s.one(ctx, organizationID, engagementID, code)
	if err != nil {
		return RuleView{}, err
	}
	layer, err := validate(cur, in)
	if err != nil {
		return RuleView{}, err
	}
	if err := s.store.Upsert(ctx, organizationID, engagementID, code, layer, userID); err != nil {
		return RuleView{}, err
	}
	cur, err = s.one(ctx, organizationID, engagementID, code)
	if err != nil {
		return RuleView{}, err
	}
	return view(cur, engagementID > 0), nil
}

// Reset removes an override.
func (s *Rules) Reset(ctx context.Context, organizationID, engagementID int64, code string) error {
	if _, ok := catalog.Lookup(code); !ok {
		return ErrUnknownCode
	}
	ok, err := s.store.Delete(ctx, organizationID, engagementID, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Rules) one(ctx context.Context, organizationID, engagementID int64, code string) (repository.CodeLayers, error) {
	if _, ok := catalog.Lookup(code); !ok {
		return repository.CodeLayers{}, ErrUnknownCode
	}
	layers, err := s.store.Layers(ctx, organizationID, engagementID, []string{code})
	if err != nil {
		return repository.CodeLayers{}, err
	}
	if len(layers) != 1 {
		return repository.CodeLayers{}, ErrUnknownCode
	}
	return layers[0], nil
}

// validate checks a PUT body against the code's default params: only known
// keys, and each value a decimal string, a number or null.
func validate(cur repository.CodeLayers, in SetInput) (engine.Layer, error) {
	var l engine.Layer
	if in.Severity != nil {
		sev := catalog.Severity(*in.Severity)
		if !sev.Valid() {
			return l, fmt.Errorf("%w: severity must be info, warning or blocker", ErrInvalid)
		}
		l.Severity = &sev
	}
	l.Enabled = in.Enabled
	var defaults map[string]any
	_ = json.Unmarshal(cur.Default.Params, &defaults)
	for k, v := range in.Params {
		if _, ok := defaults[k]; !ok {
			return l, fmt.Errorf("%w: %s has no param %q", ErrInvalid, cur.Code, k)
		}
		switch t := v.(type) {
		case nil, float64:
		case string:
			if _, err := decimal.NewFromString(t); err != nil {
				return l, fmt.Errorf("%w: param %q must be a decimal string", ErrInvalid, k)
			}
		default:
			return l, fmt.Errorf("%w: param %q must be a decimal string, a number or null", ErrInvalid, k)
		}
	}
	params := in.Params
	if params == nil {
		params = map[string]any{}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return l, fmt.Errorf("%w: params", ErrInvalid)
	}
	l.Params = raw
	return l, nil
}

func view(l repository.CodeLayers, withEngagement bool) RuleView {
	c, _ := catalog.Lookup(l.Code)
	eff := l.Effective()
	v := RuleView{
		Code: l.Code, Section: l.Section, EntityTypes: c.EntityTypes,
		Default:   layerView(l.Default),
		Effective: EffectiveView{Enabled: eff.Enabled, Severity: string(eff.Severity), Params: eff.Params},
	}
	if l.Office != nil {
		lv := layerView(*l.Office)
		v.Office = &lv
	}
	if withEngagement && l.Engagement != nil {
		lv := layerView(*l.Engagement)
		v.Engagement = &lv
	}
	return v
}

func layerView(l engine.Layer) LayerView {
	v := LayerView{Enabled: l.Enabled, Params: map[string]any{}}
	if l.Severity != nil {
		s := string(*l.Severity)
		v.Severity = &s
	}
	_ = json.Unmarshal(l.Params, &v.Params)
	if v.Params == nil {
		v.Params = map[string]any{}
	}
	return v
}
