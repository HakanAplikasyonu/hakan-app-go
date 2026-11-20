package posting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Outcome of one invoice.
const (
	OutcomeCreated  = "created"
	OutcomeExisting = "existing"
	OutcomeRefused  = "refused"
)

// ErrNotFound: the invoice is not visible to the engagement.
var ErrNotFound = errors.New("posting: invoice not found")

// DraftRef points at the invoice's journal draft.
type DraftRef struct {
	UUID    uuid.UUID `json:"uuid"`
	EntryNo int32     `json:"entry_no"`
	Period  string    `json:"period"`
	Status  string    `json:"status"`
}

// Result is what happened to one invoice.
type Result struct {
	InvoiceUUID   uuid.UUID `json:"invoice_uuid"`
	InvoiceNumber string    `json:"invoice_number"`
	Outcome       string    `json:"outcome"`
	Draft         *DraftRef `json:"draft,omitempty"`
	// Reason, LineNo, Detail: why it was refused (Refusal).
	Reason string `json:"reason,omitempty"`
	LineNo int    `json:"line_no,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Service books invoices as journal drafts.
type Service struct {
	q        *db.Queries
	journals *journals.Service
}

// NewService builds the service.
func NewService(q *db.Queries, j *journals.Service) *Service {
	return &Service{q: q, journals: j}
}

// Link returns the invoice's draft, nil when it has none.
func (s *Service) Link(ctx context.Context, sc engagementctx.Scope, invoiceID int64) (*DraftRef, error) {
	l, err := s.q.GetInvoiceJournalLink(ctx, db.GetInvoiceJournalLinkParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, InvoiceID: invoiceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &DraftRef{UUID: l.Uuid, EntryNo: l.EntryNo, Period: l.Period, Status: l.Status}, nil
}

// Invoice loads an invoice visible to the engagement.
func (s *Service) Invoice(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.Invoice, error) {
	inv, err := s.q.GetEngagementInvoice(ctx, db.GetEngagementInvoiceParams{TaxpayerID: sc.TaxpayerID, Uuid: id, EngagementID: sc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Invoice{}, ErrNotFound
	}
	return inv, err
}

// Post books one invoice: the existing draft when it has one, else a new
// draft, else the reason it cannot be booked.
func (s *Service) Post(ctx context.Context, sc engagementctx.Scope, actorID int64, inv db.Invoice) (Result, error) {
	res := Result{InvoiceUUID: inv.Uuid, InvoiceNumber: inv.InvoiceNumber}
	if ref, err := s.Link(ctx, sc, inv.ID); err != nil || ref != nil {
		res.Outcome, res.Draft = OutcomeExisting, ref
		return res, err
	}
	in, err := s.input(ctx, sc, inv)
	if err != nil {
		return res, err
	}
	entry, err := Build(in)
	var refusal *Refusal
	if errors.As(err, &refusal) {
		res.Outcome, res.Reason, res.LineNo, res.Detail = OutcomeRefused, refusal.Code, refusal.LineNo, refusal.Detail
		return res, nil
	}
	if err != nil {
		return res, err
	}
	d, err := s.journals.CreateDraft(ctx, sc, actorID, draftInput(inv, entry))
	if errors.Is(err, journals.ErrInvoicePosted) {
		// A concurrent request booked it first.
		ref, lerr := s.Link(ctx, sc, inv.ID)
		res.Outcome, res.Draft = OutcomeExisting, ref
		return res, lerr
	}
	if err != nil {
		return res, err
	}
	res.Outcome = OutcomeCreated
	res.Draft = &DraftRef{UUID: d.UUID, EntryNo: d.EntryNo, Period: d.Period, Status: d.Status}
	return res, nil
}

// PostPeriod books every unbooked active invoice of the entry period: sales
// of the document month, purchases of their KDV deduction month (#640).
func (s *Service) PostPeriod(ctx context.Context, sc engagementctx.Scope, actorID int64, period string) ([]Result, error) {
	rows, err := s.q.ListUnpostedInvoices(ctx, db.ListUnpostedInvoicesParams{
		TaxpayerID: sc.TaxpayerID, EngagementID: sc.ID, OrganizationID: sc.OrganizationID, Period: period,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(rows))
	for _, r := range rows {
		inv, err := s.Invoice(ctx, sc, r.Uuid)
		if err != nil {
			return out, err
		}
		res, err := s.Post(ctx, sc, actorID, inv)
		if err != nil {
			return out, fmt.Errorf("posting: invoice %s: %w", inv.InvoiceNumber, err)
		}
		out = append(out, res)
	}
	return out, nil
}

// draftInput maps the entry to a journal draft linked to the invoice.
func draftInput(inv db.Invoice, e Entry) journals.DraftInput {
	in := journals.DraftInput{
		Type: accountingexport.EntryMahsup, Date: e.Date, Description: e.Description, DocumentType: e.DocumentType,
		Origin:    map[string]any{"source": "invoice", "invoice_uuid": inv.Uuid.String(), "invoice_number": inv.InvoiceNumber},
		InvoiceID: inv.ID,
	}
	for _, l := range e.Lines {
		dd := e.DocumentDate
		in.Lines = append(in.Lines, journals.LineInput{
			Account: l.Account, Description: l.Description, Debit: l.Debit, Credit: l.Credit,
			DocumentNo: e.DocumentNo, DocumentDate: &dd, SourceKind: SourceKind, SourceID: e.SourceID,
		})
	}
	return in
}

// input loads what Build needs.
func (s *Service) input(ctx context.Context, sc engagementctx.Scope, inv db.Invoice) (Input, error) {
	in := Input{Invoice: header(inv)}
	rows, err := s.q.ListInvoiceItemsForPosting(ctx, db.ListInvoiceItemsForPostingParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, InvoiceID: inv.ID,
	})
	if err != nil {
		return in, err
	}
	for _, r := range rows {
		in.Items = append(in.Items, item(r))
	}
	purchase := inv.Direction == string(model.DirectionIncoming)
	cariMain, vatMain := "120", "391"
	if purchase {
		cariMain, vatMain = "320", "191"
	}
	if in.Plan, err = s.plan(ctx, sc, cariMain, vatMain); err != nil {
		return in, err
	}
	in.Cari, err = s.cari(ctx, sc, inv, cariMain, in.Plan)
	return in, err
}

// cari: the approved customer account, else the counterparty memory, else
// the plan's account with the counterparty's name; "" = main account.
func (s *Service) cari(ctx context.Context, sc engagementctx.Scope, inv db.Invoice, main string, plan []engine.PlanAccount) (string, error) {
	if inv.CustomerID.Valid {
		code, err := s.q.GetInvoiceCustomerAccount(ctx, db.GetInvoiceCustomerAccountParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CustomerID: inv.CustomerID.Int64,
		})
		if err != nil {
			return "", err
		}
		if code != "" {
			return code, nil
		}
	}
	taxID := inv.CustomerTaxNumber.String
	if taxID == "" {
		taxID = inv.CustomerIdentityNumber.String
	}
	if taxID != "" {
		c, err := s.q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CounterpartyTaxID: taxID, MainCode: main,
		})
		if err == nil {
			return c.AccountCode, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if inv.CustomerName.String != "" {
		if pr := engine.ProposeCounterparty(main, inv.CustomerName.String, plan); pr.Existing {
			return pr.AccountCode, nil
		}
	}
	return "", nil
}

// plan loads the taxpayer's active plan accounts under the given main codes.
func (s *Service) plan(ctx context.Context, sc engagementctx.Scope, mains ...string) ([]engine.PlanAccount, error) {
	target, err := s.q.ActiveAccountPlanTarget(ctx, db.ActiveAccountPlanTargetParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListPlanAccountsForMains(ctx, db.ListPlanAccountsForMainsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Target: target, Mains: mains,
	})
	if err != nil {
		return nil, err
	}
	out := make([]engine.PlanAccount, 0, len(rows))
	for _, r := range rows {
		out = append(out, engine.PlanAccount{Code: r.Code, Name: r.Name, IsLeaf: r.IsLeaf})
	}
	return out, nil
}

func header(inv db.Invoice) Invoice {
	var issue time.Time
	if inv.IssueDate.Valid {
		issue = inv.IssueDate.Time
	}
	return Invoice{
		ID: inv.ID, Number: inv.InvoiceNumber, Direction: inv.Direction, Type: inv.Type, Currency: inv.Currency,
		Status: inv.StatusNormalized, IssueDate: issue, Counterpart: inv.CustomerName.String,
		VATDeductionPeriod: inv.VatDeductionPeriod.String,
		TaxExclusive:       repository.Dec(inv.TaxExclusiveAmount), Tax: repository.Dec(inv.TaxAmount),
		Payable:     payable(inv),
		Withholding: repository.Dec(inv.WithholdingAmount), Excise: repository.Dec(inv.ExciseAmount),
		Communications: repository.Dec(inv.CommunicationsTaxAmount), IncomeTax: repository.Dec(inv.IncomeTaxWithholdingAmount),
		GlobalExemptionCode: inv.GlobalTaxExemptionCode.String,
	}
}

// payable: the amount the cari owes. With no withholding (Build refuses
// withholding) it is the tax inclusive amount; payable_amount also nets
// rounding and prepaid amounts, which Build would then flag as a mismatch.
func payable(inv db.Invoice) decimal.Decimal {
	if inv.TaxInclusiveAmount.Valid {
		return repository.Dec(inv.TaxInclusiveAmount)
	}
	return repository.Dec(inv.PayableAmount)
}

func item(r db.ListInvoiceItemsForPostingRow) Item {
	it := Item{
		LineNo: int(r.LineNo), Name: r.Name,
		Net: repository.Dec(r.TaxExclusiveAmount), VAT: vatOf(r),
		Account: r.AccountCode, VATAccount: r.VatAccountCode,
		Withholding: repository.Dec(r.WithholdingAmount), Excise: repository.Dec(r.ExciseAmount),
		Communications: repository.Dec(r.CommunicationsTaxAmount), IncomeTax: repository.Dec(r.IncomeTaxWithholdingAmount),
		ExemptionCode: r.TaxExemptionCode.String,
	}
	if r.TaxRate.Valid {
		rate := repository.Dec(r.TaxRate)
		it.Rate = &rate
	}
	if !r.TaxExclusiveAmount.Valid {
		it.Net = repository.Dec(r.LineExtensionAmount)
	}
	for _, t := range breakdowns(r.TaxBreakdowns) {
		amt, _ := decimal.NewFromString(t.TaxAmount)
		it.Taxes = append(it.Taxes, Tax{Kind: t.Kind, TypeCode: t.TaxTypeCode, Amount: amt, ExemptionCode: t.ExemptionCode})
	}
	return it
}

// vatOf: the line's KDV. total_tax_amount also carries other taxes, which
// Build refuses separately, so it is the KDV whenever Build accepts.
func vatOf(r db.ListInvoiceItemsForPostingRow) decimal.Decimal {
	return repository.Dec(r.TotalTaxAmount)
}

func breakdowns(raw []byte) []model.TaxBreakdown {
	if len(raw) == 0 {
		return nil
	}
	var out []model.TaxBreakdown
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
