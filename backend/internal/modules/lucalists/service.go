package lucalists

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/luca"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	invoiceposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	receiptposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
)

// Input is one "Luca'ya aktar (Excel)" request.
type Input struct {
	Period string `json:"period"`
	// ConfirmDocuments are document keys of documents that already went to
	// Luca and go again in this export ("yeniden dahil et").
	ConfirmDocuments []string `json:"confirm_documents,omitempty"`
	// IncludeExported re-includes every already exported document of the
	// period in this export.
	IncludeExported bool `json:"include_exported,omitempty"`
}

// Result is the file and what went into it.
type Result struct {
	File
	Report Report
	// Export names this export in expense_export_documents.
	Export uuid.UUID
}

// Service builds the list files.
type Service struct {
	pool database.Pool
	q    *db.Queries
}

// New builds the service.
func New(pool database.Pool) *Service {
	return &Service{pool: pool, q: database.QueriesFor(pool)}
}

// Export writes the period's documents of list as a Luca fiş file and
// records them for the double export guard. İşletme defteri / serbest
// meslek has no account codes: ErrInvalid. No document that can go:
// ErrNothingToExport.
func (s *Service) Export(ctx context.Context, sc engagementctx.Scope, userID int64, list List, in Input) (Result, error) {
	if !list.Valid() {
		return Result{}, fmt.Errorf("%w: unknown list", ErrInvalid)
	}
	period := strings.TrimSpace(in.Period)
	if !periodq.Valid(period) {
		return Result{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	t, err := ledger.Of(ctx, s.q, sc.OrganizationID, sc.ID)
	if err != nil {
		return Result{}, err
	}
	if ledger.SingleEntry(t) {
		return Result{}, fmt.Errorf("%w: this ledger has no account codes", ErrInvalid)
	}
	docs, skipped, err := s.docs(ctx, sc, list, period)
	if err != nil {
		return Result{}, err
	}
	// One fiş per document: two rows with the same key (the same ETTN
	// loaded twice) go once.
	keys := make([]string, 0, len(docs))
	seen := map[string]bool{}
	uniq := docs[:0:0]
	for _, d := range docs {
		if seen[d.Key] {
			continue
		}
		seen[d.Key] = true
		uniq = append(uniq, d)
		keys = append(keys, d.Key)
	}
	docs = uniq
	prior, err := s.prior(ctx, sc, keys)
	if err != nil {
		return Result{}, err
	}
	docs, confirmed, left := Guard(docs, prior, in.ConfirmDocuments, in.IncludeExported)
	entries, kept, check, bad := Entries(docs)
	rep := Report{Check: nonNil(check), Skipped: nonNil(append(skipped, bad...)), AlreadyExported: []Row{}}
	for _, d := range left {
		rep.AlreadyExported = append(rep.AlreadyExported, rowOf(d, ae.IssueDocumentAlreadyExported, ""))
	}
	if len(entries) == 0 {
		return Result{Report: rep}, ErrNothingToExport
	}
	f, err := WriteFile(list, period, entries)
	if err != nil {
		return Result{}, err
	}
	rep.Exported = len(entries)
	res := Result{File: f, Report: rep, Export: uuid.New()}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	for _, d := range kept {
		codes := []string{}
		if confirmed[d.Key] {
			codes = []string{ae.IssueDocumentAlreadyExported}
		}
		file := f.EntryFiles[d.Key]
		if file == "" {
			file = f.Name
		}
		date := pgtype.Date{}
		if !d.DocDate.IsZero() {
			date = pgtype.Date{Time: d.DocDate, Valid: true}
		}
		if err := q.InsertLucaListExportDocument(ctx, db.InsertLucaListExportDocumentParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ExportUuid: res.Export, Driver: luca.Name, Period: period,
			FileName: file, DocumentKey: d.Key, SourceKind: list.SourceKind(), SourceUuid: d.UUID,
			DocumentNo: pgtype.Text{String: d.No, Valid: d.No != ""}, DocumentDate: date,
			ConfirmedCodes: codes, ExportedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
		}); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

// prior returns the keys already exported to Luca in a live export: the
// journal export (export_batch_documents) or a list / expense file
// (expense_export_documents).
func (s *Service) prior(ctx context.Context, sc engagementctx.Scope, keys []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := s.q.ListLiveDocumentExports(ctx, db.ListLiveDocumentExportsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Driver: luca.Name, DocumentKeys: keys,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.DocumentKey] = true
	}
	return out, nil
}

// docs loads the period's documents of list; the ones that cannot become a
// fiş come back as skipped rows.
func (s *Service) docs(ctx context.Context, sc engagementctx.Scope, list List, period string) ([]Doc, []Row, error) {
	switch list {
	case IncomingInvoices:
		return s.invoiceDocs(ctx, sc, "incoming", period)
	case OutgoingInvoices:
		return s.invoiceDocs(ctx, sc, "outgoing", period)
	case BankTransactions:
		return s.bankDocs(ctx, sc, period)
	default:
		return s.zDocs(ctx, sc, period)
	}
}

func (s *Service) invoiceDocs(ctx context.Context, sc engagementctx.Scope, direction, period string) ([]Doc, []Row, error) {
	ids, err := s.q.ListLucaListInvoices(ctx, db.ListLucaListInvoicesParams{
		TaxpayerID: sc.TaxpayerID, EngagementID: sc.ID, Direction: direction, Period: period,
	})
	if err != nil {
		return nil, nil, err
	}
	var docs []Doc
	var skipped []Row
	for _, id := range ids {
		inv, err := s.q.GetEngagementInvoice(ctx, db.GetEngagementInvoiceParams{TaxpayerID: sc.TaxpayerID, Uuid: id, EngagementID: sc.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		in, err := invoiceposting.InputFor(ctx, s.q, sc, inv)
		if err != nil {
			return nil, nil, err
		}
		rows, err := s.q.ListLucaListItemSuggestions(ctx, db.ListLucaListItemSuggestionsParams{
			TaxpayerID: sc.TaxpayerID, InvoiceID: inv.ID, OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		})
		if err != nil {
			return nil, nil, err
		}
		sug := map[int]ItemSuggestion{}
		for _, r := range rows {
			v := sug[int(r.LineNo)]
			if r.TargetField == "vat_account_code" {
				v.VATAccount = r.AccountCode
			} else {
				v.Account = r.AccountCode
			}
			sug[int(r.LineNo)] = v
		}
		var ettn *uuid.UUID
		if inv.Ettn.Valid {
			e := uuid.UUID(inv.Ettn.Bytes)
			ettn = &e
		}
		taxID := inv.CustomerTaxNumber.String
		if taxID == "" {
			taxID = inv.CustomerIdentityNumber.String
		}
		d, reason := InvoiceDoc(InvoiceKey(inv.Uuid, ettn), inv.Uuid, taxID, in, sug)
		if reason != "" {
			skipped = append(skipped, rowOf(d, reason, ""))
			continue
		}
		docs = append(docs, d)
	}
	return docs, skipped, nil
}

func (s *Service) bankDocs(ctx context.Context, sc engagementctx.Scope, period string) ([]Doc, []Row, error) {
	rows, err := s.q.ListLucaListBankMovements(ctx, db.ListLucaListBankMovementsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Period: period,
	})
	if err != nil {
		return nil, nil, err
	}
	var docs []Doc
	var skipped []Row
	for _, r := range rows {
		m := Movement{UUID: r.Uuid, ID: r.ID, In: banking.Direction(r.DebitCredit) == banking.DirIn,
			Amount: repository.Dec(r.Amount), Currency: r.Currency, Description: r.Description,
			Counterparty: r.CounterpartyName, TaxID: r.CounterpartyTaxID, Detail: r.KindDetail,
			Account: r.AccountCode, BankAccount: r.BankAccountCode,
			Commission: repository.Dec(r.CommissionAmount), CommissionAccount: r.CommissionAccountCode}
		if r.TransactionDate.Valid {
			m.Date = r.TransactionDate.Time
		}
		d, reason := BankDoc(m)
		if reason != "" {
			skipped = append(skipped, rowOf(d, reason, ""))
			continue
		}
		docs = append(docs, d)
	}
	return docs, skipped, nil
}

func (s *Service) zDocs(ctx context.Context, sc engagementctx.Scope, period string) ([]Doc, []Row, error) {
	rows, err := s.q.ListLucaListRetailSales(ctx, db.ListLucaListRetailSalesParams{TaxpayerID: sc.TaxpayerID, Period: period})
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	vat, err := s.q.ListRetailSaleVatLines(ctx, db.ListRetailSaleVatLinesParams{TaxpayerID: sc.TaxpayerID, Ids: ids})
	if err != nil {
		return nil, nil, err
	}
	lines := map[int64][]receiptposting.ZVatLine{}
	for _, v := range vat {
		lines[v.RetailSaleID] = append(lines[v.RetailSaleID], receiptposting.ZVatLine{
			Rate: repository.Dec(v.TaxRate), Base: repository.Dec(v.TaxBase), Tax: repository.Dec(v.TaxAmount)})
	}
	plan, err := s.plan(ctx, sc, "100", "108", "600", "391")
	if err != nil {
		return nil, nil, err
	}
	var docs []Doc
	var skipped []Row
	for _, r := range rows {
		z := ZReport{UUID: r.Uuid, ZNumber: r.ZReportNumber, OKC: r.OkcSerialNumber, Currency: r.Currency,
			Total: repository.Dec(r.TotalAmount), Tax: repository.Dec(r.TaxAmount),
			Cash: decPtr(r.CashAmount), Card: decPtr(r.CardAmount), Lines: lines[r.ID]}
		if r.SaleDate.Valid {
			z.Date = r.SaleDate.Time
		}
		d, reason := ZDoc(z, plan)
		if reason != "" {
			skipped = append(skipped, rowOf(d, reason, ""))
			continue
		}
		docs = append(docs, d)
	}
	return docs, skipped, nil
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

func decPtr(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	d := repository.Dec(n)
	return &d
}

func nonNil(rows []Row) []Row {
	if rows == nil {
		return []Row{}
	}
	return rows
}
