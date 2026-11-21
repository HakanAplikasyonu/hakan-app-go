package taxreturns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Service is the tax return use case layer.
type Service struct {
	q     *db.Queries
	now   func() time.Time
	store ObjectStore
}

// New builds the service on pool (statements join the request's RLS scope).
func New(pool database.Pool) *Service {
	return &Service{q: database.QueriesFor(pool), now: time.Now}
}

// WithStore sets the object store of the accrual slips (#575).
func (s *Service) WithStore(store ObjectStore) *Service { s.store = store; return s }

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Calendar is the filing deadline of a return.
type Calendar struct {
	DueDate       string `json:"due_date"`
	DaysLeft      int    `json:"days_left"`
	RolledForward bool   `json:"rolled_forward"`
}

// Record is the office's stored decision for a period (nil fields: not set).
type Record struct {
	UUID          uuid.UUID       `json:"uuid"`
	Status        string          `json:"status"`
	Adjustments   json.RawMessage `json:"adjustments"`
	Acknowledged  []string        `json:"acknowledged"`
	Note          *string         `json:"note"`
	PreparedAt    *time.Time      `json:"prepared_at"`
	FiledOn       *string         `json:"filed_on"`
	AccrualNumber *string         `json:"accrual_number"`
	FiledAt       *time.Time      `json:"filed_at"`
	// Son kontrol (#669): kim hazırladı, kim kontrol etti, kim verdi.
	CheckedAt      *time.Time `json:"checked_at"`
	PreparedByName string     `json:"prepared_by_name,omitempty"`
	CheckedByName  string     `json:"checked_by_name,omitempty"`
	FiledByName    string     `json:"filed_by_name,omitempty"`
	// AccrualDocument is the GİB tahakkuk PDF attached after filing (#575).
	AccrualDocument *AccrualDocument `json:"accrual_document"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// body is what a snapshot freezes.
type body struct {
	ComputedAt time.Time   `json:"computed_at"`
	Checklist  []CheckItem `json:"checklist"`
	KDV1       *KDV1       `json:"kdv1,omitempty"`
	MUHSGK     *MUHSGK     `json:"muhsgk,omitempty"`
	BABS       *BABS       `json:"babs,omitempty"`
}

// Preview is the API view of one return of a period.
type Preview struct {
	Kind           string    `json:"kind"`
	Period         string    `json:"period"`
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	Status         string    `json:"status"`
	// Frozen: the body is the snapshot taken when the return was prepared.
	Frozen       bool     `json:"frozen"`
	Calendar     Calendar `json:"calendar"`
	Record       *Record  `json:"record"`
	OpenWarnings []string `json:"open_warnings"`
	// Sections: the period's source records, processed / total (#710).
	// Always live, also for a prepared or filed return.
	Sections []Section `json:"sections"`
	body
}

// Section keys of Preview.Sections, in screen order.
const (
	SectionRetail   = "retail"
	SectionInvoices = "invoices"
	SectionBank     = "bank"
	SectionExpenses = "expenses"
)

// Section is one source of the period: how many records there are, how
// many were processed (they count for the return) and how many were set
// aside on purpose (rejected, cancelled, ignored). The rest still waits.
type Section struct {
	Key       string `json:"key"`
	Total     int64  `json:"total"`
	Processed int64  `json:"processed"`
	Ignored   int64  `json:"ignored"`
}

// sections loads the period's section summary.
func (s *Service) sections(ctx context.Context, sc engagementctx.Scope, period string) ([]Section, error) {
	st, err := s.q.TaxReturnSectionStats(ctx, db.TaxReturnSectionStatsParams{TaxpayerID: sc.TaxpayerID, Period: period})
	if err != nil {
		return nil, fmt.Errorf("tax return sections: %w", err)
	}
	return []Section{
		{Key: SectionRetail, Total: st.RetailTotal, Processed: st.RetailProcessed, Ignored: st.RetailIgnored},
		{Key: SectionInvoices, Total: st.InvoiceTotal, Processed: st.InvoiceProcessed, Ignored: st.InvoiceIgnored},
		{Key: SectionBank, Total: st.BankTotal, Processed: st.BankProcessed, Ignored: st.BankIgnored},
		{Key: SectionExpenses, Total: st.ExpenseTotal, Processed: st.ExpenseProcessed, Ignored: st.ExpenseIgnored},
	}, nil
}

// Summary is one row of the return list.
type Summary struct {
	UUID          uuid.UUID  `json:"uuid"`
	Kind          string     `json:"kind"`
	Period        string     `json:"period"`
	Status        string     `json:"status"`
	Payable       *Money     `json:"payable"`
	CarryOut      *Money     `json:"carry_out"`
	DueDate       string     `json:"due_date"`
	PreparedAt    *time.Time `json:"prepared_at"`
	FiledOn       *string    `json:"filed_on"`
	AccrualNumber *string    `json:"accrual_number"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// DraftInput is the body of the draft update.
type DraftInput struct {
	Adjustments  json.RawMessage `json:"adjustments"`
	Acknowledged []string        `json:"acknowledged"`
	Note         *string         `json:"note"`
}

// FileInput is the body of "verildi".
type FileInput struct {
	FiledOn       string  `json:"filed_on"`
	AccrualNumber *string `json:"accrual_number"`
	Note          *string `json:"note"`
}

// UnacknowledgedError lists the warnings that block preparing.
type UnacknowledgedError struct{ Codes []string }

func (e *UnacknowledgedError) Error() string {
	return ErrUnacknowledged.Error() + ": " + strings.Join(e.Codes, ", ")
}

func (e *UnacknowledgedError) Unwrap() error { return ErrUnacknowledged }

func check(kind, period string) error {
	if !ValidKind(kind) {
		return ErrInvalidKind
	}
	_, err := ParsePeriod(period)
	return err
}

func (s *Service) calendar(kind, period string) Calendar {
	due, rolled, err := DueDate(kind, period)
	if err != nil {
		return Calendar{}
	}
	now := s.now().In(Istanbul)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	d := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
	return Calendar{DueDate: due.Format(time.DateOnly), DaysLeft: int(d.Sub(today).Hours() / 24), RolledForward: rolled}
}

func (s *Service) record(ctx context.Context, sc engagementctx.Scope, kind, period string) (*db.TaxReturn, error) {
	r, err := s.q.GetTaxReturn(ctx, db.GetTaxReturnParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tax return: %w", err)
	}
	return &r, nil
}

func recordView(r *db.TaxReturn) *Record {
	if r == nil {
		return nil
	}
	v := &Record{
		UUID: r.Uuid, Status: r.Status, Adjustments: json.RawMessage(r.Adjustments), Acknowledged: acksOf(r.Acknowledged),
		Note: textPtr(r.Note), FiledOn: datePtr(r.FiledOn), AccrualNumber: textPtr(r.AccrualNumber), UpdatedAt: r.UpdatedAt.Time,
	}
	if len(v.Adjustments) == 0 {
		v.Adjustments = json.RawMessage(`{}`)
	}
	if r.AccrualStorageKey.Valid {
		v.AccrualDocument = &AccrualDocument{
			FileName: r.AccrualFileName.String, SizeBytes: r.AccrualSizeBytes.Int64, UploadedAt: r.AccrualUploadedAt.Time,
		}
	}
	if r.PreparedAt.Valid {
		t := r.PreparedAt.Time
		v.PreparedAt = &t
	}
	if r.FiledAt.Valid {
		t := r.FiledAt.Time
		v.FiledAt = &t
	}
	if r.CheckedAt.Valid {
		t := r.CheckedAt.Time
		v.CheckedAt = &t
	}
	return v
}

// withNames fills who prepared, checked and filed the return.
func (s *Service) withNames(ctx context.Context, sc engagementctx.Scope, v *Record, r *db.TaxReturn) error {
	if v == nil || r == nil {
		return nil
	}
	for _, f := range []struct {
		id  pgtype.Int8
		out *string
	}{{r.PreparedBy, &v.PreparedByName}, {r.CheckedBy, &v.CheckedByName}, {r.FiledBy, &v.FiledByName}} {
		if !f.id.Valid {
			continue
		}
		name, err := s.q.TaxReturnUserName(ctx, db.TaxReturnUserNameParams{OrganizationID: sc.OrganizationID, UserID: f.id.Int64})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("tax return user: %w", err)
		}
		*f.out = name
	}
	return nil
}

func acksOf(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// Preview returns the return of kind for period: the frozen snapshot once
// prepared, else computed from the current data with the draft's
// adjustments and acknowledgements.
func (s *Service) Preview(ctx context.Context, sc engagementctx.Scope, kind, period string) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{
		Kind: kind, Period: period, EngagementUUID: sc.UUID, Status: StatusDraft,
		Calendar: s.calendar(kind, period), Record: recordView(rec), OpenWarnings: []string{},
	}
	if err := s.withNames(ctx, sc, p.Record, rec); err != nil {
		return Preview{}, err
	}
	if p.Sections, err = s.sections(ctx, sc, period); err != nil {
		return Preview{}, err
	}
	if rec != nil {
		p.Status = rec.Status
		if rec.Status != StatusDraft && len(rec.Snapshot) > 0 {
			if err := json.Unmarshal(rec.Snapshot, &p.body); err != nil {
				return Preview{}, fmt.Errorf("tax return snapshot: %w", err)
			}
			p.Frozen = true
			return p, nil
		}
	}
	var adjRaw, ackRaw []byte
	if rec != nil {
		adjRaw, ackRaw = rec.Adjustments, rec.Acknowledged
	}
	b, open, err := s.compute(ctx, sc, kind, period, adjRaw, acksOf(ackRaw))
	if err != nil {
		return Preview{}, err
	}
	p.body, p.OpenWarnings = b, open
	return p, nil
}

func (s *Service) compute(ctx context.Context, sc engagementctx.Scope, kind, period string, adjRaw []byte, acks []string) (body, []string, error) {
	b := body{ComputedAt: s.now().UTC()}
	var items []CheckItem
	switch kind {
	case KindKDV1:
		in, err := s.loadKDV1(ctx, sc, period, adjRaw)
		if err != nil {
			return body{}, nil, err
		}
		k, c := ComputeKDV1(in)
		b.KDV1, items = &k, c
	case KindMUHSGK:
		in, err := s.loadMUHSGK(ctx, sc, period)
		if err != nil {
			return body{}, nil, err
		}
		m, c := ComputeMUHSGK(in)
		b.MUHSGK, items = &m, c
	case KindBABS:
		in, err := s.loadBABS(ctx, sc, period)
		if err != nil {
			return body{}, nil, err
		}
		x, c := ComputeBABS(in)
		b.BABS, items = &x, c
	}
	docs, err := s.documentItems(ctx, sc, period)
	if err != nil {
		return body{}, nil, err
	}
	items = append(items, docs...)
	if items == nil {
		items = []CheckItem{}
	}
	open := applyAcks(items, acks)
	if open == nil {
		open = []string{}
	}
	b.Checklist = items
	return b, open, nil
}

// documentItems are the checklist rows of the period's document tasks
// (#575): a document still expected from the taxpayer is a warning (the
// return may change when it arrives); one that arrived but was not processed
// yet is a warning too. Both apply to every return kind.
func (s *Service) documentItems(ctx context.Context, sc engagementctx.Scope, period string) ([]CheckItem, error) {
	st, err := s.q.TaxReturnDocumentTaskStats(ctx, db.TaxReturnDocumentTaskStatsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: period})
	if err != nil {
		return nil, fmt.Errorf("tax return document tasks: %w", err)
	}
	var out []CheckItem
	if st.Open > 0 {
		first := st.FirstOpen
		switch first {
		case "undocumented_payment":
			first = "belgesiz banka ödemesi"
		case "uninvoiced_collection":
			first = "faturasız tahsilat"
		}
		msg := fmt.Sprintf("%d belge mükelleften bekleniyor (ör. %s); gelmeden beyan eksik kalabilir.", st.Open, first)
		out = append(out, item("OPEN_DOCUMENT_TASKS", SeverityWarning, msg, st.Open))
	}
	if st.Submitted > 0 {
		out = append(out, item("SUBMITTED_DOCUMENTS", SeverityWarning,
			fmt.Sprintf("Mükellefin gönderdiği %d belge henüz işlenmedi.", st.Submitted), st.Submitted))
	}
	return out, nil
}

// ParseKDV1Adjustments validates the adjustments of a KDV1 draft: known
// fields only, amounts ≥ 0.
func ParseKDV1Adjustments(raw []byte) (KDV1Adjustments, error) {
	var a KDV1Adjustments
	if len(raw) == 0 || string(raw) == "null" {
		return a, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("%w: adjustments: %v", ErrInvalidInput, err)
	}
	for _, m := range []*Money{a.CarriedIn, a.AdditionalVAT, a.OtherDeductions} {
		if m != nil && m.Dec().IsNegative() {
			return a, fmt.Errorf("%w: adjustments must not be negative", ErrInvalidInput)
		}
	}
	return a, nil
}

func (s *Service) loadKDV1(ctx context.Context, sc engagementctx.Scope, period string, adjRaw []byte) (KDV1Input, error) {
	adj, err := ParseKDV1Adjustments(adjRaw)
	if err != nil {
		return KDV1Input{}, err
	}
	in := KDV1Input{Adjustments: adj}
	tp := sc.TaxpayerID
	rows, err := s.q.TaxReturnInvoiceLines(ctx, db.TaxReturnInvoiceLinesParams{TaxpayerID: tp, Period: period})
	if err != nil {
		return in, fmt.Errorf("tax return invoice lines: %w", err)
	}
	for _, r := range rows {
		in.Invoices = append(in.Invoices, InvoiceLine{
			Direction: r.Direction, Type: r.Type, Rate: dec(r.TaxRate), WithholdingCode: r.WithholdingCode,
			ExemptionCode: r.ExemptionCode, Count: r.InvoiceCount, Base: dec(r.Base), VAT: dec(r.Vat), Withholding: dec(r.Withholding),
		})
	}
	if in.InvoiceStats, err = s.invoiceStats(ctx, tp, period); err != nil {
		return in, err
	}
	p := pgtype.Text{String: period, Valid: true}
	retail, err := s.q.TaxReturnRetailVATLines(ctx, db.TaxReturnRetailVATLinesParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return retail lines: %w", err)
	}
	for _, r := range retail {
		in.Retail = append(in.Retail, RetailLine{Rate: dec(r.TaxRate), Count: r.ReportCount, Base: dec(r.Base), VAT: dec(r.Vat)})
	}
	rs, err := s.q.TaxReturnRetailStats(ctx, db.TaxReturnRetailStatsParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return retail stats: %w", err)
	}
	in.RetailStats = RetailStats{
		Approved: rs.Approved, Pending: rs.Pending, WithoutBreakdown: rs.WithoutBreakdown,
		WithoutBreakdownBase: dec(rs.WithoutBreakdownBase), WithoutBreakdownVAT: dec(rs.WithoutBreakdownVat), CardTotal: dec(rs.CardTotal),
	}
	exp, err := s.q.TaxReturnExpenseVAT(ctx, db.TaxReturnExpenseVATParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return expenses: %w", err)
	}
	for _, r := range exp {
		in.Expenses = append(in.Expenses, ExpenseLine{Rate: dec(r.TaxRate), Count: r.ExpenseCount, Base: dec(r.Base), VAT: dec(r.Vat)})
	}
	es, err := s.q.TaxReturnExpenseStats(ctx, db.TaxReturnExpenseStatsParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return expense stats: %w", err)
	}
	in.ExpenseStats = ExpenseStats{Pending: es.Pending, ForeignCurrency: es.ForeignCurrency, NotDeductible: es.NotDeductible}
	in.Carry, err = s.carry(ctx, sc, period, adj)
	return in, err
}

// carry resolves the devreden KDV: the draft's manual amount, else the
// previous period's prepared or filed KDV1, else an opening entered for
// this period (vat_carry_forwards), else zero (checklist warning).
func (s *Service) carry(ctx context.Context, sc engagementctx.Scope, period string, adj KDV1Adjustments) (Carry, error) {
	if adj.CarriedIn != nil {
		return Carry{Source: CarryManual, Amount: adj.CarriedIn.Dec()}, nil
	}
	prev := PreviousPeriod(period)
	r, err := s.record(ctx, sc, KindKDV1, prev)
	if err != nil {
		return Carry{}, err
	}
	if r != nil && r.Status != StatusDraft {
		return Carry{Source: CarryPrevious, Period: &prev, Amount: dec(r.CarryOut)}, nil
	}
	o, err := s.q.GetVATOpeningAtOrBefore(ctx, db.GetVATOpeningAtOrBeforeParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: period,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Carry{}, fmt.Errorf("vat opening: %w", err)
	case o.Period == period:
		p := o.Period
		return Carry{Source: CarryOpening, Period: &p, Amount: dec(o.Amount)}, nil
	}
	return Carry{Source: CarryNone}, nil
}

func (s *Service) invoiceStats(ctx context.Context, tp int64, period string) (InvoiceStats, error) {
	st, err := s.q.TaxReturnInvoiceStats(ctx, db.TaxReturnInvoiceStatsParams{TaxpayerID: tp, Period: period})
	if err != nil {
		return InvoiceStats{}, fmt.Errorf("tax return invoice stats: %w", err)
	}
	return InvoiceStats{
		Included: st.Included, Draft: st.Draft, Cancelled: st.Cancelled,
		MissingExchangeRate: st.MissingExchangeRate, HeaderEstimate: st.HeaderEstimate,
		MovedIn: st.MovedIn, MovedOut: st.MovedOut,
	}, nil
}

func (s *Service) loadMUHSGK(ctx context.Context, sc engagementctx.Scope, period string) (MUHSGKInput, error) {
	var in MUHSGKInput
	tp := sc.TaxpayerID
	run, err := s.q.GetPayrollRunByPeriod(ctx, db.GetPayrollRunByPeriodParams{TaxpayerID: tp, Period: period})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return in, fmt.Errorf("payroll run: %w", err)
	default:
		var issues []json.RawMessage
		_ = json.Unmarshal(run.Issues, &issues)
		in.Run = &PayrollRun{EmployeeCount: run.EmployeeCount, IssueCount: len(issues)}
		lines, err := s.q.TaxReturnPayrollLines(ctx, db.TaxReturnPayrollLinesParams{TaxpayerID: tp, Period: period})
		if err != nil {
			return in, fmt.Errorf("payroll lines: %w", err)
		}
		for _, l := range lines {
			pl := PayrollLine{
				LineNo: l.LineNo, Name: l.EmployeeName, NationalIDMasked: l.NationalIDMasked.String, SGKNo: l.SgkNo.String,
				HireDate: dateStr(l.HireDate), ExitDate: dateStr(l.ExitDate),
				Gross: dec(l.Gross), SGKBase: dec(l.SgkBase), SGKEmployee: dec(l.SgkEmployee), UnemploymentEmployee: dec(l.UnemploymentEmployee),
				IncomeTaxBase: dec(l.IncomeTaxBase), IncomeTax: dec(l.IncomeTax), IncomeTaxExemption: dec(l.IncomeTaxExemption),
				StampTax: dec(l.StampTax), StampTaxExemption: dec(l.StampTaxExemption), SGKEmployer: dec(l.SgkEmployer),
				UnemploymentEmployer: dec(l.UnemploymentEmployer), SGKIncentive: dec(l.SgkIncentive),
			}
			if l.DaysWorked.Valid {
				d := l.DaysWorked.Int32
				pl.DaysWorked = &d
			}
			in.Lines = append(in.Lines, pl)
		}
	}
	prem, err := s.q.GetSgkPremiumByPeriod(ctx, db.GetSgkPremiumByPeriodParams{TaxpayerID: tp, Period: period})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return in, fmt.Errorf("sgk premium: %w", err)
	default:
		in.Premium = &Premium{
			Accrual: dec(prem.AccrualAmount), Incentive: dec(prem.IncentiveAmount), Payable: dec(prem.PayableAmount),
			Paid: dec(prem.PaidAmount), DueDate: dateStr(prem.DueDate),
		}
	}
	wh, err := s.q.TaxReturnIncomeWithholdings(ctx, db.TaxReturnIncomeWithholdingsParams{TaxpayerID: tp, Period: period})
	if err != nil {
		return in, fmt.Errorf("income withholdings: %w", err)
	}
	for _, w := range wh {
		in.Withholdings = append(in.Withholdings, IncomeWithholding{
			DocumentClass: w.DocumentClass, Rate: dec(w.Rate), Count: w.InvoiceCount, Base: dec(w.Base), Tax: dec(w.Tax),
		})
	}
	return in, nil
}

func (s *Service) loadBABS(ctx context.Context, sc engagementctx.Scope, period string) (BABSInput, error) {
	var in BABSInput
	tp := sc.TaxpayerID
	rows, err := s.q.TaxReturnCounterparties(ctx, db.TaxReturnCounterpartiesParams{TaxpayerID: tp, Period: period})
	if err != nil {
		return in, fmt.Errorf("tax return counterparties: %w", err)
	}
	for _, r := range rows {
		in.Counterparties = append(in.Counterparties, Counterparty{
			Direction: r.Direction, ID: r.CounterpartyID, Name: r.CounterpartyName, Count: r.DocumentCount, Amount: dec(r.Amount),
		})
	}
	miss, err := s.q.TaxReturnMissingCounterparty(ctx, db.TaxReturnMissingCounterpartyParams{TaxpayerID: tp, Period: period})
	if err != nil {
		return in, fmt.Errorf("tax return missing counterparty: %w", err)
	}
	for _, m := range miss {
		in.Missing = append(in.Missing, MissingCounterparty{Direction: m.Direction, Count: m.DocumentCount, Amount: dec(m.Amount)})
	}
	p := pgtype.Text{String: period, Valid: true}
	exps, err := s.q.TaxReturnExpenseCounterparties(ctx, db.TaxReturnExpenseCounterpartiesParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return expense counterparties: %w", err)
	}
	for _, e := range exps {
		in.Expenses = append(in.Expenses, ExpenseCounterparty{ID: e.CounterpartyID, Name: e.CounterpartyName, Count: e.DocumentCount, Amount: dec(e.Amount)})
	}
	mv, err := s.q.TaxReturnExpenseMissingVendor(ctx, db.TaxReturnExpenseMissingVendorParams{TaxpayerID: tp, Period: p})
	if err != nil {
		return in, fmt.Errorf("tax return expense missing vendor: %w", err)
	}
	in.ExpensesMissingVendor = MissingCounterparty{Direction: dirIncoming, Count: mv.DocumentCount, Amount: dec(mv.Amount)}
	in.InvoiceStats, err = s.invoiceStats(ctx, tp, period)
	return in, err
}

// List returns the stored returns of the engagement (newest period first),
// optionally of one year.
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, year string) ([]Summary, error) {
	y := pgtype.Text{String: year, Valid: year != ""}
	rows, err := s.q.ListTaxReturns(ctx, db.ListTaxReturnsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Year: y})
	if err != nil {
		return nil, fmt.Errorf("list tax returns: %w", err)
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		v := recordView(&r)
		out = append(out, Summary{
			UUID: r.Uuid, Kind: r.Kind, Period: r.Period, Status: r.Status,
			Payable: moneyPtr(r.PayableAmount), CarryOut: moneyPtr(r.CarryOut), DueDate: s.calendar(r.Kind, r.Period).DueDate,
			PreparedAt: v.PreparedAt, FiledOn: v.FiledOn, AccrualNumber: v.AccrualNumber, UpdatedAt: v.UpdatedAt,
		})
	}
	return out, nil
}

func normAcks(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		if len(a) > 64 || len(out) >= 50 {
			return nil, fmt.Errorf("%w: acknowledged", ErrInvalidInput)
		}
		seen[a] = true
		out = append(out, a)
	}
	return out, nil
}

// SaveDraft stores the draft's adjustments, acknowledgements and note.
func (s *Service) SaveDraft(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string, in DraftInput) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	adj := []byte(`{}`)
	if len(in.Adjustments) > 0 && string(in.Adjustments) != "null" {
		if kind != KindKDV1 {
			if strings.TrimSpace(string(in.Adjustments)) != "{}" {
				return Preview{}, fmt.Errorf("%w: adjustments are only for kdv1", ErrInvalidInput)
			}
		} else {
			a, err := ParseKDV1Adjustments(in.Adjustments)
			if err != nil {
				return Preview{}, err
			}
			if adj, err = json.Marshal(a); err != nil {
				return Preview{}, err
			}
		}
	}
	acks, err := normAcks(in.Acknowledged)
	if err != nil {
		return Preview{}, err
	}
	ackRaw, _ := json.Marshal(acks)
	if in.Note != nil && len(*in.Note) > 2000 {
		return Preview{}, fmt.Errorf("%w: note is too long", ErrInvalidInput)
	}
	_, err = s.q.UpsertTaxReturnDraft(ctx, db.UpsertTaxReturnDraftParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
		Adjustments: adj, Acknowledged: ackRaw, Note: optTextPtr(in.Note), CreatedBy: actor(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotDraft
	}
	if err != nil {
		return Preview{}, fmt.Errorf("save tax return draft: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

// Prepare freezes the current preview. Every warning must be acknowledged
// (stored on the draft or sent in acks).
func (s *Service) Prepare(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string, acks []string) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return Preview{}, err
	}
	if rec != nil && rec.Status != StatusDraft {
		return Preview{}, ErrNotDraft
	}
	adjRaw, stored := []byte(`{}`), []string{}
	if rec != nil {
		adjRaw, stored = rec.Adjustments, acksOf(rec.Acknowledged)
	}
	all, err := normAcks(append(stored, acks...))
	if err != nil {
		return Preview{}, err
	}
	b, open, err := s.compute(ctx, sc, kind, period, adjRaw, all)
	if err != nil {
		return Preview{}, err
	}
	if len(open) > 0 {
		return Preview{}, &UnacknowledgedError{Codes: open}
	}
	snap, err := json.Marshal(b)
	if err != nil {
		return Preview{}, err
	}
	ackRaw, _ := json.Marshal(all)
	var payable, carryOut pgtype.Numeric
	switch {
	case b.KDV1 != nil:
		payable, carryOut = num(b.KDV1.Totals.Payable.Dec()), num(b.KDV1.Totals.CarryOut.Dec())
	case b.MUHSGK != nil:
		payable = num(b.MUHSGK.Totals.TaxPayable.Dec())
	}
	_, err = s.q.PrepareTaxReturn(ctx, db.PrepareTaxReturnParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
		Adjustments: adjRaw, Acknowledged: ackRaw, Snapshot: snap, PayableAmount: payable, CarryOut: carryOut,
		PreparedBy: actor(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotDraft
	}
	if err != nil {
		return Preview{}, fmt.Errorf("prepare tax return: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

// Reopen turns a prepared return back into a draft (the snapshot is
// dropped; adjustments and acknowledgements stay).
func (s *Service) Reopen(ctx context.Context, sc engagementctx.Scope, kind, period string) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	_, err := s.q.ReopenTaxReturn(ctx, db.ReopenTaxReturnParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotPrepared
	}
	if err != nil {
		return Preview{}, fmt.Errorf("reopen tax return: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

// File records that the prepared return was filed (verildi).
// Check marks a prepared return as checked by the responsible person (#669).
func (s *Service) Check(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	_, err := s.q.CheckTaxReturn(ctx, db.CheckTaxReturnParams{
		CheckedBy: actor(actorID), OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotPrepared
	}
	if err != nil {
		return Preview{}, fmt.Errorf("check tax return: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

func (s *Service) File(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string, in FileInput) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	on, err := time.Parse(time.DateOnly, strings.TrimSpace(in.FiledOn))
	if err != nil {
		return Preview{}, fmt.Errorf("%w: filed_on must be YYYY-MM-DD", ErrInvalidInput)
	}
	start, _ := ParsePeriod(period)
	if on.Before(start.AddDate(0, 1, 0).Add(-24*time.Hour)) || on.After(s.now().AddDate(0, 0, 1)) {
		return Preview{}, fmt.Errorf("%w: filed_on must be after the period and not in the future", ErrInvalidInput)
	}
	if in.AccrualNumber != nil && len(*in.AccrualNumber) > 64 {
		return Preview{}, fmt.Errorf("%w: accrual_number is too long", ErrInvalidInput)
	}
	if in.Note != nil && len(*in.Note) > 2000 {
		return Preview{}, fmt.Errorf("%w: note is too long", ErrInvalidInput)
	}
	_, err = s.q.FileTaxReturn(ctx, db.FileTaxReturnParams{
		FiledOn: pgtype.Date{Time: on, Valid: true}, AccrualNumber: optTextPtr(in.AccrualNumber), Note: optTextPtr(in.Note),
		FiledBy: actor(actorID), OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotPrepared
	}
	if err != nil {
		return Preview{}, fmt.Errorf("file tax return: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func moneyPtr(n pgtype.Numeric) *Money {
	if !n.Valid {
		return nil
	}
	m := M(dec(n))
	return &m
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func optTextPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*s)
	return pgtype.Text{String: v, Valid: v != ""}
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}

func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(time.DateOnly)
}

func actor(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }
