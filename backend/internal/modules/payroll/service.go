package payroll

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Service errors.
var (
	ErrNotFound         = errors.New("payroll: not found")
	ErrInvalid          = errors.New("payroll: invalid input")
	ErrDuplicate        = errors.New("payroll: an employee with this national id exists")
	ErrRunLocked        = errors.New("payroll: the run's journal draft is approved; reverse it first")
	ErrJournalExists    = errors.New("payroll: the run already has a journal draft")
	ErrNotPDF           = errors.New("payroll: the document is not a PDF")
	ErrDocumentExists   = errors.New("payroll: the same document is already stored")
	ErrStorageMissing   = errors.New("payroll: storage is not configured")
	ErrJournalsDisabled = errors.New("payroll: journal drafts are not configured")
)

// MaxDocumentBytes: payroll PDFs (a whole company's döküm) stay under 20 MB.
const MaxDocumentBytes = 20 << 20

// ObjectStore is the storage subset for payroll documents.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// JournalCreator is the journal draft service subset (#143).
type JournalCreator interface {
	CreateDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, in journals.DraftInput) (journals.Draft, error)
}

// PayrollMapper is the AI column mapper (excelimport, #24).
type PayrollMapper interface {
	MapPayroll(ctx context.Context, req mapping.AIRequest) (mapping.Mapping, error)
}

// Service is the payroll use case layer.
type Service struct {
	db       database.Ambient
	q        *db.Queries
	store    ObjectStore
	journals JournalCreator
	ai       PayrollMapper
	digest   Digester
	now      func() time.Time
}

// New builds the service. secret keys the national id digest (the
// application encryption key); store, journals and ai may be nil.
func New(pool database.Ambient, store ObjectStore, secret string) *Service {
	return &Service{db: pool, q: db.New(pool), store: store, digest: NewDigester(secret), now: time.Now}
}

// WithJournals enables the journal draft of a run.
func (s *Service) WithJournals(j JournalCreator) *Service { s.journals = j; return s }

// WithAI enables AI column mapping when the headers are not recognised.
func (s *Service) WithAI(m PayrollMapper) *Service { s.ai = m; return s }

// ---- employees ----

// Employee is the API view of an employee. The national id is masked.
type Employee struct {
	UUID             uuid.UUID  `json:"uuid"`
	FullName         string     `json:"full_name"`
	NationalIDMasked *string    `json:"national_id_masked"`
	SGKNo            *string    `json:"sgk_no"`
	JobTitle         *string    `json:"job_title"`
	Department       *string    `json:"department"`
	IBAN             *string    `json:"iban"`
	HireDate         *string    `json:"hire_date"`
	ExitDate         *string    `json:"exit_date"`
	ExitReason       *string    `json:"exit_reason"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

func (s *Service) employeeView(e db.Employee) Employee {
	v := Employee{
		UUID: e.Uuid, FullName: e.FullName, NationalIDMasked: textPtr(e.NationalIDMasked), SGKNo: textPtr(e.SgkNo),
		JobTitle: textPtr(e.JobTitle), Department: textPtr(e.Department), IBAN: textPtr(e.Iban),
		HireDate: datePtr(e.HireDate), ExitDate: datePtr(e.ExitDate), ExitReason: textPtr(e.ExitReason),
		Status: "active", CreatedAt: e.CreatedAt.Time,
	}
	if e.ExitDate.Valid && e.ExitDate.Time.Before(today(s.now())) {
		v.Status = "exited"
	}
	return v
}

// EmployeeFilter narrows the list.
type EmployeeFilter struct {
	Status        string // active, exited, "" all
	Q             string
	Limit, Offset int32
}

// EmployeePage is a page of employees with the counts.
type EmployeePage struct {
	Items  []Employee `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
	Active int64      `json:"active"`
	Exited int64      `json:"exited"`
}

// ListEmployees lists a taxpayer's employees by name.
func (s *Service) ListEmployees(ctx context.Context, taxpayerID int64, f EmployeeFilter) (EmployeePage, error) {
	rows, err := s.q.ListEmployees(ctx, db.ListEmployeesParams{
		TaxpayerID: taxpayerID, Status: optText(f.Status), Q: optText(f.Q), Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return EmployeePage{}, err
	}
	counts, err := s.q.CountEmployees(ctx, taxpayerID)
	if err != nil {
		return EmployeePage{}, err
	}
	p := EmployeePage{Items: make([]Employee, len(rows)), Limit: f.Limit, Offset: f.Offset, Active: counts.Active, Exited: counts.Exited}
	for i, r := range rows {
		p.Items[i] = s.employeeView(db.Employee{
			ID: r.ID, Uuid: r.Uuid, TaxpayerID: r.TaxpayerID, FullName: r.FullName, NationalIDMasked: r.NationalIDMasked,
			NationalIDDigest: r.NationalIDDigest, SgkNo: r.SgkNo, JobTitle: r.JobTitle, Department: r.Department, Iban: r.Iban,
			HireDate: r.HireDate, ExitDate: r.ExitDate, ExitReason: r.ExitReason, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		p.Total = r.TotalCount
	}
	return p, nil
}

// EmployeeInput creates or patches an employee. On a patch, nil leaves the
// field; an empty string clears it.
type EmployeeInput struct {
	FullName   *string `json:"full_name"`
	NationalID *string `json:"national_id"`
	SGKNo      *string `json:"sgk_no"`
	JobTitle   *string `json:"job_title"`
	Department *string `json:"department"`
	IBAN       *string `json:"iban"`
	HireDate   *string `json:"hire_date"`
	ExitDate   *string `json:"exit_date"`
	ExitReason *string `json:"exit_reason"`
}

type employeeFields struct {
	name                           string
	masked, digest                 pgtype.Text
	sgk, title, dept, iban, reason pgtype.Text
	hire, exit                     pgtype.Date
}

func (s *Service) apply(taxpayerID int64, base employeeFields, in EmployeeInput) (employeeFields, error) {
	f := base
	if in.FullName != nil {
		f.name = strings.Join(strings.Fields(*in.FullName), " ")
	}
	if f.name == "" {
		return f, fmt.Errorf("%w: full_name is required", ErrInvalid)
	}
	if in.NationalID != nil {
		if strings.TrimSpace(*in.NationalID) == "" {
			f.masked, f.digest = pgtype.Text{}, pgtype.Text{}
		} else {
			id, ok := NationalID(*in.NationalID)
			if !ok {
				return f, fmt.Errorf("%w: national_id is not a valid TCKN", ErrInvalid)
			}
			f.masked, f.digest = text(MaskNationalID(id)), text(s.digest.Digest(taxpayerID, id))
		}
	}
	set := func(dst *pgtype.Text, v *string, max int) error {
		if v == nil {
			return nil
		}
		t := strings.TrimSpace(*v)
		if len(t) > max {
			return fmt.Errorf("%w: value is too long", ErrInvalid)
		}
		*dst = optText(t)
		return nil
	}
	for _, x := range []struct {
		dst *pgtype.Text
		v   *string
		max int
	}{{&f.sgk, in.SGKNo, 32}, {&f.title, in.JobTitle, 200}, {&f.dept, in.Department, 200}, {&f.reason, in.ExitReason, 200}} {
		if err := set(x.dst, x.v, x.max); err != nil {
			return f, err
		}
	}
	if in.IBAN != nil {
		iban := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*in.IBAN), " ", ""))
		if iban != "" && !validIBAN(iban) {
			return f, fmt.Errorf("%w: iban is not a valid TR IBAN", ErrInvalid)
		}
		f.iban = optText(iban)
	}
	for _, x := range []struct {
		dst *pgtype.Date
		v   *string
	}{{&f.hire, in.HireDate}, {&f.exit, in.ExitDate}} {
		if x.v == nil {
			continue
		}
		if strings.TrimSpace(*x.v) == "" {
			*x.dst = pgtype.Date{}
			continue
		}
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(*x.v))
		if err != nil {
			return f, fmt.Errorf("%w: dates are YYYY-MM-DD", ErrInvalid)
		}
		*x.dst = pgtype.Date{Time: d, Valid: true}
	}
	if f.hire.Valid && f.exit.Valid && f.exit.Time.Before(f.hire.Time) {
		return f, fmt.Errorf("%w: exit_date is before hire_date", ErrInvalid)
	}
	return f, nil
}

// CreateEmployee adds an employee by hand.
func (s *Service) CreateEmployee(ctx context.Context, taxpayerID, actorID int64, in EmployeeInput) (Employee, error) {
	f, err := s.apply(taxpayerID, employeeFields{}, in)
	if err != nil {
		return Employee{}, err
	}
	row, err := s.q.InsertEmployee(ctx, db.InsertEmployeeParams{
		TaxpayerID: taxpayerID, FullName: f.name, NationalIDMasked: f.masked, NationalIDDigest: f.digest, SgkNo: f.sgk,
		JobTitle: f.title, Department: f.dept, Iban: f.iban, HireDate: f.hire, ExitDate: f.exit, ExitReason: f.reason,
		CreatedBy: actor(actorID),
	})
	if isUnique(err) {
		return Employee{}, ErrDuplicate
	}
	if err != nil {
		return Employee{}, err
	}
	return s.employeeView(row), nil
}

// UpdateEmployee patches an employee.
func (s *Service) UpdateEmployee(ctx context.Context, taxpayerID int64, id uuid.UUID, in EmployeeInput) (Employee, error) {
	cur, err := s.q.GetEmployeeByUUID(ctx, db.GetEmployeeByUUIDParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		return Employee{}, err
	}
	f, err := s.apply(taxpayerID, employeeFields{
		name: cur.FullName, masked: cur.NationalIDMasked, digest: cur.NationalIDDigest, sgk: cur.SgkNo,
		title: cur.JobTitle, dept: cur.Department, iban: cur.Iban, reason: cur.ExitReason, hire: cur.HireDate, exit: cur.ExitDate,
	}, in)
	if err != nil {
		return Employee{}, err
	}
	row, err := s.q.UpdateEmployee(ctx, db.UpdateEmployeeParams{
		TaxpayerID: taxpayerID, ID: cur.ID, FullName: f.name, NationalIDMasked: f.masked, NationalIDDigest: f.digest,
		SgkNo: f.sgk, JobTitle: f.title, Department: f.dept, Iban: f.iban, HireDate: f.hire, ExitDate: f.exit, ExitReason: f.reason,
	})
	if isUnique(err) {
		return Employee{}, ErrDuplicate
	}
	if err != nil {
		return Employee{}, err
	}
	return s.employeeView(row), nil
}

// ---- import ----

// ImportOptions control an import.
type ImportOptions struct {
	Preview  bool
	Period   string            // overrides the period read from the file
	Override map[string]string // column letter -> field
}

// ImportResult is the preview or the committed import.
type ImportResult struct {
	Period       string              `json:"period"`
	SourceSystem string              `json:"source_system"`
	Sheet        string              `json:"sheet"`
	HeaderRow    int                 `json:"header_row"`
	Columns      []mapping.ColumnMap `json:"columns"`
	Missing      []string            `json:"missing"`
	MappedBy     string              `json:"mapped_by"`
	Lines        []Line              `json:"lines"`
	LineCount    int                 `json:"line_count"`
	Totals       Totals              `json:"totals"`
	Issues       []Issue             `json:"issues"`
	Warnings     []string            `json:"warnings"`
	Committed    bool                `json:"committed"`
	Run          *Run                `json:"run,omitempty"`
}

const previewLines = 50

// Import reads a payroll workbook. Preview returns the mapping and the
// lines without writing; commit replaces the period's run.
func (s *Service) Import(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, data []byte, opt ImportOptions) (ImportResult, error) {
	p, err := ReadFile(fileName, data)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{SourceSystem: p.SourceSystem, Sheet: p.Sheet.Name, HeaderRow: p.Layout.HeaderRow, MappedBy: "template", Warnings: []string{}}
	if len(Missing(p.Mapping)) > 0 && s.ai != nil && len(opt.Override) == 0 {
		mp, err := s.ai.MapPayroll(ctx, mapping.AIRequest{
			OrganizationID: scope.OrganizationID, TaxpayerID: scope.TaxpayerID, UserID: actorID,
			DataType: mapping.Payroll, Sheet: p.Sheet, Layout: p.Layout,
		})
		switch {
		case err == nil && len(Missing(mp)) < len(Missing(p.Mapping)):
			p.Mapping, res.MappedBy = mp, "ai"
			res.Warnings = append(res.Warnings, mp.Warnings...)
		case err != nil && !errors.Is(err, mapping.ErrAIUnavailable):
			res.Warnings = append(res.Warnings, "Yapay zekâ kolon eşlemesi yapılamadı; eşleme başlıklardan yapıldı")
		}
	}
	if p.Mapping, err = ApplyOverride(p.Mapping, opt.Override); err != nil {
		return ImportResult{}, err
	}
	if len(opt.Override) > 0 {
		res.MappedBy = "user"
	}
	res.Columns, res.Missing = p.Mapping.Columns, Missing(p.Mapping)
	if res.Missing == nil {
		res.Missing = []string{}
	}
	lines, issues, rowPeriod := Lines(p)
	res.Period = firstNonEmpty(opt.Period, p.Period, rowPeriod)
	res.Issues = issues
	if res.Issues == nil {
		res.Issues = []Issue{}
	}
	res.Totals, res.LineCount = Sum(lines), len(lines)
	res.Lines = lines
	if len(res.Lines) > previewLines {
		res.Lines = res.Lines[:previewLines]
	}
	if res.Lines == nil {
		res.Lines = []Line{}
	}
	if opt.Preview {
		return res, nil
	}
	if len(res.Missing) > 0 {
		return res, ErrMappingMissing
	}
	if res.Period == "" {
		return res, ErrPeriodRequired
	}
	if len(lines) == 0 {
		return res, fmt.Errorf("%w: no employee rows", ErrInvalid)
	}
	run, err := s.commit(ctx, scope, actorID, fileName, res, lines)
	if err != nil {
		return res, err
	}
	res.Committed, res.Run = true, &run
	return res, nil
}

func (s *Service) commit(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, res ImportResult, lines []Line) (Run, error) {
	var out Run
	err := database.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := db.New(tx)
		if prev, err := q.GetPayrollRunByPeriod(ctx, db.GetPayrollRunByPeriodParams{TaxpayerID: scope.TaxpayerID, Period: res.Period}); err == nil {
			if err := s.unlinkDraft(ctx, q, scope, prev); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		issues, _ := json.Marshal(res.Issues)
		t := res.Totals
		run, err := q.UpsertPayrollRun(ctx, db.UpsertPayrollRunParams{
			TaxpayerID: scope.TaxpayerID, Period: res.Period, Source: "import", SourceSystem: res.SourceSystem,
			SourceFileName: optText(fileName), EmployeeCount: t.EmployeeCount, GrossTotal: num(t.Gross), NetTotal: num(t.Net),
			SgkEmployeeTotal: num(t.SGKEmployee), UnemploymentEmployeeTotal: num(t.UnemploymentEmployee),
			SgkEmployerTotal: num(t.SGKEmployer), UnemploymentEmployerTotal: num(t.UnemploymentEmployer),
			SgkIncentiveTotal: num(t.SGKIncentive), IncomeTaxTotal: num(t.IncomeTax), StampTaxTotal: num(t.StampTax),
			ExemptionTotal: num(t.Exemption), OtherDeductionsTotal: num(t.OtherDeductions), EmployerCostTotal: num(t.EmployerCost),
			Issues: issues, CreatedBy: actor(actorID),
		})
		if err != nil {
			return err
		}
		if err := q.DeletePayrollLines(ctx, db.DeletePayrollLinesParams{TaxpayerID: scope.TaxpayerID, PayrollRunID: run.ID}); err != nil {
			return err
		}
		for i, l := range lines {
			empID, err := s.employeeFor(ctx, q, scope.TaxpayerID, actorID, l)
			if err != nil {
				return fmt.Errorf("payroll: row %d: %w", l.Row, err)
			}
			if err := q.InsertPayrollLine(ctx, lineParams(scope.TaxpayerID, run.ID, empID, int32(i+1), l)); err != nil { //nolint:gosec // bounded by MaxRows
				return fmt.Errorf("payroll: row %d: %w", l.Row, err)
			}
		}
		accrual, incentive, payable := t.SGKAccrual()
		due, err := SGKDueDate(res.Period)
		if err != nil {
			return err
		}
		if _, err := q.UpsertSgkAccrual(ctx, db.UpsertSgkAccrualParams{
			TaxpayerID: scope.TaxpayerID, Period: res.Period, PayrollRunID: pgtype.Int8{Int64: run.ID, Valid: true},
			AccrualAmount: num(accrual), IncentiveAmount: num(incentive), PayableAmount: num(payable),
			DueDate: pgtype.Date{Time: due, Valid: true},
		}); err != nil {
			return err
		}
		v, err := s.runView(ctx, q, scope, run)
		out = v
		return err
	})
	return out, err
}

// unlinkDraft: a re-import may replace a run whose journal draft is still
// a draft (it is left in place, no longer linked); an approved or exported
// draft locks the run.
func (s *Service) unlinkDraft(ctx context.Context, q *db.Queries, scope engagementctx.Scope, run db.PayrollRun) error {
	if !run.JournalDraftID.Valid {
		return nil
	}
	d, err := q.GetPayrollJournalDraft(ctx, db.GetPayrollJournalDraftParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: run.JournalDraftID.Int64,
	})
	if err == nil && d.Status != "draft" {
		return ErrRunLocked
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return q.SetPayrollRunJournalDraft(ctx, db.SetPayrollRunJournalDraftParams{TaxpayerID: run.TaxpayerID, ID: run.ID})
}

// employeeFor finds the line's employee (TCKN digest, then the name among
// employees without a TCKN) or creates one.
func (s *Service) employeeFor(ctx context.Context, q *db.Queries, taxpayerID, actorID int64, l Line) (pgtype.Int8, error) {
	var digest pgtype.Text
	if l.nationalID != "" {
		digest = text(s.digest.Digest(taxpayerID, l.nationalID))
		e, err := q.GetEmployeeByDigest(ctx, db.GetEmployeeByDigestParams{TaxpayerID: taxpayerID, NationalIDDigest: digest})
		if err == nil {
			return s.fill(ctx, q, taxpayerID, e.ID, l)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.Int8{}, err
		}
	}
	if l.EmployeeName == "" {
		return pgtype.Int8{}, nil
	}
	if e, err := q.FindEmployeeByName(ctx, db.FindEmployeeByNameParams{TaxpayerID: taxpayerID, FullName: l.EmployeeName}); err == nil {
		if digest.Valid {
			// The row brings the TCKN the hand-entered employee lacked.
			if _, err := q.UpdateEmployee(ctx, db.UpdateEmployeeParams{
				TaxpayerID: taxpayerID, ID: e.ID, FullName: e.FullName, NationalIDMasked: text(l.NationalIDMasked), NationalIDDigest: digest,
				SgkNo: e.SgkNo, JobTitle: e.JobTitle, Department: e.Department, Iban: e.Iban, HireDate: e.HireDate,
				ExitDate: e.ExitDate, ExitReason: e.ExitReason,
			}); err != nil {
				return pgtype.Int8{}, err
			}
		}
		return s.fill(ctx, q, taxpayerID, e.ID, l)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, err
	}
	var masked pgtype.Text
	if digest.Valid {
		masked = text(l.NationalIDMasked)
	}
	e, err := q.InsertEmployee(ctx, db.InsertEmployeeParams{
		TaxpayerID: taxpayerID, FullName: l.EmployeeName, NationalIDMasked: masked, NationalIDDigest: digest,
		SgkNo: optText(l.SGKNo), JobTitle: optText(l.JobTitle), Department: optText(l.Department),
		HireDate: dateOf(l.HireDate), ExitDate: dateOf(l.ExitDate), CreatedBy: actor(actorID),
	})
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: e.ID, Valid: true}, nil
}

func (s *Service) fill(ctx context.Context, q *db.Queries, taxpayerID, id int64, l Line) (pgtype.Int8, error) {
	err := q.FillEmployeeFromImport(ctx, db.FillEmployeeFromImportParams{
		TaxpayerID: taxpayerID, ID: id, SgkNo: optText(l.SGKNo), JobTitle: optText(l.JobTitle),
		Department: optText(l.Department), HireDate: dateOf(l.HireDate), ExitDate: dateOf(l.ExitDate),
	})
	return pgtype.Int8{Int64: id, Valid: true}, err
}

func lineParams(taxpayerID, runID int64, empID pgtype.Int8, no int32, l Line) db.InsertPayrollLineParams {
	p := db.InsertPayrollLineParams{
		TaxpayerID: taxpayerID, PayrollRunID: runID, EmployeeID: empID, LineNo: no, EmployeeName: l.EmployeeName,
		NationalIDMasked: optText(l.NationalIDMasked), Gross: num(l.Gross), SgkBase: num(l.SGKBase),
		SgkEmployee: num(l.SGKEmployee), UnemploymentEmployee: num(l.UnemploymentEmployee), IncomeTaxBase: num(l.IncomeTaxBase),
		IncomeTax: num(l.IncomeTax), IncomeTaxExemption: num(l.IncomeTaxExemption), StampTax: num(l.StampTax),
		StampTaxExemption: num(l.StampTaxExemption), Agi: num(l.AGI), OtherDeductions: num(l.OtherDeductions), Net: num(l.Net),
		SgkEmployer: num(l.SGKEmployer), UnemploymentEmployer: num(l.UnemploymentEmployer), SgkIncentive: num(l.SGKIncentive),
		EmployerCost: num(l.EmployerCost),
	}
	if p.EmployeeName == "" {
		p.EmployeeName = "(adsız)"
	}
	if l.DaysWorked != nil {
		p.DaysWorked = pgtype.Int4{Int32: *l.DaysWorked, Valid: true}
	}
	return p
}

// ---- runs ----

// Run is the API view of a payroll run.
type Run struct {
	UUID               uuid.UUID       `json:"uuid"`
	Period             string          `json:"period"`
	Source             string          `json:"source"`
	SourceSystem       string          `json:"source_system"`
	SourceFileName     *string         `json:"source_file_name"`
	Totals             Totals          `json:"totals"`
	Issues             []Issue         `json:"issues"`
	NetPaid            decimal.Decimal `json:"net_paid"`
	PaymentStatus      string          `json:"payment_status"`
	SGK                *PremiumStatus  `json:"sgk"`
	JournalDraftUUID   *uuid.UUID      `json:"journal_draft_uuid"`
	JournalDraftStatus *string         `json:"journal_draft_status"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// PremiumStatus is the SGK side of a run.
type PremiumStatus struct {
	UUID    uuid.UUID       `json:"uuid"`
	Payable decimal.Decimal `json:"payable_amount"`
	Paid    decimal.Decimal `json:"paid_amount"`
	DueDate string          `json:"due_date"`
	Status  string          `json:"status"`
}

func paymentStatus(due, paid decimal.Decimal) string {
	switch {
	case due.IsZero() || paid.GreaterThanOrEqual(due.Sub(tolerance)):
		return "paid"
	case paid.IsPositive():
		return "partial"
	default:
		return "unpaid"
	}
}

func premiumStatus(payable, paid decimal.Decimal, due time.Time, now time.Time) string {
	st := paymentStatus(payable, paid)
	if st != "paid" && today(now).After(due) {
		return "overdue"
	}
	if st == "unpaid" {
		return "accrued"
	}
	return st
}

func totalsOf(r db.PayrollRun) Totals {
	return Totals{
		EmployeeCount: r.EmployeeCount, Gross: dec(r.GrossTotal), Net: dec(r.NetTotal), SGKEmployee: dec(r.SgkEmployeeTotal),
		UnemploymentEmployee: dec(r.UnemploymentEmployeeTotal), SGKEmployer: dec(r.SgkEmployerTotal),
		UnemploymentEmployer: dec(r.UnemploymentEmployerTotal), SGKIncentive: dec(r.SgkIncentiveTotal),
		IncomeTax: dec(r.IncomeTaxTotal), StampTax: dec(r.StampTaxTotal), Exemption: dec(r.ExemptionTotal),
		OtherDeductions: dec(r.OtherDeductionsTotal), EmployerCost: dec(r.EmployerCostTotal),
	}
}

func issuesOf(raw []byte) []Issue {
	out := []Issue{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = []Issue{}
	}
	return out
}

func (s *Service) runView(ctx context.Context, q *db.Queries, scope engagementctx.Scope, r db.PayrollRun) (Run, error) {
	v := Run{
		UUID: r.Uuid, Period: r.Period, Source: r.Source, SourceSystem: r.SourceSystem, SourceFileName: textPtr(r.SourceFileName),
		Totals: totalsOf(r), Issues: issuesOf(r.Issues), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	paid, err := q.PayrollRunNetPaid(ctx, db.PayrollRunNetPaidParams{TaxpayerID: r.TaxpayerID, PayrollRunID: r.ID})
	if err != nil {
		return v, err
	}
	v.NetPaid = dec(paid)
	v.PaymentStatus = paymentStatus(v.Totals.Net, v.NetPaid)
	if p, err := q.GetSgkPremiumByPeriod(ctx, db.GetSgkPremiumByPeriodParams{TaxpayerID: r.TaxpayerID, Period: r.Period}); err == nil {
		v.SGK = &PremiumStatus{UUID: p.Uuid, Payable: dec(p.PayableAmount), Paid: dec(p.PaidAmount), DueDate: p.DueDate.Time.Format(time.DateOnly),
			Status: premiumStatus(dec(p.PayableAmount), dec(p.PaidAmount), p.DueDate.Time, s.now())}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return v, err
	}
	if r.JournalDraftID.Valid {
		d, err := q.GetPayrollJournalDraft(ctx, db.GetPayrollJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: r.JournalDraftID.Int64})
		if err == nil {
			v.JournalDraftUUID, v.JournalDraftStatus = &d.Uuid, &d.Status
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return v, err
		}
	}
	return v, nil
}

// RunPage is a page of runs.
type RunPage struct {
	Items  []Run `json:"items"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// ListRuns lists the taxpayer's payroll runs, newest period first.
func (s *Service) ListRuns(ctx context.Context, scope engagementctx.Scope, year string, limit, offset int32) (RunPage, error) {
	rows, err := s.q.ListPayrollRuns(ctx, db.ListPayrollRunsParams{
		TaxpayerID: scope.TaxpayerID, OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		Year: optText(year), Lim: limit, Off: offset,
	})
	if err != nil {
		return RunPage{}, err
	}
	p := RunPage{Items: make([]Run, len(rows)), Limit: limit, Offset: offset}
	for i, r := range rows {
		v := Run{
			UUID: r.Uuid, Period: r.Period, Source: r.Source, SourceSystem: r.SourceSystem, SourceFileName: textPtr(r.SourceFileName),
			Totals: totalsOf(db.PayrollRun{
				EmployeeCount: r.EmployeeCount, GrossTotal: r.GrossTotal, NetTotal: r.NetTotal, SgkEmployeeTotal: r.SgkEmployeeTotal,
				UnemploymentEmployeeTotal: r.UnemploymentEmployeeTotal, SgkEmployerTotal: r.SgkEmployerTotal,
				UnemploymentEmployerTotal: r.UnemploymentEmployerTotal, SgkIncentiveTotal: r.SgkIncentiveTotal,
				IncomeTaxTotal: r.IncomeTaxTotal, StampTaxTotal: r.StampTaxTotal, ExemptionTotal: r.ExemptionTotal,
				OtherDeductionsTotal: r.OtherDeductionsTotal, EmployerCostTotal: r.EmployerCostTotal,
			}),
			Issues: issuesOf(r.Issues), NetPaid: dec(r.NetPaid), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		v.PaymentStatus = paymentStatus(v.Totals.Net, v.NetPaid)
		if r.SgkUuid.Valid {
			v.SGK = &PremiumStatus{UUID: r.SgkUuid.Bytes, Payable: dec(r.SgkPayable), Paid: dec(r.SgkPaid),
				DueDate: r.SgkDueDate.Time.Format(time.DateOnly),
				Status:  premiumStatus(dec(r.SgkPayable), dec(r.SgkPaid), r.SgkDueDate.Time, s.now())}
		}
		if r.JournalDraftUuid.Valid {
			id := uuid.UUID(r.JournalDraftUuid.Bytes)
			v.JournalDraftUUID = &id
			v.JournalDraftStatus = textPtr(r.JournalDraftStatus)
		}
		p.Items[i] = v
	}
	return p, nil
}

// LineView is a stored payroll line.
type LineView struct {
	Line
	EmployeeUUID *uuid.UUID `json:"employee_uuid"`
}

// RunDetail is a run with its lines.
type RunDetail struct {
	Run
	Lines []LineView `json:"lines"`
}

func (s *Service) runRow(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.PayrollRun, error) {
	r, err := s.q.GetPayrollRunByUUID(ctx, db.GetPayrollRunByUUIDParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// GetRun returns a run and its lines.
func (s *Service) GetRun(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (RunDetail, error) {
	r, err := s.runRow(ctx, scope.TaxpayerID, id)
	if err != nil {
		return RunDetail{}, err
	}
	v, err := s.runView(ctx, s.q, scope, r)
	if err != nil {
		return RunDetail{}, err
	}
	lines, err := s.lines(ctx, r)
	if err != nil {
		return RunDetail{}, err
	}
	return RunDetail{Run: v, Lines: lines}, nil
}

func (s *Service) lines(ctx context.Context, r db.PayrollRun) ([]LineView, error) {
	rows, err := s.q.ListPayrollLines(ctx, db.ListPayrollLinesParams{TaxpayerID: r.TaxpayerID, PayrollRunID: r.ID})
	if err != nil {
		return nil, err
	}
	out := make([]LineView, len(rows))
	for i, l := range rows {
		v := LineView{Line: Line{
			Row: int(l.LineNo), EmployeeName: l.EmployeeName, NationalIDMasked: l.NationalIDMasked.String,
			Gross: dec(l.Gross), SGKBase: dec(l.SgkBase), SGKEmployee: dec(l.SgkEmployee),
			UnemploymentEmployee: dec(l.UnemploymentEmployee), IncomeTaxBase: dec(l.IncomeTaxBase), IncomeTax: dec(l.IncomeTax),
			IncomeTaxExemption: dec(l.IncomeTaxExemption), StampTax: dec(l.StampTax), StampTaxExemption: dec(l.StampTaxExemption),
			AGI: dec(l.Agi), OtherDeductions: dec(l.OtherDeductions), Net: dec(l.Net), SGKEmployer: dec(l.SgkEmployer),
			UnemploymentEmployer: dec(l.UnemploymentEmployer), SGKIncentive: dec(l.SgkIncentive), EmployerCost: dec(l.EmployerCost),
		}}
		if l.DaysWorked.Valid {
			d := l.DaysWorked.Int32
			v.DaysWorked = &d
		}
		if l.EmployeeUuid.Valid {
			id := uuid.UUID(l.EmployeeUuid.Bytes)
			v.EmployeeUUID = &id
		}
		out[i] = v
	}
	return out, nil
}

// DeleteRun removes a run (lines cascade). Its bank matches are removed and
// the movements go back to unmatched; the SGK accrual stays (paid or not).
func (s *Service) DeleteRun(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) error {
	r, err := s.runRow(ctx, scope.TaxpayerID, id)
	if err != nil {
		return err
	}
	return database.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := s.unlinkDraft(ctx, q, scope, r); err != nil {
			return err
		}
		if err := unmatch(ctx, q, scope.TaxpayerID, "payroll_run", r.ID); err != nil {
			return err
		}
		return q.DeletePayrollRun(ctx, db.DeletePayrollRunParams{TaxpayerID: scope.TaxpayerID, ID: r.ID})
	})
}

// ---- journal draft ----

// Accounts of the payroll entry (Tek Düzen Hesap Planı). Sub-accounts may
// be given ("770.01").
type Accounts struct {
	Expense         string `json:"expense_account"`          // 770 Genel Yönetim Giderleri
	Personnel       string `json:"personnel_account"`        // 335 Personele Borçlar
	Taxes           string `json:"taxes_account"`            // 360 Ödenecek Vergi ve Fonlar
	SGK             string `json:"sgk_account"`              // 361 Ödenecek Sosyal Güvenlik Kesintileri
	OtherDeductions string `json:"other_deductions_account"` // 336 Diğer Çeşitli Borçlar
}

func (a Accounts) withDefaults() Accounts {
	def := func(v, d string) string {
		if v = strings.TrimSpace(v); v == "" {
			return d
		}
		return v
	}
	return Accounts{
		Expense: def(a.Expense, "770"), Personnel: def(a.Personnel, "335"), Taxes: def(a.Taxes, "360"),
		SGK: def(a.SGK, "361"), OtherDeductions: def(a.OtherDeductions, "336"),
	}
}

// EntryLines builds the payroll entry:
//
//	770 borç   brüt + işveren SGK + işveren işsizlik − teşvik
//	335 alacak net ücretler
//	360 alacak gelir vergisi + damga vergisi − AGİ
//	361 alacak işçi ve işveren SGK + işsizlik − teşvik
//	336 alacak diğer kesintiler (avans, icra, sendika)
//
// It balances when every line satisfies net = brüt − kesintiler + AGİ.
func EntryLines(t Totals, period string, a Accounts, runID uuid.UUID) []journals.LineInput {
	a = a.withDefaults()
	agi := t.Gross.Sub(t.SGKEmployee).Sub(t.UnemploymentEmployee).Sub(t.IncomeTax).Sub(t.StampTax).Sub(t.OtherDeductions).Sub(t.Net).Neg()
	if agi.IsNegative() {
		agi = decimal.Zero
	}
	_, _, sgk := t.SGKAccrual()
	expense := t.Gross.Add(t.SGKEmployer).Add(t.UnemploymentEmployer).Sub(t.SGKIncentive)
	taxes := t.IncomeTax.Add(t.StampTax).Sub(agi)
	if taxes.IsNegative() {
		taxes = decimal.Zero
	}
	src := runID.String()
	desc := period + " dönemi bordrosu"
	var out []journals.LineInput
	add := func(acc, d string, debit, credit decimal.Decimal) {
		if debit.IsZero() && credit.IsZero() {
			return
		}
		out = append(out, journals.LineInput{Account: acc, Description: d, Debit: debit, Credit: credit, SourceKind: "payroll", SourceID: src})
	}
	add(a.Expense, desc+" personel gideri", expense, decimal.Zero)
	add(a.Personnel, desc+" net ücretler", decimal.Zero, t.Net)
	add(a.Taxes, desc+" gelir ve damga vergisi", decimal.Zero, taxes)
	add(a.SGK, desc+" SGK primleri", decimal.Zero, sgk)
	add(a.OtherDeductions, desc+" diğer kesintiler", decimal.Zero, t.OtherDeductions)
	return out
}

// CreateJournalDraft writes the run's journal draft (journal_drafts, #143),
// dated the last day of the period.
func (s *Service) CreateJournalDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, a Accounts) (journals.Draft, error) {
	if s.journals == nil {
		return journals.Draft{}, ErrJournalsDisabled
	}
	r, err := s.runRow(ctx, scope.TaxpayerID, id)
	if err != nil {
		return journals.Draft{}, err
	}
	if r.JournalDraftID.Valid {
		if _, err := s.q.GetPayrollJournalDraft(ctx, db.GetPayrollJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: r.JournalDraftID.Int64}); err == nil {
			return journals.Draft{}, ErrJournalExists
		}
	}
	t := totalsOf(r)
	if t.Gross.IsZero() {
		return journals.Draft{}, fmt.Errorf("%w: the run has no amounts", ErrInvalid)
	}
	date := periodStart(r.Period).AddDate(0, 1, -1)
	d, err := s.journals.CreateDraft(ctx, scope, actorID, journals.DraftInput{
		Date: date, Description: r.Period + " dönemi maaş tahakkuku", DocumentType: "bordro",
		Origin: map[string]any{"source": "payroll", "payroll_run_uuid": r.Uuid.String(), "rule": "bordro tahakkuku 770/335/360/361"},
		Lines:  EntryLines(t, r.Period, a, r.Uuid),
	})
	if err != nil {
		return journals.Draft{}, err
	}
	jid, err := s.q.GetJournalDraftIDByUUID(ctx, db.GetJournalDraftIDByUUIDParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: d.UUID})
	if err != nil {
		return d, err
	}
	return d, s.q.SetPayrollRunJournalDraft(ctx, db.SetPayrollRunJournalDraftParams{
		TaxpayerID: scope.TaxpayerID, ID: r.ID, JournalDraftID: pgtype.Int8{Int64: jid, Valid: true},
	})
}

// ---- SGK premiums ----

// Premium is the API view of an SGK accrual.
type Premium struct {
	UUID                uuid.UUID       `json:"uuid"`
	Period              string          `json:"period"`
	Accrual             decimal.Decimal `json:"accrual_amount"`
	Incentive           decimal.Decimal `json:"incentive_amount"`
	Payable             decimal.Decimal `json:"payable_amount"`
	DueDate             string          `json:"due_date"`
	DocumentNo          *string         `json:"document_no"`
	Paid                decimal.Decimal `json:"paid_amount"`
	PaidAt              *string         `json:"paid_at"`
	Status              string          `json:"status"`
	BankTransactionUUID *uuid.UUID      `json:"bank_transaction_uuid"`
	Note                *string         `json:"note"`
	FromPayroll         bool            `json:"from_payroll"`
}

func (s *Service) premiumView(p db.SgkPremium, bt pgtype.UUID) Premium {
	v := Premium{
		UUID: p.Uuid, Period: p.Period, Accrual: dec(p.AccrualAmount), Incentive: dec(p.IncentiveAmount), Payable: dec(p.PayableAmount),
		DueDate: p.DueDate.Time.Format(time.DateOnly), DocumentNo: textPtr(p.DocumentNo), Paid: dec(p.PaidAmount),
		PaidAt: datePtr(p.PaidAt), Note: textPtr(p.Note), FromPayroll: p.PayrollRunID.Valid,
	}
	v.Status = premiumStatus(v.Payable, v.Paid, p.DueDate.Time, s.now())
	if bt.Valid {
		id := uuid.UUID(bt.Bytes)
		v.BankTransactionUUID = &id
	}
	return v
}

// PremiumPage is a page of SGK accruals.
type PremiumPage struct {
	Items  []Premium `json:"items"`
	Limit  int32     `json:"limit"`
	Offset int32     `json:"offset"`
}

// ListPremiums lists the SGK accruals, newest first.
func (s *Service) ListPremiums(ctx context.Context, taxpayerID int64, year string, limit, offset int32) (PremiumPage, error) {
	rows, err := s.q.ListSgkPremiums(ctx, db.ListSgkPremiumsParams{TaxpayerID: taxpayerID, Year: optText(year), Lim: limit, Off: offset})
	if err != nil {
		return PremiumPage{}, err
	}
	p := PremiumPage{Items: make([]Premium, len(rows)), Limit: limit, Offset: offset}
	for i, r := range rows {
		p.Items[i] = s.premiumView(db.SgkPremium{
			ID: r.ID, Uuid: r.Uuid, TaxpayerID: r.TaxpayerID, Period: r.Period, PayrollRunID: r.PayrollRunID,
			AccrualAmount: r.AccrualAmount, IncentiveAmount: r.IncentiveAmount, PayableAmount: r.PayableAmount,
			DueDate: r.DueDate, DocumentNo: r.DocumentNo, PaidAmount: r.PaidAmount, PaidAt: r.PaidAt,
			BankTransactionID: r.BankTransactionID, Note: r.Note,
		}, r.BankTransactionUuid)
	}
	return p, nil
}

// PremiumInput creates an accrual by hand or records its payment.
type PremiumInput struct {
	Period     string           `json:"period"`
	Accrual    *decimal.Decimal `json:"accrual_amount"`
	Incentive  *decimal.Decimal `json:"incentive_amount"`
	DueDate    *string          `json:"due_date"`
	DocumentNo *string          `json:"document_no"`
	Paid       *decimal.Decimal `json:"paid_amount"`
	PaidAt     *string          `json:"paid_at"`
	Note       *string          `json:"note"`
}

// CreatePremium records an accrual for a period without a payroll run.
func (s *Service) CreatePremium(ctx context.Context, taxpayerID int64, in PremiumInput) (Premium, error) {
	if !periodRe.MatchString(in.Period) || in.Accrual == nil || in.Accrual.IsNegative() {
		return Premium{}, fmt.Errorf("%w: period (YYYY-MM) and accrual_amount are required", ErrInvalid)
	}
	incentive := decimal.Zero
	if in.Incentive != nil {
		incentive = *in.Incentive
	}
	if incentive.IsNegative() || incentive.GreaterThan(*in.Accrual) {
		return Premium{}, fmt.Errorf("%w: incentive_amount must be between 0 and accrual_amount", ErrInvalid)
	}
	due, err := SGKDueDate(in.Period)
	if err != nil {
		return Premium{}, err
	}
	if in.DueDate != nil && *in.DueDate != "" {
		if due, err = time.Parse(time.DateOnly, *in.DueDate); err != nil {
			return Premium{}, fmt.Errorf("%w: due_date is YYYY-MM-DD", ErrInvalid)
		}
	}
	row, err := s.q.InsertSgkPremium(ctx, db.InsertSgkPremiumParams{
		TaxpayerID: taxpayerID, Period: in.Period, AccrualAmount: num(*in.Accrual), IncentiveAmount: num(incentive),
		PayableAmount: num(in.Accrual.Sub(incentive)), DueDate: pgtype.Date{Time: due, Valid: true},
		DocumentNo: optTextPtr(in.DocumentNo), Note: optTextPtr(in.Note),
	})
	if isUnique(err) {
		return Premium{}, fmt.Errorf("%w: the period already has an accrual", ErrDocumentExists)
	}
	if err != nil {
		return Premium{}, err
	}
	return s.premiumView(row, pgtype.UUID{}), nil
}

// UpdatePremium records the payment (paid_amount, paid_at) or corrects a
// hand-entered accrual. An accrual from a payroll keeps its amounts.
func (s *Service) UpdatePremium(ctx context.Context, taxpayerID int64, id uuid.UUID, in PremiumInput) (Premium, error) {
	p, err := s.q.GetSgkPremiumByUUID(ctx, db.GetSgkPremiumByUUIDParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Premium{}, ErrNotFound
	}
	if err != nil {
		return Premium{}, err
	}
	accrual, incentive := dec(p.AccrualAmount), dec(p.IncentiveAmount)
	if (in.Accrual != nil || in.Incentive != nil) && p.PayrollRunID.Valid {
		return Premium{}, fmt.Errorf("%w: the accrual comes from the payroll; re-import the payroll to change it", ErrInvalid)
	}
	if in.Accrual != nil {
		accrual = *in.Accrual
	}
	if in.Incentive != nil {
		incentive = *in.Incentive
	}
	if accrual.IsNegative() || incentive.IsNegative() || incentive.GreaterThan(accrual) {
		return Premium{}, fmt.Errorf("%w: amounts must be positive and incentive ≤ accrual", ErrInvalid)
	}
	due := p.DueDate
	if in.DueDate != nil {
		d, err := time.Parse(time.DateOnly, *in.DueDate)
		if err != nil {
			return Premium{}, fmt.Errorf("%w: due_date is YYYY-MM-DD", ErrInvalid)
		}
		due = pgtype.Date{Time: d, Valid: true}
	}
	paid, paidAt := dec(p.PaidAmount), p.PaidAt
	if in.Paid != nil {
		if in.Paid.IsNegative() {
			return Premium{}, fmt.Errorf("%w: paid_amount must be positive", ErrInvalid)
		}
		paid = *in.Paid
	}
	if in.PaidAt != nil {
		if *in.PaidAt == "" {
			paidAt = pgtype.Date{}
		} else {
			d, err := time.Parse(time.DateOnly, *in.PaidAt)
			if err != nil {
				return Premium{}, fmt.Errorf("%w: paid_at is YYYY-MM-DD", ErrInvalid)
			}
			paidAt = pgtype.Date{Time: d, Valid: true}
		}
	}
	if paid.IsZero() {
		paidAt = pgtype.Date{}
	} else if !paidAt.Valid {
		paidAt = pgtype.Date{Time: today(s.now()), Valid: true}
	}
	docNo, note := p.DocumentNo, p.Note
	if in.DocumentNo != nil {
		docNo = optText(*in.DocumentNo)
	}
	if in.Note != nil {
		note = optText(*in.Note)
	}
	bt := p.BankTransactionID
	if paid.IsZero() {
		bt = pgtype.Int8{}
	}
	row, err := s.q.UpdateSgkPremium(ctx, db.UpdateSgkPremiumParams{
		TaxpayerID: taxpayerID, ID: p.ID, AccrualAmount: num(accrual), IncentiveAmount: num(incentive),
		PayableAmount: num(accrual.Sub(incentive)), DueDate: due, DocumentNo: docNo, PaidAmount: num(paid), PaidAt: paidAt,
		BankTransactionID: bt, Note: note,
	})
	if err != nil {
		return Premium{}, err
	}
	return s.premiumView(row, pgtype.UUID{}), nil
}

// ---- documents ----

// Document is the API view of a payroll document.
type Document struct {
	UUID              uuid.UUID  `json:"uuid"`
	Period            string     `json:"period"`
	Kind              string     `json:"kind"`
	Title             string     `json:"title"`
	FileName          string     `json:"file_name"`
	SizeBytes         int64      `json:"size_bytes"`
	ContentType       string     `json:"content_type"`
	VisibleToTaxpayer bool       `json:"visible_to_taxpayer"`
	EmployeeUUID      *uuid.UUID `json:"employee_uuid"`
	EmployeeName      *string    `json:"employee_name"`
	CreatedAt         time.Time  `json:"created_at"`
}

// DocumentPage is a page of documents.
type DocumentPage struct {
	Items  []Document `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// DocumentInput describes an uploaded document.
type DocumentInput struct {
	Period            string
	Kind              string
	Title             string
	EmployeeUUID      *uuid.UUID
	VisibleToTaxpayer bool
}

// UploadDocument stores a payroll PDF. The same file twice is refused.
func (s *Service) UploadDocument(ctx context.Context, taxpayerID, actorID int64, fileName string, data []byte, in DocumentInput) (Document, error) {
	if s.store == nil {
		return Document{}, ErrStorageMissing
	}
	if !periodRe.MatchString(in.Period) {
		return Document{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if in.Kind == "" {
		in.Kind = "payroll"
	}
	if !documentKinds[in.Kind] {
		return Document{}, fmt.Errorf("%w: kind must be payroll, payslip, sgk_accrual, sgk_receipt or other", ErrInvalid)
	}
	if len(data) == 0 || len(data) > MaxDocumentBytes {
		return Document{}, fmt.Errorf("%w: the file is empty or larger than 20 MB", ErrInvalid)
	}
	if !bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\t\r\n "), []byte("%PDF-")) {
		return Document{}, ErrNotPDF
	}
	var runID, empID pgtype.Int8
	if r, err := s.q.GetPayrollRunByPeriod(ctx, db.GetPayrollRunByPeriodParams{TaxpayerID: taxpayerID, Period: in.Period}); err == nil {
		runID = pgtype.Int8{Int64: r.ID, Valid: true}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Document{}, err
	}
	if in.EmployeeUUID != nil {
		e, err := s.q.GetEmployeeByUUID(ctx, db.GetEmployeeByUUIDParams{TaxpayerID: taxpayerID, Uuid: *in.EmployeeUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, fmt.Errorf("%w: employee_uuid is unknown", ErrInvalid)
		}
		if err != nil {
			return Document{}, err
		}
		empID = pgtype.Int8{Int64: e.ID, Valid: true}
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	name := cleanFileName(fileName)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.TrimSuffix(name, ".pdf")
	}
	key := fmt.Sprintf("payroll/%d/%s/%s.pdf", taxpayerID, in.Period, digest)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf", Filename: name}, key); err != nil {
		return Document{}, fmt.Errorf("payroll: upload: %w", err)
	}
	row, err := s.q.InsertPayrollDocument(ctx, db.InsertPayrollDocumentParams{
		TaxpayerID: taxpayerID, Period: in.Period, PayrollRunID: runID, EmployeeID: empID, Kind: in.Kind, Title: title,
		StorageKey: key, Sha256: digest, SizeBytes: int64(len(data)), ContentType: "application/pdf", FileName: name,
		VisibleToTaxpayer: in.VisibleToTaxpayer, CreatedBy: actor(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrDocumentExists
	}
	if err != nil {
		return Document{}, err
	}
	return Document{UUID: row.Uuid, Period: row.Period, Kind: row.Kind, Title: row.Title, FileName: row.FileName,
		SizeBytes: row.SizeBytes, ContentType: row.ContentType, VisibleToTaxpayer: row.VisibleToTaxpayer,
		EmployeeUUID: in.EmployeeUUID, CreatedAt: row.CreatedAt.Time}, nil
}

// ListDocuments lists documents; onlyVisible for the mükellef panel.
func (s *Service) ListDocuments(ctx context.Context, taxpayerID int64, onlyVisible bool, period string, limit, offset int32) (DocumentPage, error) {
	rows, err := s.q.ListPayrollDocuments(ctx, db.ListPayrollDocumentsParams{
		TaxpayerID: taxpayerID, OnlyVisible: onlyVisible, Period: optText(period), Lim: limit, Off: offset,
	})
	if err != nil {
		return DocumentPage{}, err
	}
	p := DocumentPage{Items: make([]Document, len(rows)), Limit: limit, Offset: offset}
	for i, r := range rows {
		d := Document{UUID: r.Uuid, Period: r.Period, Kind: r.Kind, Title: r.Title, FileName: r.FileName, SizeBytes: r.SizeBytes,
			ContentType: r.ContentType, VisibleToTaxpayer: r.VisibleToTaxpayer, EmployeeName: textPtr(r.EmployeeName), CreatedAt: r.CreatedAt.Time}
		if r.EmployeeUuid.Valid {
			id := uuid.UUID(r.EmployeeUuid.Bytes)
			d.EmployeeUUID = &id
		}
		p.Items[i], p.Total = d, r.TotalCount
	}
	return p, nil
}

// File is an opened document.
type File struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Name        string
}

// OpenDocument opens a document's file.
func (s *Service) OpenDocument(ctx context.Context, taxpayerID int64, id uuid.UUID, onlyVisible bool) (File, error) {
	if s.store == nil {
		return File{}, ErrStorageMissing
	}
	d, err := s.q.GetPayrollDocument(ctx, db.GetPayrollDocumentParams{TaxpayerID: taxpayerID, Uuid: id, OnlyVisible: onlyVisible})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	rc, size, err := s.store.Download(ctx, d.StorageKey)
	if err != nil {
		return File{}, fmt.Errorf("payroll: download: %w", err)
	}
	return File{Body: rc, Size: size, ContentType: d.ContentType, Name: d.FileName}, nil
}

// DeleteDocument removes a document and its object.
func (s *Service) DeleteDocument(ctx context.Context, taxpayerID int64, id uuid.UUID) error {
	d, err := s.q.GetPayrollDocument(ctx, db.GetPayrollDocumentParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.q.DeletePayrollDocument(ctx, db.DeletePayrollDocumentParams{TaxpayerID: taxpayerID, ID: d.ID}); err != nil {
		return err
	}
	if s.store != nil {
		// The row is gone; a left-over object is only storage.
		_ = s.store.Delete(ctx, d.StorageKey)
	}
	return nil
}

// ---- helpers ----

func cleanFileName(n string) string {
	n = strings.TrimSpace(n)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if n == "" {
		n = "bordro.pdf"
	}
	if len(n) > 200 {
		n = n[len(n)-200:]
	}
	if !strings.HasSuffix(strings.ToLower(n), ".pdf") {
		n += ".pdf"
	}
	return n
}

func validIBAN(iban string) bool {
	return len(iban) == 26 && strings.HasPrefix(iban, "TR") && taxid.Normalize(iban[2:]) == iban[2:]
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func today(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func num(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.StringFixed(2))
	return n
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil || n.NaN {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func optTextPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return optText(*s)
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}

func dateOf(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func actor(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }
