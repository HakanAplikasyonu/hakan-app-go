package rules

import (
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// CodeOutOfPeriod is a document that belongs to an earlier, already closed
// period (05 §3.2). The Excel import raises it on rows outside the chosen
// period; this rule raises it on invoices that reached the system after
// their own month was locked or its return filed.
const CodeOutOfPeriod = "OUT_OF_PERIOD"

// Close reasons of a period (param "reason"): the office locked it, or a
// return of the period was filed. Same values as late_documents.close_reason.
const (
	CloseReasonLock  = "lock"
	CloseReasonFiled = "filed"
)

// OutOfPeriodInput is one invoice with the closing of its own month.
type OutOfPeriodInput struct {
	InvoiceID int64
	// IssueDate is the invoice date; its month is the invoice's own period.
	IssueDate time.Time
	// ArrivedAt is when the invoice reached the system (created_at).
	ArrivedAt time.Time
	// RecordedPeriod is the period the invoice fell into: the later of its
	// period and the month it arrived (YYYY-MM). The warning is written to
	// this period, so the accountant working on it sees it.
	RecordedPeriod string
	// ClosedAt is the first closing of the issue month (lock or filed
	// return); nil while the month is open.
	ClosedAt    *time.Time
	CloseReason string
}

// OutOfPeriod checks OUT_OF_PERIOD on invoices: the invoice's month had been
// closed (locked, or a return filed) before the invoice arrived, and the
// invoice fell into a later period. It needs no threshold.
type OutOfPeriod struct{}

// Producer implements engine.Rule.
func (OutOfPeriod) Producer() string { return "control:out_of_period" }

// Codes implements engine.Rule.
func (OutOfPeriod) Codes() []string { return []string{CodeOutOfPeriod} }

// Check implements engine.Rule.
func (OutOfPeriod) Check(in OutOfPeriodInput, _ map[string]engine.Effective) ([]engine.Outcome, error) {
	id := in.InvoiceID
	o := engine.Outcome{
		Entity: engine.Entity{Type: catalog.EntityInvoice, ID: &id, Period: in.RecordedPeriod},
		Owned:  []string{CodeOutOfPeriod},
	}
	issuePeriod := in.IssueDate.Format("2006-01")
	if in.ClosedAt != nil && in.ArrivedAt.After(*in.ClosedAt) && in.RecordedPeriod > issuePeriod {
		o.Findings = []engine.Finding{{Code: CodeOutOfPeriod, Params: map[string]any{
			"issue_date":      in.IssueDate.Format("2006-01-02"),
			"issue_period":    issuePeriod,
			"recorded_period": in.RecordedPeriod,
			"closed_on":       in.ClosedAt.In(istanbul).Format("2006-01-02"),
			"reason":          in.CloseReason,
			// variant picks the text: locked or filed.
			"variant": in.CloseReason,
		}}}
	}
	return []engine.Outcome{o}, nil
}

// istanbul is Türkiye's time (UTC+3 all year since 2016); fixed so the
// check does not depend on the host's tz database.
var istanbul = time.FixedZone("Europe/Istanbul", 3*60*60)
