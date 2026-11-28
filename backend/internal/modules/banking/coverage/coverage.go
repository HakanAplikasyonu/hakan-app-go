// Package coverage is the missing bank statement follow-up (Oğuz'un geri
// bildirimi: "bu adamın Akbank, Ziraat, Kuveyt var; her ay bunları
// isteyeceğim; neyin eksik olduğunu to-do gibi bilmeliyiz").
//
// The taxpayer's bank accounts are the existing cash boxes (#144, #780). For
// every account in use and every closed month of the window, the statement
// either arrived (a movement or a day-end balance of that month, or the
// accountant marked the month "geldi, hareket yok") or is missing. The
// current month is not expected yet. An account is expected from the month
// before it was added to the list (the accountant adds it and asks for last
// month), or from its first movement when that is earlier. A closed
// (deactivated) account is expected up to the month it was closed in.
//
// A missing month becomes a document request (#835 flow: channels, due date,
// reminder) with a ready-made title, "Akbank TRY hesabı – Ekim 2026
// ekstresi"; the same request is never opened twice while it is open.
//
// The office setting bank_statement_tracking turns the follow-up off: no
// monthly status, no office to-do list, no "Listeye ekle" suggestion. The
// account list stays.
package coverage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// WindowMonths is how many closed months the status shows and the to-do
// list counts (last month and the five before it).
const WindowMonths = 6

// MaxRequestItems caps one request call (the bulk request of a taxpayer).
const MaxRequestItems = 30

// DefaultDueDays: the taxpayer has five days, like the monthly document
// collection.
const DefaultDueDays = 5

// Month statuses.
const (
	StatusReceived    = "received"
	StatusMissing     = "missing"
	StatusNotExpected = "not_expected"
)

// Errors.
var (
	// ErrDisabled: the office turned the follow-up off.
	ErrDisabled = errors.New("coverage: bank statement tracking is off")
	// ErrBadInput: a malformed request.
	ErrBadInput = errors.New("coverage: invalid input")
	// ErrNotFound: the account is not the taxpayer's.
	ErrNotFound = errors.New("coverage: account not found")
	// ErrNotMissing: the asked month is not a missing statement (it arrived,
	// is not expected, or is outside the window).
	ErrNotMissing = errors.New("coverage: the statement of this month is not missing")
)

var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// Requester opens a document request (doctasks.Service).
type Requester interface {
	CreateRequest(ctx context.Context, scope engagementctx.Scope, actorID int64, in doctasks.RequestInput) (doctasks.Task, error)
}

// Queries is what the service reads and writes.
type Queries interface {
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	ListBankStatementCoverage(ctx context.Context, arg db.ListBankStatementCoverageParams) ([]db.ListBankStatementCoverageRow, error)
	ListBankStatementGaps(ctx context.Context, arg db.ListBankStatementGapsParams) ([]db.ListBankStatementGapsRow, error)
	PeriodMatrixOpenRequestTitles(ctx context.Context, arg db.PeriodMatrixOpenRequestTitlesParams) ([]db.PeriodMatrixOpenRequestTitlesRow, error)
	GetCashBoxByUUID(ctx context.Context, arg db.GetCashBoxByUUIDParams) (db.CashBox, error)
	InsertBankStatementMark(ctx context.Context, arg db.InsertBankStatementMarkParams) error
	DeleteBankStatementMark(ctx context.Context, arg db.DeleteBankStatementMarkParams) error
	CloseDocumentRequestByTitle(ctx context.Context, arg db.CloseDocumentRequestByTitleParams) (int64, error)
}

// Service computes the monthly status and opens the requests.
type Service struct {
	q    Queries
	docs Requester
	now  func() time.Time
}

// New builds the service; docs may be nil (requests then fail).
func New(q Queries, docs Requester) *Service {
	return &Service{q: q, docs: docs, now: time.Now}
}

// WithClock sets the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Periods are the closed months of the window, oldest first (Türkiye time).
func Periods(now time.Time) []string {
	t := now.In(istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 12, 0, 0, 0, istanbul)
	out := make([]string, WindowMonths)
	for i := range WindowMonths {
		out[WindowMonths-1-i] = first.AddDate(0, -(i + 1), 0).Format("2006-01")
	}
	return out
}

// Title is the document request title of one missing statement:
// "Akbank TRY hesabı – Ekim 2026 ekstresi" (missingdocs.AccountTitle; the
// monthly collection recognises it too).
func Title(bankID, currency, iban, name, period string, sameKind bool) string {
	return missingdocs.AccountTitle(bankID, currency, iban, name, period, sameKind)
}

// requested: an open request has this title, or the older monthly
// collection title of the account ("Ekim 2026 Ziraat banka ekstresi").
func requested(open []string, title, name, period string) bool {
	return missingdocs.IsRequested(open, title) ||
		missingdocs.IsRequested(open, missingdocs.Title(missingdocs.KindBankStatement, period, name))
}

// Enabled reads the office setting.
func (s *Service) Enabled(ctx context.Context, organizationID int64) (bool, error) {
	org, err := s.q.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return false, fmt.Errorf("coverage: office settings: %w", err)
	}
	return banking.StatementTrackingEnabled(org.Settings), nil
}

// MonthStatus is one account's statement for one month.
type MonthStatus struct {
	Period string `json:"period"`
	// Status: received, missing or not_expected (before the account was
	// expected).
	Status string `json:"status"`
	// Marked: received because the accountant marked it (no movement).
	Marked bool `json:"marked"`
	// Title: the document request title of a missing month.
	Title string `json:"title,omitempty"`
	// Requested: an open request with that title exists.
	Requested bool `json:"requested"`
}

// AccountStatus is one account in use and its months.
type AccountStatus struct {
	CashBoxUUID uuid.UUID     `json:"cash_box_uuid"`
	Name        string        `json:"name"`
	BankID      *string       `json:"bank_id"`
	IBAN        *string       `json:"iban"`
	Currency    string        `json:"currency"`
	Missing     int           `json:"missing"`
	Months      []MonthStatus `json:"months"`
}

// Status is the taxpayer's statement status.
type Status struct {
	// Enabled: the office setting; off answers no accounts.
	Enabled bool `json:"enabled"`
	// Periods: the months shown, oldest first.
	Periods []string `json:"periods"`
	// Missing: missing months over all accounts.
	Missing int             `json:"missing"`
	Items   []AccountStatus `json:"items"`
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// Status computes the engagement's monthly statement status.
func (s *Service) Status(ctx context.Context, scope engagementctx.Scope) (Status, error) {
	periods := Periods(s.now())
	out := Status{Periods: periods, Items: []AccountStatus{}}
	on, err := s.Enabled(ctx, scope.OrganizationID)
	if err != nil || !on {
		return out, err
	}
	out.Enabled = true
	rows, err := s.q.ListBankStatementCoverage(ctx, db.ListBankStatementCoverageParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Periods: periods, TaxpayerID: scope.TaxpayerID,
	})
	if err != nil {
		return out, fmt.Errorf("coverage: status: %w", err)
	}
	open, err := s.openTitles(ctx, scope.OrganizationID, []int64{scope.ID}, periods)
	if err != nil {
		return out, err
	}
	kinds := map[string]int{}
	seen := map[int64]bool{}
	for _, r := range rows {
		if !seen[r.CashBoxID] {
			seen[r.CashBoxID] = true
			kinds[r.BankID.String+"/"+r.Currency]++
		}
	}
	idx := map[int64]int{}
	for _, r := range rows {
		i, ok := idx[r.CashBoxID]
		if !ok {
			i = len(out.Items)
			idx[r.CashBoxID] = i
			out.Items = append(out.Items, AccountStatus{
				CashBoxUUID: r.CashBoxUuid, Name: r.Name, BankID: textPtr(r.BankID), IBAN: textPtr(r.Iban),
				Currency: r.Currency, Months: []MonthStatus{},
			})
		}
		m := MonthStatus{Period: r.Period}
		switch {
		case r.Period < r.StartPeriod, r.Period > r.EndPeriod:
			// Before the account was expected, or after it was closed.
			m.Status = StatusNotExpected
		case r.HasMovement:
			m.Status = StatusReceived
		case r.Marked:
			m.Status, m.Marked = StatusReceived, true
		default:
			m.Status = StatusMissing
			m.Title = Title(r.BankID.String, r.Currency, r.Iban.String, r.Name, r.Period, kinds[r.BankID.String+"/"+r.Currency] > 1)
			m.Requested = requested(open[scope.ID], m.Title, r.Name, r.Period)
			out.Items[i].Missing++
			out.Missing++
		}
		out.Items[i].Months = append(out.Items[i].Months, m)
	}
	return out, nil
}

// CloseArrived completes the open statement requests whose statement has
// arrived: a month of the window that now has a movement or a day-end
// balance of the account (an import, a bank sync). Both titles count: the
// follow-up's ("Akbank TRY hesabı – Ekim 2026 ekstresi") and the monthly
// collection's ("Ekim 2026 Akbank banka ekstresi"). A month the accountant
// only marked "geldi, hareket yok" closes nothing. It returns how many
// requests were closed and works whatever the office setting.
func (s *Service) CloseArrived(ctx context.Context, scope engagementctx.Scope) (int, error) {
	periods := Periods(s.now())
	rows, err := s.q.ListBankStatementCoverage(ctx, db.ListBankStatementCoverageParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Periods: periods, TaxpayerID: scope.TaxpayerID,
	})
	if err != nil {
		return 0, fmt.Errorf("coverage: arrived: %w", err)
	}
	open, err := s.openTitles(ctx, scope.OrganizationID, []int64{scope.ID}, periods)
	if err != nil {
		return 0, err
	}
	if len(open[scope.ID]) == 0 {
		return 0, nil
	}
	kinds := map[string]int{}
	seen := map[int64]bool{}
	for _, r := range rows {
		if !seen[r.CashBoxID] {
			seen[r.CashBoxID] = true
			kinds[r.BankID.String+"/"+r.Currency]++
		}
	}
	closed := 0
	done := map[string]bool{}
	for _, r := range rows {
		if !r.HasMovement {
			continue
		}
		for _, title := range []string{
			Title(r.BankID.String, r.Currency, r.Iban.String, r.Name, r.Period, kinds[r.BankID.String+"/"+r.Currency] > 1),
			missingdocs.Title(missingdocs.KindBankStatement, r.Period, r.Name),
		} {
			key := strings.ToLower(title)
			if done[key] || !missingdocs.IsRequested(open[scope.ID], title) {
				continue
			}
			done[key] = true
			n, err := s.q.CloseDocumentRequestByTitle(ctx, db.CloseDocumentRequestByTitleParams{
				OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Title: title,
			})
			if err != nil {
				return closed, fmt.Errorf("coverage: close request: %w", err)
			}
			closed += int(n)
		}
	}
	return closed, nil
}

// openTitles lists the open request titles per engagement.
func (s *Service) openTitles(ctx context.Context, organizationID int64, engagements []int64, periods []string) (map[int64][]string, error) {
	rows, err := s.q.PeriodMatrixOpenRequestTitles(ctx, db.PeriodMatrixOpenRequestTitlesParams{
		OrganizationID: organizationID, EngagementIds: engagements, Periods: periods,
	})
	if err != nil {
		return nil, fmt.Errorf("coverage: open requests: %w", err)
	}
	out := map[int64][]string{}
	for _, r := range rows {
		out[r.EngagementID] = append(out[r.EngagementID], r.Title)
	}
	return out, nil
}

// RequestItem names one missing month.
type RequestItem struct {
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	Period      string    `json:"period"`
}

// RequestInput asks for missing statements. With no items every missing
// month of the taxpayer is requested (the bulk request). The rest is the
// #835 document request: due date (or days), channels and the reminder.
type RequestInput struct {
	Items            []RequestItem `json:"items"`
	DueDate          string        `json:"due_date"`
	DueDays          int           `json:"due_days"`
	Channels         []string      `json:"channels"`
	RemindDaysBefore *int          `json:"remind_days_before"`
}

// RequestResult lists the requests opened now and those already open.
type RequestResult struct {
	Created  []doctasks.Task `json:"created"`
	Existing []doctasks.Task `json:"existing"`
}

// Request opens a document request for each asked missing month.
func (s *Service) Request(ctx context.Context, scope engagementctx.Scope, actorID int64, in RequestInput) (RequestResult, error) {
	if len(in.Items) > MaxRequestItems {
		return RequestResult{}, fmt.Errorf("%w: items accepts at most %d entries", ErrBadInput, MaxRequestItems)
	}
	if s.docs == nil {
		return RequestResult{}, ErrDisabled
	}
	st, err := s.Status(ctx, scope)
	if err != nil {
		return RequestResult{}, err
	}
	if !st.Enabled {
		return RequestResult{}, ErrDisabled
	}
	type want struct{ title, period string }
	missing := map[string]want{}
	var all []want
	for _, a := range st.Items {
		for _, m := range a.Months {
			if m.Status == StatusMissing {
				w := want{m.Title, m.Period}
				missing[a.CashBoxUUID.String()+"/"+m.Period] = w
				all = append(all, w)
			}
		}
	}
	todo := all
	if len(in.Items) > 0 {
		todo = make([]want, 0, len(in.Items))
		for _, it := range in.Items {
			if !doctasks.ValidPeriod(it.Period) {
				return RequestResult{}, fmt.Errorf("%w: period must be YYYY-MM", ErrBadInput)
			}
			w, ok := missing[it.CashBoxUUID.String()+"/"+it.Period]
			if !ok {
				return RequestResult{}, ErrNotMissing
			}
			todo = append(todo, w)
		}
	}
	if len(todo) > MaxRequestItems {
		todo = todo[:MaxRequestItems]
	}
	due := in.DueDays
	if due <= 0 && strings.TrimSpace(in.DueDate) == "" {
		due = DefaultDueDays
	}
	out := RequestResult{Created: []doctasks.Task{}, Existing: []doctasks.Task{}}
	done := map[string]bool{}
	for _, w := range todo {
		if done[w.title] {
			continue
		}
		done[w.title] = true
		t, err := s.docs.CreateRequest(ctx, scope, actorID, doctasks.RequestInput{
			Title: w.title, Note: missingdocs.Note(missingdocs.KindBankStatement), Period: w.period,
			DueDate: in.DueDate, DueDays: due, Channels: in.Channels, RemindDaysBefore: in.RemindDaysBefore,
		})
		switch {
		case errors.Is(err, doctasks.ErrRequestExists):
			out.Existing = append(out.Existing, t)
		case errors.Is(err, doctasks.ErrBadInput), errors.Is(err, doctasks.ErrBadPeriod):
			return out, fmt.Errorf("%w: %s", ErrBadInput, strings.TrimPrefix(err.Error(), "doctasks: invalid input: "))
		case err != nil:
			return out, err
		default:
			out.Created = append(out.Created, t)
		}
	}
	return out, nil
}

// MarkInput marks a closed month of an account received (no movement) or
// takes the mark back.
type MarkInput struct {
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	Period      string    `json:"period"`
	Received    bool      `json:"received"`
}

// Mark records the accountant's "ekstre geldi, hareket yok".
func (s *Service) Mark(ctx context.Context, scope engagementctx.Scope, actorID int64, in MarkInput) error {
	on, err := s.Enabled(ctx, scope.OrganizationID)
	if err != nil {
		return err
	}
	if !on {
		return ErrDisabled
	}
	inWindow := false
	for _, p := range Periods(s.now()) {
		inWindow = inWindow || p == in.Period
	}
	if !inWindow {
		return fmt.Errorf("%w: period must be one of the last %d closed months", ErrBadInput, WindowMonths)
	}
	box, err := s.q.GetCashBoxByUUID(ctx, db.GetCashBoxByUUIDParams{TaxpayerID: scope.TaxpayerID, Uuid: in.CashBoxUUID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && box.Type != "bank_account") {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("coverage: account: %w", err)
	}
	if !in.Received {
		return s.q.DeleteBankStatementMark(ctx, db.DeleteBankStatementMarkParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, CashBoxID: box.ID, Period: in.Period,
		})
	}
	return s.q.InsertBankStatementMark(ctx, db.InsertBankStatementMarkParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
		CashBoxID: box.ID, Period: in.Period, CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
}

// Gap is one row of the office to-do list "Eksik ekstreler".
type Gap struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	CashBoxUUID    uuid.UUID `json:"cash_box_uuid"`
	AccountName    string    `json:"account_name"`
	BankID         *string   `json:"bank_id"`
	Currency       string    `json:"currency"`
	// IBANLast4 tells two accounts of the same bank apart; "" without IBAN.
	IBANLast4 string `json:"iban_last4"`
	Period    string `json:"period"`
	Title     string `json:"title"`
	Requested bool   `json:"requested"`
	// TaxpayerMissing: all missing months of the taxpayer (bulk request).
	TaxpayerMissing int64 `json:"taxpayer_missing"`
}

// GapList is one page of the office to-do list.
type GapList struct {
	Enabled bool     `json:"enabled"`
	Periods []string `json:"periods"`
	Total   int64    `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	Items   []Gap    `json:"items"`
}

// Gaps lists the office's missing statements, one row per taxpayer +
// account + month, paged. all: the caller sees every engagement; otherwise
// only the engagements assigned to userID.
func (s *Service) Gaps(ctx context.Context, organizationID int64, all bool, userID int64, limit, offset int) (GapList, error) {
	periods := Periods(s.now())
	out := GapList{Periods: periods, Limit: limit, Offset: offset, Items: []Gap{}}
	on, err := s.Enabled(ctx, organizationID)
	if err != nil || !on {
		return out, err
	}
	out.Enabled = true
	rows, err := s.q.ListBankStatementGaps(ctx, db.ListBankStatementGapsParams{
		OrganizationID: organizationID, AllEngagements: all, UserID: userID, Periods: periods,
		Lim: int32(limit), Off: int32(offset), //nolint:gosec // bounded by the handler
	})
	if err != nil {
		return out, fmt.Errorf("coverage: gaps: %w", err)
	}
	var engs []int64
	seen := map[int64]bool{}
	for _, r := range rows {
		out.Total = r.Total
		if !seen[r.EngagementID] {
			seen[r.EngagementID] = true
			engs = append(engs, r.EngagementID)
		}
	}
	open := map[int64][]string{}
	if len(engs) > 0 {
		if open, err = s.openTitles(ctx, organizationID, engs, periods); err != nil {
			return out, err
		}
	}
	for _, r := range rows {
		g := Gap{
			EngagementUUID: r.EngagementUuid, TaxpayerTitle: r.TaxpayerTitle, CashBoxUUID: r.CashBoxUuid,
			AccountName: r.Name, BankID: textPtr(r.BankID), Currency: r.Currency, Period: r.Period,
			Title:           Title(r.BankID.String, r.Currency, r.Iban.String, r.Name, r.Period, r.SameKind > 1),
			TaxpayerMissing: r.TaxpayerMissing,
		}
		if r.Iban.Valid && len(r.Iban.String) >= 4 {
			g.IBANLast4 = r.Iban.String[len(r.Iban.String)-4:]
		}
		g.Requested = requested(open[r.EngagementID], g.Title, r.Name, r.Period)
		out.Items = append(out.Items, g)
	}
	return out, nil
}
