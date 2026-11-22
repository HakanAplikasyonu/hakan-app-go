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
	// StepLucaImport: Luca'dan alınanlar (#780) — dönemin bordro dökümü ve
	// Luca mizanı yüklendi, mizan karşılaştırması fark göstermiyor.
	StepLucaImport = "luca_import"
	// StepVAT: mükellefe dönemin KDV bildirimi gitti (#780, vat_notices).
	StepVAT    = "vat"
	StepReport = "report"
)

// StepKeys is the period close chain ("ay sonu kapanış", #780): Toplandı →
// Eşleşti → Fişler onaylı → Luca'ya aktarıldı → Luca'dan alındı → KDV
// bildirildi → Rapor.
var StepKeys = []string{StepDocuments, StepBank, StepReview, StepJournals, StepTransfer, StepLucaImport, StepVAT, StepReport}

// ValidStep reports whether k is a step key.
func ValidStep(k string) bool {
	for _, s := range StepKeys {
		if s == k {
			return true
		}
	}
	return false
}

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
	// Target is the one record the step's work is about, when there is one:
	// the oldest pending document request of the documents step. The office
	// opens that request itself instead of a general list.
	Target *uuid.UUID `json:"target_uuid,omitempty"`
}

// Row is one engagement of the next-work list.
type Row struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	TaxpayerTaxID  string    `json:"taxpayer_tax_id"`
	Period         string    `json:"period"`
	// LedgerType is the engagement's defter türü (bilanco, isletme,
	// serbest_meslek): the single-entry books have no bank postings,
	// journal drafts or transfers, so those steps read "not kept".
	LedgerType string `json:"ledger_type"`
	Steps      []Step `json:"steps"`
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
	DocsOpen, DocsSubmitted int64
	// DocsFirst: the oldest pending document request (zero when none).
	DocsFirst                                              uuid.UUID
	BankTotal, BankUnassigned, BankSuggested, BankApproved int64
	ReviewPending                                          int64
	JournalDraft, JournalApproved, JournalExported         int64
	TransferOpen, TransferImported, TransferFailed         int64
	// TransferDownloaded: Luca / Zirve exports only downloaded (no live
	// transfer); "Luca'ya yükledim" closes them (#780).
	TransferDownloaded                      int64
	ReportSent, ReportPending, ReportFailed int64
	// Pending masraf fişleri and Z raporları of the period (#780).
	ExpensesPending, ZReportsPending int64
	// Approved masraf fişleri and Z raporları of the period: on a
	// single-entry book they close the journals step.
	ExpensesApproved, ZReportsApproved int64
	// BankAccounts: the taxpayer's bank accounts and cards on file. On a
	// single-entry book the bank step is expected only with one (or with a
	// statement in the period); otherwise it is optional.
	BankAccounts int64
	// Luca'dan alınanlar (#780).
	PayrollExpected, PayrollImported, TrialBalanceImported, Reconciled bool
	ReconciliationIssues                                               int64
	// NoTrialBalance: işletme / serbest meslek defteri, Luca mizanı yok.
	NoTrialBalance bool
	// VATNotices: KDV bildirimleri of the period (#780).
	VATNotices int64
	// SingleEntry: işletme defteri / serbest meslek kazanç defteri.
	// No account postings, journal drafts or transfers exist for them, so
	// the chain never offers an action the API refuses with
	// LEDGER_NOT_BALANCE_SHEET (bank suggestions, drafts, approval,
	// transfer). Masraf fişleri and Z raporları make the journals step;
	// the bank step checks the statement (optional without an account).
	SingleEntry bool
}

// ledgerSingleEntry reports whether a ledger type keeps a single-entry
// book (anything but bilanço).
func ledgerSingleEntry(t string) bool { return t != "" && t != "bilanco" }

// withoutLedger drops the bilanço-only counts of a single-entry engagement.
func (c Counts) withoutLedger() Counts {
	if !c.SingleEntry {
		return c
	}
	// BankTotal stays: the statement check of the bank step reads it.
	c.BankUnassigned, c.BankSuggested, c.BankApproved = 0, 0, 0
	c.JournalDraft, c.JournalApproved, c.JournalExported = 0, 0, 0
	c.TransferOpen, c.TransferImported, c.TransferFailed, c.TransferDownloaded = 0, 0, 0, 0
	return c
}

func act(a string) *string { return &a }

// Chain computes the steps from the counts.
func Chain(c Counts) []Step {
	c = c.withoutLedger()
	steps := make([]Step, 0, len(StepKeys))
	// Documents: the period's open document requests.
	if n := c.DocsOpen + c.DocsSubmitted; n > 0 {
		st := Step{Key: StepDocuments, Status: StatusTodo, Count: n, Action: act(ActionOpenDocuments)}
		if c.DocsFirst != uuid.Nil {
			id := c.DocsFirst
			st.Target = &id
		}
		steps = append(steps, st)
	} else {
		steps = append(steps, Step{Key: StepDocuments, Status: StatusDone})
	}
	// Bank: every movement proposed, then approved. Single-entry book: no
	// account suggestion; the step checks that the period's statement is
	// in (optional when the taxpayer has no bank account on file).
	switch {
	case c.SingleEntry && c.BankTotal > 0:
		steps = append(steps, Step{Key: StepBank, Status: StatusDone, Count: c.BankTotal})
	case c.SingleEntry && c.BankAccounts > 0:
		steps = append(steps, Step{Key: StepBank, Status: StatusTodo, Count: c.BankAccounts, Action: act(ActionOpenBank)})
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
	// Onay bekleyen masraf ve Z raporları fişe dönmedi: adım bitmiş
	// görünmesin (#780).
	case c.ExpensesPending > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusTodo, Count: c.ExpensesPending, Action: act(ActionOpenExpenses)})
	case c.ZReportsPending > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusTodo, Count: c.ZReportsPending, Action: act(ActionOpenZReports)})
	case c.JournalApproved+c.JournalExported > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusDone, Count: c.JournalApproved + c.JournalExported})
	// İşletme defteri: onaylanan masraf ve Z raporları defterdedir.
	case c.SingleEntry && c.ExpensesApproved+c.ZReportsApproved > 0:
		steps = append(steps, Step{Key: StepJournals, Status: StatusDone, Count: c.ExpensesApproved + c.ZReportsApproved})
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
	// Yalnız indirilen çıktı aktarılmış sayılmaz: dosya programa yüklenince
	// "Luca'ya yükledim" ile işaretlenir (#780).
	case c.TransferDownloaded > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusTodo, Count: c.TransferDownloaded, Action: act(ActionOpenExport)})
	case c.JournalExported > 0:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusDone, Count: c.JournalExported})
	default:
		steps = append(steps, Step{Key: StepTransfer, Status: StatusNA})
	}
	// Luca'dan alındı (#780): bordro Luca'da kalır; dönemin bordro dökümü
	// (çalışanı olan mükellefte) ve Luca mizanı yüklenir. Mizan
	// karşılaştırması fark gösteriyorsa adım bakılmayı bekler. İşletme ve
	// serbest meslek defterinde mizan yok.
	missingPayroll := c.PayrollExpected && !c.PayrollImported
	missingMizan := !c.NoTrialBalance && !c.TrialBalanceImported
	var missing int64
	if missingPayroll {
		missing++
	}
	if missingMizan {
		missing++
	}
	switch {
	case c.NoTrialBalance && !c.PayrollExpected:
		steps = append(steps, Step{Key: StepLucaImport, Status: StatusNA})
	case missingPayroll:
		steps = append(steps, Step{Key: StepLucaImport, Status: StatusTodo, Count: missing, Action: act(ActionOpenPayroll)})
	case missingMizan:
		steps = append(steps, Step{Key: StepLucaImport, Status: StatusTodo, Count: missing, Action: act(ActionOpenTrialBalance)})
	case c.Reconciled && c.ReconciliationIssues > 0:
		steps = append(steps, Step{Key: StepLucaImport, Status: StatusBlocked, Count: c.ReconciliationIssues, Action: act(ActionOpenReconciliation)})
	default:
		steps = append(steps, Step{Key: StepLucaImport, Status: StatusDone})
	}
	// KDV bildirildi (#780): mükellefe dönemin KDV durumu gönderildi.
	if c.VATNotices > 0 {
		steps = append(steps, Step{Key: StepVAT, Status: StatusDone, Count: c.VATNotices})
	} else {
		steps = append(steps, Step{Key: StepVAT, Status: StatusTodo, Action: act(ActionOpenVAT)})
	}
	// Report: the monthly report emailed to the taxpayer. It closes the
	// chain: while an earlier step is open the report waits (a report sent
	// before the books are done is sent again after them, #562).
	earlierOpen := false
	for _, s := range steps {
		if s.Status == StatusTodo || s.Status == StatusBlocked || s.Status == StatusWaiting {
			earlierOpen = true
		}
	}
	switch {
	case earlierOpen:
		steps = append(steps, Step{Key: StepReport, Status: StatusWaiting, Count: c.ReportSent + c.ReportPending + c.ReportFailed})
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
	// The planner reads r.counts too: a single-entry engagement gets no
	// bank or journal step there either.
	c = c.withoutLedger()
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
	CopilotReceiptCounts(ctx context.Context, arg db.CopilotReceiptCountsParams) ([]db.CopilotReceiptCountsRow, error)
	CopilotLucaImportCounts(ctx context.Context, arg db.CopilotLucaImportCountsParams) ([]db.CopilotLucaImportCountsRow, error)
	CopilotVATNoticeCounts(ctx context.Context, arg db.CopilotVATNoticeCountsParams) ([]db.CopilotVATNoticeCountsRow, error)
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
	// Step: only the engagements whose next step is this one (#780).
	Step string
	// Summary: count the engagements by next step (office counters, #780).
	Summary bool
}

// Page is a page of the next-work list.
type Page struct {
	Period string `json:"period"`
	Items  []Row  `json:"items"`
	Total  int64  `json:"total"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
	// Summary is set when asked for (summary=true).
	Summary *Summary `json:"summary,omitempty"`
}

// StepCount is how many engagements have a step as their next step.
type StepCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// Summary counts the office's engagements by their next step for the
// period ("12 mükellef Toplandı'da, 8'i Luca'ya aktarımda", #780).
type Summary struct {
	// Steps: one entry per step key, in chain order.
	Steps []StepCount `json:"steps"`
	// Waiting: nothing to do, an outcome is not back yet.
	Waiting int64 `json:"waiting"`
	// Closed: every step done.
	Closed int64 `json:"closed"`
	// Scanned: engagements counted; Truncated when the office has more
	// than MaxScan engagements (the rest is not counted).
	Scanned   int64 `json:"scanned"`
	Truncated bool  `json:"truncated"`
}

// MaxScan bounds the engagements the counters and the step filter read
// (in pages of scanChunk).
const (
	MaxScan   = 2000
	scanChunk = 200
)

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
	if f.Step != "" && !ValidStep(f.Step) {
		return Page{}, ErrInvalid
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	search := searchParam(f.Search)
	if f.Step != "" || f.Summary {
		return s.scanned(ctx, v, f, search)
	}
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

// scanned answers a list with counters or a step filter: the chains of up to
// MaxScan engagements are computed in pages, counted, filtered and paged.
func (s *Service) scanned(ctx context.Context, v Viewer, f Filter, search pgtype.Text) (Page, error) {
	var all []Row
	truncated := false
	for off := int32(0); ; off += scanChunk {
		rows, err := s.q.ListCopilotEngagements(ctx, db.ListCopilotEngagementsParams{
			OrganizationID: v.OrganizationID, AllEngagements: v.ReadAll, UserID: v.UserID, Search: search,
			Lim: scanChunk, Off: off,
		})
		if err != nil {
			return Page{}, err
		}
		chunk := make([]Row, 0, len(rows))
		for _, r := range rows {
			chunk = append(chunk, Row{EngagementUUID: r.Uuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle,
				TaxpayerTaxID: r.TaxpayerTaxID, Period: f.Period, id: r.ID})
		}
		if err := s.fillRows(ctx, v.OrganizationID, f.Period, chunk); err != nil {
			return Page{}, err
		}
		all = append(all, chunk...)
		if len(rows) < scanChunk {
			break
		}
		if len(all) >= MaxScan {
			truncated = true
			break
		}
	}
	page := Page{Period: f.Period, Limit: f.Limit, Offset: f.Offset}
	if f.Summary {
		sum := Summarize(all)
		sum.Truncated = truncated
		page.Summary = &sum
	}
	list := all
	if f.Step != "" {
		list = make([]Row, 0, len(all))
		for _, r := range all {
			if r.NextStep != nil && *r.NextStep == f.Step {
				list = append(list, r)
			}
		}
	}
	page.Total = int64(len(list))
	if f.Step == "" && truncated {
		total, err := s.q.CountCopilotEngagements(ctx, db.CountCopilotEngagementsParams{
			OrganizationID: v.OrganizationID, AllEngagements: v.ReadAll, UserID: v.UserID, Search: search,
		})
		if err != nil {
			return Page{}, err
		}
		page.Total = total
	}
	from := min(int(f.Offset), len(list))
	to := min(from+int(f.Limit), len(list))
	page.Items = append([]Row{}, list[from:to]...)
	return page, nil
}

// Summarize counts rows by their next step.
func Summarize(rows []Row) Summary {
	by := make(map[string]int64, len(StepKeys))
	sum := Summary{Scanned: int64(len(rows))}
	for _, r := range rows {
		switch {
		case r.NextStep != nil:
			by[*r.NextStep]++
		case hasStatus(r.Steps, StatusWaiting):
			sum.Waiting++
		default:
			sum.Closed++
		}
	}
	sum.Steps = make([]StepCount, 0, len(StepKeys))
	for _, k := range StepKeys {
		sum.Steps = append(sum.Steps, StepCount{Key: k, Count: by[k]})
	}
	return sum
}

func hasStatus(steps []Step, status string) bool {
	for _, s := range steps {
		if s.Status == status {
			return true
		}
	}
	return false
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
			c.DocsOpen, c.DocsSubmitted, c.DocsFirst = r.Open, r.Submitted, r.FirstUuid
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
			c.TransferOpen, c.TransferImported, c.TransferFailed, c.TransferDownloaded = r.Open, r.Imported, r.Failed, r.Downloaded
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
	rc, err := s.q.CopilotReceiptCounts(ctx, db.CopilotReceiptCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return err
	}
	for _, r := range rc {
		if c := counts[r.EngagementID]; c != nil {
			c.ExpensesPending, c.ZReportsPending = r.ExpensesPending, r.ZReportsPending
			c.ExpensesApproved, c.ZReportsApproved, c.BankAccounts = r.ExpensesApproved, r.ZReportsApproved, r.BankAccounts
		}
	}
	li, err := s.q.CopilotLucaImportCounts(ctx, db.CopilotLucaImportCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	ledgerOf := make(map[int64]string, len(li))
	for _, r := range li {
		ledgerOf[r.EngagementID] = r.LedgerType
		if c := counts[r.EngagementID]; c != nil {
			c.PayrollExpected, c.PayrollImported, c.TrialBalanceImported = r.PayrollExpected, r.PayrollImported, r.TrialBalanceImported
			c.NoTrialBalance = !r.TrialBalanceExpected
			c.SingleEntry = ledgerSingleEntry(r.LedgerType)
			c.Reconciled, c.ReconciliationIssues = r.Reconciled, r.ReconciliationIssues
		}
	}
	vn, err := s.q.CopilotVATNoticeCounts(ctx, db.CopilotVATNoticeCountsParams{OrganizationID: organizationID, EngagementIds: ids, Period: period})
	if err != nil {
		return err
	}
	for _, r := range vn {
		if c := counts[r.EngagementID]; c != nil {
			c.VATNotices = r.Notices
		}
	}
	for i := range items {
		c := *counts[items[i].id]
		items[i].LedgerType = "bilanco"
		if li := ledgerOf[items[i].id]; li != "" {
			items[i].LedgerType = li
		}
		fill(&items[i], c)
	}
	return nil
}
