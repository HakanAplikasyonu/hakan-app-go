// Package engine is the warning engine (issue #13, 05-uyari-kodlari.md §1).
//
// A producer (adapter, UBL parser, a control rule) reports, for one entity,
// the full set of findings it currently sees for the codes it owns. The
// engine then:
//
//   - raises each finding with the severity read from configuration
//     (engagement > office > warning_codes default), skipping disabled codes;
//   - keeps at most one open warning per (entity_type, entity_id, code):
//     a repeated sync only refreshes updated_at and params;
//   - auto-resolves the producer's open warnings on that entity whose code is
//     no longer found ("koşul ortadan kalktı").
//
// Persistence is behind Store; configuration behind ConfigResolver. The
// engine itself has no SQL, so it is tested with MemoryStore.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
)

// Errors.
var (
	ErrUnknownCode    = errors.New("warnings: unknown code")
	ErrEntityNotAllow = errors.New("warnings: code not allowed on entity type")
	ErrBadEntity      = errors.New("warnings: invalid entity")
	ErrBadProducer    = errors.New("warnings: invalid producer")
	ErrNotOwned       = errors.New("warnings: finding code not owned by producer")
	ErrBadSubject     = errors.New("warnings: subject needs organization, engagement and taxpayer")
)

// Subject is whose data the warnings are about: the taxpayer, seen through
// one office engagement (configuration is per office / engagement).
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

// Entity is what a warning points at: the internal id (BIGINT) of the
// record, nil for period-wide checks.
type Entity struct {
	Type   string
	ID     *int64
	Period string // YYYY-MM; empty when not period-bound
}

// Key is the dedupe key of an open warning, the same as the
// uq_data_warnings_open index: engagement, entity, code and period.
type Key struct {
	EngagementID int64
	EntityType   string
	EntityID     *int64
	Period       string
	Code         string
}

// Finding is one condition a producer sees now.
type Finding struct {
	Code   string
	Params map[string]any
}

// Warning is what the store persists.
type Warning struct {
	Key
	OrganizationID int64
	TaxpayerID     int64
	Severity       catalog.Severity
	Params         json.RawMessage
	Producer       string
	Status         catalog.Status
	// SeenAt is when the producer last reported the condition.
	SeenAt time.Time
}

// RaiseResult says what Raise did.
type RaiseResult int

const (
	// Created: a new open warning was written.
	Created RaiseResult = iota + 1
	// Refreshed: the open warning existed; params/severity/updated_at refreshed.
	Refreshed
	// Suppressed: a warning with the same key was acknowledged or resolved by
	// the accountant and the condition still holds; it is not reopened.
	Suppressed
)

// Store persists warnings.
type Store interface {
	// Raise writes or refreshes the single open warning for w.Key. An
	// acknowledged / resolved warning with the same key and params is left
	// as is (Suppressed): the accountant's decision stands until the
	// condition changes.
	Raise(ctx context.Context, w Warning) (RaiseResult, error)
	// AutoResolve marks the open warnings of producer on the entity whose
	// code is in codes and not in keep as auto_resolved; returns how many.
	AutoResolve(ctx context.Context, s Subject, producer string, e Entity, codes, keep []string) (int, error)
}

// Effective is a resolved rule configuration.
type Effective struct {
	Code     string
	Enabled  bool
	Severity catalog.Severity
	Params   map[string]any
}

// ConfigResolver returns the effective configuration of codes for a subject.
type ConfigResolver interface {
	Resolve(ctx context.Context, s Subject, codes []string) (map[string]Effective, error)
}

// Engine applies findings.
type Engine struct {
	store  Store
	config ConfigResolver
	now    func() time.Time
}

// New builds an engine.
func New(store Store, config ConfigResolver) *Engine {
	return &Engine{store: store, config: config, now: time.Now}
}

// WithClock overrides the clock (tests).
func (e *Engine) WithClock(now func() time.Time) *Engine { e.now = now; return e }

// Report is what Apply did.
type Report struct {
	Created, Refreshed, Suppressed, Disabled, AutoResolved int
}

var producerRe = regexp.MustCompile(`^(adapter:[a-z0-9_]+|ubl_parser|excel_import|sync_engine|control:[a-z0-9_]+)$`)
var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// ValidProducer reports whether p follows 05 §1 producer naming.
func ValidProducer(p string) bool { return producerRe.MatchString(p) }

// Apply records the current findings of producer on entity. owned lists
// every code the producer checks on this entity; owned codes without a
// finding are auto-resolved. Findings must be a subset of owned.
func (e *Engine) Apply(ctx context.Context, s Subject, producer string, ent Entity, owned []string, findings []Finding) (Report, error) {
	var rep Report
	if s.OrganizationID <= 0 || s.EngagementID <= 0 || s.TaxpayerID <= 0 {
		return rep, ErrBadSubject
	}
	if !ValidProducer(producer) {
		return rep, fmt.Errorf("%w: %q", ErrBadProducer, producer)
	}
	if err := validateEntity(ent); err != nil {
		return rep, err
	}
	ownedSet := map[string]bool{}
	for _, c := range owned {
		cat, ok := catalog.Lookup(c)
		if !ok {
			return rep, fmt.Errorf("%w: %s", ErrUnknownCode, c)
		}
		if !cat.Allows(ent.Type) {
			return rep, fmt.Errorf("%w: %s on %s", ErrEntityNotAllow, c, ent.Type)
		}
		ownedSet[c] = true
	}
	found := map[string]Finding{}
	for _, f := range findings {
		if !ownedSet[f.Code] {
			return rep, fmt.Errorf("%w: %s", ErrNotOwned, f.Code)
		}
		found[f.Code] = f // last one wins; one warning per code per entity
	}

	cfg, err := e.config.Resolve(ctx, s, owned)
	if err != nil {
		return rep, fmt.Errorf("resolve warning config: %w", err)
	}
	now := e.now()
	var keep []string
	for code, f := range found {
		eff, ok := cfg[code]
		if !ok {
			return rep, fmt.Errorf("%w: %s has no configuration", ErrUnknownCode, code)
		}
		if !eff.Enabled {
			rep.Disabled++
			continue // disabled: not kept, so an open one auto-resolves
		}
		params, err := json.Marshal(nonNil(f.Params))
		if err != nil {
			return rep, fmt.Errorf("warning params %s: %w", code, err)
		}
		res, err := e.store.Raise(ctx, Warning{
			Key:            keyFor(s, ent, code),
			OrganizationID: s.OrganizationID,
			TaxpayerID:     s.TaxpayerID,
			Severity:       eff.Severity,
			Params:         params,
			Producer:       producer,
			Status:         catalog.StatusOpen,
			SeenAt:         now,
		})
		if err != nil {
			return rep, fmt.Errorf("raise %s: %w", code, err)
		}
		keep = append(keep, code)
		switch res {
		case Created:
			rep.Created++
		case Refreshed:
			rep.Refreshed++
		case Suppressed:
			rep.Suppressed++
		}
	}
	n, err := e.store.AutoResolve(ctx, s, producer, ent, owned, keep)
	if err != nil {
		return rep, fmt.Errorf("auto-resolve: %w", err)
	}
	rep.AutoResolved = n
	return rep, nil
}

func keyFor(s Subject, ent Entity, code string) Key {
	return Key{EngagementID: s.EngagementID, EntityType: ent.Type, EntityID: ent.ID, Period: ent.Period, Code: code}
}

func validateEntity(ent Entity) error {
	if !catalog.ValidEntityType(ent.Type) {
		return fmt.Errorf("%w: type %q", ErrBadEntity, ent.Type)
	}
	if ent.Period != "" && !periodRe.MatchString(ent.Period) {
		return fmt.Errorf("%w: period %q", ErrBadEntity, ent.Period)
	}
	if ent.ID == nil && ent.Period == "" {
		return fmt.Errorf("%w: entity without id needs a period", ErrBadEntity)
	}
	if ent.ID != nil && *ent.ID <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrBadEntity)
	}
	if ent.Type == catalog.EntityPeriod && ent.ID != nil {
		return fmt.Errorf("%w: period entity has no id", ErrBadEntity)
	}
	return nil
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// Outcome is a rule's verdict on one entity: the codes it checked there
// (owned) and the ones that hold (findings).
type Outcome struct {
	Entity   Entity
	Owned    []string
	Findings []Finding
}

// Rule is a control that checks an input of type T. Thresholds come from
// cfg (the effective params of the rule's codes), never from constants.
type Rule[T any] interface {
	Producer() string
	Codes() []string
	Check(in T, cfg map[string]Effective) ([]Outcome, error)
}

// Run resolves the rule's configuration, checks in and applies every outcome.
func Run[T any](ctx context.Context, e *Engine, s Subject, r Rule[T], in T) (Report, error) {
	var total Report
	cfg, err := e.config.Resolve(ctx, s, r.Codes())
	if err != nil {
		return total, fmt.Errorf("resolve warning config: %w", err)
	}
	outs, err := r.Check(in, cfg)
	if err != nil {
		return total, err
	}
	for _, o := range outs {
		rep, err := e.Apply(ctx, s, r.Producer(), o.Entity, o.Owned, o.Findings)
		if err != nil {
			return total, err
		}
		total.Created += rep.Created
		total.Refreshed += rep.Refreshed
		total.Suppressed += rep.Suppressed
		total.Disabled += rep.Disabled
		total.AutoResolved += rep.AutoResolved
	}
	return total, nil
}
