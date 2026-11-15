package statement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Producer is the data_warning producer name of this channel (05 §1:
// adapter:<source_system>).
const Producer = "adapter:bank_file"

// SourceSystem of every file channel row (21 §2).
const SourceSystem = "bank_file"

// ErrAccountRequired: the file names no account; the uploader must pick the
// cash box (23 §2 step 2: "Bulunamazsa hesap seçimi istenir").
var ErrAccountRequired = errors.New("statement: the file names no account; choose the bank account")

// UnknownAccountError: the file names an account the taxpayer has no
// cash_box for. Nothing is imported; the panel asks "add it as a new bank
// account?" (23 §3.4, §7 test 10).
type UnknownAccountError struct {
	Accounts []string
}

func (e *UnknownAccountError) Error() string {
	return "statement: account not defined for this taxpayer: " + strings.Join(e.Accounts, ", ")
}

// parseError marks a file the parsers could not read (a client error).
type parseError struct{ err error }

func (e parseError) Error() string { return e.err.Error() }
func (e parseError) Unwrap() error { return e.err }

// Request is one uploaded statement file.
type Request struct {
	TaxpayerID int64
	// OrganizationID and EngagementID scope the BNK_GAP warning (office work
	// product, #13). Zero: the taxpayer uploaded; no office warning is written.
	OrganizationID int64
	EngagementID   int64
	FileName       string
	Data           []byte
	// CashBoxID picks the account when the file does not name one.
	CashBoxID int64
	// Preview parses and dry-runs the duplicate checks without writing.
	Preview bool
}

// AccountResult is the report of one account block.
type AccountResult struct {
	Account            string     `json:"account"`
	CashBoxID          int64      `json:"-"`
	CashBoxUUID        uuid.UUID  `json:"cash_box_uuid"`
	BankID             string     `json:"bank_id,omitempty"`
	From               string     `json:"from,omitempty"`
	To                 string     `json:"to,omitempty"`
	Lines              int        `json:"lines"`
	Inserted           int        `json:"inserted"`
	Merged             int        `json:"merged"`
	Skipped            int        `json:"skipped"`
	PossibleDuplicates int        `json:"possible_duplicates"`
	TotalCredit        string     `json:"total_credit"`
	TotalDebit         string     `json:"total_debit"`
	Opening            *string    `json:"opening_balance,omitempty"`
	Closing            *string    `json:"closing_balance,omitempty"`
	Gaps               []string   `json:"gaps,omitempty"`
	Errors             []RowError `json:"errors,omitempty"`
}

// Report is what an import (or a preview) did.
type Report struct {
	ImportID uuid.UUID       `json:"import_id"`
	Format   string          `json:"format"`
	Encoding string          `json:"encoding,omitempty"`
	Preview  bool            `json:"preview"`
	Accounts []AccountResult `json:"accounts"`
}

// Importer is the statement upload flow (23 §2).
type Importer struct {
	pool *pgxpool.Pool
	bank *banking.Service
}

// NewImporter builds the importer.
func NewImporter(pool *pgxpool.Pool, bank *banking.Service) *Importer {
	return &Importer{pool: pool, bank: bank}
}

var reAccountDigits = regexp.MustCompile(`(\d{6,})\s*$`)

type resolved struct {
	st  Statement
	box db.CashBox
}

// Import parses the file, finds each block's account, checks the running
// balance and writes the movements (or only reports them when Preview).
func (im *Importer) Import(ctx context.Context, req Request) (Report, error) {
	sts, format, err := Parse(req.FileName, req.Data)
	if err != nil {
		return Report{}, parseError{err}
	}
	rep := Report{ImportID: uuid.New(), Format: format, Preview: req.Preview}
	if format == "mt940" || format == "csv" {
		_, rep.Encoding = Decode(head(req.Data, 4096))
	}

	q := db.New(im.pool)
	var blocks []resolved
	var unknown []string
	for _, st := range sts {
		box, ok, err := im.findBox(ctx, q, req, st, len(sts))
		if err != nil {
			return rep, err
		}
		if !ok {
			unknown = append(unknown, accountKey(&st))
			continue
		}
		blocks = append(blocks, resolved{st: st, box: box})
	}
	if len(unknown) > 0 {
		if len(unknown) == 1 && unknown[0] == "" {
			return rep, ErrAccountRequired
		}
		return rep, &UnknownAccountError{Accounts: unknown}
	}

	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var inserted []banking.IngestResult
	for _, b := range blocks {
		ar, res, err := im.importBlock(ctx, tx, req, rep.ImportID, b)
		if err != nil {
			return rep, err
		}
		rep.Accounts = append(rep.Accounts, ar)
		inserted = append(inserted, res)
	}
	if req.Preview {
		return rep, nil // deferred rollback: nothing is kept
	}
	if err := tx.Commit(ctx); err != nil {
		return rep, err
	}
	for _, res := range inserted {
		if err := im.bank.ClassifyInserted(ctx, req.TaxpayerID, res); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func (im *Importer) findBox(ctx context.Context, q *db.Queries, req Request, st Statement, blocks int) (db.CashBox, bool, error) {
	if st.AccountIBAN != "" {
		box, err := q.FindCashBoxByIBAN(ctx, db.FindCashBoxByIBANParams{TaxpayerID: req.TaxpayerID, Iban: pgtype.Text{String: st.AccountIBAN, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CashBox{}, false, nil
		}
		return box, err == nil, err
	}
	if m := reAccountDigits.FindStringSubmatch(st.AccountRef); m != nil {
		boxes, err := q.FindCashBoxByAccountNumber(ctx, db.FindCashBoxByAccountNumberParams{TaxpayerID: req.TaxpayerID, BankAccountNumber: pgtype.Text{String: m[1], Valid: true}})
		if err != nil {
			return db.CashBox{}, false, err
		}
		if len(boxes) == 1 {
			return boxes[0], true, nil
		}
	}
	// The file names no account: the uploader's choice, for a single block.
	if req.CashBoxID != 0 && blocks == 1 {
		box, err := q.GetCashBox(ctx, db.GetCashBoxParams{TaxpayerID: req.TaxpayerID, ID: req.CashBoxID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CashBox{}, false, nil
		}
		return box, err == nil, err
	}
	return db.CashBox{}, false, nil
}

func (im *Importer) importBlock(ctx context.Context, tx pgx.Tx, req Request, importID uuid.UUID, b resolved) (AccountResult, banking.IngestResult, error) {
	st, box := b.st, b.box
	ar := AccountResult{
		Account: accountKey(&st), CashBoxID: box.ID, CashBoxUUID: box.Uuid, Lines: len(st.Lines), Errors: st.Errors,
		BankID: box.BankID.String,
	}
	if ar.Account == "" && box.Iban.Valid {
		ar.Account = box.Iban.String
	}
	if ar.BankID == "" {
		ar.BankID = BankFromIBAN(ar.Account)
	}

	txns := make([]banking.Transaction, 0, len(st.Lines))
	credit, debit := decimal.Zero, decimal.Zero
	for _, l := range st.Lines {
		t := l.Transaction
		t.SourceSystem = SourceSystem
		if t.Currency == "" {
			t.Currency = box.Currency
		}
		if t.Raw == nil {
			t.Raw = map[string]any{}
		}
		t.Raw["file"] = req.FileName
		amt := decimal.RequireFromString(t.Amount)
		if t.Direction == banking.DirIn {
			credit = credit.Add(amt)
		} else {
			debit = debit.Add(amt)
		}
		txns = append(txns, t)
	}
	ar.TotalCredit, ar.TotalDebit = credit.StringFixed(2), debit.StringFixed(2)
	if n := len(st.Lines); n > 0 {
		ar.From = st.Lines[0].TransactionDate.Format("2006-01-02")
		ar.To = st.Lines[n-1].TransactionDate.Format("2006-01-02")
	}
	if st.Opening != nil {
		v := st.Opening.Amount.StringFixed(2)
		ar.Opening = &v
	}
	if st.Closing != nil {
		v := st.Closing.Amount.StringFixed(2)
		ar.Closing = &v
	}

	// Continuity (20 §7.4, 23 §3.4): within the block, line by line, and
	// against the previous statement's closing balance.
	var gaps []*Gap
	if g := CheckBlock(st); g != nil {
		gaps = append(gaps, g)
	}
	if g := CheckRunning(st.Lines); g != nil {
		gaps = append(gaps, g)
	}
	q := db.New(tx)
	if st.Opening != nil {
		prev, err := q.FindPreviousDailySummary(ctx, db.FindPreviousDailySummaryParams{
			TaxpayerID: req.TaxpayerID, CashBoxID: box.ID, Date: pgtype.Date{Time: st.Opening.Date, Valid: true},
		})
		switch {
		case err == nil:
			if pc, ok := numericDecimal(prev.ClosingBalance); ok && !pc.Equal(st.Opening.Amount) {
				gaps = append(gaps, &Gap{Expected: pc, Actual: st.Opening.Amount,
					Reason: fmt.Sprintf("önceki kapanış (%s) %s, açılış %s", prev.Date.Time.Format("2006-01-02"), pc.StringFixed(2), st.Opening.Amount.StringFixed(2))})
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return ar, banking.IngestResult{}, err
		}
	}
	for _, g := range gaps {
		ar.Gaps = append(ar.Gaps, g.Reason)
	}

	var imp *uuid.UUID
	if !req.Preview {
		imp = &importID
	}
	res, err := im.bank.IngestTx(ctx, tx, req.TaxpayerID, box.ID, imp, txns)
	if err != nil {
		return ar, res, err
	}
	ar.Inserted, ar.Merged, ar.Skipped, ar.PossibleDuplicates = res.Inserted, res.Merged, res.Skipped, res.PossibleDuplicates

	if st.Closing != nil {
		params := db.UpsertBankDailySummaryParams{
			TaxpayerID: req.TaxpayerID, CashBoxID: box.ID, Date: pgtype.Date{Time: st.Closing.Date, Valid: true},
			ClosingBalance: numeric(st.Closing.Amount), TotalCredit: numeric(credit), TotalDebit: numeric(debit),
			TransactionCount: pgtype.Int4{Int32: int32(len(st.Lines)), Valid: true}, Source: banking.CaptureMT940,
		}
		if st.Opening != nil {
			params.OpeningBalance = numeric(st.Opening.Amount)
		}
		if err := q.UpsertBankDailySummary(ctx, params); err != nil {
			return ar, res, err
		}
		snap := db.InsertBankBalanceSnapshotParams{
			TaxpayerID: req.TaxpayerID, CashBoxID: box.ID,
			CapturedAt: pgtype.Timestamptz{Time: endOfDay(st.Closing.Date), Valid: true},
			Balance:    numeric(st.Closing.Amount), Source: banking.CaptureMT940,
		}
		if st.Available != nil {
			snap.AvailableBalance = numeric(st.Available.Amount)
		}
		if imp != nil {
			snap.ImportID = pgtype.UUID{Bytes: *imp, Valid: true}
		}
		if err := q.InsertBankBalanceSnapshot(ctx, snap); err != nil {
			return ar, res, err
		}
	}

	if len(gaps) > 0 && req.OrganizationID > 0 && req.EngagementID > 0 {
		if err := raiseGap(ctx, q, req, importID, ar, gaps); err != nil {
			return ar, res, err
		}
	}
	return ar, res, nil
}

// raiseGap writes BNK_GAP (05 §3.4, blocker by default) for the period of
// the statement; the accountant sees which file and which balance broke.
func raiseGap(ctx context.Context, q *db.Queries, req Request, importID uuid.UUID, ar AccountResult, gaps []*Gap) error {
	setting, err := q.ResolveWarningSetting(ctx, db.ResolveWarningSettingParams{
		Fallback: "blocker", Code: "BNK_GAP", OrganizationID: req.OrganizationID,
		EngagementID: pgtype.Int8{Int64: req.EngagementID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("warning BNK_GAP: %w", err)
	}
	if !setting.Enabled {
		return nil
	}
	reasons := make([]string, len(gaps))
	for i, g := range gaps {
		reasons[i] = g.Reason
	}
	period := ""
	if len(ar.To) >= 7 {
		period = ar.To[:7]
	}
	params, _ := json.Marshal(map[string]any{
		"cash_box": ar.CashBoxUUID, "account": maskAccount(ar.Account), "file": req.FileName,
		"import_id": importID, "reasons": reasons, "expected": gaps[0].Expected.StringFixed(2),
		"actual": gaps[0].Actual.StringFixed(2),
	})
	_, err = q.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
		OrganizationID: req.OrganizationID, EngagementID: req.EngagementID, TaxpayerID: req.TaxpayerID,
		Period: pgtype.Text{String: period, Valid: period != ""}, EntityType: "period",
		Code: "BNK_GAP", Severity: setting.Severity, Params: params, Producer: Producer,
	})
	if err != nil {
		return fmt.Errorf("warning BNK_GAP: %w", err)
	}
	return nil
}

func maskAccount(a string) string {
	if len(a) <= 8 {
		return a
	}
	return a[:4] + strings.Repeat("*", len(a)-8) + a[len(a)-4:]
}

func endOfDay(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, time.UTC)
}

func numeric(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func numericDecimal(n pgtype.Numeric) (decimal.Decimal, bool) {
	if !n.Valid {
		return decimal.Decimal{}, false
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return decimal.Decimal{}, false
	}
	d, err := decimal.NewFromString(fmt.Sprint(v))
	return d, err == nil
}
