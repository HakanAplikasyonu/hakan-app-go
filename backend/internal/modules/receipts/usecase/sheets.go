package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// MaxSheetBytes caps an uploaded sheet.
const MaxSheetBytes = 10 << 20

// exportLimit bounds one export (a year of daily Z reports for a few
// devices, or a busy month of receipts).
const exportLimit = 10_000

// RowError is one rejected row of an import.
type RowError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ImportResult is what an import did.
type ImportResult struct {
	Rows     int        `json:"rows"`
	Inserted int        `json:"inserted"`
	Skipped  int        `json:"skipped"` // already imported (same row content)
	Errors   []RowError `json:"errors"`
}

func readSheet(name string, data []byte, cols []sheet.Column) ([]sheet.Parsed, error) {
	if len(data) == 0 || len(data) > MaxSheetBytes {
		return nil, fmt.Errorf("%w: file must be 1 byte to 10 MB", ErrInvalid)
	}
	rows, err := sheet.Read(name, data, cols)
	if errors.Is(err, sheet.ErrNoHeader) || errors.Is(err, parse.ErrUnsupported) || errors.Is(err, parse.ErrEmpty) {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return rows, nil
}

// ExportExpenses writes the filtered expense list as .xlsx.
func (s *Service) ExportExpenses(ctx context.Context, sc engagementctx.Scope, f ExpenseFilter) ([]byte, error) {
	f.Limit, f.Offset = exportLimit, 0
	page, err := s.Expenses(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	rows := make([]sheet.Row, 0, len(page.Items))
	for _, x := range page.Items {
		r := sheet.Row{"description": x.Description, "receipt_type": x.ReceiptType, "status": sheet.StatusLabel(x.ApprovalStatus)}
		if x.Date != nil {
			if d, err := time.Parse(time.DateOnly, *x.Date); err == nil {
				r["date"] = d.Format("02.01.2006")
			}
		}
		setStr(r, "vendor_name", x.VendorName)
		setStr(r, "vendor_tax_number", x.VendorTaxNumber)
		setStr(r, "receipt_number", x.ReceiptNumber)
		setStr(r, "vehicle_plate", x.VehiclePlate)
		if x.CategoryName != nil {
			r["category"] = *x.CategoryName
		}
		setStr(r, "account_code", x.AccountCode)
		setDec(r, "tax_rate", x.TaxRate)
		setDec(r, "tax_amount", x.TaxAmount)
		setDec(r, "total", x.Amount)
		if x.PaymentMethod != nil {
			r["payment_method"] = sheet.PaymentLabel(*x.PaymentMethod)
		}
		if x.Journal != nil {
			r["journal"] = strconv.Itoa(int(x.Journal.EntryNo))
		}
		rows = append(rows, r)
	}
	return sheet.Write("Masraflar", sheet.ExpenseColumns, rows)
}

// ImportExpenses reads the expense template (.xlsx / .csv) into pending
// expenses (capture_method excel_import). A Kategori / Hesap Kodu cell
// becomes the office's choice; rows already imported are skipped.
func (s *Service) ImportExpenses(ctx context.Context, sc engagementctx.Scope, userID int64, name string, data []byte) (ImportResult, error) {
	rows, err := readSheet(name, data, sheet.ExpenseColumns)
	if err != nil {
		return ImportResult{}, err
	}
	cats, err := s.categories(ctx, sc.OrganizationID, sc.ID)
	if err != nil {
		return ImportResult{}, err
	}
	plan, err := s.plan(ctx, sc, "770", "760", "780", "689")
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Rows: len(rows), Errors: []RowError{}}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	var inserted []db.Expense
	for _, pr := range rows {
		p, post, msg := expenseRow(sc, pr, cats, plan)
		if msg != "" {
			res.Errors = append(res.Errors, RowError{Line: pr.Line, Message: msg})
			continue
		}
		e, err := q.InsertImportedExpense(ctx, p)
		if errors.Is(err, pgx.ErrNoRows) {
			res.Skipped++
			continue
		}
		if err != nil {
			return ImportResult{}, fmt.Errorf("receipts: import line %d: %w", pr.Line, err)
		}
		// KDV kırılımı (#780): satırın açık tek oranı tek oranlı kırılımdır;
		// oran yoksa satır yazılmaz ("kırılım yok").
		if p.TaxRate.Valid && p.TaxAmount.Valid && p.TaxExclusiveAmount.Valid {
			if err := insertVatLines(ctx, q, sc.TaxpayerID, e.ID, []VatLine{{Rate: dec(p.TaxRate), Base: dec(p.TaxExclusiveAmount), Tax: dec(p.TaxAmount)}}); err != nil {
				return ImportResult{}, err
			}
		}
		res.Inserted++
		inserted = append(inserted, e)
		if post != nil {
			post.TargetID, post.DecidedBy = e.ID, optID(userID)
			if _, err := q.UpsertReceiptPosting(ctx, *post); err != nil {
				return ImportResult{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, err
	}
	for _, e := range inserted {
		s.syncExpenseApproval(ctx, sc, e)
	}
	return res, nil
}

func expenseRow(sc engagementctx.Scope, pr sheet.Parsed, cats []posting.Category, plan []engine.PlanAccount) (db.InsertImportedExpenseParams, *db.UpsertReceiptPostingParams, string) {
	r := pr.Row
	date, ok := sheet.Date(r["date"])
	if !ok {
		return db.InsertImportedExpenseParams{}, nil, "Tarih okunamadı: " + r["date"]
	}
	total, ok := sheet.Amount(r["total"])
	if !ok || !total.IsPositive() {
		return db.InsertImportedExpenseParams{}, nil, "Toplam okunamadı: " + r["total"]
	}
	p := db.InsertImportedExpenseParams{
		TaxpayerID: sc.TaxpayerID, SourceID: "xlsx:" + pr.Key, ExpenseDate: pgtype.Date{Time: date, Valid: true},
		Period: text(date.Format("2006-01")), Amount: toNumeric(total), Description: r["description"], ReceiptType: "okc_receipt",
		VendorName: text(r["vendor_name"]), ReceiptNumber: text(r["receipt_number"]), ContentHash: text(pr.Key),
	}
	if v := r["vendor_tax_number"]; v != "" {
		id, ok := sheet.TaxID(v)
		if !ok {
			return p, nil, "VKN / TCKN 10 ya da 11 hane olmalı: " + v
		}
		p.VendorTaxNumber = text(id)
	}
	if v := r["tax_amount"]; v != "" {
		tax, ok := sheet.Amount(v)
		if !ok || tax.GreaterThan(total) {
			return p, nil, "KDV okunamadı ya da toplamdan büyük: " + v
		}
		p.TaxAmount = toNumeric(tax)
		p.TaxExclusiveAmount = toNumeric(total.Sub(tax))
	}
	if v := r["tax_rate"]; v != "" {
		rate, ok := sheet.Rate(v)
		if !ok || rate.GreaterThan(decimal.NewFromInt(100)) {
			return p, nil, "KDV oranı okunamadı: " + v
		}
		p.TaxRate = toNumeric(rate)
		if !p.TaxAmount.Valid && rate.IsPositive() {
			// Only the rate given: VAT from the inclusive total.
			tax := total.Sub(total.Div(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100))))).Round(2)
			p.TaxAmount, p.TaxExclusiveAmount = toNumeric(tax), toNumeric(total.Sub(tax))
		}
	}
	if v := r["payment_method"]; v != "" {
		m, ok := sheet.PaymentMethod(v)
		if !ok {
			return p, nil, "Ödeme şekli anlaşılamadı: " + v + " (nakit, kart, havale, çek, senet, veresiye)"
		}
		p.PaymentMethod = text(m)
	}
	if v := r["vehicle_plate"]; v != "" {
		plate := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(v))
		p.VehiclePlate = text(plate)
	}
	var post *db.UpsertReceiptPostingParams
	if v := strings.TrimSpace(r["category"]); v != "" {
		c, ok := findCategory(cats, v)
		if !ok {
			return p, nil, "Kategori bulunamadı: " + v
		}
		sug := posting.ForCategory(c, plan)
		post = &db.UpsertReceiptPostingParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
			TargetType: TargetExpense, CategoryCode: text(c.Code), AccountCode: text(sug.AccountCode),
			AccountSource: text(sug.Source), Confidence: toNumeric(sug.Confidence)}
	}
	if v := strings.TrimSpace(r["account_code"]); v != "" {
		if !accountRe.MatchString(v) {
			return p, nil, "Hesap kodu 770 ya da 770.01.001 biçiminde olmalı: " + v
		}
		if post == nil {
			post = &db.UpsertReceiptPostingParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, TargetType: TargetExpense}
		}
		post.AccountCode, post.AccountSource, post.Confidence = text(v), text(posting.SourceManual), toNumeric(posting.ManualConfidence())
	}
	return p, post, ""
}

// findCategory matches a Kategori cell by code or name.
func findCategory(cats []posting.Category, v string) (posting.Category, bool) {
	if c, ok := posting.Find(cats, strings.ToLower(v)); ok {
		return c, true
	}
	key := engine.Fold(v)
	for _, c := range posting.Active(cats) {
		if engine.Fold(c.Name) == key {
			return c, true
		}
	}
	return posting.Category{}, false
}

// ExportRetailSales writes the filtered Z list as .xlsx.
func (s *Service) ExportRetailSales(ctx context.Context, sc engagementctx.Scope, f RetailFilter) ([]byte, error) {
	f.Limit, f.Offset = exportLimit, 0
	page, err := s.RetailSales(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	rows := make([]sheet.Row, 0, len(page.Items))
	for _, x := range page.Items {
		r := sheet.Row{"status": sheet.StatusLabel(x.ApprovalStatus), "warnings": strings.Join(x.Warnings, ", ")}
		if x.Date != nil {
			if d, err := time.Parse(time.DateOnly, *x.Date); err == nil {
				r["date"] = d.Format("02.01.2006")
			}
		}
		setStr(r, "okc", x.OKCSerial)
		setStr(r, "z_number", x.ZNumber)
		if x.ReceiptCount != nil {
			r["receipt_count"] = strconv.Itoa(int(*x.ReceiptCount))
		}
		setDec(r, "cash", x.Cash)
		setDec(r, "card", x.Card)
		setDec(r, "other", x.OtherPayment)
		for _, l := range x.VatLines {
			rate := l.Rate.String()
			r["base_"+rate], r["vat_"+rate] = l.Base.String(), l.Tax.String()
		}
		setDec(r, "tax_amount", x.TaxAmount)
		setDec(r, "total", x.Total)
		if x.Journal != nil {
			r["journal"] = strconv.Itoa(int(x.Journal.EntryNo))
		}
		rows = append(rows, r)
	}
	return sheet.Write("Z raporları", sheet.ZColumns, rows)
}

// ImportRetailSales reads the Z template into pending Z reports with their
// rate and payment breakdown, then runs the continuity check of every ÖKC
// touched. Rows already imported are skipped.
func (s *Service) ImportRetailSales(ctx context.Context, sc engagementctx.Scope, name string, data []byte) (ImportResult, error) {
	rows, err := readSheet(name, data, sheet.ZColumns)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Rows: len(rows), Errors: []RowError{}}
	devices := map[string]bool{}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	for _, pr := range rows {
		p, lines, msg := retailRow(sc, pr)
		if msg != "" {
			res.Errors = append(res.Errors, RowError{Line: pr.Line, Message: msg})
			continue
		}
		row, err := q.InsertImportedRetailSale(ctx, p)
		if errors.Is(err, pgx.ErrNoRows) {
			res.Skipped++
			continue
		}
		if err != nil {
			return ImportResult{}, fmt.Errorf("receipts: import line %d: %w", pr.Line, err)
		}
		if err := replaceVatLines(ctx, q, sc.TaxpayerID, row.ID, lines); err != nil {
			return ImportResult{}, err
		}
		res.Inserted++
		devices[row.OkcSerialNumber.String] = true
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, err
	}
	for okc := range devices {
		s.CheckZContinuity(ctx, sc, okc)
	}
	return res, nil
}

func retailRow(sc engagementctx.Scope, pr sheet.Parsed) (db.InsertImportedRetailSaleParams, []VatLine, string) {
	r := pr.Row
	date, ok := sheet.Date(r["date"])
	if !ok {
		return db.InsertImportedRetailSaleParams{}, nil, "Tarih okunamadı: " + r["date"]
	}
	total, ok := sheet.Amount(r["total"])
	if !ok || !total.IsPositive() {
		return db.InsertImportedRetailSaleParams{}, nil, "Toplam okunamadı: " + r["total"]
	}
	p := db.InsertImportedRetailSaleParams{
		TaxpayerID: sc.TaxpayerID, SourceID: "xlsx:" + pr.Key, SaleDate: pgtype.Date{Time: date, Valid: true},
		Period: text(date.Format("2006-01")), ZReportNumber: text(r["z_number"]), OkcSerialNumber: text(r["okc"]),
		TotalAmount: toNumeric(total), ContentHash: text(pr.Key),
	}
	for _, m := range []struct {
		key string
		dst *pgtype.Numeric
	}{{"cash", &p.CashAmount}, {"card", &p.CardAmount}, {"other", &p.OtherPaymentAmount}} {
		if v := r[m.key]; v != "" {
			d, ok := sheet.Amount(v)
			if !ok {
				return p, nil, "Tutar okunamadı: " + v
			}
			*m.dst = toNumeric(d)
		}
	}
	if v := r["receipt_count"]; v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			return p, nil, "Fiş sayısı okunamadı: " + v
		}
		p.ReceiptCount = pgtype.Int4{Int32: int32(n), Valid: true} //nolint:gosec // parsed int, bounded by Atoi
	}
	var lines []VatLine
	var vat decimal.Decimal
	for _, rate := range sheet.ZRates {
		base, okB := sheet.Amount(r["base_"+rate])
		tax, okT := sheet.Amount(r["vat_"+rate])
		if !okB && !okT {
			continue
		}
		if !okB {
			return p, nil, "%" + rate + " matrahı eksik"
		}
		lines = append(lines, VatLine{Rate: decimal.RequireFromString(rate), Base: base, Tax: tax})
		vat = vat.Add(tax)
	}
	if v := r["tax_amount"]; v != "" {
		t, ok := sheet.Amount(v)
		if !ok || t.GreaterThan(total) {
			return p, nil, "Toplam KDV okunamadı ya da toplamdan büyük: " + v
		}
		vat = t
	}
	if len(lines) > 0 || r["tax_amount"] != "" {
		p.TaxAmount = toNumeric(vat)
		p.SubtotalAmount = toNumeric(total.Sub(vat))
	}
	return p, lines, ""
}

func setStr(r sheet.Row, k string, v *string) {
	if v != nil {
		r[k] = *v
	}
}

func setDec(r sheet.Row, k string, v *decimal.Decimal) {
	if v != nil {
		r[k] = v.String()
	}
}
