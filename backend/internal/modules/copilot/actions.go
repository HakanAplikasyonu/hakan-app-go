// Package copilot is the accountant's co-pilot (issue #392): the "next
// work" flow that walks every engagement through the period close (documents
// → bank matching → review → journal drafts → transfer to the accounting
// program → report to the taxpayer) and the command bar that turns a
// natural language command into an action plan.
//
// The co-pilot never writes by itself. Every step of a plan is a call to an
// existing API endpoint, run by the client only after the user approves it,
// with the user's own permissions; the server checks the plan against the
// action catalog, the engagements the user may reach and the permissions the
// user holds, and keeps the outcome of each step.
package copilot

import "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"

// Actions. Write actions are steps of a plan and need the user's approval;
// open_* actions only take the user to a screen.
const (
	ActionBankSuggest     = "bank_suggest"
	ActionBankApprove     = "bank_approve"
	ActionBankDrafts      = "bank_drafts"
	ActionJournalsApprove = "journals_approve"
	ActionExportTransfer  = "export_transfer"
	ActionReportSend      = "report_send"
	ActionOpenDocuments   = "open_documents"
	ActionOpenReview      = "open_review"
	ActionOpenJournals    = "open_journals"
	// Ay sonu kapanış ekranları (#780).
	ActionOpenExpenses       = "open_expenses"
	ActionOpenZReports       = "open_z_reports"
	ActionOpenExport         = "open_export"
	ActionOpenPayroll        = "open_payroll"
	ActionOpenTrialBalance   = "open_trial_balance"
	ActionOpenReconciliation = "open_reconciliation"
	ActionOpenVAT            = "open_vat"
	// ActionOpenBank: the bank screen of a single-entry book, to load or
	// check the period's statement (no account suggestion there).
	ActionOpenBank = "open_bank"
)

// ActionSpec describes one action of the catalog.
type ActionSpec struct {
	Name string
	// Write: the action changes data (a plan step, needs approval).
	Write bool
	// Permissions are all required (tenant.* slugs) on top of engagement
	// read access.
	Permissions []string
	// Endpoint documents the existing API call(s) the client makes.
	Endpoint string
	// Description is what the planner model reads.
	Description string
}

// Catalog is every action the co-pilot knows, in workflow order.
var Catalog = []ActionSpec{
	{Name: ActionBankSuggest, Write: true, Permissions: []string{rbac.PermTenantBankWrite},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/bank-postings/suggest {period}",
		Description: "Propose the counter account of the period's bank movements that have no suggestion yet."},
	{Name: ActionBankApprove, Write: true, Permissions: []string{rbac.PermTenantBankWrite},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/bank-postings/approve {transaction_uuids}",
		Description: "Approve the pending counter-account suggestions of the period's bank movements as proposed."},
	{Name: ActionBankDrafts, Write: true, Permissions: []string{rbac.PermTenantBankWrite, rbac.PermTenantJournalsWrite},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/bank-postings/drafts {period}",
		Description: "Create journal drafts (mahsup fişleri) from the period's approved bank movements."},
	{Name: ActionJournalsApprove, Write: true, Permissions: []string{rbac.PermTenantJournalsApprove},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/approve (each draft of the period)",
		Description: "Approve the period's journal drafts so they can be exported."},
	{Name: ActionExportTransfer, Write: true, Permissions: []string{rbac.PermTenantJournalsExport},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/journal-exports {driver, period} then POST .../journal-exports/{batchUuid}/transfers",
		Description: "Export the period's approved journal entries to Luca or Zirve and send the file to the program through the desktop agent."},
	{Name: ActionReportSend, Write: true, Permissions: []string{rbac.PermTenantEngagementsWrite},
		Endpoint:    "POST /v1/tenant/engagements/{engagementUuid}/report/deliveries {period}",
		Description: "Email the taxpayer's monthly report for the period."},
	{Name: ActionOpenDocuments, Endpoint: "screen: engagement documents",
		Description: "Open the taxpayer's missing documents."},
	{Name: ActionOpenReview, Endpoint: "screen: review queue",
		Description: "Open the taxpayer's pending review suggestions."},
	{Name: ActionOpenJournals, Endpoint: "screen: journals",
		Description: "Open the taxpayer's journal drafts and transfers."},
	{Name: ActionOpenExpenses, Endpoint: "screen: engagement expenses",
		Description: "Open the taxpayer's expense receipts waiting for approval."},
	{Name: ActionOpenZReports, Endpoint: "screen: engagement Z reports",
		Description: "Open the taxpayer's Z reports waiting for approval."},
	{Name: ActionOpenExport, Endpoint: "screen: journals export",
		Description: "Open the taxpayer's downloaded exports to mark them uploaded to Luca."},
	{Name: ActionOpenPayroll, Endpoint: "screen: engagement payroll",
		Description: "Open the taxpayer's payroll to upload the Luca payroll export."},
	{Name: ActionOpenTrialBalance, Endpoint: "screen: engagement trial balance",
		Description: "Open the taxpayer's trial balance to upload the Luca mizan."},
	{Name: ActionOpenReconciliation, Endpoint: "screen: engagement reconciliation",
		Description: "Open the trial balance comparison that shows differences."},
	{Name: ActionOpenVAT, Endpoint: "screen: engagement report (VAT notice)",
		Description: "Open the taxpayer's report to send the period's VAT notice."},
	{Name: ActionOpenBank, Endpoint: "screen: engagement bank",
		Description: "Open the taxpayer's bank screen to load or check the period's statement."},
}

// Spec returns the catalog entry of an action.
func Spec(name string) (ActionSpec, bool) {
	for _, a := range Catalog {
		if a.Name == name {
			return a, true
		}
	}
	return ActionSpec{}, false
}

// WriteActions returns the names of the plan-step actions.
func WriteActions() []string {
	var out []string
	for _, a := range Catalog {
		if a.Write {
			out = append(out, a.Name)
		}
	}
	return out
}

// Allowed reports whether a holder of perms may run the action.
func (a ActionSpec) Allowed(has func(string) bool) bool {
	for _, p := range a.Permissions {
		if !has(p) {
			return false
		}
	}
	return true
}
