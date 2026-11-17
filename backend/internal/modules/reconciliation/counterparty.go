package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// BABSThreshold is the Form BA / BS limit (VUK 396 sıra no.lu tebliğ): a
// counterparty whose monthly invoices (KDV hariç) reach it is reported.
var BABSThreshold = decimal.NewFromInt(5000)

// BABSTolerance: the forms are filed in whole liras (küsurat atılır).
var BABSTolerance = decimal.NewFromInt(1)

// Counterparty results.
const (
	CPMatched         = "matched"
	CPDifference      = "difference"
	CPNoLedgerAccount = "no_ledger_account"
)

// BA / BS results.
const (
	BABSNotRequired    = "not_required"
	BABSOK             = "ok"
	BABSMissingInForm  = "missing_in_form"
	BABSExtraInForm    = "extra_in_form"
	BABSAmountMismatch = "amount_mismatch"
	BABSCountMismatch  = "count_mismatch"
	BABSNoDeclaration  = "no_declaration"
)

// BABSIssue reports whether a BA / BS result needs attention.
func BABSIssue(r string) bool {
	switch r {
	case BABSMissingInForm, BABSExtraInForm, BABSAmountMismatch, BABSCountMismatch:
		return true
	}
	return false
}

// FormAmount is one form's amount (KDV hariç) and document count.
type FormAmount struct {
	Amount decimal.Decimal
	Count  int
}

// BABSResult compares the invoices' total for a counterparty with the
// declared form line. imported: the form has any line for the period (else
// a missing line is no_declaration, not missing_in_form).
func BABSResult(computed FormAmount, declared *FormAmount, imported bool) string {
	required := computed.Amount.GreaterThanOrEqual(BABSThreshold)
	switch {
	case !required && declared == nil:
		return BABSNotRequired
	case !required:
		return BABSExtraInForm
	case declared == nil && imported:
		return BABSMissingInForm
	case declared == nil:
		return BABSNoDeclaration
	case computed.Amount.Truncate(0).Sub(declared.Amount).Abs().GreaterThan(BABSTolerance):
		return BABSAmountMismatch
	case computed.Count != declared.Count:
		return BABSCountMismatch
	}
	return BABSOK
}

// CounterpartyRow is one counterparty of a run.
type CounterpartyRow struct {
	TaxID            string           `json:"counterparty_tax_id"`
	Name             string           `json:"counterparty_name"`
	AccountCode      *string          `json:"account_code"`
	LedgerBalance    *decimal.Decimal `json:"ledger_balance"`
	SourceBalance    decimal.Decimal  `json:"source_balance"`
	Difference       *decimal.Decimal `json:"difference"`
	Result           string           `json:"result"`
	BAAmount         decimal.Decimal  `json:"ba_amount"`
	BACount          int              `json:"ba_count"`
	BSAmount         decimal.Decimal  `json:"bs_amount"`
	BSCount          int              `json:"bs_count"`
	DeclaredBAAmount *decimal.Decimal `json:"declared_ba_amount"`
	DeclaredBACount  *int             `json:"declared_ba_count"`
	DeclaredBSAmount *decimal.Decimal `json:"declared_bs_amount"`
	DeclaredBSCount  *int             `json:"declared_bs_count"`
	BAResult         string           `json:"ba_result"`
	BSResult         string           `json:"bs_result"`
	Status           string           `json:"status"`
	Note             *string          `json:"note"`
}

// CounterpartyInput is what the counterparty comparison reads.
type CounterpartyInput struct {
	// Invoices: tax id → name, invoice balance, BA, BS.
	Invoices map[string]CounterpartyInvoices
	// Paid: tax id → matched bank payments (signed).
	Paid map[string]decimal.Decimal
	// Accounts: tax id → the counterparty's account codes (cari hafızası).
	Accounts map[string][]string
	// AccountNames: account code → name (trial balance, cari hafızası).
	AccountNames map[string]string
	// Ledger: account code → trial balance balance (signed).
	Ledger map[string]decimal.Decimal
	// Declared BA / BS lines of the period: tax id → amount and count.
	DeclaredBA, DeclaredBS map[string]FormAmount
	DeclaredNames          map[string]string
	Tolerance              decimal.Decimal
}

// CounterpartyInvoices are one counterparty's invoice totals.
type CounterpartyInvoices struct {
	Name    string
	Balance decimal.Decimal
	BA, BS  FormAmount
}

// CompareCounterparties builds the counterparty rows, sorted by name.
func CompareCounterparties(in CounterpartyInput) []CounterpartyRow {
	ids := map[string]bool{}
	for id := range in.Invoices {
		ids[id] = true
	}
	for id := range in.DeclaredBA {
		ids[id] = true
	}
	for id := range in.DeclaredBS {
		ids[id] = true
	}
	for id, codes := range in.Accounts {
		for _, c := range codes {
			if b, ok := in.Ledger[c]; ok && !b.IsZero() {
				ids[id] = true
			}
		}
	}
	out := make([]CounterpartyRow, 0, len(ids))
	for id := range ids {
		inv := in.Invoices[id]
		r := CounterpartyRow{
			TaxID: id, Name: inv.Name, Status: StatusOpen,
			SourceBalance: inv.Balance.Add(in.Paid[id]).Round(2),
			BAAmount:      inv.BA.Amount.Round(2), BACount: inv.BA.Count,
			BSAmount: inv.BS.Amount.Round(2), BSCount: inv.BS.Count,
		}
		if codes := in.Accounts[id]; len(codes) > 0 {
			sorted := append([]string(nil), codes...)
			sort.Strings(sorted)
			code := strings.Join(sorted, ", ")
			r.AccountCode = &code
			bal := decimal.Zero
			for _, c := range sorted {
				bal = bal.Add(in.Ledger[c])
				if r.Name == "" {
					r.Name = in.AccountNames[c]
				}
			}
			bal = bal.Round(2)
			diff := bal.Sub(r.SourceBalance).Round(2)
			r.LedgerBalance, r.Difference = &bal, &diff
			r.Result = CPDifference
			if diff.Abs().LessThanOrEqual(in.Tolerance) {
				r.Result, r.Status = CPMatched, StatusResolved
			}
		} else {
			r.Result = CPNoLedgerAccount
		}
		if r.Name == "" {
			r.Name = in.DeclaredNames[id]
		}
		var ba, bs *FormAmount
		if v, ok := in.DeclaredBA[id]; ok {
			ba = &v
			amt, n := v.Amount, v.Count
			r.DeclaredBAAmount, r.DeclaredBACount = &amt, &n
		}
		if v, ok := in.DeclaredBS[id]; ok {
			bs = &v
			amt, n := v.Amount, v.Count
			r.DeclaredBSAmount, r.DeclaredBSCount = &amt, &n
		}
		r.BAResult = BABSResult(inv.BA, ba, len(in.DeclaredBA) > 0)
		r.BSResult = BABSResult(inv.BS, bs, len(in.DeclaredBS) > 0)
		if r.Status == StatusResolved && (BABSIssue(r.BAResult) || BABSIssue(r.BSResult)) {
			r.Status = StatusOpen
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TaxID < out[j].TaxID
	})
	return out
}

// counterpartyStep runs inside the run's transaction (Service.afterRun).
func (s *Service) counterpartyStep(ctx context.Context, q *db.Queries, sc engagementctx.Scope, run db.ReconciliationRun, c collected) error {
	_, end := periodBounds(c.windowFrom, c.windowTo)
	dateTo := pgtype.Date{Time: end, Valid: true}
	in := CounterpartyInput{
		Invoices: map[string]CounterpartyInvoices{}, Paid: map[string]decimal.Decimal{}, Accounts: map[string][]string{},
		AccountNames: map[string]string{}, Ledger: map[string]decimal.Decimal{},
		DeclaredBA: map[string]FormAmount{}, DeclaredBS: map[string]FormAmount{}, DeclaredNames: map[string]string{},
		Tolerance: dec(run.Tolerance),
	}
	inv, err := q.ReconciliationCounterpartyInvoices(ctx, db.ReconciliationCounterpartyInvoicesParams{TaxpayerID: sc.TaxpayerID, DateTo: dateTo, Period: run.Period})
	if err != nil {
		return err
	}
	for _, r := range inv {
		in.Invoices[r.TaxID] = CounterpartyInvoices{Name: r.Name, Balance: dec(r.InvoiceBalance),
			BA: FormAmount{Amount: dec(r.BaAmount), Count: int(r.BaCount)}, BS: FormAmount{Amount: dec(r.BsAmount), Count: int(r.BsCount)}}
	}
	paid, err := q.ReconciliationCounterpartyPayments(ctx, db.ReconciliationCounterpartyPaymentsParams{TaxpayerID: sc.TaxpayerID, DateTo: dateTo})
	if err != nil {
		return err
	}
	for _, p := range paid {
		in.Paid[p.TaxID] = dec(p.Paid)
	}
	accts, err := q.ReconciliationCounterpartyAccounts(ctx, db.ReconciliationCounterpartyAccountsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	if err != nil {
		return err
	}
	for _, a := range accts {
		in.Accounts[a.CounterpartyTaxID] = append(in.Accounts[a.CounterpartyTaxID], a.AccountCode)
		if a.AccountName != "" {
			in.AccountNames[a.AccountCode] = a.AccountName
		}
	}
	for _, l := range c.current {
		in.Ledger[l.Code] = l.Debit.Sub(l.Credit)
		if l.Name != "" {
			in.AccountNames[l.Code] = l.Name
		}
	}
	decl, err := q.ListBabsDeclarations(ctx, db.ListBabsDeclarationsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: run.Period})
	if err != nil {
		return err
	}
	for _, d := range decl {
		fa := FormAmount{Amount: dec(d.Amount), Count: int(d.DocumentCount)}
		if d.FormType == "ba" {
			in.DeclaredBA[d.CounterpartyTaxID] = fa
		} else {
			in.DeclaredBS[d.CounterpartyTaxID] = fa
		}
		if d.CounterpartyName != "" {
			in.DeclaredNames[d.CounterpartyTaxID] = d.CounterpartyName
		}
	}
	rows := CompareCounterparties(in)

	carried := map[string]db.ListCounterpartyReconciliationsRow{}
	if prev, err := q.PreviousReconciliationRun(ctx, db.PreviousReconciliationRunParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: run.Period, BeforeID: run.ID,
	}); err == nil {
		old, err := q.ListCounterpartyReconciliations(ctx, db.ListCounterpartyReconciliationsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: prev.ID})
		if err != nil {
			return err
		}
		for _, o := range old {
			carried[o.CounterpartyTaxID] = o
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var sm Summary
	_ = json.Unmarshal(run.Summary, &sm)
	sm.Counterparties, sm.CounterpartyDifferences, sm.BABSIssues = len(rows), 0, 0
	for i := range rows {
		r := &rows[i]
		if o, ok := carried[r.TaxID]; ok && r.Difference != nil && o.Difference.Valid && dec(o.Difference).Equal(*r.Difference) {
			r.Note = textPtr(o.Note)
			switch o.Status {
			case StatusExplained, StatusResolved, StatusIgnored:
				r.Status = o.Status
			}
		}
		if r.Result == CPDifference {
			sm.CounterpartyDifferences++
		}
		if BABSIssue(r.BAResult) || BABSIssue(r.BSResult) {
			sm.BABSIssues++
		}
	}
	body, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if _, err := q.InsertCounterpartyReconciliations(ctx, db.InsertCounterpartyReconciliationsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: run.ID, Rows: body,
	}); err != nil {
		return err
	}
	summary, err := json.Marshal(sm)
	if err != nil {
		return err
	}
	return q.UpdateReconciliationRunSummary(ctx, db.UpdateReconciliationRunSummaryParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: run.ID, Summary: summary,
	})
}

// ---- views ----

// CounterpartyView is the API view of a counterparty reconciliation.
type CounterpartyView struct {
	UUID             uuid.UUID  `json:"uuid"`
	TaxID            string     `json:"counterparty_tax_id"`
	Name             string     `json:"counterparty_name"`
	AccountCode      *string    `json:"account_code"`
	LedgerBalance    *string    `json:"ledger_balance"`
	SourceBalance    string     `json:"source_balance"`
	Difference       *string    `json:"difference"`
	Result           string     `json:"result"`
	BAAmount         string     `json:"ba_amount"`
	BACount          int        `json:"ba_count"`
	BSAmount         string     `json:"bs_amount"`
	BSCount          int        `json:"bs_count"`
	DeclaredBAAmount *string    `json:"declared_ba_amount"`
	DeclaredBACount  *int       `json:"declared_ba_count"`
	DeclaredBSAmount *string    `json:"declared_bs_amount"`
	DeclaredBSCount  *int       `json:"declared_bs_count"`
	BAResult         string     `json:"ba_result"`
	BSResult         string     `json:"bs_result"`
	Status           string     `json:"status"`
	Note             *string    `json:"note"`
	Review           *ReviewRef `json:"review"`
	DecidedAt        *time.Time `json:"decided_at"`
	Email            *string    `json:"email"`
	LastLetter       *LetterRef `json:"last_letter"`
}

// LetterRef is the newest letter of a counterparty.
type LetterRef struct {
	UUID     uuid.UUID `json:"uuid"`
	Status   string    `json:"status"`
	Response *string   `json:"response"`
	SentAt   *string   `json:"sent_at"`
}

// CounterpartyPage is a run's counterparties.
type CounterpartyPage struct {
	Run   Run                `json:"run"`
	Items []CounterpartyView `json:"items"`
}

func numStr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v := dec(n).StringFixed(2)
	return &v
}

func intPtr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}

func counterpartyView(c db.CounterpartyReconciliation, reviewUUID pgtype.UUID, reviewStatus pgtype.Text) CounterpartyView {
	v := CounterpartyView{
		UUID: c.Uuid, TaxID: c.CounterpartyTaxID, Name: c.CounterpartyName, AccountCode: textPtr(c.AccountCode),
		LedgerBalance: numStr(c.LedgerBalance), SourceBalance: dec(c.SourceBalance).StringFixed(2), Difference: numStr(c.Difference),
		Result: c.Result, BAAmount: dec(c.BaAmount).StringFixed(2), BACount: int(c.BaCount), BSAmount: dec(c.BsAmount).StringFixed(2),
		BSCount: int(c.BsCount), DeclaredBAAmount: numStr(c.DeclaredBaAmount), DeclaredBACount: intPtr(c.DeclaredBaCount),
		DeclaredBSAmount: numStr(c.DeclaredBsAmount), DeclaredBSCount: intPtr(c.DeclaredBsCount),
		BAResult: c.BaResult, BSResult: c.BsResult, Status: EffectiveStatus(c.Status, reviewStatus), Note: textPtr(c.Note),
	}
	if reviewUUID.Valid {
		v.Review = &ReviewRef{UUID: reviewUUID.Bytes, Status: reviewStatus.String}
	}
	if c.DecidedAt.Valid {
		t := c.DecidedAt.Time
		v.DecidedAt = &t
	}
	return v
}

// Counterparties lists a run's counterparty rows ("latest" or a uuid) with
// the contact email and the newest letter of each.
func (s *Service) Counterparties(ctx context.Context, sc engagementctx.Scope, which, period string) (CounterpartyPage, error) {
	run, err := s.pickRun(ctx, sc, which, period)
	if err != nil {
		return CounterpartyPage{}, err
	}
	rows, err := s.q.ListCounterpartyReconciliations(ctx, db.ListCounterpartyReconciliationsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, RunID: run.ID})
	if err != nil {
		return CounterpartyPage{}, err
	}
	contacts, err := s.q.ReconciliationCounterpartyContacts(ctx, sc.TaxpayerID)
	if err != nil {
		return CounterpartyPage{}, err
	}
	emails := map[string]string{}
	for _, c := range contacts {
		if c.Email != "" {
			emails[c.TaxID] = c.Email
		}
	}
	letters, err := s.q.ListReconciliationLetters(ctx, db.ListReconciliationLettersParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Lim: 1000})
	if err != nil {
		return CounterpartyPage{}, err
	}
	last := map[string]db.ReconciliationLetter{}
	for _, l := range letters { // newest first
		if _, ok := last[l.CounterpartyTaxID]; !ok {
			last[l.CounterpartyTaxID] = l
		}
	}
	out := CounterpartyPage{Run: run, Items: make([]CounterpartyView, 0, len(rows))}
	for _, r := range rows {
		v := counterpartyView(db.CounterpartyReconciliation{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, RunID: r.RunID,
			CounterpartyTaxID: r.CounterpartyTaxID, CounterpartyName: r.CounterpartyName, AccountCode: r.AccountCode,
			LedgerBalance: r.LedgerBalance, SourceBalance: r.SourceBalance, Difference: r.Difference, Result: r.Result,
			BaAmount: r.BaAmount, BaCount: r.BaCount, BsAmount: r.BsAmount, BsCount: r.BsCount,
			DeclaredBaAmount: r.DeclaredBaAmount, DeclaredBaCount: r.DeclaredBaCount, DeclaredBsAmount: r.DeclaredBsAmount,
			DeclaredBsCount: r.DeclaredBsCount, BaResult: r.BaResult, BsResult: r.BsResult, Status: r.Status, Note: r.Note,
			ReviewSuggestionID: r.ReviewSuggestionID, DecidedAt: r.DecidedAt,
		}, r.ReviewUuid, r.ReviewStatus)
		if e, ok := emails[r.CounterpartyTaxID]; ok {
			v.Email = &e
		}
		if l, ok := last[r.CounterpartyTaxID]; ok {
			v.LastLetter = &LetterRef{UUID: l.Uuid, Status: l.Status, Response: textPtr(l.Response)}
			if l.SentAt.Valid {
				t := l.SentAt.Time.Format(time.RFC3339)
				v.LastLetter.SentAt = &t
			}
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

func (s *Service) counterpartyByUUID(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.GetCounterpartyReconciliationRow, error) {
	c, err := s.q.GetCounterpartyReconciliation(ctx, db.GetCounterpartyReconciliationParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func cpFromGet(c db.GetCounterpartyReconciliationRow) db.CounterpartyReconciliation {
	return db.CounterpartyReconciliation{
		ID: c.ID, Uuid: c.Uuid, OrganizationID: c.OrganizationID, EngagementID: c.EngagementID, RunID: c.RunID,
		CounterpartyTaxID: c.CounterpartyTaxID, CounterpartyName: c.CounterpartyName, AccountCode: c.AccountCode,
		LedgerBalance: c.LedgerBalance, SourceBalance: c.SourceBalance, Difference: c.Difference, Result: c.Result,
		BaAmount: c.BaAmount, BaCount: c.BaCount, BsAmount: c.BsAmount, BsCount: c.BsCount,
		DeclaredBaAmount: c.DeclaredBaAmount, DeclaredBaCount: c.DeclaredBaCount, DeclaredBsAmount: c.DeclaredBsAmount,
		DeclaredBsCount: c.DeclaredBsCount, BaResult: c.BaResult, BsResult: c.BsResult, Status: c.Status, Note: c.Note,
		ReviewSuggestionID: c.ReviewSuggestionID, DecidedAt: c.DecidedAt,
	}
}

// DecideCounterparty sets a counterparty row's status and note.
func (s *Service) DecideCounterparty(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, in DecisionInput) (CounterpartyView, error) {
	if !ValidDecision(in.Status) {
		return CounterpartyView{}, fmt.Errorf("%w: status must be open, explained, resolved or ignored", ErrInvalid)
	}
	note, err := cleanNote(in.Note)
	if err != nil {
		return CounterpartyView{}, err
	}
	if in.Status == StatusExplained && note == nil {
		return CounterpartyView{}, fmt.Errorf("%w: an explanation needs a note", ErrInvalid)
	}
	c, err := s.counterpartyByUUID(ctx, sc, id)
	if err != nil {
		return CounterpartyView{}, err
	}
	if c.Status == StatusInReview && c.ReviewStatus.String == string(reviewusecase.StatusPending) {
		return CounterpartyView{}, ErrInReview
	}
	row, err := s.q.UpdateCounterpartyReconciliationDecision(ctx, db.UpdateCounterpartyReconciliationDecisionParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: c.ID, Status: in.Status, Note: ptrText(note), DecidedBy: optID(userID),
	})
	if err != nil {
		return CounterpartyView{}, err
	}
	return counterpartyView(row, c.ReviewUuid, c.ReviewStatus), nil
}

// CounterpartyExplanation is the default explanation of a counterparty row.
func CounterpartyExplanation(c db.CounterpartyReconciliation) string {
	var parts []string
	switch c.Result {
	case CPDifference:
		parts = append(parts, fmt.Sprintf("Cari bakiye mizanda %s, portal kayıtlarında %s: fark %s. Açılış bakiyesi, eşleşmemiş ödeme ya da portala yüklenmemiş fatura olabilir.",
			Money(dec(c.LedgerBalance)), Money(dec(c.SourceBalance)), Money(dec(c.Difference).Abs())))
	case CPNoLedgerAccount:
		parts = append(parts, "Karşı firmanın mizandaki cari hesabı bilinmiyor; cari hafızasında 120 / 320 alt hesabı eşleştirilmeli.")
	}
	for _, f := range []struct{ form, result string }{{"BA", c.BaResult}, {"BS", c.BsResult}} {
		switch f.result {
		case BABSMissingInForm:
			parts = append(parts, fmt.Sprintf("Form %s'de bu firma yok, faturalar %s TL eşiğini aşıyor.", f.form, BABSThreshold.String()))
		case BABSExtraInForm:
			parts = append(parts, fmt.Sprintf("Form %s'de bu firma var, faturalar eşiğin altında.", f.form))
		case BABSAmountMismatch:
			parts = append(parts, fmt.Sprintf("Form %s tutarı faturalarla tutmuyor.", f.form))
		case BABSCountMismatch:
			parts = append(parts, fmt.Sprintf("Form %s belge sayısı faturalarla tutmuyor.", f.form))
		}
	}
	if len(parts) == 0 {
		return "Cari mutabakat incelensin."
	}
	return strings.Join(parts, " ")
}

// SendCounterpartyToReview puts a counterparty row into the review queue.
func (s *Service) SendCounterpartyToReview(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, note *string) (CounterpartyView, error) {
	if s.review == nil {
		return CounterpartyView{}, ErrNoReview
	}
	n, err := cleanNote(note)
	if err != nil {
		return CounterpartyView{}, err
	}
	c, err := s.counterpartyByUUID(ctx, sc, id)
	if err != nil {
		return CounterpartyView{}, err
	}
	row := cpFromGet(c)
	if c.Result == CPMatched && !BABSIssue(c.BaResult) && !BABSIssue(c.BsResult) {
		return CounterpartyView{}, ErrMatched
	}
	if c.ReviewStatus.Valid && c.ReviewStatus.String == string(reviewusecase.StatusPending) {
		return CounterpartyView{}, ErrInReview
	}
	explanation := CounterpartyExplanation(row)
	if n != nil {
		explanation = *n
	}
	value, _ := json.Marshal(ReviewValue{Status: StatusResolved, Note: explanation})
	cid := c.ID
	sug, err := s.review.Submit(ctx, reviewusecase.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID},
		reviewusecase.Input{
			Kind: SuggestionKind, TargetType: TargetCounterparty, TargetID: &cid, TargetField: TargetField, Period: c.RunPeriod,
			Value: value, Confidence: decimal.NewFromFloat(0.3),
			Rationale: fmt.Sprintf("Cari mutabakat: %s (%s). %s", c.CounterpartyName, c.CounterpartyTaxID, explanation),
			Provenance: reviewusecase.Provenance{
				Origin: reviewusecase.OriginRule, RuleRef: "reconciliation:counterparty",
				Inputs: map[string]any{
					"run": c.RunUuid.String(), "counterparty_tax_id": c.CounterpartyTaxID, "result": c.Result,
					"ba_result": c.BaResult, "bs_result": c.BsResult, "source_balance": dec(c.SourceBalance).StringFixed(2),
				},
			},
		})
	if err != nil {
		return CounterpartyView{}, err
	}
	if !sug.Status.Applied() {
		if err := s.q.SetCounterpartyReconciliationReview(ctx, db.SetCounterpartyReconciliationReviewParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: c.ID, ReviewSuggestionID: pgtype.Int8{Int64: sug.ID, Valid: true}, DecidedBy: optID(userID),
		}); err != nil {
			return CounterpartyView{}, err
		}
	}
	c, err = s.counterpartyByUUID(ctx, sc, id)
	if err != nil {
		return CounterpartyView{}, err
	}
	return counterpartyView(cpFromGet(c), c.ReviewUuid, c.ReviewStatus), nil
}

func init() {
	applyCounterparty = func(ctx context.Context, q *db.Queries, sug reviewusecase.Suggestion, status string, note pgtype.Text) (json.RawMessage, error) {
		c, err := q.GetCounterpartyReconciliationByID(ctx, db.GetCounterpartyReconciliationByIDParams{
			OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, ID: *sug.Input.TargetID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: the counterparty reconciliation is gone", reviewusecase.ErrInvalidValue)
		}
		if err != nil {
			return nil, err
		}
		prev := previousValue(c.Status, c.Note)
		if _, err := q.UpdateCounterpartyReconciliationDecision(ctx, db.UpdateCounterpartyReconciliationDecisionParams{
			OrganizationID: sug.OrganizationID, EngagementID: sug.EngagementID, ID: c.ID, Status: status, Note: note, DecidedBy: decider(sug),
		}); err != nil {
			return nil, err
		}
		return prev, nil
	}
}
