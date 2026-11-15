package banking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

var (
	ErrInvalidTransaction = errors.New("banking: invalid transaction")
	ErrNotFound           = errors.New("banking: not found")
	ErrInvalidKind        = errors.New("banking: invalid kind or kind_detail")
)

// Service writes bank movements (Ingest), runs the classification pipeline
// over them (Classify) and records the accountant's corrections (Correct).
type Service struct {
	pool *pgxpool.Pool
	cl   Classifier
	ai   Suggester
}

// NewService builds the service. ai may be nil: the pipeline then stops at
// the keyword step and leaves the rest as unknown.
func NewService(pool *pgxpool.Pool, ai Suggester) *Service {
	return &Service{pool: pool, ai: ai}
}

// IngestResult counts what Ingest did with a batch.
type IngestResult struct {
	Inserted int
	// Merged: the movement already existed through another channel; only a
	// bank_transaction_sources row was added (21 §5 rule 2).
	Merged int
	// Skipped: same channel and same source key, nothing new (rule 1).
	Skipped int
	// PossibleDuplicates: stored but flagged for the accountant (rule 3).
	PossibleDuplicates int
	// IDs of the inserted movements, for classification.
	InsertedIDs []int64
}

// Ingest writes one batch of movements of a cash box inside one database
// transaction. importID ties the rows to a file upload so it can be undone
// (#19). Inserted movements are classified right away.
func (s *Service) Ingest(ctx context.Context, taxpayerID, cashBoxID int64, importID *uuid.UUID, txns []Transaction) (IngestResult, error) {
	var res IngestResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		res, err = s.IngestTx(ctx, tx, taxpayerID, cashBoxID, importID, txns)
		return err
	})
	if err != nil {
		return IngestResult{}, err
	}
	if err := s.ClassifyInserted(ctx, taxpayerID, res); err != nil {
		return res, err
	}
	return res, nil
}

// IngestTx is Ingest inside the caller's transaction, without classifying:
// a preview runs it and rolls back (23 §2 step 4), an import commits and then
// calls ClassifyInserted.
func (s *Service) IngestTx(ctx context.Context, tx pgx.Tx, taxpayerID, cashBoxID int64, importID *uuid.UUID, txns []Transaction) (IngestResult, error) {
	var res IngestResult
	q := db.New(tx)
	box, err := q.GetCashBox(ctx, db.GetCashBoxParams{TaxpayerID: taxpayerID, ID: cashBoxID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return res, ErrNotFound
		}
		return res, err
	}
	for i := range txns {
		if err := s.ingestOne(ctx, q, taxpayerID, box, importID, txns[i], &res); err != nil {
			return res, fmt.Errorf("movement %d: %w", i+1, err)
		}
	}
	return res, nil
}

// ClassifyInserted classifies what an ingest added.
func (s *Service) ClassifyInserted(ctx context.Context, taxpayerID int64, res IngestResult) error {
	if len(res.InsertedIDs) == 0 {
		return nil
	}
	_, err := s.classifyIDs(ctx, taxpayerID, res.InsertedIDs)
	return err
}

// UndoImport removes one file upload (21 §2): its source rows go; a movement
// left without any source is deleted; one that keeps another channel gets
// its primary source re-picked by priority. It returns the deleted count.
func (s *Service) UndoImport(ctx context.Context, taxpayerID int64, importID uuid.UUID) (deleted int, err error) {
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		imp := pgtype.UUID{Bytes: importID, Valid: true}
		ids, err := q.DeleteBankTransactionSourcesByImport(ctx, db.DeleteBankTransactionSourcesByImportParams{TaxpayerID: taxpayerID, ImportID: imp})
		if err != nil {
			return err
		}
		if err := q.DeleteBankBalanceSnapshotsByImport(ctx, db.DeleteBankBalanceSnapshotsByImportParams{TaxpayerID: taxpayerID, ImportID: imp}); err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			left, err := q.ListBankTransactionSources(ctx, db.ListBankTransactionSourcesParams{TaxpayerID: taxpayerID, BankTransactionID: id})
			if err != nil {
				return err
			}
			if len(left) == 0 {
				if err := q.ClearPossibleDuplicateLinks(ctx, db.ClearPossibleDuplicateLinksParams{TaxpayerID: taxpayerID, ID: pgtype.Int8{Int64: id, Valid: true}}); err != nil {
					return err
				}
				if err := q.DeleteBankTransaction(ctx, db.DeleteBankTransactionParams{TaxpayerID: taxpayerID, ID: id}); err != nil {
					return err
				}
				deleted++
				continue
			}
			best := left[0]
			for _, l := range left[1:] {
				if CapturePriority(l.CaptureMethod) > CapturePriority(best.CaptureMethod) {
					best = l
				}
			}
			if err := q.SetBankTransactionPrimarySource(ctx, db.SetBankTransactionPrimarySourceParams{
				SourceSystem: best.SourceSystem, SourceID: best.SourceID, CaptureMethod: best.CaptureMethod,
				RawPayload: best.RawPayload, TaxpayerID: taxpayerID, ID: id,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return deleted, err
}

type normalized struct {
	params db.InsertBankTransactionParams
	raw    []byte
}

func (s *Service) normalize(taxpayerID int64, box db.CashBox, t Transaction) (normalized, error) {
	if t.Direction != DirIn && t.Direction != DirOut {
		return normalized{}, fmt.Errorf("%w: direction must be A or B", ErrInvalidTransaction)
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(t.Amount))
	if err != nil || amount.IsNegative() {
		return normalized{}, fmt.Errorf("%w: amount must be a positive decimal", ErrInvalidTransaction)
	}
	if t.TransactionDate.IsZero() {
		return normalized{}, fmt.Errorf("%w: transaction_date is required", ErrInvalidTransaction)
	}
	if !strings.HasPrefix(t.SourceSystem, "bank_") {
		return normalized{}, fmt.Errorf("%w: source_system must be bank_*", ErrInvalidTransaction)
	}
	if CapturePriority(t.CaptureMethod) == 0 {
		return normalized{}, fmt.Errorf("%w: unknown capture_method %q", ErrInvalidTransaction, t.CaptureMethod)
	}
	booked := t.BookedAt
	if booked.IsZero() {
		booked = time.Date(t.TransactionDate.Year(), t.TransactionDate.Month(), t.TransactionDate.Day(), 0, 0, 0, 0, istanbul)
	}
	balAfter, err := optDecimal(t.BalanceAfter)
	if err != nil {
		return normalized{}, fmt.Errorf("%w: balance_after: %v", ErrInvalidTransaction, err)
	}
	balBefore, err := optDecimal(t.BalanceBefore)
	if err != nil {
		return normalized{}, fmt.Errorf("%w: balance_before: %v", ErrInvalidTransaction, err)
	}
	raw := t.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return normalized{}, err
	}
	vkn, tckn := SplitTaxID(t.CounterpartyTaxID)
	sourceID := strings.TrimSpace(t.SourceID)
	if sourceID == "" {
		// 20 §7.3: sha256(iban | tarih-saat | tutar | yön | yürüyen bakiye | açıklama)
		iban := ""
		if box.Iban.Valid {
			iban = box.Iban.String
		}
		bal := ""
		if balAfter != nil {
			bal = balAfter.String()
		}
		sum := sha256.Sum256([]byte(strings.Join([]string{iban, booked.UTC().Format(time.RFC3339), amount.String(),
			string(t.Direction), bal, t.Description}, "|")))
		sourceID = "sha256:" + hex.EncodeToString(sum[:])
	}
	p := db.InsertBankTransactionParams{
		TaxpayerID:                 taxpayerID,
		CashBoxID:                  box.ID,
		SourceSystem:               t.SourceSystem,
		SourceID:                   sourceID,
		CaptureMethod:              t.CaptureMethod,
		BookedAt:                   pgtype.Timestamptz{Time: booked, Valid: true},
		HasTime:                    t.HasTime,
		TransactionDate:            pgtype.Date{Time: t.TransactionDate, Valid: true},
		DebitCredit:                string(t.Direction),
		Amount:                     numeric(amount),
		Currency:                   NormalizeCurrency(t.Currency),
		Description:                t.Description,
		DescriptionExtra:           text(t.DescriptionExtra),
		DescriptionNormalized:      NormalizeDescription(t.Description + " " + t.DescriptionExtra),
		BankReference:              text(t.BankReference),
		ReceiptNumber:              text(t.ReceiptNumber),
		BankTxnCode:                text(t.BankTxnCode),
		Mt940Code:                  text(strings.ToUpper(t.MT940Code)),
		Channel:                    text(t.Channel),
		CounterpartyName:           text(t.CounterpartyName),
		CounterpartyIban:           text(NormalizeIBAN(t.CounterpartyIBAN)),
		CounterpartyTaxNumber:      text(vkn),
		CounterpartyIdentityNumber: text(tckn),
		CounterpartyBankCode:       text(t.CounterpartyBankCode),
		EftQueryNumber:             text(t.EFTQueryNumber),
		PaymentPurpose:             text(strings.ToUpper(t.PaymentPurpose)),
		IsCancelled:                t.IsCancelled,
		Period:                     t.TransactionDate.Format("2006-01"),
		RawPayload:                 rawJSON,
	}
	if t.BankSequence != nil {
		p.BankSequence = pgtype.Int4{Int32: *t.BankSequence, Valid: true}
	}
	if t.ValueDate != nil {
		p.ValueDate = pgtype.Date{Time: *t.ValueDate, Valid: true}
	}
	if balAfter != nil {
		p.BalanceAfter = numeric(*balAfter)
	}
	if balBefore != nil {
		p.BalanceBefore = numeric(*balBefore)
	}
	return normalized{params: p, raw: rawJSON}, nil
}

func (s *Service) ingestOne(ctx context.Context, q *db.Queries, taxpayerID int64, box db.CashBox, importID *uuid.UUID, t Transaction, res *IngestResult) error {
	n, err := s.normalize(taxpayerID, box, t)
	if err != nil {
		return err
	}
	p := n.params
	source := db.InsertBankTransactionSourceParams{
		TaxpayerID: taxpayerID, CashBoxID: box.ID, SourceSystem: p.SourceSystem,
		CaptureMethod: p.CaptureMethod, SourceID: p.SourceID, RawPayload: n.raw,
	}
	if importID != nil {
		source.ImportID = pgtype.UUID{Bytes: *importID, Valid: true}
	}

	// Rule 1: same cash box + channel + source key → same record.
	if _, err := q.FindBankTransactionBySource(ctx, db.FindBankTransactionBySourceParams{
		TaxpayerID: taxpayerID, CashBoxID: box.ID, SourceSystem: p.SourceSystem, SourceID: p.SourceID,
	}); err == nil {
		res.Skipped++
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// Rule 2: same cash box + date + direction + amount + running balance.
	if p.BalanceAfter.Valid {
		existing, err := q.FindBankTransactionByBalance(ctx, db.FindBankTransactionByBalanceParams{
			TaxpayerID: taxpayerID, CashBoxID: box.ID, TransactionDate: p.TransactionDate,
			DebitCredit: p.DebitCredit, Amount: p.Amount, BalanceAfter: p.BalanceAfter,
		})
		if err == nil {
			return s.merge(ctx, q, existing, p, source, res)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	} else {
		// Rule 3: no balance → same date + direction + amount + first 30
		// characters of the normalized description is a possible duplicate.
		prefix := p.DescriptionNormalized
		if len(prefix) > 30 {
			prefix = prefix[:30]
		}
		existing, err := q.FindBankTransactionLookalike(ctx, db.FindBankTransactionLookalikeParams{
			TaxpayerID: taxpayerID, CashBoxID: box.ID, TransactionDate: p.TransactionDate,
			DebitCredit: p.DebitCredit, Amount: p.Amount, DescriptionPrefix: prefix,
		})
		if err == nil {
			p.PossibleDuplicateOf = pgtype.Int8{Int64: existing.ID, Valid: true}
			res.PossibleDuplicates++
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}

	row, err := q.InsertBankTransaction(ctx, p)
	if err != nil {
		return err
	}
	source.BankTransactionID = row.ID
	if err := q.InsertBankTransactionSource(ctx, source); err != nil {
		return err
	}
	res.Inserted++
	res.InsertedIDs = append(res.InsertedIDs, row.ID)
	return nil
}

// merge adds a channel to an existing movement. A higher-priority channel
// becomes the primary source; a lower one only fills empty fields (21 §5).
func (s *Service) merge(ctx context.Context, q *db.Queries, existing db.BankTransaction, p db.InsertBankTransactionParams, source db.InsertBankTransactionSourceParams, res *IngestResult) error {
	source.BankTransactionID = existing.ID
	if err := q.InsertBankTransactionSource(ctx, source); err != nil {
		return err
	}
	if CapturePriority(p.CaptureMethod) > CapturePriority(existing.CaptureMethod) {
		if err := q.SetBankTransactionPrimarySource(ctx, db.SetBankTransactionPrimarySourceParams{
			SourceSystem: p.SourceSystem, SourceID: p.SourceID, CaptureMethod: p.CaptureMethod,
			RawPayload: p.RawPayload, TaxpayerID: existing.TaxpayerID, ID: existing.ID,
		}); err != nil {
			return err
		}
	}
	if err := q.FillBankTransactionGaps(ctx, db.FillBankTransactionGapsParams{
		DescriptionExtra: p.DescriptionExtra, BankReference: p.BankReference, ReceiptNumber: p.ReceiptNumber,
		BankTxnCode: p.BankTxnCode, Mt940Code: p.Mt940Code, CounterpartyName: p.CounterpartyName,
		CounterpartyIban: p.CounterpartyIban, CounterpartyTaxNumber: p.CounterpartyTaxNumber,
		CounterpartyIdentityNumber: p.CounterpartyIdentityNumber, PaymentPurpose: p.PaymentPurpose,
		BalanceAfter: p.BalanceAfter, TaxpayerID: existing.TaxpayerID, ID: existing.ID,
	}); err != nil {
		return err
	}
	res.Merged++
	return nil
}

// ClassifyResult counts a classification run.
type ClassifyResult struct {
	Total       int
	NeedsReview int
}

// Classify (re)classifies the taxpayer's movements, optionally one period
// and only those never classified. Manual corrections are never overwritten.
func (s *Service) Classify(ctx context.Context, taxpayerID int64, period string, onlyUnclassified bool) (ClassifyResult, error) {
	q := db.New(s.pool)
	rows, err := q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{
		TaxpayerID: taxpayerID, Period: text(period), OnlyUnclassified: onlyUnclassified,
	})
	if err != nil {
		return ClassifyResult{}, err
	}
	return s.classifyRows(ctx, q, taxpayerID, rows)
}

func (s *Service) classifyIDs(ctx context.Context, taxpayerID int64, ids []int64) (ClassifyResult, error) {
	q := db.New(s.pool)
	rows := make([]db.BankTransaction, 0, len(ids))
	for _, id := range ids {
		r, err := q.GetBankTransaction(ctx, db.GetBankTransactionParams{TaxpayerID: taxpayerID, ID: id})
		if err != nil {
			return ClassifyResult{}, err
		}
		rows = append(rows, r)
	}
	return s.classifyRows(ctx, q, taxpayerID, rows)
}

func (s *Service) classifyRows(ctx context.Context, q *db.Queries, taxpayerID int64, rows []db.BankTransaction) (ClassifyResult, error) {
	var out ClassifyResult
	if len(rows) == 0 {
		return out, nil
	}
	cctx, err := s.loadContext(ctx, q, taxpayerID)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		c := s.cl.Classify(ctx, fromRow(r), cctx)
		params := db.SetBankTransactionClassificationParams{
			Kind: string(c.Kind), KindDetail: string(c.Detail), KindReason: text(c.Reason),
			TaxpayerID: taxpayerID, ID: r.ID,
		}
		if c.Source != "" {
			params.KindSource = text(string(c.Source))
			params.KindConfidence = numeric(decimal.NewFromFloat(c.Confidence).Round(2))
		}
		if err := q.SetBankTransactionClassification(ctx, params); err != nil {
			return out, err
		}
		if c.RuleID != 0 {
			if err := q.IncrementBankRuleHits(ctx, db.IncrementBankRuleHitsParams{TaxpayerID: taxpayerID, ID: c.RuleID}); err != nil {
				return out, err
			}
		}
		out.Total++
		if c.NeedsReview() {
			out.NeedsReview++
		}
	}
	return out, nil
}

func (s *Service) loadContext(ctx context.Context, q *db.Queries, taxpayerID int64) (Context, error) {
	c := Context{OwnIBANs: map[string]bool{}, AI: s.ai}
	boxes, err := q.ListCashBoxes(ctx, taxpayerID)
	if err != nil {
		return c, err
	}
	for _, b := range boxes {
		if b.Iban.Valid {
			c.OwnIBANs[b.Iban.String] = true
		}
	}
	rules, err := q.ListBankRules(ctx, taxpayerID)
	if err != nil {
		return c, err
	}
	for _, r := range rules {
		c.Rules = append(c.Rules, Rule{ID: r.ID, MatchType: r.MatchType, MatchValue: r.MatchValue,
			Dir: Direction(r.DebitCredit.String), Kind: Kind(r.Kind), Detail: Detail(r.KindDetail)})
	}
	customers, err := q.ListCustomersForBankClassification(ctx, taxpayerID)
	if err != nil {
		return c, err
	}
	for _, cu := range customers {
		cp := Counterparty{CustomerID: cu.ID, Type: cu.Type, TaxID: cu.TaxNumber.String + cu.IdentityNumber.String}
		var accounts []map[string]any
		if err := json.Unmarshal(cu.BankAccounts, &accounts); err == nil {
			for _, a := range accounts {
				if iban, ok := a["iban"].(string); ok {
					cp.IBANs = append(cp.IBANs, iban)
				}
			}
		}
		c.Counterparties = append(c.Counterparties, cp)
	}
	return c, nil
}

// Correct records the accountant's classification (confidence 1, source
// manual) and learns it as a bank_rules row for the same counterparty or
// description pattern (21 §4): IBAN first, then VKN/TCKN, then the
// counterparty name or the first words of the description.
func (s *Service) Correct(ctx context.Context, taxpayerID, transactionID int64, kind Kind, detail Detail, userID *int64) (db.BankRule, error) {
	if !ValidKind(kind) || !ValidDetail(detail) {
		return db.BankRule{}, ErrInvalidKind
	}
	var rule db.BankRule
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.GetBankTransaction(ctx, db.GetBankTransactionParams{TaxpayerID: taxpayerID, ID: transactionID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := q.SetBankTransactionClassification(ctx, db.SetBankTransactionClassificationParams{
			Kind: string(kind), KindDetail: string(detail), KindSource: text(string(SourceManual)),
			KindConfidence: numeric(decimal.NewFromInt(1)), KindReason: text("müşavir düzeltmesi"),
			TaxpayerID: taxpayerID, ID: transactionID,
		}); err != nil {
			return err
		}
		matchType, matchValue := learnKey(row)
		if matchValue == "" {
			return nil
		}
		params := db.UpsertBankRuleParams{
			TaxpayerID: taxpayerID, MatchType: matchType, MatchValue: matchValue,
			DebitCredit: text(row.DebitCredit), Kind: string(kind), KindDetail: string(detail),
		}
		if userID != nil {
			params.CreatedBy = pgtype.Int8{Int64: *userID, Valid: true}
		}
		rule, err = q.UpsertBankRule(ctx, params)
		return err
	})
	return rule, err
}

// learnKey picks what a correction is remembered by.
func learnKey(r db.BankTransaction) (string, string) {
	switch {
	case r.CounterpartyIban.Valid && r.CounterpartyIban.String != "":
		return "counterparty_iban", r.CounterpartyIban.String
	case r.CounterpartyTaxNumber.Valid && r.CounterpartyTaxNumber.String != "":
		return "counterparty_tax_id", r.CounterpartyTaxNumber.String
	case r.CounterpartyIdentityNumber.Valid && r.CounterpartyIdentityNumber.String != "":
		return "counterparty_tax_id", r.CounterpartyIdentityNumber.String
	case r.CounterpartyName.Valid && NormalizeDescription(r.CounterpartyName.String) != "":
		return "description", firstWords(NormalizeDescription(r.CounterpartyName.String), 3)
	}
	return "description", firstWords(r.DescriptionNormalized, 3)
}

func firstWords(s string, n int) string {
	w := strings.Fields(s)
	if len(w) > n {
		w = w[:n]
	}
	return strings.Join(w, " ")
}

func fromRow(r db.BankTransaction) Transaction {
	return Transaction{
		SourceSystem: r.SourceSystem, Direction: Direction(r.DebitCredit),
		Description: r.Description, DescriptionExtra: r.DescriptionExtra.String,
		BankTxnCode: r.BankTxnCode.String, MT940Code: r.Mt940Code.String,
		CounterpartyName: r.CounterpartyName.String, CounterpartyIBAN: r.CounterpartyIban.String,
		CounterpartyTaxID: r.CounterpartyTaxNumber.String + r.CounterpartyIdentityNumber.String,
		PaymentPurpose:    r.PaymentPurpose.String,
	}
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

func optDecimal(s *string) (*decimal.Decimal, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	d, err := decimal.NewFromString(strings.TrimSpace(*s))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func numeric(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}
