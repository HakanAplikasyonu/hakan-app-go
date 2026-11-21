package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
)

// Posting target types (receipt_postings.target_type).
const (
	TargetExpense    = "expense"
	TargetRetailSale = "retail_sale"
)

// JournalWriter creates journal drafts (*journals.Service).
type JournalWriter interface {
	CreateDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, in journals.DraftInput) (journals.Draft, error)
}

// WithJournals sets where approved receipts become journal drafts (#384).
// Nil: approval only changes the status.
func (s *Service) WithJournals(j JournalWriter) *Service { s.journals = j; return s }

// JournalRef is the draft an approved receipt produced.
type JournalRef struct {
	UUID    uuid.UUID `json:"uuid"`
	EntryNo int32     `json:"entry_no"`
	Status  string    `json:"status"`
}

// ExpenseView is one expense in the office panel.
type ExpenseView struct {
	UUID               uuid.UUID        `json:"uuid"`
	Date               *string          `json:"expense_date"`
	Period             *string          `json:"period"`
	Amount             *decimal.Decimal `json:"amount"`
	TaxAmount          *decimal.Decimal `json:"tax_amount"`
	TaxRate            *decimal.Decimal `json:"tax_rate"`
	TaxExclusiveAmount *decimal.Decimal `json:"tax_exclusive_amount"`
	Currency           string           `json:"currency"`
	Description        string           `json:"description"`
	ReceiptType        string           `json:"receipt_type"`
	VendorName         *string          `json:"vendor_name"`
	VendorTaxNumber    *string          `json:"vendor_tax_number"`
	ReceiptNumber      *string          `json:"receipt_number"`
	VehiclePlate       *string          `json:"vehicle_plate"`
	PaymentMethod      *string          `json:"payment_method"`
	SourceSystem       string           `json:"source_system"`
	CaptureMethod      string           `json:"capture_method"`
	ApprovalStatus     string           `json:"approval_status"`
	OCRConfidence      *decimal.Decimal `json:"ocr_confidence"`
	HasAttachment      bool             `json:"has_attachment"`
	CategoryCode       *string          `json:"category_code"`
	CategoryName       *string          `json:"category_name"`
	AccountCode        *string          `json:"account_code"`
	AccountSource      *string          `json:"account_source"`
	AccountConfidence  *decimal.Decimal `json:"account_confidence"`
	// Suggested: category and account are a proposal, nobody chose them yet.
	Suggested  bool        `json:"suggested"`
	Journal    *JournalRef `json:"journal"`
	ApprovedAt *time.Time  `json:"approved_at"`
	CreatedAt  time.Time   `json:"created_at"`
}

// ExpenseFilter narrows the list.
type ExpenseFilter struct {
	Period   string
	Status   string // pending | approved | rejected
	Category string // code, or "-" for none
	Q        string
	Limit    int32
	Offset   int32
}

// ExpensePage is one page with totals over the whole filter.
type ExpensePage struct {
	Items     []ExpenseView   `json:"items"`
	Total     int64           `json:"total"`
	Pending   int64           `json:"pending"`
	Amount    decimal.Decimal `json:"amount"`
	TaxAmount decimal.Decimal `json:"tax_amount"`
	Limit     int32           `json:"limit"`
	Offset    int32           `json:"offset"`
}

var statuses = map[string]bool{"": true, "pending": true, "approved": true, "rejected": true}

// Expenses lists the taxpayer's expenses with the office's category and
// account (or the suggestion when none is chosen yet).
func (s *Service) Expenses(ctx context.Context, sc engagementctx.Scope, f ExpenseFilter) (ExpensePage, error) {
	if !statuses[f.Status] {
		return ExpensePage{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	p := db.ListEngagementExpensesParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
		Period: text(f.Period), Status: text(f.Status), Category: text(f.Category), Q: text(strings.TrimSpace(f.Q)),
		Lim: f.Limit, Off: f.Offset,
	}
	rows, err := s.q.ListEngagementExpenses(ctx, p)
	if err != nil {
		return ExpensePage{}, err
	}
	sum, err := s.q.SummarizeEngagementExpenses(ctx, db.SummarizeEngagementExpensesParams{
		OrganizationID: p.OrganizationID, EngagementID: p.EngagementID, TaxpayerID: p.TaxpayerID,
		Period: p.Period, Status: p.Status, Category: p.Category, Q: p.Q,
	})
	if err != nil {
		return ExpensePage{}, err
	}
	sg, err := s.suggester(ctx, sc)
	if err != nil {
		return ExpensePage{}, err
	}
	out := ExpensePage{Items: make([]ExpenseView, 0, len(rows)), Total: sum.Total, Pending: sum.Pending,
		Amount: dec(sum.Amount), TaxAmount: dec(sum.TaxAmount), Limit: f.Limit, Offset: f.Offset}
	for _, r := range rows {
		out.Items = append(out.Items, sg.view(ctx, r))
	}
	return out, nil
}

// Expense returns one expense.
func (s *Service) Expense(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (ExpenseView, error) {
	r, err := s.expenseRow(ctx, sc, id)
	if err != nil {
		return ExpenseView{}, err
	}
	sg, err := s.suggester(ctx, sc)
	if err != nil {
		return ExpenseView{}, err
	}
	return sg.view(ctx, r), nil
}

func (s *Service) expenseRow(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.ListEngagementExpensesRow, error) {
	e, err := s.q.GetExpenseByUUID(ctx, db.GetExpenseByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ListEngagementExpensesRow{}, ErrNotFound
	}
	if err != nil {
		return db.ListEngagementExpensesRow{}, err
	}
	r := expenseToRow(e)
	if p, err := s.q.GetReceiptPosting(ctx, db.GetReceiptPostingParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TargetType: TargetExpense, TargetID: e.ID}); err == nil {
		r.CategoryCode, r.AccountCode, r.AccountSource, r.AccountConfidence = p.CategoryCode, p.AccountCode, p.AccountSource, p.Confidence
		if p.JournalDraftID.Valid {
			if j, err := s.q.GetJournalDraftByID(ctx, db.GetJournalDraftByIDParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: p.JournalDraftID.Int64}); err == nil {
				r.JournalUuid = pgtype.UUID{Bytes: j.Uuid, Valid: true}
				r.JournalEntryNo = pgtype.Int4{Int32: j.EntryNo, Valid: true}
				r.JournalStatus = pgtype.Text{String: j.Status, Valid: true}
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.ListEngagementExpensesRow{}, err
	}
	return r, nil
}

func expenseToRow(e db.Expense) db.ListEngagementExpensesRow {
	return db.ListEngagementExpensesRow{
		ID: e.ID, Uuid: e.Uuid, SourceSystem: e.SourceSystem, CaptureMethod: e.CaptureMethod, ExpenseDate: e.ExpenseDate,
		Period: e.Period, Amount: e.Amount, Currency: e.Currency, TaxRate: e.TaxRate, TaxExclusiveAmount: e.TaxExclusiveAmount,
		TaxAmount: e.TaxAmount, Description: e.Description, ReceiptType: e.ReceiptType, VendorTaxNumber: e.VendorTaxNumber,
		VendorName: e.VendorName, ReceiptNumber: e.ReceiptNumber, VehiclePlate: e.VehiclePlate, PaymentMethod: e.PaymentMethod,
		IsDeductible: e.IsDeductible, ApprovalStatus: e.ApprovalStatus, OcrConfidence: e.OcrConfidence,
		AttachmentID: e.AttachmentID, ApprovedAt: e.ApprovedAt, CreatedAt: e.CreatedAt,
	}
}

// suggester holds what the suggestions of one engagement need.
type suggester struct {
	s    *Service
	sc   engagementctx.Scope
	cats []posting.Category
	plan []engine.PlanAccount
	mem  map[string]*posting.Memory
}

func (s *Service) suggester(ctx context.Context, sc engagementctx.Scope) (*suggester, error) {
	cats, err := s.categories(ctx, sc.OrganizationID, sc.ID)
	if err != nil {
		return nil, err
	}
	plan, err := s.plan(ctx, sc, "770", "760", "780", "689", "191", "100", "102", "108", "309", "320", "600", "391")
	if err != nil {
		return nil, err
	}
	return &suggester{s: s, sc: sc, cats: cats, plan: plan, mem: map[string]*posting.Memory{}}, nil
}

// memory returns the vendor memory for vkn (cached per request).
func (g *suggester) memory(ctx context.Context, vkn string, exclude int64) *posting.Memory {
	if vkn == "" {
		return nil
	}
	if m, ok := g.mem[vkn]; ok {
		return m
	}
	var m *posting.Memory
	if r, err := g.s.q.VendorExpenseMemory(ctx, db.VendorExpenseMemoryParams{
		OrganizationID: g.sc.OrganizationID, EngagementID: g.sc.ID, TaxpayerID: g.sc.TaxpayerID,
		VendorTaxNumber: text(vkn), ExcludeID: exclude,
	}); err == nil && r.AccountCode.Valid {
		m = &posting.Memory{CategoryCode: r.CategoryCode.String, AccountCode: r.AccountCode.String}
	}
	g.mem[vkn] = m
	return m
}

func (g *suggester) suggest(ctx context.Context, r db.ListEngagementExpensesRow) (posting.Suggestion, bool) {
	return posting.Suggest(g.cats, r.ReceiptType, r.VendorName.String, r.Description,
		g.memory(ctx, r.VendorTaxNumber.String, r.ID), g.plan)
}

func (g *suggester) view(ctx context.Context, r db.ListEngagementExpensesRow) ExpenseView {
	v := ExpenseView{
		UUID: r.Uuid, Period: str(r.Period), Amount: decPtr(r.Amount), TaxAmount: decPtr(r.TaxAmount), TaxRate: decPtr(r.TaxRate),
		TaxExclusiveAmount: decPtr(r.TaxExclusiveAmount), Currency: r.Currency, Description: r.Description,
		ReceiptType: r.ReceiptType, VendorName: str(r.VendorName), VendorTaxNumber: str(r.VendorTaxNumber),
		ReceiptNumber: str(r.ReceiptNumber), VehiclePlate: str(r.VehiclePlate), PaymentMethod: str(r.PaymentMethod),
		SourceSystem: r.SourceSystem, CaptureMethod: r.CaptureMethod, ApprovalStatus: r.ApprovalStatus,
		OCRConfidence: decPtr(r.OcrConfidence), HasAttachment: r.AttachmentID.Valid,
		CategoryCode: str(r.CategoryCode), AccountCode: str(r.AccountCode), AccountSource: str(r.AccountSource),
		AccountConfidence: decPtr(r.AccountConfidence), CreatedAt: r.CreatedAt.Time,
	}
	if r.ExpenseDate.Valid {
		d := r.ExpenseDate.Time.Format(time.DateOnly)
		v.Date = &d
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		v.ApprovedAt = &t
	}
	if r.JournalUuid.Valid {
		v.Journal = &JournalRef{UUID: r.JournalUuid.Bytes, EntryNo: r.JournalEntryNo.Int32, Status: r.JournalStatus.String}
	}
	if v.AccountCode == nil && r.ApprovalStatus == "pending" {
		if sug, ok := g.suggest(ctx, r); ok {
			v.Suggested = true
			v.CategoryCode, v.AccountCode, v.AccountSource = &sug.CategoryCode, &sug.AccountCode, &sug.Source
			c := sug.Confidence
			v.AccountConfidence = &c
		}
	}
	if v.CategoryCode != nil {
		for _, c := range g.cats {
			if c.Code == *v.CategoryCode {
				n := c.Name
				v.CategoryName = &n
			}
		}
	}
	return v
}

// PostingInput is the office's choice for an expense.
type PostingInput struct {
	CategoryCode *string `json:"category_code"`
	AccountCode  *string `json:"account_code"`
}

var accountRe = regexp.MustCompile(`^[0-9]{3}(\.[0-9A-Za-z]+)*$`)

// SetExpensePosting stores the category and / or account of a pending
// expense. A category without an account takes the category's account
// (matched to the plan); a typed account is "manual".
func (s *Service) SetExpensePosting(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, in PostingInput) (ExpenseView, error) {
	r, err := s.expenseRow(ctx, sc, id)
	if err != nil {
		return ExpenseView{}, err
	}
	if r.ApprovalStatus != "pending" {
		return ExpenseView{}, ErrState
	}
	sg, err := s.suggester(ctx, sc)
	if err != nil {
		return ExpenseView{}, err
	}
	cat, account, source := r.CategoryCode.String, r.AccountCode.String, r.AccountSource.String
	conf := dec(r.AccountConfidence)
	if in.CategoryCode != nil {
		c, ok := posting.Find(sg.cats, strings.TrimSpace(*in.CategoryCode))
		if !ok {
			return ExpenseView{}, fmt.Errorf("%w: unknown category %q", ErrInvalid, *in.CategoryCode)
		}
		sug := posting.ForCategory(c, sg.plan)
		cat, account, source, conf = c.Code, sug.AccountCode, sug.Source, sug.Confidence
	}
	if in.AccountCode != nil {
		a := strings.TrimSpace(*in.AccountCode)
		if !accountRe.MatchString(a) {
			return ExpenseView{}, fmt.Errorf("%w: account_code must look like 770 or 770.01.001", ErrInvalid)
		}
		account, source, conf = a, posting.SourceManual, posting.ManualConfidence()
	}
	if cat == "" && account == "" {
		return ExpenseView{}, fmt.Errorf("%w: category_code or account_code is required", ErrInvalid)
	}
	if _, err := s.q.UpsertReceiptPosting(ctx, db.UpsertReceiptPostingParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, TargetType: TargetExpense, TargetID: r.ID,
		CategoryCode: text(cat), AccountCode: text(account), AccountSource: text(source), Confidence: optNum(conf, account != ""),
		DecidedBy: optID(userID),
	}); err != nil {
		return ExpenseView{}, err
	}
	return s.Expense(ctx, sc, id)
}

// ErrIncomplete: the expense cannot be booked (no date or amount).
var ErrIncomplete = errors.New("receipts: the record needs a date and an amount before approval")

// DecideExpense approves or rejects a pending expense from the office's
// expense list. Approval fixes the category and account (the suggestion
// when none was chosen) and creates the journal draft.
func (s *Service) DecideExpense(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, approve bool) (ExpenseView, error) {
	e, err := s.q.GetExpenseByUUID(ctx, db.GetExpenseByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExpenseView{}, ErrNotFound
	}
	if err != nil {
		return ExpenseView{}, err
	}
	if e.ApprovalStatus != "pending" {
		return ExpenseView{}, ErrState
	}
	if approve && (!e.ExpenseDate.Valid || !dec(e.Amount).IsPositive()) {
		return ExpenseView{}, ErrIncomplete
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return ExpenseView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tctx := dbtx.WithTx(ctx, tx)
	q := s.q.WithTx(tx)
	status := "rejected"
	if approve {
		status = "approved"
	}
	row, err := q.SetExpenseApprovalByID(tctx, db.SetExpenseApprovalByIDParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: sc.TaxpayerID, ID: e.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExpenseView{}, ErrState
	}
	if err != nil {
		return ExpenseView{}, err
	}
	if err := q.SetCaptureStatusForExpense(tctx, db.SetCaptureStatusForExpenseParams{
		Status:         map[bool]string{true: StatusApproved, false: StatusRejected}[approve],
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, ExpenseID: pgtype.Int8{Int64: e.ID, Valid: true},
	}); err != nil {
		return ExpenseView{}, err
	}
	if approve {
		if err := s.postExpense(tctx, q, sc, row, userID); err != nil {
			return ExpenseView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ExpenseView{}, err
	}
	s.syncExpenseApproval(ctx, sc, row)
	return s.Expense(ctx, sc, id)
}

// postExpense fixes the posting of an approved expense and creates its
// journal draft (when a journal writer is set and the expense has a date
// and an amount). It runs in the approval's transaction (ctx carries it).
func (s *Service) postExpense(ctx context.Context, q *db.Queries, sc engagementctx.Scope, e db.Expense, userID int64) error {
	sg, err := s.suggester(ctx, sc)
	if err != nil {
		return err
	}
	r := expenseToRow(e)
	// İşletme defteri / serbest meslek (#780): kategori gider türünü verir;
	// hesap kodu ve yevmiye taslağı yok.
	single, err := singleEntry(ctx, q, sc)
	if err != nil {
		return err
	}
	p, err := q.GetReceiptPosting(ctx, db.GetReceiptPostingParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TargetType: TargetExpense, TargetID: e.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		sug, ok := sg.suggest(ctx, r)
		if !ok {
			return fmt.Errorf("%w: no expense category is active", ErrInvalid)
		}
		account := text(sug.AccountCode)
		if single {
			account = pgtype.Text{}
		}
		p, err = q.UpsertReceiptPosting(ctx, db.UpsertReceiptPostingParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, TargetType: TargetExpense, TargetID: e.ID,
			CategoryCode: text(sug.CategoryCode), AccountCode: account, AccountSource: text(sug.Source),
			Confidence: optNum(sug.Confidence, true), DecidedBy: optID(userID),
		})
		if err != nil {
			return err
		}
	case err != nil:
		return err
	}
	if single {
		return nil
	}
	if !p.AccountCode.Valid {
		// A category was chosen without a resolvable account.
		c, ok := posting.Find(sg.cats, p.CategoryCode.String)
		if !ok {
			return fmt.Errorf("%w: the expense has no account code", ErrInvalid)
		}
		p.AccountCode = text(posting.ForCategory(c, sg.plan).AccountCode)
	}
	if s.journals == nil || !e.ExpenseDate.Valid || !dec(e.Amount).IsPositive() {
		return nil
	}
	cat, _ := posting.Find(sg.cats, p.CategoryCode.String)
	deductible := e.IsDeductible != "no" && cat.Deductible != "no"
	vatAccount := cat.VATAccountCode
	payment, err := s.paymentAccount(ctx, q, sc, e, sg.plan)
	if err != nil {
		return err
	}
	entry, err := posting.ExpenseEntry(posting.ExpenseInput{
		SourceID: "expense:" + e.Uuid.String(), Date: e.ExpenseDate.Time, Total: dec(e.Amount), Tax: dec(e.TaxAmount),
		TaxRate: decPtr(e.TaxRate), Deductible: deductible, ExpenseAccount: p.AccountCode.String, VATAccount: vatAccount,
		PaymentAccount: payment, VendorName: e.VendorName.String, ReceiptNumber: e.ReceiptNumber.String,
		CategoryName: cat.Name, Confidence: dec(p.Confidence), Plan: sg.plan,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIncomplete, err)
	}
	return s.writeJournal(ctx, q, sc, userID, TargetExpense, e.ID, entry, map[string]any{
		"source": "receipt", "target_type": TargetExpense, "expense_uuid": e.Uuid.String(),
		"category_code": p.CategoryCode.String, "account_source": p.AccountSource.String,
	})
}

// paymentAccount is the credit account of an expense: the payment method's
// main account matched to the plan; on credit (320) the vendor's cari from
// the counterparty memory when known.
func (s *Service) paymentAccount(ctx context.Context, q *db.Queries, sc engagementctx.Scope, e db.Expense, plan []engine.PlanAccount) (string, error) {
	main, hint := posting.PaymentMain(e.PaymentMethod.String)
	if main == "320" && e.VendorTaxNumber.Valid {
		c, err := q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CounterpartyTaxID: e.VendorTaxNumber.String, MainCode: "320",
		})
		if err == nil {
			return c.AccountCode, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if pr := engine.ProposeCounterparty("320", e.VendorName.String, plan); pr.Existing {
			return pr.AccountCode, nil
		}
	}
	return posting.ResolveLeaf(main, hint, plan).Code, nil
}

// writeJournal creates the draft and links it to the posting.
func (s *Service) writeJournal(ctx context.Context, q *db.Queries, sc engagementctx.Scope, userID int64, target string, targetID int64, entry posting.Entry, origin map[string]any) error {
	if !entry.Balanced() {
		return fmt.Errorf("%w: the entry does not balance", ErrInvalid)
	}
	in := journals.DraftInput{Type: accountingexport.EntryMahsup, Date: entry.Date, Description: entry.Description,
		DocumentType: entry.DocumentType, Origin: origin}
	for _, l := range entry.Lines {
		c := l.Confidence
		li := journals.LineInput{Account: l.Account, Description: l.Description, Debit: l.Debit, Credit: l.Credit,
			DocumentNo: entry.DocumentNo, SourceKind: posting.SourceKind, SourceID: l.SourceID}
		if !c.IsZero() {
			li.Confidence = &c
		}
		d := entry.Date
		li.DocumentDate = &d
		in.Lines = append(in.Lines, li)
	}
	d, err := s.journals.CreateDraft(ctx, sc, userID, in)
	if err != nil {
		return fmt.Errorf("receipts: journal draft: %w", err)
	}
	jid, err := q.GetJournalDraftIDByUUID(ctx, db.GetJournalDraftIDByUUIDParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: d.UUID})
	if err != nil {
		return err
	}
	return q.SetReceiptPostingJournal(ctx, db.SetReceiptPostingJournalParams{
		JournalDraftID: pgtype.Int8{Int64: jid, Valid: true}, OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		TargetType: target, TargetID: targetID,
	})
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

func str(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func decPtr(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return nil
	}
	d := decimal.NewFromBigInt(n.Int, n.Exp)
	return &d
}

// singleEntry reports whether the engagement keeps an işletme defteri or
// serbest meslek kazanç defteri (#780): no account postings, no journal.
func singleEntry(ctx context.Context, q *db.Queries, sc engagementctx.Scope) (bool, error) {
	t, err := ledger.Of(ctx, q, sc.OrganizationID, sc.ID)
	if err != nil {
		return false, err
	}
	return ledger.SingleEntry(t), nil
}
