package copilot

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Errors.
var (
	ErrInvalid  = errors.New("copilot: invalid input")
	ErrNotFound = errors.New("copilot: not found")
	ErrState    = errors.New("copilot: plan state does not allow this")
)

// Workflow step keys, in order.
const (
	StepDocuments = "documents"
	StepBank      = "bank"
	StepReview    = "review"
	StepJournals  = "journals"
	StepTransfer  = "transfer"
	StepReport    = "report"
)

// StepKeys is the period close chain.
var StepKeys = []string{StepDocuments, StepBank, StepReview, StepJournals, StepTransfer, StepReport}

// Step statuses.
const (
	// StatusDone: nothing left for this step.
	StatusDone = "done"
	// StatusTodo: the office has work here (Action says what).
	StatusTodo = "todo"
	// StatusWaiting: started, the outcome is not back yet (a queued transfer
	// or report email).
	StatusWaiting = "waiting"
	// StatusBlocked: it failed and needs a look.
	StatusBlocked = "blocked"
	// StatusNA: nothing of this kind in the period.
	StatusNA = "na"
)

var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// DefaultPeriod is the month being closed: the previous month.
func DefaultPeriod(now time.Time) string {
	t := now.In(istanbul)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0).Format("2006-01")
}

// Step is one link of an engagement's chain.
type Step struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	// Count is how many items the status is about (open document requests,
	// pending suggestions, drafts to approve ...).
	Count int64 `json:"count"`
	// Action is what runs this step now; null when nothing to do.
	Action *string `json:"action"`
}

// Row is one engagement of the next-work list.
type Row struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	TaxpayerTaxID  string    `json:"taxpayer_tax_id"`
	Period         string    `json:"period"`
	Steps          []Step    `json:"steps"`
	// NextStep is the first step with work (todo or blocked); null when the
	// period is closed (or only waits for outcomes).
	NextStep  *string `json:"next_step"`
	DoneSteps int     `json:"done_steps"`
	// ActiveSteps excludes the steps with nothing in the period (na).
	ActiveSteps int `json:"active_steps"`

	id     int64
	counts Counts
}

// Counts are the inputs of one engagement's chain.
type Counts struct {
	DocsOpen, DocsSubmitted                                int64
	BankTotal, BankUnassigned, BankSuggested, BankApproved int64
	ReviewPending                                          int64
	JournalDraft, JournalApproved, JournalExported         int64
	TransferOpen, TransferImported, TransferFailed         int64
	ReportSent, ReportPending, ReportFailed                int64
}

func act(a string) *string { return &a }

// Chain computes the steps from the counts.
func Chain(c Counts) []Step {
	steps := make([]Step, 0, len(StepKeys))
	// Documents: the period's open document requests.
	if n := c.DocsOpen + c.DocsSubmitted; n > 0 {
		steps = append(steps, Step{Key: StepDocuments, Status: StatusTodo, Count: n, Action: act(ActionOpenDocuments)})
	} else {
		steps = append(steps, Step{Key: StepDocuments, Status: StatusDone})
	}
	// Bank: every movement proposed, then approved.
	switch {
	case c.BankTotal == 0:
		steps = append(steps, Step{Key: StepBank, Status: StatusNA})
	case c.BankUnassigned > 0:
		steps = append(steps, Step{Key: StepBank, Status: StatusTodo, Count: c.BankUnassigned, Action: act(ActionBankSuggest)})
	case c.BankSuggested > 0:
		steps = append(steps, Step{Key: StepBank, Status: StatusTodo, Count: c.BankSuggested, Action: act(ActionBankApprove)})
	default:
		steps = append(steps, Step{Key: StepBank, Status: StatusDone, Count: c.BankTotal})
	}
	// Review: the period's pending suggestions (bank ones are above).
	if c.ReviewPending > 0 {
		steps = append(steps, Step{Key: StepReview, Status: StatusTodo, Count: c.ReviewPending, Action: act(ActionOpenReview)})
	} else {
		steps = append(steps, Step{Key: StepReview, Status: StatusDone})
	}
	// Journals: approved movements become drafts, drafts get approved.
	switch {
	case c.BankApproved > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusTodo, Count: c.BankApproved, Action: act(ActionBankDrafts)})
	case c.JournalDraft > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusTodo, Count: c.JournalDraft, Action: act(ActionJournalsApprove)})
	case c.JournalApproved+c.JournalExported > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusDone, Count: c.JournalApproved + c.JournalExported})
	default:
		steps = append(steps, Step{Key: StepJournals, Status: StatusNA})
	}
	// Transfer: approved entries go to Luca / Zirve.
	switch {
	case c.TransferFailed > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusBlocked, Count: c.TransferFailed, Action: act(ActionOpenJournals)})
	case c.JournalApproved > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusTodo, Count: c.JournalApproved, Action: act(ActionExportTransfer)})
	case c.TransferOpen > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusWaiting, Count: c.TransferOpen})
	case c.JournalExported > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusDone, Count: c.JournalExported})
	default:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusNA})
	}
	// Report: the monthly report emailed to the taxpayer.
	switch {
	case c.ReportSent > 0:
		steps = append(steps, Step{Key: StepReport, Status: StatusDone, Count: c.ReportSent})
	case c.ReportPending > 0:
		steps = append(steps, Step{Key: StepReport, Status: StatusWaiting, Count: c.ReportPending})
	case c.ReportFailed > 0:
		steps = append(steps, Step{Key: StepReport, Status: StatusBlocked, Count: c.ReportFailed, Action: act(ActionReportSend)})
	default:
		steps = append(steps, Step{Key: StepReport, Status: StatusTodo, Action: act(ActionReportSend)})
	}
	return steps
}

func fill(r *Row, c Counts) {
	r.counts = c
	r.Steps = Chain(c)
	r.NextStep, r.DoneSteps, r.ActiveSteps = nil, 0, 0
	for _, s := range r.Steps {
		if s.Status != StatusNA {
			r.ActiveSteps++
		}
		if s.Status == StatusDone {
			r.DoneSteps++
		}
		if r.NextStep == nil && (s.Status == StatusTodo || s.Status == StatusBlocked) {
			k := s.Key
			r.NextStep = &k
		}
	}
}

// Queries is the db subset of the co-pilot.
type Queries interface {
	ListCopilotEngagements(ctx context.Context, arg db.ListCopilotEngagementsParams) ([]db.ListCopilotEngagementsRow, error)
	CountCopilotEngagements(ctx context.Context, arg db.CountCopilotEngagementsParams) (int64, error)
	CopilotDocumentTaskCounts(ctx context.Context, arg db.CopilotDocumentTaskCountsParams) ([]db.CopilotDocumentTaskCountsRow, error)
	CopilotBankCounts(ctx context.Context, arg db.CopilotBankCountsParams) ([]db.CopilotBankCountsRow, error)
	CopilotSuggestionCounts(ctx context.Context, arg db.CopilotSuggestionCountsParams) ([]db.CopilotSuggestionCountsRow, error)
	CopilotJournalCounts(ctx context.Context, arg db.CopilotJournalCountsParams) ([]db.CopilotJournalCountsRow, error)
	CopilotTransferCounts(ctx context.Context, arg db.CopilotTransferCountsParams) ([]db.CopilotTransferCountsRow, error)
	CopilotReportCounts(ctx context.Context, arg db.CopilotReportCountsParams) ([]db.CopilotReportCountsRow, error)
	InsertCopilotPlan(ctx context.Context, arg db.InsertCopilotPlanParams) (db.CopilotPlan, error)
	GetCopilotPlan(ctx context.Context, arg db.GetCopilotPlanParams) (db.CopilotPlan, error)
	GetCopilotPlanForUpdate(ctx context.Context, arg db.GetCopilotPlanForUpdateParams) (db.CopilotPlan, error)
	ListCopilotPlans(ctx context.Context, arg db.ListCopilotPlansParams) ([]db.CopilotPlan, error)
	UpdateCopilotPlan(ctx context.Context, arg db.UpdateCopilotPlanParams) (db.CopilotPlan, error)
	GetCopilotEngagement(ctx context.Context, arg db.GetCopilotEngagementParams) (db.GetCopilotEngagementRow, error)
}

// Viewer is who asks: the office, the user and whether the user reaches
// every engagement of the office (tenant.engagements.read_all).
type Viewer struct {
	OrganizationID int64
	UserID         int64
	ReadAll        bool
	// Has reports a tenant permission of the user.
	Has func(string) bool
}

// Filter narrows the next-work list.
type Filter struct {
	Period        string
	Search        string
	Limit, Offset int32
}

// Page is a page of the next-work list.
type Page struct {
	Period string `json:"period"`
	Items  []Row  `json:"items"`
	Total  int64  `json:"total"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}

// Service is the co-pilot service.
type Service struct {
	q   Queries
	now func() time.Time
}

// NewService builds the service.
func NewService(q Queries) *Service { return &Service{q: q, now: time.Now} }

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func searchParam(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: likeEscape(s), Valid: true}
}

// Workflow returns the next-work list of the engagements the viewer reaches.
func (s *Service) Workflow(ctx context.Context, v Viewer, f Filter) (Page, error) {
	if f.Period == "" {
		f.Period = DefaultPeriod(s.now())
	}
	if !ValidPeriod(f.Period) {
		return Page{}, ErrInvalid
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	search := searchParam(f.Search)
	rows, err := s.q.ListCopilotEngagements(ctx, db.ListCopilotEngagementsParams{
		OrganizationID: v.OrganizationID, AllEngagements: v.ReadAll, UserID: v.UserID, Search: search,
		Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return Page{}, err
	}
	total, err := s.q.CountCopilotEngagements(ctx, db.CountCopilotEngagementsParams{
		OrganizationID: v.OrganizationID, AllEngagements: v.ReadAll, UserID: v.UserID, Search: search,
	})
	if err != nil {
		return Page{}, err
	}
	items := make([]Row, 0, len(rows))
	for _, r := range rows {
		items = append(items, Row{EngagementUUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle,
			TaxpayerTaxID: r.TaxpayerTaxID, Period: f.Period, id: r.ID})
	}
	if err := s.fillRows(ctx, v.OrganizationID, f.Period, items); err != nil {
		return Page{}, err
	}
	return Page{Period: f.Period, Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

// EngagementRef is one engagement resolved for the viewer.
type EngagementRef struct {
	ID            int64
	UUID          uuid.UUID
	TaxpayerUUID  uuid.UUID
	TaxpayerTitle string
	TaxpayerTaxID string
}

// One returns one engagement's chain.
func (s *Service) One(ctx context.Context, organizationID int64, e EngagementRef, period string) (Row, error) {
	if period == "" {
		period = DefaultPeriod(s.now())
	}
	if !ValidPeriod(period) {
		return Row{}, ErrInvalid
	}
	rows := []Row{{EngagementUUID: e.UUID, TaxpayerUUID: e.TaxpayerUUID, TaxpayerTitle: e.TaxpayerTitle,
		TaxpayerTaxID: e.TaxpayerTaxID, Period: period, id: e.ID}}
	if err := s.fillRows(ctx, organizationID, period, rows); err != nil {
		return Row{}, err
	}
	return rows[0], nil
}

func (s *Service) fillRows(ctx context.Context, organizationID int64, period string, items []Row) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, len(items))
	for i, r := range items {
		ids[i] = r.id
	}
	counts := make(map[int64]*Counts, len(ids))
	for _, id := range ids {
		counts[id] = &Counts{}
	}
	docs, err := s.q.CopilotDocumentTaskCounts(ctx, db.CopilotDocumentTaskCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	for _, r := range docs {
		if c := counts[r.EngagementID]; c != nil {
			c.DocsOpen, c.DocsSubmitted = r.Open, r.Submitted
		}
	}
	bank, err := s.q.CopilotBankCounts(ctx, db.CopilotBankCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	for _, r := range bank {
		if c := counts[r.EngagementID]; c != nil {
			c.BankTotal, c.BankUnassigned, c.BankSuggested, c.BankApproved = r.Total, r.Unassigned, r.Suggested, r.Approved
		}
	}
	sugg, err := s.q.CopilotSuggestionCounts(ctx, db.CopilotSuggestionCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return err
	}
	for _, r := range sugg {
		if c := counts[r.EngagementID]; c != nil {
			c.ReviewPending = r.Pending
		}
	}
	jr, err := s.q.CopilotJournalCounts(ctx, db.CopilotJournalCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	for _, r := range jr {
		if c := counts[r.EngagementID]; c != nil {
			c.JournalDraft, c.JournalApproved, c.JournalExported = r.Draft, r.Approved, r.Exported
		}
	}
	tr, err := s.q.CopilotTransferCounts(ctx, db.CopilotTransferCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return err
	}
	for _, r := range tr {
		if c := counts[r.EngagementID]; c != nil {
			c.TransferOpen, c.TransferImported, c.TransferFailed = r.Open, r.Imported, r.Failed
		}
	}
	rep, err := s.q.CopilotReportCounts(ctx, db.CopilotReportCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	for _, r := range rep {
		if c := counts[r.EngagementID]; c != nil {
			c.ReportSent, c.ReportPending, c.ReportFailed = r.Sent, r.Pending, r.Failed
		}
	}
	for i := range items {
		fill(&items[i], *counts[items[i].id])
	}
	return nil
}
