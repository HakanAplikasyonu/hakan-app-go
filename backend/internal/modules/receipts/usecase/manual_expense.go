package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ManualExpenseInput is an expense typed in the office (#780). VatLines is
// the per-rate breakdown (matrah + KDV per rate). Without lines Amount is
// the KDV dahil total and the expense has no breakdown ("kırılım yok").
type ManualExpenseInput struct {
	Date            string           `json:"expense_date"`
	VendorName      string           `json:"vendor_name"`
	VendorTaxNumber string           `json:"vendor_tax_number"`
	ReceiptNumber   string           `json:"receipt_number"`
	Description     string           `json:"description"`
	ReceiptType     string           `json:"receipt_type"`
	PaymentMethod   string           `json:"payment_method"`
	Amount          *decimal.Decimal `json:"amount"`
	VatLines        []VatLine        `json:"vat_lines"`
}

var (
	paymentMethods = map[string]bool{"": true, "cash": true, "bank_transfer": true, "credit_card": true, "check": true,
		"promissory_note": true, "credit": true}
	hundred = decimal.NewFromInt(100)
)

// manualExpense is the checked input.
type manualExpense struct {
	date               time.Time
	total, base, tax   decimal.Decimal
	rate               *decimal.Decimal
	lines              []VatLine
	vendorTaxNumber    string
	receiptType        string
	vendorName, number string
}

// checkManualExpense validates the input and computes the totals: with
// lines the total is Σ(matrah + KDV); the expense's single rate is set only
// when there is exactly one line.
func checkManualExpense(in ManualExpenseInput) (manualExpense, error) {
	var m manualExpense
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(in.Date))
	if err != nil {
		return m, fmt.Errorf("%w: expense_date must be YYYY-MM-DD", ErrInvalid)
	}
	m.date = d
	m.vendorName, m.number = strings.TrimSpace(in.VendorName), strings.TrimSpace(in.ReceiptNumber)
	if v := strings.TrimSpace(in.VendorTaxNumber); v != "" {
		if (len(v) != 10 && len(v) != 11) || strings.Trim(v, "0123456789") != "" {
			return m, fmt.Errorf("%w: vendor_tax_number must be 10 or 11 digits", ErrInvalid)
		}
		m.vendorTaxNumber = v
	}
	m.receiptType = strings.TrimSpace(in.ReceiptType)
	if m.receiptType == "" {
		m.receiptType = "okc_receipt"
	}
	if !receiptTypes[m.receiptType] {
		return m, fmt.Errorf("%w: unknown receipt_type %q", ErrInvalid, in.ReceiptType)
	}
	if !paymentMethods[in.PaymentMethod] {
		return m, fmt.Errorf("%w: unknown payment_method %q", ErrInvalid, in.PaymentMethod)
	}
	seen := map[string]bool{}
	for _, l := range in.VatLines {
		if l.Rate.IsNegative() || l.Rate.GreaterThan(hundred) {
			return m, fmt.Errorf("%w: tax_rate must be between 0 and 100", ErrInvalid)
		}
		if l.Base.IsNegative() || l.Tax.IsNegative() {
			return m, fmt.Errorf("%w: tax_base and tax_amount must not be negative", ErrInvalid)
		}
		if l.Base.IsZero() && l.Tax.IsZero() {
			continue
		}
		k := l.Rate.String()
		if seen[k] {
			return m, fmt.Errorf("%w: rate %s is given twice", ErrInvalid, k)
		}
		seen[k] = true
		m.lines = append(m.lines, l)
		m.base, m.tax = m.base.Add(l.Base), m.tax.Add(l.Tax)
	}
	if len(m.lines) > 0 {
		m.total = m.base.Add(m.tax)
		if len(m.lines) == 1 {
			r := m.lines[0].Rate
			m.rate = &r
		}
	} else if in.Amount != nil {
		m.total = *in.Amount
	}
	if !m.total.IsPositive() {
		return m, fmt.Errorf("%w: amount or vat_lines is required", ErrInvalid)
	}
	return m, nil
}

// CreateManualExpense stores an expense typed in the office with its VAT
// breakdown. It waits for approval like any other expense.
func (s *Service) CreateManualExpense(ctx context.Context, sc engagementctx.Scope, userID int64, in ManualExpenseInput) (ExpenseView, error) {
	m, err := checkManualExpense(in)
	if err != nil {
		return ExpenseView{}, err
	}
	sid := uuid.New().String()
	h := sha256.Sum256([]byte("manual:" + sid))
	p := db.InsertManualExpenseParams{
		TaxpayerID: sc.TaxpayerID, SourceID: sid, ExpenseDate: pgtype.Date{Time: m.date, Valid: true},
		Period: text(m.date.Format("2006-01")), Amount: toNumeric(m.total), Description: strings.TrimSpace(in.Description),
		ReceiptType: m.receiptType, VendorTaxNumber: text(m.vendorTaxNumber), VendorName: text(m.vendorName),
		ReceiptNumber: text(m.number), PaymentMethod: text(in.PaymentMethod), ContentHash: text(hex.EncodeToString(h[:])),
	}
	if len(m.lines) > 0 {
		p.TaxAmount, p.TaxExclusiveAmount = toNumeric(m.tax), toNumeric(m.base)
	}
	if m.rate != nil {
		p.TaxRate = toNumeric(*m.rate)
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return ExpenseView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	e, err := q.InsertManualExpense(ctx, p)
	if err != nil {
		return ExpenseView{}, err
	}
	if err := insertVatLines(ctx, q, sc.TaxpayerID, e.ID, m.lines); err != nil {
		return ExpenseView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExpenseView{}, err
	}
	s.syncExpenseApproval(ctx, sc, e)
	return s.Expense(ctx, sc, e.Uuid)
}

// insertVatLines stores an expense's per-rate breakdown; no lines → none.
func insertVatLines(ctx context.Context, q *db.Queries, taxpayerID, expenseID int64, lines []VatLine) error {
	rows := make([]map[string]string, 0, len(lines))
	for _, l := range lines {
		rows = append(rows, map[string]string{"tax_rate": l.Rate.String(), "tax_base": l.Base.String(), "tax_amount": l.Tax.String()})
	}
	if len(rows) == 0 {
		return nil
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	_, err = q.InsertExpenseVatLines(ctx, db.InsertExpenseVatLinesParams{TaxpayerID: taxpayerID, ExpenseID: expenseID, Rows: raw})
	return err
}
