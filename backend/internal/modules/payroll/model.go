// Package payroll holds employees, monthly payroll runs and SGK premiums of
// a taxpayer (issue #382): Excel/CSV payroll import (Luca, Zirve, Logo and
// free-form sheets), payroll PDFs as documents, the journal draft of a run
// (770 / 335 / 360 / 361) and bank matching of salary and SGK payments.
//
// Payroll is the taxpayer's source data: tables are taxpayer scoped like
// bank transactions. The national id (TCKN) is never stored in clear; rows
// keep a masked form and a keyed digest to recognise the same person again.
package payroll

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Source systems of an imported payroll.
const (
	SourceLuca    = "luca"
	SourceZirve   = "zirve"
	SourceLogo    = "logo"
	SourceGeneric = "generic"
)

// Document kinds.
var documentKinds = map[string]bool{"payroll": true, "payslip": true, "sgk_accrual": true, "sgk_receipt": true, "other": true}

// Line is one employee row of a payroll. Taxes are payable amounts (after
// the minimum wage exemption); exemptions are kept beside them.
type Line struct {
	Row                  int             `json:"row"`
	EmployeeName         string          `json:"employee_name"`
	NationalIDMasked     string          `json:"national_id_masked,omitempty"`
	nationalID           string          // clear TCKN, only in memory during an import
	SGKNo                string          `json:"sgk_no,omitempty"`
	JobTitle             string          `json:"job_title,omitempty"`
	Department           string          `json:"department,omitempty"`
	HireDate             *time.Time      `json:"hire_date,omitempty"`
	ExitDate             *time.Time      `json:"exit_date,omitempty"`
	DaysWorked           *int32          `json:"days_worked,omitempty"`
	Gross                decimal.Decimal `json:"gross"`
	SGKBase              decimal.Decimal `json:"sgk_base"`
	SGKEmployee          decimal.Decimal `json:"sgk_employee"`
	UnemploymentEmployee decimal.Decimal `json:"unemployment_employee"`
	IncomeTaxBase        decimal.Decimal `json:"income_tax_base"`
	IncomeTax            decimal.Decimal `json:"income_tax"`
	IncomeTaxExemption   decimal.Decimal `json:"income_tax_exemption"`
	StampTax             decimal.Decimal `json:"stamp_tax"`
	StampTaxExemption    decimal.Decimal `json:"stamp_tax_exemption"`
	AGI                  decimal.Decimal `json:"agi"`
	OtherDeductions      decimal.Decimal `json:"other_deductions"`
	Net                  decimal.Decimal `json:"net"`
	SGKEmployer          decimal.Decimal `json:"sgk_employer"`
	UnemploymentEmployer decimal.Decimal `json:"unemployment_employer"`
	SGKIncentive         decimal.Decimal `json:"sgk_incentive"`
	EmployerCost         decimal.Decimal `json:"employer_cost"`
}

// Totals sums the lines of a run.
type Totals struct {
	EmployeeCount        int32           `json:"employee_count"`
	Gross                decimal.Decimal `json:"gross_total"`
	Net                  decimal.Decimal `json:"net_total"`
	SGKEmployee          decimal.Decimal `json:"sgk_employee_total"`
	UnemploymentEmployee decimal.Decimal `json:"unemployment_employee_total"`
	SGKEmployer          decimal.Decimal `json:"sgk_employer_total"`
	UnemploymentEmployer decimal.Decimal `json:"unemployment_employer_total"`
	SGKIncentive         decimal.Decimal `json:"sgk_incentive_total"`
	IncomeTax            decimal.Decimal `json:"income_tax_total"`
	StampTax             decimal.Decimal `json:"stamp_tax_total"`
	Exemption            decimal.Decimal `json:"exemption_total"`
	OtherDeductions      decimal.Decimal `json:"other_deductions_total"`
	EmployerCost         decimal.Decimal `json:"employer_cost_total"`
}

// Issue is a finding of the row checks.
type Issue struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Issue codes.
const (
	IssueNetMismatch      = "NET_MISMATCH"
	IssueInvalidTCKN      = "INVALID_TCKN"
	IssueMissingName      = "MISSING_NAME"
	IssueZeroGross        = "ZERO_GROSS"
	IssueCostMismatch     = "EMPLOYER_COST_MISMATCH"
	IssueDuplicateTCKN    = "DUPLICATE_TCKN"
	IssueNegativeSkipped  = "NEGATIVE_AMOUNT"
	IssueMissingRequired  = "MISSING_REQUIRED_COLUMN"
	IssueNoRows           = "NO_ROWS"
	IssuePeriodNotInFile  = "PERIOD_UNKNOWN"
	IssueRowTotalsSkipped = "TOTAL_ROW_SKIPPED"
)

// tolerance: rounding in payroll programs (kuruş farkları).
var tolerance = decimal.NewFromInt(1)

// Complete fills derived amounts the sheet did not carry: employer cost
// (gross + employer shares − incentive) and, when the sheet has no net
// column, net (gross − employee deductions − taxes + AGİ).
func (l *Line) Complete(hasNet, hasCost bool) {
	if !hasNet {
		l.Net = l.expectedNet()
	}
	if !hasCost || l.EmployerCost.IsZero() {
		l.EmployerCost = l.expectedCost()
	}
	if l.SGKBase.IsZero() {
		l.SGKBase = l.Gross
	}
}

func (l Line) expectedNet() decimal.Decimal {
	return l.Gross.Sub(l.SGKEmployee).Sub(l.UnemploymentEmployee).Sub(l.IncomeTax).Sub(l.StampTax).
		Sub(l.OtherDeductions).Add(l.AGI)
}

func (l Line) expectedCost() decimal.Decimal {
	return l.Gross.Add(l.SGKEmployer).Add(l.UnemploymentEmployer).Sub(l.SGKIncentive)
}

// Check returns the row findings. They do not block the import: the
// accountant sees them on the run.
func (l Line) Check() []Issue {
	var out []Issue
	if l.EmployeeName == "" {
		out = append(out, Issue{l.Row, IssueMissingName, "Personel adı boş"})
	}
	if l.Gross.IsZero() {
		out = append(out, Issue{l.Row, IssueZeroGross, "Brüt ücret sıfır"})
	}
	if d := l.Net.Sub(l.expectedNet()).Abs(); d.GreaterThan(tolerance) {
		out = append(out, Issue{l.Row, IssueNetMismatch,
			fmt.Sprintf("Net ücret, brüt eksi kesintilerden %s TL farklı", d.StringFixed(2))})
	}
	if d := l.EmployerCost.Sub(l.expectedCost()).Abs(); d.GreaterThan(tolerance) {
		out = append(out, Issue{l.Row, IssueCostMismatch,
			fmt.Sprintf("İşverene maliyet, brüt ve işveren paylarından %s TL farklı", d.StringFixed(2))})
	}
	return out
}

// Sum totals the lines.
func Sum(lines []Line) Totals {
	var t Totals
	for _, l := range lines {
		t.EmployeeCount++
		t.Gross = t.Gross.Add(l.Gross)
		t.Net = t.Net.Add(l.Net)
		t.SGKEmployee = t.SGKEmployee.Add(l.SGKEmployee)
		t.UnemploymentEmployee = t.UnemploymentEmployee.Add(l.UnemploymentEmployee)
		t.SGKEmployer = t.SGKEmployer.Add(l.SGKEmployer)
		t.UnemploymentEmployer = t.UnemploymentEmployer.Add(l.UnemploymentEmployer)
		t.SGKIncentive = t.SGKIncentive.Add(l.SGKIncentive)
		t.IncomeTax = t.IncomeTax.Add(l.IncomeTax)
		t.StampTax = t.StampTax.Add(l.StampTax)
		t.Exemption = t.Exemption.Add(l.IncomeTaxExemption).Add(l.StampTaxExemption)
		t.OtherDeductions = t.OtherDeductions.Add(l.OtherDeductions)
		t.EmployerCost = t.EmployerCost.Add(l.EmployerCost)
	}
	return t
}

// SGKAccrual is the period's SGK premium accrual (işçi + işveren SGK ve
// işsizlik payları) and the payable amount after incentives.
func (t Totals) SGKAccrual() (accrual, incentive, payable decimal.Decimal) {
	accrual = t.SGKEmployee.Add(t.UnemploymentEmployee).Add(t.SGKEmployer).Add(t.UnemploymentEmployer)
	incentive = t.SGKIncentive
	payable = accrual.Sub(incentive)
	if payable.IsNegative() {
		payable = decimal.Zero
	}
	return accrual, incentive, payable
}

// SGKDueDate: a period's premiums are paid by the end of the next month
// (5510 s. Kanun md. 88).
func SGKDueDate(period string) (time.Time, error) {
	p, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, err
	}
	return p.AddDate(0, 2, -1), nil
}

// MaskNationalID keeps the first three and the last two digits.
func MaskNationalID(tckn string) string {
	if len(tckn) != 11 {
		return ""
	}
	return tckn[:3] + "******" + tckn[9:]
}

// NationalID parses a TCKN cell: digits only, checksum valid.
func NationalID(s string) (string, bool) {
	n := taxid.Normalize(s)
	if !taxid.ValidTCKN(n) {
		return "", false
	}
	return n, true
}

// Digester makes the keyed digest of a TCKN, per taxpayer so the same
// person at two employers cannot be linked through the digest.
type Digester struct{ key []byte }

// NewDigester derives the digest key from the application secret.
func NewDigester(secret string) Digester {
	sum := sha256.Sum256([]byte("payroll-national-id:" + secret))
	return Digester{key: sum[:]}
}

// Digest returns hex HMAC-SHA256(taxpayerID:tckn).
func (d Digester) Digest(taxpayerID int64, tckn string) string {
	m := hmac.New(sha256.New, d.key)
	_, _ = fmt.Fprintf(m, "%d:%s", taxpayerID, tckn)
	return hex.EncodeToString(m.Sum(nil))
}
