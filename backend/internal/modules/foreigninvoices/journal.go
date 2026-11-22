package foreigninvoices

// Journal draft of a confirmed foreign service invoice (#780 KDV2 B). The
// product does not file the return; it builds the entry and the entry goes
// to Luca / Zirve with the other entries (journals export).
//
// Lines (TL, with the currency details on the invoice lines):
//
//	gider hesabı (770 / 760)   borç   brüt TL (KDV2 matrahı)
//	191 indirilecek KDV        borç   KDV2 TL
//	360 ödenecek KDV2          alacak KDV2 TL
//	360 ödenecek stopaj        alacak stopaj TL (varsa)
//	320 satıcılar / 102 banka  alacak ödenen net TL
//
// Net paid: with gross-up the invoice amount is what the vendor gets
// (tutar × kur); without gross-up the vendor gets brüt − stopaj. With
// gross-up the TL gross comes from the gross rounded in the invoice
// currency (compute.go), so brüt TL − stopaj TL may differ from tutar × kur
// by a few kuruş (100 USD, kur 34,50, %15: 4.058,93 − 608,84 = 3.450,09 vs
// 3.450,00). That difference is a separate line to the rounding accounts
// (679 / 689 by default) so every tax amount stays as computed and the
// entry balances.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// SourceKind is journal_draft_lines.source_kind of the lines (000189).
const SourceKind = "foreign_invoice"

// DocumentType of the entry in the export: the purchase invoice pattern of
// invoices/posting.
const DocumentType = "invoice_purchase"

// Journal errors.
var (
	// ErrJournalExists: the invoice already has an entry.
	ErrJournalExists = errors.New("foreigninvoices: the invoice already has a journal entry")
	// ErrJournalLocked: the entry is approved or was exported; it cannot
	// be removed here.
	ErrJournalLocked = errors.New("foreigninvoices: the journal entry is approved or exported")
	// ErrNoJournal: the invoice has no entry.
	ErrNoJournal = errors.New("foreigninvoices: the invoice has no journal entry")
	// ErrJournalsOff: the journal service is not wired.
	ErrJournalsOff = errors.New("foreigninvoices: journal drafts are not configured")
)

// JournalCreator is the journals service subset.
type JournalCreator interface {
	CreateDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, in journals.DraftInput) (journals.Draft, error)
}

// WithJournals enables the journal drafts.
func (s *Service) WithJournals(j JournalCreator) *Service { s.journals = j; return s }

// Accounts are the office's accounts of the entry. Empty: the default.
type Accounts struct {
	KDV2            string `json:"kdv2_account"`
	Stopaj          string `json:"stopaj_account"`
	DeductibleVAT   string `json:"deductible_vat_account"`
	Counter         string `json:"counter_account"`
	RoundingIncome  string `json:"rounding_income_account"`
	RoundingExpense string `json:"rounding_expense_account"`
}

// DefaultAccounts (Tek Düzen Hesap Planı): 360 Ödenecek Vergi ve Fonlar
// (KDV2 and stopaj; a sub-account like 360.02 may be set), 191 İndirilecek
// KDV, 320 Satıcılar (or 102 Bankalar when paid at once), 679 / 689 Diğer
// Olağandışı Gelir / Gider for the kuruş difference.
var DefaultAccounts = Accounts{
	KDV2: "360", Stopaj: "360", DeductibleVAT: "191", Counter: "320",
	RoundingIncome: "679", RoundingExpense: "689",
}

func (a Accounts) withDefaults() Accounts {
	def := func(v, d string) string {
		if v = strings.TrimSpace(v); v == "" {
			return d
		}
		return v
	}
	d := DefaultAccounts
	return Accounts{
		KDV2: def(a.KDV2, d.KDV2), Stopaj: def(a.Stopaj, d.Stopaj), DeductibleVAT: def(a.DeductibleVAT, d.DeductibleVAT),
		Counter: def(a.Counter, d.Counter), RoundingIncome: def(a.RoundingIncome, d.RoundingIncome),
		RoundingExpense: def(a.RoundingExpense, d.RoundingExpense),
	}
}

// DefaultExpenseAccount of a service kind: advertising 760 Pazarlama Satış
// ve Dağıtım Giderleri, the others 770 Genel Yönetim Giderleri.
func DefaultExpenseAccount(kind string) string {
	if kind == KindAdvertising {
		return "760"
	}
	return "770"
}

// accountRe: a Tek Düzen code with optional sub-accounts (770, 770.01,
// 360.02.001, 320-ABC). Whether the taxpayer's plan has it is checked on the
// entry (journals plan gate): a code missing from the plan is a warning.
var accountRe = regexp.MustCompile(`^[0-9]{3}([.\-][0-9A-Za-z]{1,12}){0,5}$`)

// ValidAccount reports whether code is a well-formed account code.
func ValidAccount(code string) bool { return accountRe.MatchString(code) }

// JournalInvoice is what the entry needs from an invoice.
type JournalInvoice struct {
	UUID              uuid.UUID
	Period            string
	Vendor, InvoiceNo string
	InvoiceDate       *time.Time
	Currency          string
	Terms             Terms
	ServiceKindLabel  string
	ExpenseAccount    string
}

// JournalPlan is the built entry.
type JournalPlan struct {
	Date        time.Time
	Description string
	Lines       []journals.LineInput
	// Rounding: brüt TL − stopaj TL − net TL; positive goes to the
	// rounding income account (alacak), negative to the expense (borç).
	Rounding decimal.Decimal
}

// EntryDate is the invoice date, or the last day of the period without one.
func EntryDate(period string, invoiceDate *time.Time) time.Time {
	if invoiceDate != nil {
		return *invoiceDate
	}
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}
	}
	return t.AddDate(0, 1, -1)
}

// BuildJournal builds the entry of inv (pure). A missing kur is an error.
func BuildJournal(inv JournalInvoice, a Accounts) (JournalPlan, error) {
	a = a.withDefaults()
	res := Compute(inv.Terms)
	if res.MissingRate {
		return JournalPlan{}, fmt.Errorf("%w: kur girilmemiş", ErrInvalid)
	}
	gross, kdv2, stopaj := *res.GrossTRY, *res.KDV2TRY, *res.StopajTRY
	withStopaj := inv.Terms.Treatment == TreatmentKDV2Stopaj
	net := gross.Sub(stopaj)
	if withStopaj && inv.Terms.GrossUp {
		net = *res.AmountTRY
	}
	rounding := gross.Sub(stopaj).Sub(net)
	expense := strings.TrimSpace(inv.ExpenseAccount)
	if expense == "" {
		expense = "770"
	}
	ref := strings.TrimSpace(inv.Vendor)
	if inv.InvoiceNo != "" {
		ref += " " + inv.InvoiceNo
	}
	foreign := inv.Currency != "" && inv.Currency != "TRY"
	src := inv.UUID.String()
	plan := JournalPlan{
		Date:        EntryDate(inv.Period, inv.InvoiceDate),
		Description: "Yurt dışı hizmet faturası: " + ref,
		Rounding:    rounding,
	}
	add := func(acc, desc string, debit, credit decimal.Decimal, fx *decimal.Decimal) {
		if debit.IsZero() && credit.IsZero() {
			return
		}
		l := journals.LineInput{
			Account: acc, Description: desc, Debit: debit, Credit: credit,
			DocumentNo: inv.InvoiceNo, DocumentDate: inv.InvoiceDate, SourceKind: SourceKind, SourceID: src,
		}
		if foreign && fx != nil {
			rate, amt := *inv.Terms.Rate, *fx
			l.Currency, l.ExchangeRate, l.ForeignAmount = inv.Currency, &rate, &amt
		}
		plan.Lines = append(plan.Lines, l)
	}
	zero := decimal.Zero
	grossFX, kdvFX, stopajFX := res.Gross, res.KDV2, res.Stopaj
	netFX := res.Gross.Sub(res.Stopaj)
	if withStopaj && inv.Terms.GrossUp {
		netFX = inv.Terms.Amount
	}
	add(expense, ref+" yurt dışı hizmet gideri ("+firstNonEmpty(inv.ServiceKindLabel, "hizmet")+")", gross, zero, &grossFX)
	add(a.DeductibleVAT, ref+" sorumlu sıfatıyla KDV (indirilecek)", kdv2, zero, &kdvFX)
	add(a.KDV2, ref+" sorumlu sıfatıyla KDV (KDV2) ödenecek", zero, kdv2, &kdvFX)
	if withStopaj {
		add(a.Stopaj, ref+" stopaj ödenecek", zero, stopaj, &stopajFX)
	}
	add(a.Counter, ref+" ödenecek net tutar", zero, net, &netFX)
	switch {
	case rounding.IsPositive():
		add(a.RoundingIncome, ref+" kuruş yuvarlama farkı (brüt döviz tutarı yuvarlandı)", zero, rounding, nil)
	case rounding.IsNegative():
		add(a.RoundingExpense, ref+" kuruş yuvarlama farkı (brüt döviz tutarı yuvarlandı)", rounding.Neg(), zero, nil)
	}
	return plan, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// JournalRef is the entry of an invoice as the list shows it.
type JournalRef struct {
	UUID    uuid.UUID `json:"uuid"`
	EntryNo int32     `json:"entry_no"`
	Period  string    `json:"period"`
	// Status: draft, approved, exported (journal_drafts.status).
	Status string `json:"status"`
}

// JournalLine is one line of a created entry.
type JournalLine struct {
	Account     string `json:"account"`
	Description string `json:"description"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
}

// JournalResult is a created entry.
type JournalResult struct {
	InvoiceUUID uuid.UUID     `json:"invoice_uuid"`
	Journal     JournalRef    `json:"journal"`
	Lines       []JournalLine `json:"lines"`
	Debit       string        `json:"debit"`
	Credit      string        `json:"credit"`
	// Rounding: the kuruş difference booked on its own line ("0.00": none).
	Rounding string `json:"rounding"`
}

// JournalFailure is an invoice of the period whose entry was not created.
type JournalFailure struct {
	InvoiceUUID uuid.UUID `json:"invoice_uuid"`
	Vendor      string    `json:"vendor"`
	Message     string    `json:"message"`
}

// PeriodJournals is the result of the period's bulk run.
type PeriodJournals struct {
	Period  string           `json:"period"`
	Created []JournalResult  `json:"created"`
	Skipped int              `json:"skipped"`
	Failed  []JournalFailure `json:"failed"`
}

// accounts returns the office's entry accounts.
func (s *Service) accounts(ctx context.Context, orgID int64) (Accounts, error) {
	row, err := s.q.GetForeignInvoiceSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Accounts{}, nil
	}
	if err != nil {
		return Accounts{}, fmt.Errorf("foreign invoice settings: %w", err)
	}
	return Accounts{
		KDV2: row.Kdv2Account.String, Stopaj: row.StopajAccount.String, DeductibleVAT: row.DeductibleVatAccount.String,
		Counter: row.CounterAccount.String, RoundingIncome: row.RoundingIncomeAccount.String,
		RoundingExpense: row.RoundingExpenseAccount.String,
	}, nil
}

// journalOf returns the invoice's entry (nil when none).
func (s *Service) journalOf(ctx context.Context, sub Subject, invoiceID int64) (*db.GetForeignInvoiceJournalRow, error) {
	j, err := s.q.GetForeignInvoiceJournal(ctx, db.GetForeignInvoiceJournalParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ForeignInvoiceID: invoiceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("foreign invoice journal: %w", err)
	}
	return &j, nil
}

// CreateJournal builds the entry of a confirmed invoice. A draft invoice
// is ErrConflict; an invoice with an entry ErrJournalExists; a taxpayer
// with an income/expense book ledger.ErrNotBalanceSheet (from journals).
func (s *Service) CreateJournal(ctx context.Context, scope engagementctx.Scope, id uuid.UUID, userID int64) (JournalResult, error) {
	sub := Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}
	row, err := s.get(ctx, sub, id)
	if err != nil {
		return JournalResult{}, err
	}
	acc, kinds, err := s.journalSetup(ctx, scope.OrganizationID)
	if err != nil {
		return JournalResult{}, err
	}
	return s.createJournal(ctx, scope, row, acc, kinds, userID)
}

func (s *Service) journalSetup(ctx context.Context, orgID int64) (Accounts, map[string]KindSetting, error) {
	if s.journals == nil {
		return Accounts{}, nil, ErrJournalsOff
	}
	on, err := s.enabled(ctx, orgID)
	if err != nil {
		return Accounts{}, nil, err
	}
	if !on {
		return Accounts{}, nil, ErrDisabled
	}
	acc, err := s.accounts(ctx, orgID)
	if err != nil {
		return Accounts{}, nil, err
	}
	kinds, err := s.kindDefaults(ctx, orgID)
	if err != nil {
		return Accounts{}, nil, err
	}
	return acc, kinds, nil
}

func (s *Service) createJournal(ctx context.Context, scope engagementctx.Scope, row db.ForeignServiceInvoice, acc Accounts, kinds map[string]KindSetting, userID int64) (JournalResult, error) {
	sub := Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}
	if row.Status != StatusConfirmed {
		return JournalResult{}, fmt.Errorf("%w: önce faturayı onaylayın", ErrConflict)
	}
	existing, err := s.journalOf(ctx, sub, row.ID)
	if err != nil {
		return JournalResult{}, err
	}
	if existing != nil {
		return JournalResult{}, ErrJournalExists
	}
	inv := journalInvoiceOf(row)
	if k, ok := kinds[row.ServiceKind]; ok {
		inv.ExpenseAccount = k.ExpenseAccount
	}
	plan, err := BuildJournal(inv, acc)
	if err != nil {
		return JournalResult{}, err
	}
	d, err := s.journals.CreateDraft(ctx, scope, userID, journals.DraftInput{
		Date: plan.Date, Description: plan.Description, DocumentType: DocumentType,
		Origin: map[string]any{
			"source": SourceKind, "foreign_invoice_uuid": row.Uuid.String(),
			"rule": "yabancı hizmet: gider / 191 / 360 KDV2 / 360 stopaj / 320", "rounding": plan.Rounding.StringFixed(2),
		},
		Lines: plan.Lines, ForeignInvoiceID: row.ID,
	})
	if errors.Is(err, journals.ErrForeignInvoicePosted) {
		return JournalResult{}, ErrJournalExists
	}
	if err != nil {
		return JournalResult{}, err
	}
	out := JournalResult{
		InvoiceUUID: row.Uuid,
		Journal:     JournalRef{UUID: d.UUID, EntryNo: d.EntryNo, Period: d.Period, Status: d.Status},
		Debit:       d.Debit.StringFixed(2), Credit: d.Credit.StringFixed(2), Rounding: plan.Rounding.StringFixed(2),
	}
	for _, l := range plan.Lines {
		out.Lines = append(out.Lines, JournalLine{
			Account: l.Account, Description: l.Description, Debit: l.Debit.StringFixed(2), Credit: l.Credit.StringFixed(2),
		})
	}
	return out, nil
}

// CreatePeriodJournals builds the entries of the period's confirmed
// invoices that have none. An invoice that fails is reported, the others
// go on; a taxpayer without a balance-sheet ledger stops the run.
func (s *Service) CreatePeriodJournals(ctx context.Context, scope engagementctx.Scope, period string, userID int64) (PeriodJournals, error) {
	if !ValidPeriod(period) {
		return PeriodJournals{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	acc, kinds, err := s.journalSetup(ctx, scope.OrganizationID)
	if err != nil {
		return PeriodJournals{}, err
	}
	rows, err := s.q.ListForeignInvoices(ctx, db.ListForeignInvoicesParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Period: period,
	})
	if err != nil {
		return PeriodJournals{}, fmt.Errorf("list foreign invoices: %w", err)
	}
	linked, err := s.journalsByInvoice(ctx, Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}, period)
	if err != nil {
		return PeriodJournals{}, err
	}
	out := PeriodJournals{Period: period, Created: []JournalResult{}, Failed: []JournalFailure{}}
	for _, r := range rows {
		if r.Status != StatusConfirmed {
			continue
		}
		if _, ok := linked[r.ID]; ok {
			out.Skipped++
			continue
		}
		res, err := s.createJournal(ctx, scope, r, acc, kinds, userID)
		switch {
		case err == nil:
			out.Created = append(out.Created, res)
		case errors.Is(err, ErrJournalExists):
			out.Skipped++
		case errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict), errors.Is(err, journals.ErrInvalid):
			out.Failed = append(out.Failed, JournalFailure{InvoiceUUID: r.Uuid, Vendor: r.Vendor, Message: err.Error()})
		default:
			return out, err
		}
	}
	return out, nil
}

// RemoveJournal deletes the invoice's entry while it is a draft that was
// never exported; the invoice can then be changed or booked again.
func (s *Service) RemoveJournal(ctx context.Context, sub Subject, id uuid.UUID) error {
	row, err := s.get(ctx, sub, id)
	if err != nil {
		return err
	}
	j, err := s.journalOf(ctx, sub, row.ID)
	if err != nil {
		return err
	}
	if j == nil {
		return ErrNoJournal
	}
	if j.JournalStatus != "draft" || j.EverExported {
		return ErrJournalLocked
	}
	n, err := s.q.DeleteForeignInvoiceJournalDraft(ctx, db.DeleteForeignInvoiceJournalDraftParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ForeignInvoiceID: row.ID,
	})
	if err != nil {
		return fmt.Errorf("delete foreign invoice journal: %w", err)
	}
	if n == 0 {
		return ErrJournalLocked
	}
	return nil
}

// journalsByInvoice maps the period's invoice ids to their entries.
func (s *Service) journalsByInvoice(ctx context.Context, sub Subject, period string) (map[int64]JournalRef, error) {
	rows, err := s.q.ListForeignInvoiceJournals(ctx, db.ListForeignInvoiceJournalsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Period: period,
	})
	if err != nil {
		return nil, fmt.Errorf("foreign invoice journals: %w", err)
	}
	out := make(map[int64]JournalRef, len(rows))
	for _, r := range rows {
		out[r.ForeignInvoiceID] = JournalRef{UUID: r.JournalUuid, EntryNo: r.EntryNo, Period: r.JournalPeriod, Status: r.JournalStatus}
	}
	return out, nil
}

// unconfirmGuard refuses to take back the confirmation of an invoice with
// an entry: the entry is removed first (or, once exported, cancelled in the
// program and in Yevmiye).
func (s *Service) unconfirmGuard(ctx context.Context, sub Subject, invoiceID int64) error {
	j, err := s.journalOf(ctx, sub, invoiceID)
	if err != nil || j == nil {
		return err
	}
	if j.JournalStatus == "exported" || j.EverExported {
		return fmt.Errorf("%w: faturanın fişi (%s/%d) muhasebe programına aktarıldı; onay geri alınamaz", ErrJournalLocked, j.JournalPeriod, j.EntryNo)
	}
	return fmt.Errorf("%w: faturanın fişi var (%s/%d); önce fişi kaldırın", ErrConflict, j.JournalPeriod, j.EntryNo)
}

func journalInvoiceOf(r db.ForeignServiceInvoice) JournalInvoice {
	inv := JournalInvoice{
		UUID: r.Uuid, Period: r.Period, Vendor: r.Vendor, InvoiceNo: r.InvoiceNo, Currency: r.Currency,
		Terms: Terms{
			Amount: dec(r.Amount), Treatment: r.Treatment, StopajRate: dec(r.StopajRate),
			GrossUp: r.GrossUp, VATRate: dec(r.VatRate),
		},
	}
	if k, ok := DefaultOf(r.ServiceKind); ok {
		inv.ServiceKindLabel = k.Label
	}
	if r.InvoiceDate.Valid {
		t := r.InvoiceDate.Time
		inv.InvoiceDate = &t
	}
	if r.ExchangeRate.Valid {
		rate := dec(r.ExchangeRate)
		inv.Terms.Rate = &rate
	}
	return inv
}
