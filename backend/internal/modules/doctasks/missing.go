package doctasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Ay başı belge toplama (#780): the documents a taxpayer usually sends
// every month (bank statements, Z reports, expense receipts, purchase
// invoices) that have not arrived for a closed month, and asking for them
// in one go. Each item becomes a document request with a fixed title, so the
// same item is never opened twice while a request is open.

// SMSOutcome is what happened to the optional SMS of a missing document
// request: sent, dry_run (office test mode), failed, uncertain, or skipped
// (nothing new was requested, or the office cannot send SMS).
type SMSOutcome struct {
	Status string `json:"status"`
	// Code: why it was skipped or failed (not_configured, no_contact, …).
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// SMS statuses the service itself sets.
const (
	SMSSkipped = "skipped"
	SMSFailed  = "failed"
)

// SMS codes of a skipped / refused message.
const (
	SMSCodeNotConfigured = "not_configured"
	SMSCodeNoContact     = "no_contact"
	SMSCodeNothingNew    = "nothing_new"
)

// ErrSMSNotConfigured and ErrSMSNoContact are what an SMSSender returns
// when the office has no SMS account or the taxpayer no phone.
var (
	ErrSMSNotConfigured = errors.New("doctasks: office sms is not set up")
	ErrSMSNoContact     = errors.New("doctasks: taxpayer has no phone on record")
)

// SMSSender sends an SMS to an engagement's taxpayer through the office's
// account and records it in the taxpayer's communication history
// (communication module). Nil means SMS is never offered.
type SMSSender interface {
	// SMSStatus: the office can send SMS; testMode: records are written
	// but nothing is sent.
	SMSStatus(ctx context.Context, organizationID int64) (enabled, testMode bool, err error)
	SendToEngagement(ctx context.Context, organizationID int64, engagement uuid.UUID, text, sourceRule, sourceRef string, actorID int64) (SMSOutcome, error)
}

// WithSMS lets missing document requests also go out as SMS.
func (s *Service) WithSMS(sms SMSSender) *Service {
	s.sms = sms
	return s
}

// MissingView is the engagement's missing documents for one period.
type MissingView struct {
	Period string             `json:"period"`
	Items  []missingdocs.Item `json:"items"`
	// SMSAvailable: the office has SMS on; SMSTestMode: it is in test mode.
	SMSAvailable bool `json:"sms_available"`
	SMSTestMode  bool `json:"sms_test_mode"`
}

// MissingItemInput names one item to request.
type MissingItemInput struct {
	Kind string `json:"kind"`
	// Name: the bank account of a statement.
	Name string `json:"name"`
}

// MissingRequestInput asks for missing documents. With no items every
// missing item of the period is requested.
type MissingRequestInput struct {
	Period  string             `json:"period"`
	Items   []MissingItemInput `json:"items"`
	SMS     bool               `json:"sms"`
	DueDays int                `json:"due_days"`
}

// MissingRequestResult lists the requests opened now and the ones already
// open (not opened again).
type MissingRequestResult struct {
	Period   string      `json:"period"`
	Created  []Task      `json:"created"`
	Existing []Task      `json:"existing"`
	SMS      *SMSOutcome `json:"sms"`
}

// maxMissingItems caps one request.
const maxMissingItems = 30

// defaultMissingDueDays: the taxpayer has five days, the end of the
// document collection week.
const defaultMissingDueDays = 5

// previousPeriod is last month in Türkiye time: the month collected on
// days 1–5.
func (s *Service) previousPeriod() string {
	t := s.now().In(istanbul)
	return time.Date(t.Year(), t.Month()-1, 1, 12, 0, 0, 0, istanbul).Format("2006-01")
}

// Missing lists the engagement's missing documents for period (default:
// last month).
func (s *Service) Missing(ctx context.Context, scope engagementctx.Scope, period string) (MissingView, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = s.previousPeriod()
	}
	if !ValidPeriod(period) {
		return MissingView{}, ErrBadPeriod
	}
	items, err := s.missingItems(ctx, scope, period)
	if err != nil {
		return MissingView{}, err
	}
	v := MissingView{Period: period, Items: items}
	if s.sms != nil {
		on, test, err := s.sms.SMSStatus(ctx, scope.OrganizationID)
		if err != nil {
			s.log.Warn("doctasks_sms_status_failed", "error", err)
		}
		v.SMSAvailable, v.SMSTestMode = on, test
	}
	return v, nil
}

func (s *Service) missingItems(ctx context.Context, scope engagementctx.Scope, period string) ([]missingdocs.Item, error) {
	ids, periods := []int64{scope.ID}, []string{period}
	st, err := s.q.PeriodMatrixMissingStatements(ctx, db.PeriodMatrixMissingStatementsParams{OrganizationID: scope.OrganizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return nil, fmt.Errorf("doctasks: missing statements: %w", err)
	}
	names := make([]string, 0, len(st))
	for _, r := range st {
		names = append(names, r.Name)
	}
	docs, err := s.q.PeriodMatrixMissingDocuments(ctx, db.PeriodMatrixMissingDocumentsParams{OrganizationID: scope.OrganizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return nil, fmt.Errorf("doctasks: missing documents: %w", err)
	}
	var md db.PeriodMatrixMissingDocumentsRow
	if len(docs) > 0 {
		md = docs[0]
	}
	open, err := s.q.PeriodMatrixOpenRequestTitles(ctx, db.PeriodMatrixOpenRequestTitlesParams{OrganizationID: scope.OrganizationID, EngagementIds: ids, Periods: periods})
	if err != nil {
		return nil, fmt.Errorf("doctasks: open requests: %w", err)
	}
	titles := make([]string, 0, len(open))
	for _, r := range open {
		titles = append(titles, r.Title)
	}
	return missingdocs.Build(period, names, md.ZReports, md.Expenses, md.PurchaseInvoices, titles), nil
}

// RequestMissing opens a document request for every given item (or every
// missing item when none is given). An item already requested and still
// open is not opened again. With SMS the taxpayer also gets one short
// message listing the newly requested items.
func (s *Service) RequestMissing(ctx context.Context, scope engagementctx.Scope, actorID int64, in MissingRequestInput) (MissingRequestResult, error) {
	period := strings.TrimSpace(in.Period)
	if period == "" {
		period = s.previousPeriod()
	}
	if !ValidPeriod(period) {
		return MissingRequestResult{}, ErrBadPeriod
	}
	if len(in.Items) > maxMissingItems {
		return MissingRequestResult{}, fmt.Errorf("%w: items accepts at most %d entries", ErrBadInput, maxMissingItems)
	}
	items := make([]MissingItemInput, 0, len(in.Items))
	for _, it := range in.Items {
		it.Kind, it.Name = strings.TrimSpace(it.Kind), strings.TrimSpace(it.Name)
		if !missingdocs.ValidKind(it.Kind) {
			return MissingRequestResult{}, fmt.Errorf("%w: unknown kind %q", ErrBadInput, it.Kind)
		}
		if it.Kind == missingdocs.KindBankStatement && it.Name == "" {
			return MissingRequestResult{}, fmt.Errorf("%w: a statement needs the account name", ErrBadInput)
		}
		if len([]rune(it.Name)) > 120 {
			return MissingRequestResult{}, fmt.Errorf("%w: name is too long", ErrBadInput)
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		found, err := s.missingItems(ctx, scope, period)
		if err != nil {
			return MissingRequestResult{}, err
		}
		for _, f := range found {
			items = append(items, MissingItemInput{Kind: f.Kind, Name: f.Name})
		}
	}
	due := in.DueDays
	if due <= 0 {
		due = defaultMissingDueDays
	}
	out := MissingRequestResult{Period: period, Created: []Task{}, Existing: []Task{}}
	seen := map[string]bool{}
	labels := make([]string, 0, len(items))
	for _, it := range items {
		title := missingdocs.Title(it.Kind, period, it.Name)
		if seen[strings.ToLower(title)] {
			continue
		}
		seen[strings.ToLower(title)] = true
		t, err := s.CreateRequest(ctx, scope, actorID, RequestInput{Title: title, Note: missingdocs.Note(it.Kind), DueDays: due, Period: period})
		switch {
		case errors.Is(err, ErrRequestExists):
			out.Existing = append(out.Existing, t)
		case err != nil:
			return MissingRequestResult{}, err
		default:
			out.Created = append(out.Created, t)
			labels = append(labels, missingdocs.ShortLabel(it.Kind, it.Name))
		}
	}
	if in.SMS {
		out.SMS = s.missingSMS(ctx, scope, actorID, period, labels, out.Created)
	}
	return out, nil
}

// missingSMS sends one SMS for the newly opened requests. It never fails the
// request: the outcome says what happened.
func (s *Service) missingSMS(ctx context.Context, scope engagementctx.Scope, actorID int64, period string, labels []string, created []Task) *SMSOutcome {
	if s.sms == nil {
		return &SMSOutcome{Status: SMSSkipped, Code: SMSCodeNotConfigured}
	}
	if len(created) == 0 {
		return &SMSOutcome{Status: SMSSkipped, Code: SMSCodeNothingNew}
	}
	names, err := s.q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: scope.OrganizationID, TaxpayerID: scope.TaxpayerID})
	if err != nil {
		s.log.Warn("doctasks_sms_names_failed", "error", err)
		return &SMSOutcome{Status: SMSFailed, Code: "error"}
	}
	text := MissingSMSText(names.OrganizationName, period, labels)
	o, err := s.sms.SendToEngagement(ctx, scope.OrganizationID, scope.UUID, text, missingdocs.SourceRule, created[0].UUID.String(), actorID)
	switch {
	case errors.Is(err, ErrSMSNotConfigured):
		return &SMSOutcome{Status: SMSSkipped, Code: SMSCodeNotConfigured}
	case errors.Is(err, ErrSMSNoContact):
		return &SMSOutcome{Status: SMSSkipped, Code: SMSCodeNoContact}
	case err != nil:
		s.log.Warn("doctasks_sms_failed", "error", err, "engagement_id", scope.ID)
		return &SMSOutcome{Status: SMSFailed, Code: "error"}
	}
	return &o
}

// MissingSMSText is the SMS of a missing document request, short enough for
// one or two messages.
func MissingSMSText(office, period string, labels []string) string {
	list := strings.Join(labels, ", ")
	if r := []rune(list); len(r) > 200 {
		list = string(r[:197]) + "..."
	}
	return fmt.Sprintf("%s: %s için %s bekleniyor. Mükellef uygulamasından fotoğraf ya da PDF olarak yükleyebilirsiniz.",
		strings.TrimSpace(office), missingdocs.PeriodText(period), list)
}
