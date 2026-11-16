package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

// Rule candidate settings (issue #333, user decision of 16 November): the
// office decides how many matching decisions make a "make it a rule?"
// question and which decisions count. Automatic acceptances never count.
const (
	DecisionsCorrectionsOnly       = "corrections_only"
	DecisionsCorrectionsAndAccepts = "corrections_and_accepts"
	MinCandidateCountLow           = 2
	MinCandidateCountHigh          = 10
)

// RuleSettings is an office's rule candidate setting.
type RuleSettings struct {
	MinCount  int        `json:"rule_candidate_min_count"`
	Decisions string     `json:"rule_candidate_decisions"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Default   bool       `json:"default"`
}

// DefaultRuleSettings is today's behaviour (3 decisions, corrections and
// manual acceptances).
func DefaultRuleSettings() RuleSettings {
	return RuleSettings{MinCount: MinCandidateDecisions, Decisions: DecisionsCorrectionsAndAccepts, Default: true}
}

// ErrInvalidSettings is a setting outside the allowed values.
var ErrInvalidSettings = errors.New("accountrules: invalid rule candidate settings")

// Validate checks the allowed range and choices.
func (r RuleSettings) Validate() error {
	if r.MinCount < MinCandidateCountLow || r.MinCount > MinCandidateCountHigh {
		return fmt.Errorf("%w: rule_candidate_min_count must be %d to %d", ErrInvalidSettings, MinCandidateCountLow, MinCandidateCountHigh)
	}
	if r.Decisions != DecisionsCorrectionsOnly && r.Decisions != DecisionsCorrectionsAndAccepts {
		return fmt.Errorf("%w: rule_candidate_decisions must be %s or %s", ErrInvalidSettings, DecisionsCorrectionsOnly, DecisionsCorrectionsAndAccepts)
	}
	return nil
}

func (r RuleSettings) includeAccepts() bool { return r.Decisions == DecisionsCorrectionsAndAccepts }

// RuleSettings returns the office's setting.
func (s *Service) RuleSettings(ctx context.Context, org int64) (RuleSettings, error) {
	return s.store.RuleSettings(ctx, org)
}

// SaveRuleSettings stores the office's setting.
func (s *Service) SaveRuleSettings(ctx context.Context, org int64, in RuleSettings, userID int64) (RuleSettings, error) {
	if err := in.Validate(); err != nil {
		return RuleSettings{}, err
	}
	return s.store.SaveRuleSettings(ctx, org, in, userID)
}

// SettingsPreview compares the candidates the office has today with the
// ones a proposed setting would give, over every active engagement.
type SettingsPreview struct {
	Current  PreviewSide `json:"current"`
	Proposed PreviewSide `json:"proposed"`
}

// PreviewSide is one setting and its candidate count.
type PreviewSide struct {
	RuleSettings
	Candidates  int `json:"candidates"`
	Engagements int `json:"engagements"`
}

// PreviewRuleSettings counts the open candidates under the current and the
// proposed setting (rules already covering a candidate excluded, like the
// candidate list). Nothing is stored.
func (s *Service) PreviewRuleSettings(ctx context.Context, org int64, proposed RuleSettings) (SettingsPreview, error) {
	if err := proposed.Validate(); err != nil {
		return SettingsPreview{}, err
	}
	cur, err := s.store.RuleSettings(ctx, org)
	if err != nil {
		return SettingsPreview{}, err
	}
	engs, err := s.store.ActiveEngagements(ctx, org)
	if err != nil {
		return SettingsPreview{}, err
	}
	out := SettingsPreview{Current: PreviewSide{RuleSettings: cur}, Proposed: PreviewSide{RuleSettings: proposed}}
	for _, sub := range engs {
		for _, side := range []*PreviewSide{&out.Current, &out.Proposed} {
			cs, err := s.candidates(ctx, sub, side.RuleSettings)
			if err != nil {
				return SettingsPreview{}, err
			}
			side.Candidates += len(cs)
			if len(cs) > 0 {
				side.Engagements++
			}
		}
	}
	return out, nil
}

// PreviewLine is one invoice line a rule preview evaluates.
type PreviewLine struct {
	EngagementID   int64
	InvoiceID      int64
	InvoiceUUID    uuid.UUID
	InvoiceNumber  string
	IssueDate      time.Time
	Direction      string
	InvoiceType    string
	CustomerID     int64
	TaxNumber      string
	IdentityNumber string
	CustomerName   string
	ItemID         int64
	ItemName       string
	LineType       string
	TaxRate        decimal.Decimal
	LineCode       string // approved account code of the line ("" = none)
	VATCode        string
	CustomerCode   string
}

// RulePreview is what a rule would have matched (#333): in the last
// `months` months, how many documents and lines it fits and how many of
// those were approved with another account code.
type RulePreview struct {
	Months            int                `json:"months"`
	Since             string             `json:"since"`
	Documents         int                `json:"documents"`
	Lines             int                `json:"lines"`
	Approved          int                `json:"approved"`
	ApprovedDifferent int                `json:"approved_different"`
	Truncated         bool               `json:"truncated"`
	Samples           []RulePreviewMatch `json:"samples"`
}

// RulePreviewMatch is a matched line approved with another code.
type RulePreviewMatch struct {
	InvoiceUUID   uuid.UUID `json:"invoice_uuid"`
	InvoiceNumber string    `json:"invoice_number"`
	IssueDate     string    `json:"issue_date"`
	ItemName      string    `json:"item_name,omitempty"`
	ApprovedCode  string    `json:"approved_code"`
}

// Preview bounds.
const (
	PreviewMonths    = 6
	previewLineLimit = 20000
	previewSamples   = 10
)

// PreviewRule evaluates an unsaved rule over the last PreviewMonths months
// of one engagement (eng > 0) or of every active engagement of the office.
// A line rule matches invoice lines, a VAT rule lines with VAT, a
// counterparty rule whole invoices; "approved" is the field's confirmed
// account code (account_assignments). Nothing is stored.
func (s *Service) PreviewRule(ctx context.Context, org, eng int64, in RuleInput, now time.Time) (RulePreview, error) {
	if err := validateRule(&in, engine.OriginManual); err != nil {
		return RulePreview{}, err
	}
	since := now.AddDate(0, -PreviewMonths, 0)
	lines, err := s.store.PreviewLines(ctx, org, eng, since, previewLineLimit+1)
	if err != nil {
		return RulePreview{}, err
	}
	out := RulePreview{Months: PreviewMonths, Since: since.Format(time.DateOnly), Samples: []RulePreviewMatch{}}
	if len(lines) > previewLineLimit {
		lines, out.Truncated = lines[:previewLineLimit], true
	}
	docs := map[int64]bool{}
	counted := map[int64]bool{} // counterparty rules: one decision per invoice
	for _, l := range lines {
		x := engine.Context{Direction: l.Direction, InvoiceType: l.InvoiceType,
			CounterpartyTaxID: firstNonEmpty(l.TaxNumber, l.IdentityNumber), CounterpartyName: l.CustomerName}
		approved := ""
		switch in.Role {
		case engine.RoleCounterparty:
			if counted[l.InvoiceID] {
				continue
			}
			approved = l.CustomerCode
		case engine.RoleVAT:
			if !l.TaxRate.IsPositive() {
				continue
			}
			x.ItemName, x.LineType, x.TaxRate = l.ItemName, l.LineType, l.TaxRate
			approved = l.VATCode
		default:
			x.ItemName, x.LineType, x.TaxRate = l.ItemName, l.LineType, l.TaxRate
			approved = l.LineCode
		}
		if ok, _ := in.Conditions.Match(x); !ok {
			continue
		}
		counted[l.InvoiceID] = true
		docs[l.InvoiceID] = true
		out.Lines++
		if approved == "" {
			continue
		}
		out.Approved++
		if approved != in.AccountCode {
			out.ApprovedDifferent++
			if len(out.Samples) < previewSamples {
				m := RulePreviewMatch{InvoiceUUID: l.InvoiceUUID, InvoiceNumber: l.InvoiceNumber,
					IssueDate: l.IssueDate.Format(time.DateOnly), ApprovedCode: approved}
				if in.Role != engine.RoleCounterparty {
					m.ItemName = l.ItemName
				}
				out.Samples = append(out.Samples, m)
			}
		}
	}
	out.Documents = len(docs)
	return out, nil
}
