package posting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
)

// SuggestionKind is the review queue kind of a movement's counter account.
const SuggestionKind = "bank_account"

// Target of the suggestion.
const (
	TargetType  = "bank_transaction"
	TargetField = "account_code"
)

// DraftSourceKind is journal_draft_lines.source_kind of a bank line; its
// source_id is the bank_transactions id (Luca / Zirve export dedups on it).
const DraftSourceKind = "bank_transaction"

// Errors of the workflow.
var (
	ErrDrafted  = errors.New("posting: the movement is already in a journal draft")
	ErrNoReview = errors.New("posting: review queue is not configured")
)

// Reviewer is the review queue (#14) subset the workflow uses.
type Reviewer interface {
	Submit(ctx context.Context, sub reviewusecase.Subject, in reviewusecase.Input) (reviewusecase.Suggestion, error)
	Accept(ctx context.Context, sub reviewusecase.Subject, id uuid.UUID, userID int64, reason string) (reviewusecase.Suggestion, error)
	Correct(ctx context.Context, sub reviewusecase.Subject, id uuid.UUID, userID int64, value json.RawMessage, reason string) (reviewusecase.Suggestion, error)
}

// DraftCreator is the journals service subset (#26).
type DraftCreator interface {
	CreateDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, in journals.DraftInput) (journals.Draft, error)
}

// Workflow is the movement → account → journal draft flow.
type Workflow struct {
	pool   *pgxpool.Pool
	review Reviewer
	drafts DraftCreator
}

// NewWorkflow builds the workflow. drafts may be nil (no draft creation).
func NewWorkflow(pool *pgxpool.Pool, review Reviewer, drafts DraftCreator) *Workflow {
	return &Workflow{pool: pool, review: review, drafts: drafts}
}

func (w *Workflow) q() *db.Queries { return database.QueriesFor(w.pool) }

func (s Subject) review() reviewusecase.Subject {
	return reviewusecase.Subject{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TaxpayerID: s.TaxpayerID}
}

type ctxKey struct{}

// automatic marks a Submit call: an auto-accepted suggestion is not the
// accountant's decision and teaches the alias memory nothing (#333).
func automatic(ctx context.Context) context.Context { return context.WithValue(ctx, ctxKey{}, true) }

func isAutomatic(ctx context.Context) bool { v, _ := ctx.Value(ctxKey{}).(bool); return v }

// accountValue is the suggestion value.
type accountValue struct {
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name,omitempty"`
}

// RunResult counts what a suggestion run did.
type RunResult struct {
	Movements    int `json:"movements"`
	Suggested    int `json:"suggested"`
	AutoApproved int `json:"auto_approved"`
	Unchanged    int `json:"unchanged"`
	Approved     int `json:"already_approved"`
	NoSuggestion int `json:"no_suggestion"`
}

func movementOf(r db.ListBankTransactionsForPostingRow) Movement {
	m := Movement{ID: r.ID, Direction: banking.Direction(r.DebitCredit), Description: r.Description,
		Kind: banking.Kind(r.Kind), Detail: banking.Detail(r.KindDetail), MatchedTaxID: r.MatchedTaxID, MatchedName: r.MatchedName,
		Counterparty: Counterparty{Name: r.CounterpartyName.String, IBAN: r.CounterpartyIban.String,
			TaxID: r.CounterpartyTaxNumber.String + r.CounterpartyIdentityNumber.String, Direction: banking.Direction(r.DebitCredit)}}
	if v, err := r.Amount.Value(); err == nil && v != nil {
		m.Amount, _ = decimal.NewFromString(fmt.Sprint(v))
	}
	if r.KindConfidence.Valid {
		if v, err := r.KindConfidence.Value(); err == nil && v != nil {
			if d, err := decimal.NewFromString(fmt.Sprint(v)); err == nil {
				m.KindConfidence = &d
			}
		}
	}
	return m
}

// Run proposes the counter account of every movement of the period that is
// not approved yet and sends it to the review queue. The engagement's
// review threshold may approve it at once (auto_accepted, revertible).
func (w *Workflow) Run(ctx context.Context, sub Subject, period string) (RunResult, error) {
	if w.review == nil {
		return RunResult{}, ErrNoReview
	}
	q := w.q()
	// İşletme defteri / serbest meslek (#780): banka hareketine hesap kodu
	// önerilmez.
	if err := ledger.RequireBalance(ctx, q, sub.OrganizationID, sub.EngagementID); err != nil {
		return RunResult{}, err
	}
	rows, err := q.ListBankTransactionsForPosting(ctx, db.ListBankTransactionsForPostingParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, Period: period,
	})
	if err != nil {
		return RunResult{}, err
	}
	res := RunResult{Movements: len(rows)}
	for _, r := range rows {
		if r.PostingStatus.String == "approved" {
			res.Approved++
			continue
		}
		m := movementOf(r)
		s, err := Suggest(ctx, q, sub, m)
		if err != nil {
			return res, err
		}
		if s == nil {
			res.NoSuggestion++
			continue
		}
		// The same proposal, not surer than before: leave the pending one.
		if r.PostingID.Valid && r.PostingAccountCode.String == s.AccountCode && r.PostingSuggestionID.Valid &&
			!s.Confidence.GreaterThan(numDecimal(r.PostingConfidence)) {
			res.Unchanged++
			continue
		}
		auto, err := w.submit(ctx, q, sub, r.Uuid, r.Period, m, s)
		if err != nil {
			return res, fmt.Errorf("movement %s: %w", r.Uuid, err)
		}
		if auto {
			res.AutoApproved++
		} else {
			res.Suggested++
		}
	}
	return res, nil
}

func (w *Workflow) submit(ctx context.Context, q *db.Queries, sub Subject, txnUUID uuid.UUID, period string, m Movement, s *Suggestion) (bool, error) {
	if _, err := q.UpsertBankPosting(ctx, db.UpsertBankPostingParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionID: m.ID,
		AccountCode: s.AccountCode, AccountName: s.AccountName, Confidence: num(s.Confidence), Source: s.Source, Reason: s.Reason,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	val, _ := json.Marshal(accountValue{AccountCode: s.AccountCode, AccountName: s.AccountName})
	id := m.ID
	prov := reviewusecase.Provenance{Origin: reviewusecase.OriginRule, Inputs: map[string]any{
		"transaction_uuid": txnUUID.String(), "direction": string(m.Direction), "amount": m.Amount.StringFixed(2),
		"counterparty_name": m.Counterparty.Name, "source": s.Source, "description": m.Description,
	}}
	switch {
	case s.Marketplace != "":
		prov.RuleRef = "bank_marketplace:" + s.Marketplace
	case s.Source == SourceAlias:
		prov.RuleRef = "bank_alias:" + strconv.FormatInt(s.AliasID, 10)
	case s.Source == SourceCounterpartyAccount, s.Source == SourceInvoiceMatch:
		prov.RuleRef = "counterparty_account"
	default:
		prov.RuleRef = "bank_kind:" + string(m.Detail)
	}
	sug, err := w.review.Submit(automatic(ctx), sub.review(), reviewusecase.Input{
		Kind: SuggestionKind, TargetType: TargetType, TargetID: &id, TargetField: TargetField, Period: period,
		Value: val, Confidence: s.Confidence, Rationale: s.Reason, Provenance: prov,
	})
	if err != nil {
		return false, err
	}
	if sug.Status == reviewusecase.StatusAutoAccepted {
		return true, nil
	}
	return false, q.SetBankPostingSuggestion(ctx, db.SetBankPostingSuggestionParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: m.ID,
		SuggestionID: pgtype.Int8{Int64: sug.ID, Valid: true},
	})
}

// Item is one movement of the screen list.
type Item struct {
	UUID             uuid.UUID        `json:"uuid"`
	Date             string           `json:"transaction_date"`
	Direction        string           `json:"direction"`
	Amount           string           `json:"amount"`
	Currency         string           `json:"currency"`
	Description      string           `json:"description"`
	CounterpartyName string           `json:"counterparty_name"`
	CounterpartyIBAN string           `json:"counterparty_iban,omitempty"`
	Kind             string           `json:"kind"`
	KindDetail       string           `json:"kind_detail"`
	MatchStatus      string           `json:"match_status"`
	CashBoxUUID      uuid.UUID        `json:"cash_box_uuid"`
	CashBoxName      string           `json:"cash_box_name"`
	Status           string           `json:"status"`
	AccountCode      string           `json:"account_code,omitempty"`
	AccountName      string           `json:"account_name,omitempty"`
	Confidence       *decimal.Decimal `json:"confidence,omitempty"`
	Source           string           `json:"source,omitempty"`
	Reason           string           `json:"reason,omitempty"`
	SuggestionUUID   *uuid.UUID       `json:"suggestion_uuid,omitempty"`
	SuggestionStatus string           `json:"suggestion_status,omitempty"`
	JournalDraftUUID *uuid.UUID       `json:"journal_draft_uuid,omitempty"`
	// Marketplace: the suggestion came from the marketplace table.
	Marketplace *MarketplaceNote `json:"marketplace,omitempty"`
}

// Summary counts the movements by posting state.
type Summary struct {
	Unassigned int64 `json:"unassigned"`
	Suggested  int64 `json:"suggested"`
	Approved   int64 `json:"approved"`
	Drafted    int64 `json:"drafted"`
	Total      int64 `json:"total"`
}

// Page is the screen list.
type Page struct {
	Items   []Item  `json:"items"`
	Summary Summary `json:"summary"`
	Limit   int32   `json:"limit"`
	Offset  int32   `json:"offset"`
}

// Statuses of the list filter.
var Statuses = []string{"unassigned", "suggested", "approved", "drafted"}

// List is the movement → account screen.
func (w *Workflow) List(ctx context.Context, sub Subject, period, status string, limit, offset int32) (Page, error) {
	q := w.q()
	per := pgtype.Text{String: period, Valid: period != ""}
	sum, err := q.CountBankPostings(ctx, db.CountBankPostingsParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, Period: per})
	if err != nil {
		return Page{}, err
	}
	rows, err := q.ListBankPostings(ctx, db.ListBankPostingsParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, Period: per, Status: pgtype.Text{String: status, Valid: status != ""}, Lim: limit, Off: offset})
	if err != nil {
		return Page{}, err
	}
	p := Page{Items: make([]Item, 0, len(rows)), Limit: limit, Offset: offset,
		Summary: Summary{Unassigned: sum.Unassigned, Suggested: sum.Suggested, Approved: sum.Approved, Drafted: sum.Drafted, Total: sum.Total}}
	for _, r := range rows {
		it := Item{UUID: r.Uuid, Date: r.TransactionDate.Time.Format("2006-01-02"), Direction: r.DebitCredit,
			Amount: decString(r.Amount, 2), Currency: r.Currency, Description: r.Description, CounterpartyName: r.CounterpartyName.String,
			CounterpartyIBAN: r.CounterpartyIban.String, Kind: r.Kind, KindDetail: r.KindDetail, MatchStatus: r.MatchStatus,
			CashBoxUUID: r.CashBoxUuid, CashBoxName: firstNonEmpty(r.CashBoxName, r.CashBoxIban.String),
			Status: "unassigned", AccountCode: r.AccountCode.String, AccountName: r.AccountName.String,
			Source: r.Source.String, Reason: r.Reason.String, SuggestionStatus: r.SuggestionStatus.String}
		if r.Status.Valid {
			it.Status = r.Status.String
			if d, err := decimal.NewFromString(decString(r.Confidence, 4)); err == nil {
				it.Confidence = &d
			}
		}
		it.Marketplace = marketplaceNote(it.Source, it.Reason, it.Description, it.AccountCode)
		if r.JournalDraftUuid.Valid {
			id := uuid.UUID(r.JournalDraftUuid.Bytes)
			it.JournalDraftUUID, it.Status = &id, "drafted"
		}
		if r.SuggestionUuid.Valid {
			id := uuid.UUID(r.SuggestionUuid.Bytes)
			it.SuggestionUUID = &id
		}
		p.Items = append(p.Items, it)
	}
	return p, nil
}

// ApproveResult counts a bulk approval.
type ApproveResult struct {
	Approved int `json:"approved"`
	Skipped  int `json:"skipped"`
}

// Approve accepts the pending suggestions of the given movements as
// proposed (one click per movement, or many at once).
func (w *Workflow) Approve(ctx context.Context, sub Subject, userID int64, txns []uuid.UUID) (ApproveResult, error) {
	if w.review == nil {
		return ApproveResult{}, ErrNoReview
	}
	if len(txns) == 0 || len(txns) > 500 {
		return ApproveResult{}, fmt.Errorf("%w: 1 to 500 transaction_uuids", ErrInvalid)
	}
	q := w.q()
	var res ApproveResult
	for _, id := range txns {
		t, err := q.GetBankTransactionForPosting(ctx, db.GetBankTransactionForPostingParams{TaxpayerID: sub.TaxpayerID, Uuid: pgtype.UUID{Bytes: id, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			res.Skipped++
			continue
		} else if err != nil {
			return res, err
		}
		p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: t.ID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (p.Status != "suggested" || !p.SuggestionUuid.Valid || p.SuggestionStatus.String != string(reviewusecase.StatusPending))) {
			res.Skipped++
			continue
		} else if err != nil {
			return res, err
		}
		if _, err := w.review.Accept(ctx, sub.review(), uuid.UUID(p.SuggestionUuid.Bytes), userID, "banka eşleştirme ekranından onay"); err != nil {
			return res, err
		}
		res.Approved++
	}
	return res, nil
}

// SetAccount books one movement to the account the accountant chose: a
// pending suggestion is corrected; otherwise a manual suggestion is
// submitted and accepted. The decision teaches the alias memory.
func (w *Workflow) SetAccount(ctx context.Context, sub Subject, userID int64, txn uuid.UUID, code, name string) error {
	if w.review == nil {
		return ErrNoReview
	}
	code = strings.TrimSpace(code)
	if !engine.ValidCode(code) {
		return fmt.Errorf("%w: account_code %q", ErrInvalid, code)
	}
	q := w.q()
	t, err := q.GetBankTransactionForPosting(ctx, db.GetBankTransactionForPostingParams{TaxpayerID: sub.TaxpayerID, Uuid: pgtype.UUID{Bytes: txn, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	val, _ := json.Marshal(accountValue{AccountCode: code, AccountName: strings.TrimSpace(name)})
	p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionID: t.ID})
	switch {
	case err == nil && p.JournalDraftUuid.Valid:
		return ErrDrafted
	case err == nil && p.SuggestionUuid.Valid && p.SuggestionStatus.String == string(reviewusecase.StatusPending):
		_, err = w.review.Correct(ctx, sub.review(), uuid.UUID(p.SuggestionUuid.Bytes), userID, val, "banka eşleştirme ekranında hesap seçildi")
		return err
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	id := t.ID
	reason := "Müşavir hesabı elle seçti: " + code
	if name = strings.TrimSpace(name); name != "" {
		reason += " " + name
	}
	if _, err := q.UpsertBankPosting(ctx, db.UpsertBankPostingParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, BankTransactionID: t.ID,
		AccountCode: code, AccountName: name, Confidence: num(decimal.NewFromInt(1)), Source: SourceManual, Reason: reason,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	sug, err := w.review.Submit(ctx, sub.review(), reviewusecase.Input{
		Kind: SuggestionKind, TargetType: TargetType, TargetID: &id, TargetField: TargetField, Period: t.Period,
		Value: val, Confidence: decimal.NewFromInt(1), Rationale: reason,
		Provenance: reviewusecase.Provenance{Origin: reviewusecase.OriginInput, Inputs: map[string]any{
			"transaction_uuid": txn.String(), "manual": true, "counterparty_name": t.CounterpartyName.String}},
	})
	if err != nil {
		return err
	}
	if sug.Status == reviewusecase.StatusPending {
		_, err = w.review.Accept(ctx, sub.review(), sug.UUID, userID, "banka eşleştirme ekranında hesap seçildi")
	}
	return err
}

// DraftResult counts a draft run.
type DraftResult struct {
	Drafts         []journals.Draft `json:"drafts"`
	Transactions   int              `json:"transactions"`
	SkippedForeign int              `json:"skipped_foreign_currency"`
	// SkippedSameAccount: movements whose counter account is the bank
	// account itself (or its main / sub account); they would book the bank
	// against itself, so they go back to review for another account.
	SkippedSameAccount int `json:"skipped_same_account"`
}

// SameAccount reports whether two account codes are the same account or
// one contains the other (102 and 102.01): a two-line entry between them
// leaves the account unchanged, the same amount on its debit and credit.
func SameAccount(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
}

// CreateDrafts books the approved movements that are in no draft yet: one
// mahsup draft per bank account and day, two lines per movement (bank
// account against the counter account; a netted POS collection adds its
// commission line, #565), source_kind bank_transaction.
// The drafts follow the journals flow (approval, Luca / Zirve export).
// Foreign-currency movements are left out: their TRY amount is not known.
// A movement booked to the bank account itself is left out and sent back
// to review (SameAccount).
func (w *Workflow) CreateDrafts(ctx context.Context, scope engagementctx.Scope, userID int64, period string) (DraftResult, error) {
	if err := ledger.RequireBalance(ctx, w.q(), scope.OrganizationID, scope.ID); err != nil {
		return DraftResult{}, err
	}
	res := DraftResult{Drafts: []journals.Draft{}}
	if w.drafts == nil {
		return res, errors.New("posting: journal drafts are not configured")
	}
	q := w.q()
	rows, err := q.ListApprovedBankPostingsWithoutDraft(ctx, db.ListApprovedBankPostingsWithoutDraftParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
		Period: pgtype.Text{String: period, Valid: period != ""},
	})
	if err != nil {
		return res, err
	}
	type group struct {
		box  int64
		day  time.Time
		name string
		rows []db.ListApprovedBankPostingsWithoutDraftRow
	}
	groups := map[string]*group{}
	var keys []string
	// Transfer between own accounts (#710): the outgoing leg books
	// 102.B / 102.A; the incoming leg joins the same draft, no lines.
	var incoming []db.ListApprovedBankPostingsWithoutDraftRow
	drafted := map[int64]uuid.UUID{}
	for _, r := range rows {
		if r.TransferPartnerID > 0 && banking.Direction(r.DebitCredit) == banking.DirIn {
			incoming = append(incoming, r)
			continue
		}
		if r.Currency != "TRY" {
			res.SkippedForeign++
			continue
		}
		if SameAccount(r.AccountCode, r.BankAccountCode) {
			// Back to review: the accountant picks the real counter account.
			if _, err := q.UnapproveBankPosting(ctx, db.UnapproveBankPostingParams{OrganizationID: scope.OrganizationID,
				EngagementID: scope.ID, BankTransactionID: r.BankTransactionID, AccountCode: r.AccountCode}); err != nil {
				return res, err
			}
			res.SkippedSameAccount++
			continue
		}
		k := fmt.Sprintf("%d|%s", r.CashBoxID, r.TransactionDate.Time.Format("2006-01-02"))
		g, ok := groups[k]
		if !ok {
			g = &group{box: r.CashBoxID, day: r.TransactionDate.Time, name: r.CashBoxName}
			groups[k] = g
			keys = append(keys, k)
		}
		g.rows = append(g.rows, r)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		in := journals.DraftInput{Type: accountingexport.EntryMahsup, Date: g.day,
			Description: strings.TrimSpace(firstNonEmpty(g.name, "Banka") + " hareketleri " + g.day.Format("02.01.2006")),
			Origin:      map[string]any{"source": "bank_transactions", "transactions": len(g.rows)}}
		ids := make([]int64, 0, len(g.rows))
		for _, r := range g.rows {
			amt, err := decimal.NewFromString(decString(r.Amount, 2))
			if err != nil {
				return res, err
			}
			src := strconv.FormatInt(r.BankTransactionID, 10)
			desc := strings.TrimSpace(firstNonEmpty(r.CounterpartyName.String, r.Description))
			conf, _ := decimal.NewFromString(decString(r.Confidence, 4))
			bank := journals.LineInput{Account: r.BankAccountCode, Description: desc, SourceKind: DraftSourceKind, SourceID: src}
			counter := journals.LineInput{Account: r.AccountCode, Description: desc, SourceKind: DraftSourceKind, SourceID: src, Confidence: &conf}
			if banking.Direction(r.DebitCredit) == banking.DirIn {
				bank.Debit, counter.Credit = amt, amt
			} else {
				counter.Debit, bank.Credit = amt, amt
			}
			// POS netting (#565): the bank paid in the net; the commission
			// is an expense and the counter account takes the gross.
			if c, code, ok := commissionLine(r); ok {
				counter.Credit = amt.Add(c)
				in.Lines = append(in.Lines, bank, journals.LineInput{Account: code, Description: desc + " POS komisyonu",
					Debit: c, SourceKind: DraftSourceKind, SourceID: src}, counter)
				ids = append(ids, r.BankTransactionID)
				continue
			}
			in.Lines = append(in.Lines, bank, counter)
			ids = append(ids, r.BankTransactionID)
		}
		d, err := w.drafts.CreateDraft(ctx, scope, userID, in)
		if err != nil {
			return res, err
		}
		if err := q.SetBankPostingsDraft(ctx, db.SetBankPostingsDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
			JournalDraftUuid: pgtype.UUID{Bytes: d.UUID, Valid: true}, BankTransactionIds: ids}); err != nil {
			return res, err
		}
		for _, id := range ids {
			drafted[id] = d.UUID
		}
		res.Drafts = append(res.Drafts, d)
		res.Transactions += len(ids)
	}
	for _, r := range incoming {
		draft, ok := drafted[r.TransferPartnerID]
		if !ok {
			p, err := q.GetBankPostingView(ctx, db.GetBankPostingViewParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, BankTransactionID: r.TransferPartnerID})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !p.JournalDraftUuid.Valid) {
				continue // the outgoing leg is not drafted yet
			} else if err != nil {
				return res, err
			}
			draft = uuid.UUID(p.JournalDraftUuid.Bytes)
		}
		if err := q.SetBankPostingsDraft(ctx, db.SetBankPostingsDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
			JournalDraftUuid: pgtype.UUID{Bytes: draft, Valid: true}, BankTransactionIds: []int64{r.BankTransactionID}}); err != nil {
			return res, err
		}
		res.Transactions++
	}
	return res, nil
}

// LedgerAccount is a bank account and its trial-balance account.
type LedgerAccount struct {
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	Name        string    `json:"name"`
	IBAN        string    `json:"iban,omitempty"`
	BankID      string    `json:"bank_id,omitempty"`
	Currency    string    `json:"currency"`
	AccountCode string    `json:"account_code"`
	Default     bool      `json:"default"`
}

// LedgerAccounts lists the taxpayer's bank accounts with their 102 code.
func (w *Workflow) LedgerAccounts(ctx context.Context, sub Subject) ([]LedgerAccount, error) {
	rows, err := w.q().ListBankLedgerAccounts(ctx, db.ListBankLedgerAccountsParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID})
	if err != nil {
		return nil, err
	}
	out := make([]LedgerAccount, len(rows))
	for i, r := range rows {
		out[i] = LedgerAccount{CashBoxUUID: r.CashBoxUuid, Name: r.Name, IBAN: r.Iban.String, BankID: r.BankID.String,
			Currency: r.Currency, AccountCode: firstNonEmpty(r.AccountCode.String, DefaultBankAccount), Default: !r.AccountCode.Valid}
	}
	return out, nil
}

// SetLedgerAccount sets a bank account's trial-balance account (102.01 ...).
func (w *Workflow) SetLedgerAccount(ctx context.Context, sub Subject, userID int64, box uuid.UUID, code string) error {
	code = strings.TrimSpace(code)
	if !engine.ValidCode(code) {
		return fmt.Errorf("%w: account_code %q", ErrInvalid, code)
	}
	q := w.q()
	b, err := q.GetCashBoxByUUID(ctx, db.GetCashBoxByUUIDParams{TaxpayerID: sub.TaxpayerID, Uuid: box})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return q.UpsertBankLedgerAccount(ctx, db.UpsertBankLedgerAccountParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		TaxpayerID: sub.TaxpayerID, CashBoxID: b.ID, AccountCode: code, UpdatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0}})
}

func num(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func numDecimal(n pgtype.Numeric) decimal.Decimal {
	d, _ := decimal.NewFromString(decString(n, 4))
	return d
}

func decString(n pgtype.Numeric, places int32) string {
	if !n.Valid {
		return "0"
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return "0"
	}
	d, err := decimal.NewFromString(fmt.Sprint(v))
	if err != nil {
		return "0"
	}
	return d.StringFixed(places)
}
