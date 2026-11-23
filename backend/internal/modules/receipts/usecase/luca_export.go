package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/luca"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
)

// NoBreakdownNote marks the single VAT line of an expense without a VAT
// breakdown in the Luca file (#780): the VAT is not split by a guessed rate.
const NoBreakdownNote = "(KDV kırılımı yok, oranı kontrol edin)"

// ErrNothingToExport: the period has no expense with a date, an amount and
// an account.
var ErrNothingToExport = errors.New("receipts: no expense to export in this period")

// LucaFile is the Luca fiş file of a period's expenses (#780).
type LucaFile struct {
	Name        string
	ContentType string
	Data        []byte
	// Exported expenses; NoBreakdown of them went out with one VAT line;
	// Skipped had no date, amount or account; AlreadyExported were left out
	// because they already went to Luca (not re-included).
	Exported, NoBreakdown, Skipped, AlreadyExported int
	// Export names this export in expense_export_documents.
	Export uuid.UUID
	// entryFiles: expense uuid → file it went into.
	entryFiles map[string]string
}

// LucaExportInput is one "Luca'ya aktar" request (#780).
type LucaExportInput struct {
	Period string `json:"period"`
	// ConfirmDocuments are document keys ("exp:<uuid>") of expenses that
	// already went to Luca and go again in this export ("yeniden dahil
	// et"), like the journal export's confirm_documents.
	ConfirmDocuments []string `json:"confirm_documents,omitempty"`
	// IncludeExported re-includes every already exported expense of the
	// period in this export.
	IncludeExported bool `json:"include_exported,omitempty"`
}

// lucaExpense is one expense ready for the Luca fiş: accounts resolved.
type lucaExpense struct {
	uuid           uuid.UUID
	ID             string // expense uuid
	Date           time.Time
	Total, Tax     decimal.Decimal
	Deductible     bool
	ExpenseAccount string
	VATAccount     string
	PaymentAccount string
	VendorName     string
	VendorTaxID    string
	ReceiptNumber  string
	CategoryName   string
	VatLines       []VatLine
}

// lucaEntries turns expenses into Luca vouchers with the same lines as the
// approval's journal draft (posting.ExpenseEntry): expense net on debit,
// one 191 line per VAT rate, the payment account on credit. An expense
// without a breakdown keeps one 191 line marked NoBreakdownNote (never split
// by a guessed rate). Fiş No runs 1…n in date order.
//
// An expense whose lines cannot be built (VAT over the total, unbalanced)
// is left out and counted in skipped.
func lucaEntries(xs []lucaExpense, plan []engine.PlanAccount) (out []ae.Entry, noBreakdown, skipped int) {
	sort.SliceStable(xs, func(i, j int) bool {
		if !xs[i].Date.Equal(xs[j].Date) {
			return xs[i].Date.Before(xs[j].Date)
		}
		return xs[i].ReceiptNumber < xs[j].ReceiptNumber
	})
	out = make([]ae.Entry, 0, len(xs))
	for _, x := range xs {
		in := posting.ExpenseInput{
			SourceID: "expense:" + x.ID, Date: x.Date, Total: x.Total, Tax: x.Tax, Deductible: x.Deductible,
			ExpenseAccount: x.ExpenseAccount, VATAccount: x.VATAccount, PaymentAccount: x.PaymentAccount,
			VendorName: x.VendorName, ReceiptNumber: x.ReceiptNumber, CategoryName: x.CategoryName, Plan: plan,
		}
		in.VatLines = bookableRates(x.VatLines, x.Tax)
		if len(in.VatLines) == 0 {
			in.NoBreakdownNote = NoBreakdownNote
		}
		e, err := posting.ExpenseEntry(in)
		if err != nil || !e.Balanced() {
			skipped++
			continue
		}
		if len(in.VatLines) == 0 {
			noBreakdown++
		}
		d := x.Date
		entry := ae.Entry{ID: x.ID, No: strconv.Itoa(len(out) + 1), Date: x.Date, Type: ae.EntryMahsup,
			Description: e.Description, DocumentType: e.DocumentType}
		for _, l := range e.Lines {
			entry.Lines = append(entry.Lines, ae.Line{Account: l.Account, Description: l.Description, Debit: l.Debit,
				Credit: l.Credit, DocNo: x.ReceiptNumber, DocDate: &d, TaxID: x.VendorTaxID,
				Source: ae.SourceRef{Kind: posting.SourceKind, ID: l.SourceID}})
		}
		out = append(out, entry)
	}
	return out, noBreakdown, skipped
}

// lucaFile writes the vouchers with the Luca driver's default (verified)
// fiş template — the same header row as the journal export. More than one
// file (Luca: at most 50 vouchers per upload) comes as a zip.
func lucaFile(period string, entries []ae.Entry) (LucaFile, error) {
	drv, err := ae.Open(luca.Name, ae.Config{})
	if err != nil {
		return LucaFile{}, err
	}
	mw := ae.NewMemoryWriter()
	res, err := drv.Export(context.Background(), ae.Batch{Period: period, Entries: entries}, mw)
	if err != nil {
		return LucaFile{}, err
	}
	if ae.HasBlocker(res.Issues) {
		return LucaFile{}, fmt.Errorf("%w: the Luca file cannot be written (%s)", ErrInvalid, res.Issues[0].Code)
	}
	files := map[string]string{}
	for id, f := range res.EntryFiles {
		files[id] = "masraf_" + f
	}
	if len(res.Files) == 1 {
		data, _ := mw.File(res.Files[0].Name)
		return LucaFile{Name: "masraf_" + res.Files[0].Name, ContentType: luca.ContentType, Data: data, entryFiles: files}, nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range res.Files {
		data, _ := mw.File(f.Name)
		w, err := zw.Create("masraf_" + f.Name)
		if err != nil {
			return LucaFile{}, err
		}
		if _, err := w.Write(data); err != nil {
			return LucaFile{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return LucaFile{}, err
	}
	name := "masraf_luca_fis"
	if period != "" {
		name += "_" + period
	}
	return LucaFile{Name: name + ".zip", ContentType: "application/zip", Data: buf.Bytes(), entryFiles: files}, nil
}

// ExportExpensesLuca writes the period's expenses (pending and approved,
// not rejected) as a Luca fiş file (#780). The account is the office's
// choice, else the suggestion shown on the screen; the VAT goes to the
// plan's 191 sub-account of each rate. İşletme defteri / serbest meslek has
// no account codes: ErrInvalid.
func (s *Service) ExportExpensesLuca(ctx context.Context, sc engagementctx.Scope, userID int64, in LucaExportInput) (LucaFile, error) {
	period := strings.TrimSpace(in.Period)
	if !periodq.Valid(period) {
		return LucaFile{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	single, err := singleEntry(ctx, s.q, sc)
	if err != nil {
		return LucaFile{}, err
	}
	if single {
		return LucaFile{}, fmt.Errorf("%w: this ledger has no account codes", ErrInvalid)
	}
	var items []ExpenseView
	for _, st := range []string{"approved", "pending"} {
		page, err := s.Expenses(ctx, sc, ExpenseFilter{Period: period, Status: st, Limit: exportLimit})
		if err != nil {
			return LucaFile{}, err
		}
		items = append(items, page.Items...)
	}
	sg, err := s.suggester(ctx, sc)
	if err != nil {
		return LucaFile{}, err
	}
	var xs []lucaExpense
	skipped := 0
	for _, v := range items {
		e, err := s.q.GetExpenseByUUID(ctx, db.GetExpenseByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: v.UUID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return LucaFile{}, err
		}
		if !e.ExpenseDate.Valid || !dec(e.Amount).IsPositive() || v.AccountCode == nil || *v.AccountCode == "" {
			skipped++
			continue
		}
		cat, _ := posting.Find(sg.cats, deref(v.CategoryCode))
		pay, err := s.paymentAccount(ctx, s.q, sc, e, sg.plan)
		if err != nil {
			return LucaFile{}, err
		}
		xs = append(xs, lucaExpense{
			uuid: e.Uuid, ID: e.Uuid.String(), Date: e.ExpenseDate.Time, Total: dec(e.Amount), Tax: dec(e.TaxAmount),
			Deductible: e.IsDeductible != "no" && cat.Deductible != "no", ExpenseAccount: *v.AccountCode,
			VATAccount: cat.VATAccountCode, PaymentAccount: pay, VendorName: e.VendorName.String,
			VendorTaxID: e.VendorTaxNumber.String, ReceiptNumber: e.ReceiptNumber.String, CategoryName: cat.Name,
			VatLines: v.VatLines,
		})
	}
	// Çift aktarım koruması (#780, faturalardaki #466 gibi): daha önce
	// Luca'ya gitmiş masraf, bu çıktı için yeniden dahil edilmedikçe girmez.
	keys := make([]string, 0, len(xs))
	for _, x := range xs {
		keys = append(keys, x.key())
	}
	prior, err := s.liveExports(ctx, sc, keys)
	if err != nil {
		return LucaFile{}, err
	}
	xs, confirmed, already := guardExpenses(xs, prior, in.ConfirmDocuments, in.IncludeExported)
	entries, noBreakdown, bad := lucaEntries(xs, sg.plan)
	if len(entries) == 0 {
		return LucaFile{}, ErrNothingToExport
	}
	f, err := lucaFile(period, entries)
	if err != nil {
		return LucaFile{}, err
	}
	f.Exported, f.NoBreakdown, f.Skipped, f.AlreadyExported = len(entries), noBreakdown, skipped+bad, already
	f.Export = uuid.New()
	byID := map[string]lucaExpense{}
	for _, x := range xs {
		byID[x.ID] = x
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return LucaFile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	for _, e := range entries {
		x := byID[e.ID]
		codes := []string{}
		if confirmed[x.key()] {
			codes = []string{ae.IssueDocumentAlreadyExported}
		}
		file := f.entryFiles[e.ID]
		if file == "" {
			file = f.Name
		}
		if err := q.InsertExpenseExportDocument(ctx, db.InsertExpenseExportDocumentParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ExportUuid: f.Export, Driver: luca.Name, Period: period,
			FileName: file, DocumentKey: x.key(), ExpenseUuid: x.uuid, DocumentNo: text(x.ReceiptNumber),
			DocumentDate: pgtype.Date{Time: x.Date, Valid: true}, ConfirmedCodes: codes, ExportedBy: optID(userID),
		}); err != nil {
			return LucaFile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return LucaFile{}, err
	}
	return f, nil
}

// bookableRates are the breakdown's rates when their VAT adds up to the
// expense's VAT (a photo receipt may leave an unread rate out: then the
// expense is booked as one VAT line, like one without a breakdown).
func bookableRates(lines []VatLine, tax decimal.Decimal) []posting.VatRate {
	sum := decimal.Zero
	out := make([]posting.VatRate, 0, len(lines))
	for _, l := range lines {
		sum = sum.Add(l.Tax)
		out = append(out, posting.VatRate{Rate: l.Rate, Tax: l.Tax})
	}
	if len(out) == 0 || sum.Sub(tax).Abs().GreaterThan(decimal.New(1, -2)) {
		return nil
	}
	return out
}

func (x lucaExpense) key() string { return journals.ExpenseDocumentKey(x.uuid) }

// guardExpenses leaves out the expenses already exported (prior holds their
// document keys) unless this export re-includes them (confirm or all). It
// returns the expenses that go, the re-included keys and how many stayed
// out.
func guardExpenses(xs []lucaExpense, prior map[string]ExportMark, confirm []string, all bool) ([]lucaExpense, map[string]bool, int) {
	ok := map[string]bool{}
	for _, c := range confirm {
		ok[strings.ToLower(strings.TrimSpace(c))] = true
	}
	out := xs[:0:0]
	confirmed := map[string]bool{}
	left := 0
	for _, x := range xs {
		k := x.key()
		if _, done := prior[k]; done {
			if !all && !ok[k] {
				left++
				continue
			}
			confirmed[k] = true
		}
		out = append(out, x)
	}
	return out, confirmed, left
}

// ExportMark is the latest live export of an expense to Luca (#780): the
// journal export or the Masraflar screen's Luca file.
type ExportMark struct {
	At   time.Time `json:"exported_at"`
	File string    `json:"file_name"`
}

// liveExports returns the latest live Luca export per document key.
func (s *Service) liveExports(ctx context.Context, sc engagementctx.Scope, keys []string) (map[string]ExportMark, error) {
	out := map[string]ExportMark{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := s.q.ListLiveDocumentExports(ctx, db.ListLiveDocumentExportsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Driver: luca.Name, DocumentKeys: keys,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows { // oldest first: the latest wins
		out[r.DocumentKey] = ExportMark{At: r.CreatedAt.Time, File: r.FileName}
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
