// Package repository writes the common invoice model to Postgres (tables from
// migration 000037). Every call is scoped by taxpayer_id.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

// SaveResult says what SaveInvoice did.
type SaveResult struct {
	InvoiceID   int64
	InvoiceUUID uuid.UUID
	// Duplicate: an invoice with the same ETTN (or the same source key)
	// already exists for this taxpayer; nothing was written.
	Duplicate bool
	// ItemIDs maps line_no to invoice_items.id for new invoices.
	ItemIDs map[int]int64
}

// SaveInvoice stores one mapped invoice inside tx: finds or creates the
// counterparty's customer record, then writes the invoice, its lines and its
// tax summary. An existing invoice with the same ETTN or the same
// (source_system, source_id) is reported as a duplicate and left untouched
// (`01` §6 rule 1; source priority and source links arrive with #12).
func SaveInvoice(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice, ublAttachmentID int64) (SaveResult, error) {
	q := db.New(tx)
	if inv.UUID != "" {
		ettn, err := uuid.Parse(inv.UUID)
		if err != nil {
			return SaveResult{}, fmt.Errorf("repository: ettn: %w", err)
		}
		found, err := q.FindInvoicesByETTN(ctx, db.FindInvoicesByETTNParams{TaxpayerID: taxpayerID, Ettn: pgtype.UUID{Bytes: ettn, Valid: true}})
		if err != nil {
			return SaveResult{}, err
		}
		if len(found) > 0 {
			return SaveResult{InvoiceID: found[0].ID, InvoiceUUID: found[0].Uuid, Duplicate: true}, nil
		}
	}
	if existing, err := q.GetInvoiceBySource(ctx, db.GetInvoiceBySourceParams{TaxpayerID: taxpayerID, SourceSystem: inv.SourceSystem, SourceID: inv.SourceID}); err == nil {
		return SaveResult{InvoiceID: existing.ID, InvoiceUUID: existing.Uuid, Duplicate: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SaveResult{}, err
	}

	customerID, err := findOrCreateCustomer(ctx, tx, taxpayerID, inv)
	if err != nil {
		return SaveResult{}, fmt.Errorf("repository: customer: %w", err)
	}
	params, err := invoiceParams(taxpayerID, inv, customerID, ublAttachmentID)
	if err != nil {
		return SaveResult{}, err
	}
	row, err := q.InsertInvoice(ctx, params)
	if err != nil {
		return SaveResult{}, fmt.Errorf("repository: invoice: %w", err)
	}
	itemIDs := make(map[int]int64, len(inv.Items))
	for _, it := range inv.Items {
		p, err := itemParams(taxpayerID, row.ID, it)
		if err != nil {
			return SaveResult{}, err
		}
		id, err := q.InsertInvoiceItem(ctx, p)
		if err != nil {
			return SaveResult{}, fmt.Errorf("repository: item %d: %w", it.LineNo, err)
		}
		itemIDs[it.LineNo] = id
	}
	for _, s := range inv.TaxSummaries {
		if err := q.InsertInvoiceTaxSummary(ctx, db.InsertInvoiceTaxSummaryParams{
			TaxpayerID: taxpayerID, InvoiceID: row.ID,
			TaxRate: Num(s.TaxRate), Base: Num(s.Base), Vat: Num(s.VAT), Withholding: Num(s.Withholding),
			WithholdingCode: Text(s.WithholdingCode), TaxExemptionCode: Text(s.TaxExemptionCode),
		}); err != nil {
			return SaveResult{}, fmt.Errorf("repository: tax summary: %w", err)
		}
	}
	return SaveResult{InvoiceID: row.ID, InvoiceUUID: row.Uuid, ItemIDs: itemIDs}, nil
}

// findOrCreateCustomer applies the `01` §4.3 rule: one customer per VKN (or
// per TCKN without VKN) within a taxpayer. Without any id the invoice keeps
// only the denormalised name.
func findOrCreateCustomer(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice) (pgtype.Int8, error) {
	cp := inv.Counterparty
	if cp.TaxNumber == "" && cp.IdentityNumber == "" {
		return pgtype.Int8{}, nil
	}
	want := "customer"
	if inv.Direction == model.DirectionIncoming {
		want = "supplier"
	}
	q := db.New(tx)
	find := func() (db.Customer, error) {
		if cp.TaxNumber != "" {
			return q.FindCustomerByTaxNumber(ctx, db.FindCustomerByTaxNumberParams{TaxpayerID: taxpayerID, TaxNumber: Text(cp.TaxNumber)})
		}
		return q.FindCustomerByIdentityNumber(ctx, db.FindCustomerByIdentityNumberParams{TaxpayerID: taxpayerID, IdentityNumber: Text(cp.IdentityNumber)})
	}
	sourceID := cp.TaxNumber
	if sourceID == "" {
		sourceID = cp.IdentityNumber
	}
	c, err := find()
	if errors.Is(err, pgx.ErrNoRows) {
		c, err = insertCustomer(ctx, tx, taxpayerID, inv, want, sourceID)
		if isUniqueViolation(err) { // created concurrently by another upload
			c, err = find()
		}
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	if c.Type != want && c.Type != "both" && c.Type != "employee" {
		if err := q.SetCustomerType(ctx, db.SetCustomerTypeParams{TaxpayerID: taxpayerID, ID: c.ID, Type: "both"}); err != nil {
			return pgtype.Int8{}, err
		}
	}
	raw, _ := json.Marshal(cp)
	if err := q.UpsertCustomerSourceLink(ctx, db.UpsertCustomerSourceLinkParams{
		TaxpayerID: taxpayerID, CustomerID: c.ID, SourceSystem: inv.SourceSystem, SourceID: sourceID, RawPayload: raw,
	}); err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: c.ID, Valid: true}, nil
}

func insertCustomer(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice, typ, sourceID string) (db.Customer, error) {
	cp := inv.Counterparty
	raw, _ := json.Marshal(cp)
	// A savepoint keeps the outer transaction usable if a concurrent insert wins.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return db.Customer{}, err
	}
	c, err := db.New(sp).InsertCustomer(ctx, db.InsertCustomerParams{
		TaxpayerID: taxpayerID, SourceSystem: inv.SourceSystem, SourceID: sourceID, CaptureMethod: inv.CaptureMethod,
		Type: typ, CustomerLegalType: Text(cp.LegalType), TradeName: tradeName(cp), FirstName: Text(cp.FirstName), LastName: Text(cp.LastName),
		Name: cp.Name, TaxNumber: Text(cp.TaxNumber), IdentityNumber: Text(cp.IdentityNumber), TaxOffice: Text(cp.TaxOffice),
		Address: Text(cp.Address), City: Text(cp.City), District: Text(cp.District), PostalCode: Text(cp.PostalCode),
		Country: Text(cp.Country), Email: Text(cp.Email), Phone: Text(cp.Phone),
		IsDefaultRetailCustomer: cp.IsDefaultRetail, IsAbroad: false, RawPayload: raw,
	})
	if err != nil {
		_ = sp.Rollback(ctx)
		return db.Customer{}, err
	}
	return c, sp.Commit(ctx)
}

func tradeName(cp model.Party) pgtype.Text {
	if cp.LegalType == "corporate" {
		return Text(cp.Name)
	}
	return pgtype.Text{}
}

func invoiceParams(taxpayerID int64, inv model.Invoice, customerID pgtype.Int8, ublAttachmentID int64) (db.InsertInvoiceParams, error) {
	p := db.InsertInvoiceParams{
		TaxpayerID: taxpayerID, SourceSystem: inv.SourceSystem, SourceID: inv.SourceID, CaptureMethod: inv.CaptureMethod,
		InvoiceNumber: inv.InvoiceNumber, Direction: string(inv.Direction), Type: inv.Type,
		InvoiceTypeCodeRaw: Text(inv.TypeCodeRaw), ProfileRaw: Text(inv.ProfileRaw), DocumentClass: inv.DocumentClass,
		StatusNormalized: "active", IssueDate: Date(&inv.IssueDate), IssueTime: Text(inv.IssueTime),
		PaymentDueDate: Date(inv.PaymentDueDate), Period: inv.Period,
		CustomerID: customerID, CustomerTaxNumber: Text(inv.Counterparty.TaxNumber),
		CustomerIdentityNumber: Text(inv.Counterparty.IdentityNumber), CustomerName: Text(inv.Counterparty.Name),
		Currency: inv.Currency, ExchangeRate: NumPtr(inv.ExchangeRate),
		LineExtensionAmount: Num(inv.Amounts.LineExtensionAmount), AllowanceTotalAmount: Num(inv.Amounts.AllowanceTotalAmount),
		TaxExclusiveAmount: Num(inv.Amounts.TaxExclusiveAmount), TaxAmount: Num(inv.Amounts.TaxAmount),
		TaxInclusiveAmount: Num(inv.Amounts.TaxInclusiveAmount), WithholdingAmount: Num(inv.Amounts.WithholdingAmount),
		ExciseAmount: Num(inv.Amounts.ExciseAmount), CommunicationsTaxAmount: Num(inv.Amounts.CommunicationsTaxAmount),
		IncomeTaxWithholdingAmount: Num(inv.Amounts.IncomeTaxWithholdingAmount), IncomeTaxWithholdingRate: NumPtr(inv.IncomeTaxWithholdingRate),
		PayableAmount: Num(inv.Amounts.PayableAmount), PayableAmountTry: Num(inv.Amounts.PayableAmountTRY),
		GlobalTaxExemptionCode: Text(inv.GlobalTaxExemptionCode), OriginalInvoiceNumber: Text(inv.OriginalInvoiceNumber),
		OriginalInvoiceIssueDate: Date(inv.OriginalInvoiceIssueDate), Notes: Text(inv.Notes),
		HasItems: inv.HasItems, TaxSummarySource: inv.TaxSummarySource, ContentHash: Text(inv.ContentHash),
	}
	if inv.Profile != nil {
		p.Profile = Text(*inv.Profile)
	}
	if inv.UUID != "" {
		id, err := uuid.Parse(inv.UUID)
		if err != nil {
			return p, err
		}
		p.Ettn = pgtype.UUID{Bytes: id, Valid: true}
	}
	if ublAttachmentID > 0 {
		p.UblAttachmentID = pgtype.Int8{Int64: ublAttachmentID, Valid: true}
	}
	var err error
	if inv.OrderReference != nil {
		if p.OrderReference, err = json.Marshal(inv.OrderReference); err != nil {
			return p, err
		}
	}
	raw := inv.RawPayload
	if raw == nil {
		raw = map[string]any{}
	}
	if p.RawPayload, err = json.Marshal(raw); err != nil {
		return p, err
	}
	return p, nil
}

func itemParams(taxpayerID, invoiceID int64, it model.InvoiceItem) (db.InsertInvoiceItemParams, error) {
	tb := it.TaxBreakdowns
	if tb == nil {
		tb = []model.TaxBreakdown{}
	}
	raw, err := json.Marshal(tb)
	if err != nil {
		return db.InsertInvoiceItemParams{}, err
	}
	lineType := it.LineType
	if lineType == "" {
		lineType = model.LineUnknown
	}
	return db.InsertInvoiceItemParams{
		TaxpayerID: taxpayerID, InvoiceID: invoiceID, LineNo: int32(it.LineNo), //nolint:gosec // line numbers are small
		Name: it.Name, Description: Text(it.Description), ProductCode: Text(it.ProductCode),
		Quantity: Num(it.Quantity), UnitCode: Text(it.UnitCode), UnitPrice: Num(it.UnitPrice),
		LineExtensionAmount: Num(it.LineExtensionAmount), OwnAllowanceAmount: Num(it.OwnAllowanceAmount),
		AllowanceTotalAmount: Num(it.AllowanceTotalAmount), TaxExclusiveAmount: Num(it.TaxExclusiveAmount),
		TaxRate: Num(it.TaxRate), TotalTaxAmount: Num(it.TotalTaxAmount), TaxInclusiveAmount: Num(it.TaxInclusiveAmount),
		WithholdingCode: Text(it.WithholdingCode), WithholdingRate: Num(it.WithholdingRate), WithholdingAmount: Num(it.WithholdingAmount),
		TaxExemptionCode: Text(it.TaxExemptionCode), SpecialTaxBaseCode: Text(it.SpecialTaxBaseCode),
		SpecialTaxBaseAmount: NumPtr(it.SpecialTaxBaseAmount), ExportRegisteredCode: Text(it.ExportRegisteredCode),
		GtipCode: Text(it.GTIPCode), ExciseRate: Num(it.ExciseRate), ExciseAmount: Num(it.ExciseAmount),
		CommunicationsTaxRate: Num(it.CommunicationsTaxRate), CommunicationsTaxAmount: Num(it.CommunicationsTaxAmount),
		IncomeTaxWithholdingAmount: Num(it.IncomeTaxWithholdingAmount), LineType: lineType, TaxBreakdowns: raw,
	}, nil
}

// --- conversions -------------------------------------------------------------

// Num converts a decimal to pgtype.Numeric without going through float.
func Num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

// NumPtr is Num for optional values.
func NumPtr(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return Num(*d)
}

// Dec converts pgtype.Numeric back to decimal (zero when NULL).
func Dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

// Text maps "" to NULL.
func Text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// Date maps nil / zero to NULL.
func Date(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
