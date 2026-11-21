// Package bookkeeping serves the defter türü of an engagement and the
// işletme hesabı (income/expense book) of taxpayers that keep an işletme
// defteri or a serbest meslek kazanç defteri (#780).
//
// The book has no chart of accounts: purchase invoices, approved expenses
// and receipts are gider rows; sales invoices and approved Z reports are
// gelir rows. VAT is shown apart from the amounts; the KDV1 return is not
// affected (it keeps reading the same invoices, receipts and Z reports).
package bookkeeping

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
)

// Errors mapped to HTTP by the handler.
var (
	ErrInvalid     = errors.New("bookkeeping: invalid request")
	ErrNotFound    = errors.New("bookkeeping: record not found")
	ErrBalanceType = errors.New("bookkeeping: the taxpayer keeps a balance-sheet ledger")
)

// Sides of a row.
const (
	SideIncome  = "income"
	SideExpense = "expense"
)

// Sources of a row.
const (
	SourceInvoice    = "invoice"
	SourceExpense    = "expense"
	SourceRetailSale = "retail_sale"
)

// Categories (gelir / gider türü). The frontend shows them by code.
const (
	CatSalesRevenue        = "sales_revenue"        // Satış hasılatı
	CatRetailRevenue       = "retail_revenue"       // Perakende satış hasılatı (Z raporu)
	CatProfessionalIncome  = "professional_income"  // Serbest meslek kazancı
	CatOtherIncome         = "other_income"         // Diğer gelir
	CatSalesReturn         = "sales_return"         // Satıştan iade (gelir eksi)
	CatGoodsPurchase       = "goods_purchase"       // Mal alışı
	CatGeneralExpense      = "general_expense"      // Genel gider
	CatPersonnelExpense    = "personnel_expense"    // Personel gideri
	CatRentExpense         = "rent_expense"         // Kira gideri
	CatVehicleExpense      = "vehicle_expense"      // Taşıt / akaryakıt gideri
	CatTaxExpense          = "tax_expense"          // Vergi, resim ve harç
	CatFinanceExpense      = "finance_expense"      // Banka ve finansman gideri
	CatFixedAsset          = "fixed_asset"          // Demirbaş / amortismana tabi
	CatPurchaseReturn      = "purchase_return"      // Alıştan iade (gider eksi)
	CatProfessionalExpense = "professional_expense" // Serbest meslek gideri (SMM)
)

// incomeCategories and expenseCategories are what the office may choose for
// a row of that side.
var (
	incomeCategories  = []string{CatSalesRevenue, CatRetailRevenue, CatProfessionalIncome, CatOtherIncome, CatSalesReturn}
	expenseCategories = []string{CatGoodsPurchase, CatGeneralExpense, CatPersonnelExpense, CatRentExpense, CatVehicleExpense,
		CatTaxExpense, CatFinanceExpense, CatFixedAsset, CatProfessionalExpense, CatPurchaseReturn}
)

// negative reports whether rows of the category reduce their side.
func negative(cat string) bool { return cat == CatSalesReturn || cat == CatPurchaseReturn }

// Service is the use case layer.
type Service struct {
	q *db.Queries
}

// New builds the service on pool (statements join the request's RLS scope).
func New(pool database.Pool) *Service {
	return &Service{q: database.QueriesFor(pool)}
}

// LedgerType is the API view of an engagement's defter türü.
type LedgerType struct {
	LedgerType string `json:"ledger_type"`
}

// SetLedgerType changes the defter türü of the engagement.
func (s *Service) SetLedgerType(ctx context.Context, sc engagementctx.Scope, t string) (LedgerType, error) {
	if !ledger.Valid(t) {
		return LedgerType{}, fmt.Errorf("%w: ledger_type must be bilanco, isletme or serbest_meslek", ErrInvalid)
	}
	out, err := s.q.SetEngagementLedgerType(ctx, db.SetEngagementLedgerTypeParams{LedgerType: t, OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return LedgerType{}, ErrNotFound
	}
	if err != nil {
		return LedgerType{}, err
	}
	return LedgerType{LedgerType: out}, nil
}

// Row is one line of the book.
type Row struct {
	Source       string          `json:"source"`
	RecordUUID   uuid.UUID       `json:"record_uuid"`
	Date         *string         `json:"date"`
	Period       string          `json:"period"`
	DocumentNo   string          `json:"document_no"`
	Counterparty string          `json:"counterparty"`
	Side         string          `json:"side"`
	Category     string          `json:"category"`
	CategoryName string          `json:"category_name,omitempty"`
	Overridden   bool            `json:"overridden"`
	Net          decimal.Decimal `json:"net"`
	VAT          decimal.Decimal `json:"vat"`
	Total        decimal.Decimal `json:"total"`
}

// CategoryTotal is the sum of one category.
type CategoryTotal struct {
	Side         string          `json:"side"`
	Category     string          `json:"category"`
	CategoryName string          `json:"category_name,omitempty"`
	Count        int             `json:"count"`
	Net          decimal.Decimal `json:"net"`
	VAT          decimal.Decimal `json:"vat"`
}

// Inventory is the dönem başı / dönem sonu mal mevcudu of a year.
type Inventory struct {
	Year    int32            `json:"year"`
	Opening decimal.Decimal  `json:"opening"`
	Closing *decimal.Decimal `json:"closing"`
}

// Summary is the işletme hesabı özeti.
type Summary struct {
	Income     decimal.Decimal `json:"income"`
	Expense    decimal.Decimal `json:"expense"`
	IncomeVAT  decimal.Decimal `json:"income_vat"`
	ExpenseVAT decimal.Decimal `json:"expense_vat"`
	// InventoryOpening / InventoryClosing join the result only in a yearly
	// view of an işletme defteri.
	InventoryOpening *decimal.Decimal `json:"inventory_opening"`
	InventoryClosing *decimal.Decimal `json:"inventory_closing"`
	// Profit: income - expense - opening + closing (negative: zarar).
	Profit decimal.Decimal `json:"profit"`
}

// Pending counts the records that are not in the book yet.
type Pending struct {
	Expenses            int64 `json:"expenses"`
	RetailSales         int64 `json:"retail_sales"`
	MissingExchangeRate int64 `json:"missing_exchange_rate"`
}

// Book is the API view of the işletme hesabı of a period or a year.
type Book struct {
	LedgerType string          `json:"ledger_type"`
	Period     string          `json:"period,omitempty"`
	Year       int32           `json:"year,omitempty"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Summary    Summary         `json:"summary"`
	Categories []CategoryTotal `json:"categories"`
	Rows       []Row           `json:"rows"`
	Inventory  *Inventory      `json:"inventory"`
	Pending    Pending         `json:"pending"`
	// Choices: what the office may pick for a row of each side.
	Choices map[string][]string `json:"choices"`
}

var (
	periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
	yearRe   = regexp.MustCompile(`^(20[0-9]{2}|2100)$`)
)

// Book returns the işletme hesabı of one period (YYYY-MM) or one year.
func (s *Service) Book(ctx context.Context, sc engagementctx.Scope, period, year string) (Book, error) {
	lt, err := ledger.Of(ctx, s.q, sc.OrganizationID, sc.ID)
	if err != nil {
		return Book{}, err
	}
	if !ledger.SingleEntry(lt) {
		return Book{}, ErrBalanceType
	}
	b := Book{LedgerType: lt, Choices: map[string][]string{SideIncome: incomeCategories, SideExpense: expenseCategories}}
	switch {
	case period != "" && year == "":
		if !periodRe.MatchString(period) {
			return Book{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
		}
		b.Period, b.From, b.To = period, period, period
	case year != "" && period == "":
		if !yearRe.MatchString(year) {
			return Book{}, fmt.Errorf("%w: year must be YYYY", ErrInvalid)
		}
		y, _ := strconv.Atoi(year)
		b.Year, b.From, b.To = int32(y), year+"-01", year+"-12" //nolint:gosec // 2000..2100
	default:
		return Book{}, fmt.Errorf("%w: send either period or year", ErrInvalid)
	}
	rows, err := s.q.ListOperatingAccountRows(ctx, db.ListOperatingAccountRowsParams{
		TaxpayerID: sc.TaxpayerID, FromPeriod: b.From, ToPeriod: b.To, OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
	})
	if err != nil {
		return Book{}, err
	}
	b.Rows = make([]Row, 0, len(rows))
	cats := map[string]*CategoryTotal{}
	for _, r := range rows {
		row := toRow(lt, r)
		b.Rows = append(b.Rows, row)
		key := row.Side + "|" + row.Category + "|" + row.CategoryName
		c, ok := cats[key]
		if !ok {
			c = &CategoryTotal{Side: row.Side, Category: row.Category, CategoryName: row.CategoryName}
			cats[key] = c
		}
		c.Count++
		c.Net, c.VAT = c.Net.Add(row.Net), c.VAT.Add(row.VAT)
		if row.Side == SideIncome {
			b.Summary.Income, b.Summary.IncomeVAT = b.Summary.Income.Add(row.Net), b.Summary.IncomeVAT.Add(row.VAT)
		} else {
			b.Summary.Expense, b.Summary.ExpenseVAT = b.Summary.Expense.Add(row.Net), b.Summary.ExpenseVAT.Add(row.VAT)
		}
	}
	b.Categories = make([]CategoryTotal, 0, len(cats))
	for _, c := range cats {
		b.Categories = append(b.Categories, *c)
	}
	sort.Slice(b.Categories, func(i, j int) bool {
		a, c := b.Categories[i], b.Categories[j]
		if a.Side != c.Side {
			return a.Side == SideIncome
		}
		return a.Net.GreaterThan(c.Net)
	})
	b.Summary.Profit = b.Summary.Income.Sub(b.Summary.Expense)
	// Mal mevcudu yalnız işletme defterinde ve yıllık görünümde (#780).
	if b.Year > 0 && lt == ledger.Operating {
		inv, err := s.inventory(ctx, sc, b.Year)
		if err != nil {
			return Book{}, err
		}
		b.Inventory = &inv
		opening := inv.Opening
		b.Summary.InventoryOpening = &opening
		b.Summary.Profit = b.Summary.Profit.Sub(opening)
		if inv.Closing != nil {
			closing := *inv.Closing
			b.Summary.InventoryClosing = &closing
			b.Summary.Profit = b.Summary.Profit.Add(closing)
		}
	}
	p, err := s.q.OperatingAccountPending(ctx, db.OperatingAccountPendingParams{TaxpayerID: sc.TaxpayerID, FromPeriod: b.From, ToPeriod: b.To})
	if err != nil {
		return Book{}, err
	}
	b.Pending = Pending{Expenses: p.PendingExpenses, RetailSales: p.PendingRetailSales, MissingExchangeRate: p.MissingExchangeRate}
	return b, nil
}

// toRow classifies one record: side and default category from the record,
// the office's choice when there is one.
func toRow(lt string, r db.ListOperatingAccountRowsRow) Row {
	side, cat := Classify(lt, r.Source, r.Direction, r.DocType, r.DocClass)
	row := Row{
		Source: r.Source, RecordUUID: r.RecordUuid, Period: r.Period, DocumentNo: r.DocumentNo,
		Counterparty: r.Counterparty, Side: side, Category: cat,
	}
	if r.Source == SourceExpense && r.CategoryName != "" {
		// The office's own expense category (masraf kategorisi) names it.
		row.CategoryName = r.CategoryName
	}
	if r.OverrideCategory != "" {
		if s, ok := sideOf(r.OverrideCategory); ok {
			row.Side, row.Category, row.CategoryName, row.Overridden = s, r.OverrideCategory, "", true
		}
	}
	if r.DocDate.Valid {
		d := r.DocDate.Time.Format("2006-01-02")
		row.Date = &d
	}
	row.Net, row.VAT = repository.Dec(r.Net), repository.Dec(r.Vat)
	if negative(row.Category) {
		row.Net, row.VAT = row.Net.Abs().Neg(), row.VAT.Abs().Neg()
	}
	row.Total = row.Net.Add(row.VAT)
	return row
}

// Classify returns the side and the default category of a record.
//
//   - Alış faturası: mal alışı (serbest meslekte genel gider); serbest
//     meslek makbuzu: serbest meslek gideri; iade faturası (gelen): satıştan
//     iade, gelir eksi.
//   - Satış faturası: satış hasılatı (serbest meslekte serbest meslek
//     kazancı); iade faturası (giden): alıştan iade, gider eksi.
//   - Masraf fişi: türüne göre gider (maaş, vergi, banka, akaryakıt, genel).
//   - Z raporu: perakende satış hasılatı.
func Classify(lt, source, direction, docType, docClass string) (side, category string) {
	self := lt == ledger.SelfEmployedBook
	switch source {
	case SourceRetailSale:
		return SideIncome, CatRetailRevenue
	case SourceExpense:
		switch docType {
		case "salary":
			return SideExpense, CatPersonnelExpense
		case "tax_payment":
			return SideExpense, CatTaxExpense
		case "bank_fee":
			return SideExpense, CatFinanceExpense
		case "fuel_receipt":
			return SideExpense, CatVehicleExpense
		case "smm":
			return SideExpense, CatProfessionalExpense
		}
		return SideExpense, CatGeneralExpense
	}
	if direction == "outgoing" {
		if docType == "IADE" {
			return SideExpense, CatPurchaseReturn
		}
		if self {
			return SideIncome, CatProfessionalIncome
		}
		return SideIncome, CatSalesRevenue
	}
	if docType == "IADE" {
		return SideIncome, CatSalesReturn
	}
	switch docClass {
	case "smm":
		return SideExpense, CatProfessionalExpense
	case "mustahsil":
		if !self {
			return SideExpense, CatGoodsPurchase
		}
	}
	if self {
		return SideExpense, CatGeneralExpense
	}
	return SideExpense, CatGoodsPurchase
}

func sideOf(cat string) (string, bool) {
	for _, c := range incomeCategories {
		if c == cat {
			return SideIncome, true
		}
	}
	for _, c := range expenseCategories {
		if c == cat {
			return SideExpense, true
		}
	}
	return "", false
}

// CategoryInput sets (or with an empty category, clears) the office's
// category of one record.
type CategoryInput struct {
	Source     string    `json:"source"`
	RecordUUID uuid.UUID `json:"record_uuid"`
	Category   string    `json:"category"`
}

// SetCategory stores the office's category of a record of the taxpayer.
func (s *Service) SetCategory(ctx context.Context, sc engagementctx.Scope, userID int64, in CategoryInput) error {
	lt, err := ledger.Of(ctx, s.q, sc.OrganizationID, sc.ID)
	if err != nil {
		return err
	}
	if !ledger.SingleEntry(lt) {
		return ErrBalanceType
	}
	switch in.Source {
	case SourceInvoice, SourceExpense, SourceRetailSale:
	default:
		return fmt.Errorf("%w: source must be invoice, expense or retail_sale", ErrInvalid)
	}
	if in.RecordUUID == uuid.Nil {
		return fmt.Errorf("%w: record_uuid is required", ErrInvalid)
	}
	if in.Category != "" {
		if _, ok := sideOf(in.Category); !ok {
			return fmt.Errorf("%w: unknown category %q", ErrInvalid, in.Category)
		}
	}
	ok, err := s.q.OperatingAccountRecordExists(ctx, db.OperatingAccountRecordExistsParams{Source: in.Source, TaxpayerID: sc.TaxpayerID, RecordUuid: in.RecordUUID})
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if in.Category == "" {
		return s.q.DeleteOperatingAccountCategory(ctx, db.DeleteOperatingAccountCategoryParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Source: in.Source, RecordUuid: in.RecordUUID,
		})
	}
	return s.q.UpsertOperatingAccountCategory(ctx, db.UpsertOperatingAccountCategoryParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Source: in.Source, RecordUuid: in.RecordUUID,
		Category: in.Category, DecidedBy: optID(userID),
	})
}

// InventoryInput sets the mal mevcudu of a year.
type InventoryInput struct {
	Year    int32            `json:"year"`
	Opening decimal.Decimal  `json:"opening"`
	Closing *decimal.Decimal `json:"closing"`
}

// SetInventory stores the dönem başı / dönem sonu mal mevcudu (işletme
// defteri only).
func (s *Service) SetInventory(ctx context.Context, sc engagementctx.Scope, userID int64, in InventoryInput) (Inventory, error) {
	lt, err := ledger.Of(ctx, s.q, sc.OrganizationID, sc.ID)
	if err != nil {
		return Inventory{}, err
	}
	if lt != ledger.Operating {
		return Inventory{}, ErrBalanceType
	}
	if in.Year < 2000 || in.Year > 2100 {
		return Inventory{}, fmt.Errorf("%w: year must be between 2000 and 2100", ErrInvalid)
	}
	if in.Opening.IsNegative() || (in.Closing != nil && in.Closing.IsNegative()) {
		return Inventory{}, fmt.Errorf("%w: inventory cannot be negative", ErrInvalid)
	}
	var closing pgtype.Numeric
	if in.Closing != nil {
		closing = repository.Num(in.Closing.Round(2))
	}
	row, err := s.q.UpsertOperatingAccountInventory(ctx, db.UpsertOperatingAccountInventoryParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Year: in.Year,
		OpeningInventory: repository.Num(in.Opening.Round(2)), ClosingInventory: closing, UpdatedBy: optID(userID),
	})
	if err != nil {
		return Inventory{}, err
	}
	return inventoryOf(row), nil
}

func (s *Service) inventory(ctx context.Context, sc engagementctx.Scope, year int32) (Inventory, error) {
	row, err := s.q.GetOperatingAccountInventory(ctx, db.GetOperatingAccountInventoryParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Year: year})
	if errors.Is(err, pgx.ErrNoRows) {
		return Inventory{Year: year}, nil
	}
	if err != nil {
		return Inventory{}, err
	}
	return inventoryOf(row), nil
}

func inventoryOf(row db.OperatingAccountInventory) Inventory {
	out := Inventory{Year: row.Year, Opening: repository.Dec(row.OpeningInventory)}
	if row.ClosingInventory.Valid {
		c := repository.Dec(row.ClosingInventory)
		out.Closing = &c
	}
	return out
}

func optID(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
