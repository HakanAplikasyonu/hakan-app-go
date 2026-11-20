package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Service runs the review queue.
type Service struct {
	store    Store
	appliers map[string]Applier
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store, appliers: map[string]Applier{}}
}

// Register sets the applier for a suggestion kind.
func (s *Service) Register(kind string, a Applier) *Service {
	s.appliers[kind] = a
	return s
}

var (
	identRe  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	fieldRe  = regexp.MustCompile(`^[a-z0-9_]*$`)
	periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	one      = decimal.NewFromInt(1)
)

// Submit stores a suggestion. A pending suggestion for the same target field
// is superseded. When the policy accepts the confidence and the kind has an
// applier, the value is applied at once (auto_accepted, revertible).
func (s *Service) Submit(ctx context.Context, sub Subject, in Input) (Suggestion, error) {
	if sub.OrganizationID <= 0 || sub.EngagementID <= 0 || sub.TaxpayerID <= 0 {
		return Suggestion{}, ErrBadSubject
	}
	if err := validateInput(&in); err != nil {
		return Suggestion{}, err
	}
	rows, err := s.store.Settings(ctx, sub.OrganizationID, sub.EngagementID)
	if err != nil {
		return Suggestion{}, err
	}
	policy := ResolvePolicy(rows, sub.EngagementID, in.Kind)
	applier := s.appliers[in.Kind]

	var out Suggestion
	err = s.store.InTx(ctx, func(tx Tx) error {
		old, err := tx.SupersedePending(ctx, sub, in)
		if err != nil {
			return err
		}
		for _, o := range old {
			if err := tx.InsertDecision(ctx, sub, o.ID, Decision{
				Action: ActionSupersede, Reason: "replaced by a newer suggestion", Confidence: o.Confidence,
			}); err != nil {
				return err
			}
		}
		sug, err := tx.Insert(ctx, sub, in)
		if err != nil {
			return err
		}
		if applier == nil || !policy.Accepts(in.Confidence) {
			out = sug
			return nil
		}
		prev, err := applier.Apply(ctx, tx, sug, in.Value)
		if errors.Is(err, ErrInvalidValue) {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrApplyFailed, err)
		}
		sug.Status, sug.AppliedValue, sug.PreviousValue = StatusAutoAccepted, in.Value, nullIfEmpty(prev)
		if sug, err = tx.UpdateDecision(ctx, sug); err != nil {
			return err
		}
		thr := policy.Threshold
		out = sug
		return tx.InsertDecision(ctx, sub, sug.ID, Decision{
			Action: ActionAutoAccept, Value: in.Value, Confidence: in.Confidence, Threshold: &thr,
			Reason: fmt.Sprintf("confidence %s >= threshold %s", in.Confidence.StringFixed(4), thr.StringFixed(4)),
		})
	})
	return out, err
}

// Accept applies a pending suggestion as proposed.
func (s *Service) Accept(ctx context.Context, sub Subject, id uuid.UUID, userID int64, reason string) (Suggestion, error) {
	return s.decide(ctx, sub, id, func(ctx context.Context, tx Tx, sug *Suggestion) (Decision, error) {
		if sug.Status != StatusPending {
			return Decision{}, ErrState
		}
		a := s.appliers[sug.Input.Kind]
		if a == nil {
			return Decision{}, ErrNoApplier
		}
		prev, err := a.Apply(ctx, tx, *sug, sug.Input.Value)
		if errors.Is(err, ErrInvalidValue) {
			return Decision{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err != nil {
			return Decision{}, fmt.Errorf("%w: %v", ErrApplyFailed, err)
		}
		sug.Status, sug.AppliedValue, sug.PreviousValue = StatusAccepted, sug.Input.Value, nullIfEmpty(prev)
		if strings.TrimSpace(reason) == "" {
			reason = "accepted as proposed"
		}
		return Decision{Action: ActionAccept, Reason: reason, Value: sug.Input.Value}, nil
	}, userID)
}

// Reject closes a pending suggestion without applying it.
func (s *Service) Reject(ctx context.Context, sub Subject, id uuid.UUID, userID int64, reason string) (Suggestion, error) {
	if strings.TrimSpace(reason) == "" {
		return Suggestion{}, fmt.Errorf("%w: reason is required", ErrInvalid)
	}
	return s.decide(ctx, sub, id, func(_ context.Context, _ Tx, sug *Suggestion) (Decision, error) {
		if sug.Status != StatusPending {
			return Decision{}, ErrState
		}
		sug.Status = StatusRejected
		return Decision{Action: ActionReject, Reason: reason}, nil
	}, userID)
}

// Correct applies a different value than proposed (pending or already
// applied suggestions) and marks the suggestion as a rule candidate.
func (s *Service) Correct(ctx context.Context, sub Subject, id uuid.UUID, userID int64, value json.RawMessage, reason string) (Suggestion, error) {
	if strings.TrimSpace(reason) == "" {
		return Suggestion{}, fmt.Errorf("%w: reason is required", ErrInvalid)
	}
	if !json.Valid(value) || len(bytes.TrimSpace(value)) == 0 {
		return Suggestion{}, fmt.Errorf("%w: value must be JSON", ErrInvalid)
	}
	return s.decide(ctx, sub, id, func(ctx context.Context, tx Tx, sug *Suggestion) (Decision, error) {
		if sug.Status != StatusPending && !sug.Status.Applied() {
			return Decision{}, ErrState
		}
		a := s.appliers[sug.Input.Kind]
		if a == nil {
			return Decision{}, ErrNoApplier
		}
		prev, err := a.Apply(ctx, tx, *sug, value)
		if errors.Is(err, ErrInvalidValue) {
			return Decision{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err != nil {
			return Decision{}, fmt.Errorf("%w: %v", ErrApplyFailed, err)
		}
		// Keep the value from before the first application so a revert
		// restores the original, not the auto-applied one.
		if !sug.Status.Applied() {
			sug.PreviousValue = nullIfEmpty(prev)
		}
		sug.Status, sug.AppliedValue, sug.RuleCandidate = StatusCorrected, value, true
		return Decision{Action: ActionCorrect, Reason: reason, Value: value}, nil
	}, userID)
}

// Revert undoes an applied suggestion (auto-accepted, accepted or corrected).
func (s *Service) Revert(ctx context.Context, sub Subject, id uuid.UUID, userID int64, reason string) (Suggestion, error) {
	if strings.TrimSpace(reason) == "" {
		return Suggestion{}, fmt.Errorf("%w: reason is required", ErrInvalid)
	}
	return s.decide(ctx, sub, id, func(ctx context.Context, tx Tx, sug *Suggestion) (Decision, error) {
		if !sug.Status.Applied() {
			return Decision{}, ErrState
		}
		a := s.appliers[sug.Input.Kind]
		if a == nil {
			return Decision{}, ErrNoApplier
		}
		if err := a.Revert(ctx, tx, *sug, sug.PreviousValue); err != nil {
			return Decision{}, fmt.Errorf("%w: %v", ErrApplyFailed, err)
		}
		sug.Status = StatusReverted
		return Decision{Action: ActionRevert, Reason: reason, Value: sug.PreviousValue}, nil
	}, userID)
}

// Get returns a suggestion with its decision log.
func (s *Service) Get(ctx context.Context, sub Subject, id uuid.UUID) (Suggestion, []Decision, error) {
	return s.store.Get(ctx, sub, id)
}

// List returns one page of suggestions.
func (s *Service) List(ctx context.Context, f Filter) ([]ListRow, int64, error) {
	if f.Status != "" && !Status(f.Status).Valid() {
		return nil, 0, fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if f.Kind != "" && !identRe.MatchString(f.Kind) {
		return nil, 0, fmt.Errorf("%w: kind", ErrInvalid)
	}
	return s.store.List(ctx, f)
}

// ByEngagement counts the queue per taxpayer with the same filter as List
// (status, kind, period, visibility), largest first (#710).
func (s *Service) ByEngagement(ctx context.Context, f Filter) ([]EngagementCount, error) {
	if f.Status != "" && !Status(f.Status).Valid() {
		return nil, fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if f.Kind != "" && !identRe.MatchString(f.Kind) {
		return nil, fmt.Errorf("%w: kind", ErrInvalid)
	}
	return s.store.ByEngagement(ctx, f)
}

func (s *Service) decide(ctx context.Context, sub Subject, id uuid.UUID,
	fn func(context.Context, Tx, *Suggestion) (Decision, error), userID int64) (Suggestion, error) {
	var out Suggestion
	err := s.store.InTx(ctx, func(tx Tx) error {
		sug, err := tx.GetForUpdate(ctx, sub, id)
		if err != nil {
			return err
		}
		d, err := fn(ctx, tx, &sug)
		if err != nil {
			return err
		}
		var actor *int64
		if userID > 0 {
			actor = &userID
			sug.DecidedBy = actor
		}
		if sug, err = tx.UpdateDecision(ctx, sug); err != nil {
			return err
		}
		d.ActorUserID, d.Confidence = actor, sug.Input.Confidence
		out = sug
		return tx.InsertDecision(ctx, sub, sug.ID, d)
	})
	return out, err
}

func validateInput(in *Input) error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrInvalid, msg) }
	in.Rationale = strings.TrimSpace(in.Rationale)
	switch {
	case !identRe.MatchString(in.Kind) || len(in.Kind) > 64:
		return bad("kind must be snake_case")
	case !identRe.MatchString(in.TargetType) || len(in.TargetType) > 64:
		return bad("target_type must be snake_case")
	case !fieldRe.MatchString(in.TargetField) || len(in.TargetField) > 64:
		return bad("target_field must be snake_case")
	case in.TargetID != nil && *in.TargetID <= 0:
		return bad("target_id must be positive")
	case in.Period != "" && !periodRe.MatchString(in.Period):
		return bad("period must be YYYY-MM")
	case len(bytes.TrimSpace(in.Value)) == 0 || !json.Valid(in.Value):
		return bad("value must be JSON")
	case in.Confidence.IsNegative() || in.Confidence.GreaterThan(one):
		return bad("confidence must be between 0 and 1")
	case in.Rationale == "":
		return bad("rationale is required")
	}
	p := in.Provenance
	switch p.Origin {
	case OriginDocument:
		if p.SourceEntityType == "" || p.SourceEntityID == nil {
			return bad("document origin needs the source document")
		}
	case OriginRule:
		if p.RuleRef == "" {
			return bad("rule origin needs rule_ref")
		}
	case OriginModel:
		if p.Model == "" {
			return bad("model origin needs model")
		}
	case OriginInput:
	default:
		return bad("origin must be document, rule, model or input")
	}
	if p.Inputs == nil {
		in.Provenance.Inputs = map[string]any{}
	}
	in.Confidence = in.Confidence.Round(4)
	return nil
}

func nullIfEmpty(v json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(v)) == 0 {
		return json.RawMessage("null")
	}
	return v
}

// IsNotFound reports whether err is a not-found error.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// SettingsView is the review_settings rows of a level plus the effective
// policy of every kind they mention.
type SettingsView struct {
	Rows      []SettingRow      `json:"-"`
	Default   Policy            `json:"default"`
	Effective map[string]Policy `json:"effective"`
}

// Settings returns the office (engagementID 0) or engagement settings.
func (s *Service) Settings(ctx context.Context, organizationID, engagementID int64) (SettingsView, error) {
	rows, err := s.store.Settings(ctx, organizationID, engagementID)
	if err != nil {
		return SettingsView{}, err
	}
	v := SettingsView{
		Rows:      rows,
		Default:   Policy{AutoAccept: DefaultAutoAccept, Threshold: DefaultThreshold},
		Effective: map[string]Policy{AnyKind: ResolvePolicy(rows, engagementID, AnyKind)},
	}
	for _, r := range rows {
		v.Effective[r.Kind] = ResolvePolicy(rows, engagementID, r.Kind)
	}
	return v, nil
}

// SetSetting writes one level's setting for a kind ("*" = every kind).
func (s *Service) SetSetting(ctx context.Context, organizationID, engagementID int64, row SettingRow, userID int64) error {
	if row.Kind == "" {
		row.Kind = AnyKind
	}
	if row.Kind != AnyKind && !identRe.MatchString(row.Kind) {
		return fmt.Errorf("%w: kind", ErrInvalid)
	}
	if row.Threshold != nil && (row.Threshold.IsNegative() || row.Threshold.GreaterThan(one)) {
		return fmt.Errorf("%w: threshold must be between 0 and 1", ErrInvalid)
	}
	if row.Threshold != nil {
		t := row.Threshold.Round(4)
		row.Threshold = &t
	}
	return s.store.UpsertSetting(ctx, organizationID, engagementID, row, userID)
}

// DeleteSetting removes one level's setting for a kind.
func (s *Service) DeleteSetting(ctx context.Context, organizationID, engagementID int64, kind string) error {
	ok, err := s.store.DeleteSetting(ctx, organizationID, engagementID, kind)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// Threshold-impact bounds (issue #333).
var (
	MinImpactThreshold  = decimal.RequireFromString("0.50")
	DefaultImpactMonths = 6
	MaxImpactMonths     = 24
)

// ThresholdImpact summarises what an office-wide threshold would have done
// to the suggestions decided in the last Months months.
type ThresholdImpact struct {
	Threshold         string  `json:"threshold"`
	Months            int     `json:"months"`
	Kind              *string `json:"kind"`
	CurrentThreshold  string  `json:"current_threshold"`
	CurrentAutoAccept bool    `json:"current_auto_accept"`
	ImpactCounts
}

// ThresholdImpact validates f and runs the aggregate. Threshold must be
// 0.50–1.00 with at most two decimals, Months 1–24 (0 = default 6), Kind
// optional. current_* is the office's effective policy for the kind.
func (s *Service) ThresholdImpact(ctx context.Context, f ImpactFilter) (ThresholdImpact, error) {
	organizationID, threshold, months, kind := f.OrganizationID, f.Threshold, f.Months, f.Kind
	switch {
	case threshold.LessThan(MinImpactThreshold) || threshold.GreaterThan(one):
		return ThresholdImpact{}, fmt.Errorf("%w: threshold must be between 0.50 and 1.00", ErrInvalid)
	case !threshold.Equal(threshold.Round(2)):
		return ThresholdImpact{}, fmt.Errorf("%w: threshold must have at most two decimals", ErrInvalid)
	}
	if months == 0 {
		months = DefaultImpactMonths
	}
	if months < 1 || months > MaxImpactMonths {
		return ThresholdImpact{}, fmt.Errorf("%w: months must be between 1 and %d", ErrInvalid, MaxImpactMonths)
	}
	if kind != "" && (!identRe.MatchString(kind) || len(kind) > 64) {
		return ThresholdImpact{}, fmt.Errorf("%w: kind", ErrInvalid)
	}
	rows, err := s.store.Settings(ctx, organizationID, 0)
	if err != nil {
		return ThresholdImpact{}, err
	}
	policyKind := kind
	if policyKind == "" {
		policyKind = AnyKind
	}
	policy := ResolvePolicy(rows, 0, policyKind)
	f.Months = months
	counts, err := s.store.ImpactCounts(ctx, f)
	if err != nil {
		return ThresholdImpact{}, err
	}
	out := ThresholdImpact{
		Threshold: threshold.StringFixed(2), Months: months,
		CurrentThreshold: policy.Threshold.StringFixed(4), CurrentAutoAccept: policy.AutoAccept, ImpactCounts: counts,
	}
	if kind != "" {
		out.Kind = &kind
	}
	return out, nil
}
