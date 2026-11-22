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
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// Outcomes of SaveInvoice.
const (
	OutcomeCreated  = "created"  // new invoice
	OutcomeMerged   = "merged"   // same invoice, new source; existing content kept
	OutcomeReplaced = "replaced" // same invoice, higher-priority source; content rewritten
	OutcomeSeen     = "seen"     // this source already delivered the invoice
	// OutcomeHeld: the write would change a locked period (`02` §3.4); the
	// invoice is untouched and a change request holds the new content.
	OutcomeHeld = "held"
)

// PendingChange is a write SaveInvoiceGuarded is about to make: a new
// invoice (Existing nil) or new content for an existing one.
type PendingChange struct {
	TaxpayerID   int64
	Invoice      model.Invoice
	AttachmentID int64
	Existing     *db.Invoice
	MatchRule    string
}

// Hold is a PeriodGuard verdict.
type Hold struct {
	Held              bool
	ChangeRequestUUID uuid.UUID
	Period            string
}

// PeriodGuard keeps locked periods unchanged (`02` §3.4). Hold is called
// inside the write transaction before an invoice is inserted or rewritten;
// when the period is locked it records a change request instead (same tx)
// and returns Held. The implementation lives in modules/periods.
type PeriodGuard interface {
	Hold(ctx context.Context, tx pgx.Tx, c PendingChange) (Hold, error)
}

// SaveResult says what SaveInvoice did.
type SaveResult struct {
	InvoiceID   int64
	InvoiceUUID uuid.UUID
	Outcome     string
	MatchRule   string
	// Written: the invoice content, lines and summary were written by this call
	// (created or replaced); warnings for the new content belong to it.
	Written bool
	// ItemIDs maps line_no to invoice_items.id when Written.
	ItemIDs map[int]int64
	// PossibleDuplicates: other invoices with the same counterparty, date and
	// payable amount (`01` §6 rule 3). Not merged; the caller raises
	// POSSIBLE_DUPLICATE.
	PossibleDuplicates []uuid.UUID
	// ChangeRequestUUID is the change request opened when Outcome is held.
	ChangeRequestUUID uuid.UUID
}

// Duplicate reports whether the invoice already existed.
func (r SaveResult) Duplicate() bool { return r.Outcome != OutcomeCreated && r.Outcome != OutcomeHeld }

// payableTolerance is `01` §6 rule 3 (±0,01).
var payableTolerance = decimal.RequireFromString("0.01")

// SaveInvoice stores one mapped invoice inside tx, applying `01` §6:
//
//  1. the same (source_system, source_id) seen before → nothing changes;
//  2. same ETTN → same invoice;
//  3. otherwise same counterparty VKN/TCKN + invoice number + date (when one
//     side has no ETTN) → same invoice;
//  4. otherwise a new invoice; same counterparty + date + payable (±0,01)
//     are returned as possible duplicates and not merged.
//
// A matched invoice gains a row in invoice_sources. Its content is rewritten
// only when the new source has a higher priority (entegratör / UBL > program
// > OCR / elle).
func SaveInvoice(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice, ublAttachmentID int64) (SaveResult, error) {
	return SaveInvoiceGuarded(ctx, tx, taxpayerID, inv, ublAttachmentID, nil)
}

// SaveInvoiceGuarded is SaveInvoice with a period guard: a write that would
// insert or rewrite an invoice is first offered to guard, and a held write
// changes nothing (no source row either, so the same content can be applied
// later). A nil guard writes unconditionally (applying a change request).
func SaveInvoiceGuarded(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice, ublAttachmentID int64, guard PeriodGuard) (SaveResult, error) {
	q := db.New(tx)
	priority := model.SourcePriority(inv.SourceSystem, inv.CaptureMethod)

	if src, err := q.GetInvoiceSourceBySource(ctx, db.GetInvoiceSourceBySourceParams{TaxpayerID: taxpayerID, SourceSystem: inv.SourceSystem, SourceID: inv.SourceID}); err == nil {
		existing, err := q.GetInvoiceByID(ctx, db.GetInvoiceByIDParams{TaxpayerID: taxpayerID, ID: src.InvoiceID})
		if err != nil {
			return SaveResult{}, err
		}
		if err := upsertSource(ctx, q, taxpayerID, existing.ID, inv, priority, model.MatchSource, ublAttachmentID); err != nil {
			return SaveResult{}, err
		}
		return SaveResult{InvoiceID: existing.ID, InvoiceUUID: existing.Uuid, Outcome: OutcomeSeen, MatchRule: model.MatchSource}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SaveResult{}, err
	}

	match, rule, err := findMatch(ctx, q, taxpayerID, inv)
	if err != nil {
		return SaveResult{}, err
	}
	if match != nil {
		res := SaveResult{InvoiceID: match.ID, InvoiceUUID: match.Uuid, Outcome: OutcomeMerged, MatchRule: rule}
		if priority > model.SourcePriority(match.SourceSystem, match.CaptureMethod) {
			held, err := hold(ctx, tx, guard, PendingChange{TaxpayerID: taxpayerID, Invoice: inv, AttachmentID: ublAttachmentID, Existing: match, MatchRule: rule})
			if err != nil {
				return SaveResult{}, err
			}
			if held.Held {
				res.Outcome, res.ChangeRequestUUID = OutcomeHeld, held.ChangeRequestUUID
				return res, nil
			}
			ids, err := replaceContent(ctx, tx, taxpayerID, match.ID, inv, ublAttachmentID)
			if err != nil {
				return SaveResult{}, err
			}
			res.Outcome, res.Written, res.ItemIDs = OutcomeReplaced, true, ids
		}
		if err := upsertSource(ctx, q, taxpayerID, match.ID, inv, priority, rule, ublAttachmentID); err != nil {
			return SaveResult{}, err
		}
		return res, nil
	}

	held, err := hold(ctx, tx, guard, PendingChange{TaxpayerID: taxpayerID, Invoice: inv, AttachmentID: ublAttachmentID, MatchRule: model.MatchNew})
	if err != nil {
		return SaveResult{}, err
	}
	if held.Held {
		return SaveResult{Outcome: OutcomeHeld, MatchRule: model.MatchNew, ChangeRequestUUID: held.ChangeRequestUUID}, nil
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
	ids, err := insertChildren(ctx, q, taxpayerID, row.ID, inv)
	if err != nil {
		return SaveResult{}, err
	}
	// Kaynakta kur yoksa TCMB tablosundan otomatik kur (#780): faturanın
	// tarihinden önceki son bülten, ofisin kur türü. Tabloda yoksa kur eksik
	// kalır; worker yeni bülteni çekince doldurur.
	if inv.Currency != "" && inv.Currency != "TRY" && inv.ExchangeRate == nil {
		if _, err := q.FillInvoiceExchangeRates(ctx, db.FillInvoiceExchangeRatesParams{
			InvoiceID: pgtype.Int8{Int64: row.ID, Valid: true},
		}); err != nil {
			return SaveResult{}, fmt.Errorf("repository: exchange rate: %w", err)
		}
	}
	if err := upsertSource(ctx, q, taxpayerID, row.ID, inv, priority, model.MatchNew, ublAttachmentID); err != nil {
		return SaveResult{}, err
	}
	res := SaveResult{InvoiceID: row.ID, InvoiceUUID: row.Uuid, Outcome: OutcomeCreated, MatchRule: model.MatchNew, Written: true, ItemIDs: ids}
	if res.PossibleDuplicates, err = possibleDuplicates(ctx, q, taxpayerID, row.ID, inv); err != nil {
		return SaveResult{}, err
	}
	return res, nil
}

func hold(ctx context.Context, tx pgx.Tx, guard PeriodGuard, c PendingChange) (Hold, error) {
	if guard == nil {
		return Hold{}, nil
	}
	h, err := guard.Hold(ctx, tx, c)
	if err != nil {
		return Hold{}, fmt.Errorf("repository: period guard: %w", err)
	}
	return h, nil
}

func findMatch(ctx context.Context, q *db.Queries, taxpayerID int64, inv model.Invoice) (*db.Invoice, string, error) {
	var ettn pgtype.UUID
	if inv.UUID != "" {
		id, err := uuid.Parse(inv.UUID)
		if err != nil {
			return nil, "", fmt.Errorf("repository: ettn: %w", err)
		}
		ettn = pgtype.UUID{Bytes: id, Valid: true}
		found, err := q.FindInvoicesByETTN(ctx, db.FindInvoicesByETTNParams{TaxpayerID: taxpayerID, Ettn: ettn})
		if err != nil {
			return nil, "", err
		}
		if len(found) > 0 {
			return &found[0], model.MatchETTN, nil
		}
	}
	cp := inv.Counterparty
	if inv.InvoiceNumber == "" || (cp.TaxNumber == "" && cp.IdentityNumber == "") {
		return nil, "", nil
	}
	found, err := q.FindInvoicesByPartyNumberDate(ctx, db.FindInvoicesByPartyNumberDateParams{
		TaxpayerID: taxpayerID, InvoiceNumber: inv.InvoiceNumber, IssueDate: Date(&inv.IssueDate),
		CustomerTaxNumber: Text(cp.TaxNumber), CustomerIdentityNumber: Text(cp.IdentityNumber),
	})
	if err != nil {
		return nil, "", err
	}
	for i := range found {
		// Two different ETTNs are two documents, whatever their numbers say.
		if ettn.Valid && found[i].Ettn.Valid && found[i].Ettn != ettn {
			continue
		}
		return &found[i], model.MatchNumber, nil
	}
	return nil, "", nil
}

func possibleDuplicates(ctx context.Context, q *db.Queries, taxpayerID, invoiceID int64, inv model.Invoice) ([]uuid.UUID, error) {
	cp := inv.Counterparty
	if cp.TaxNumber == "" && cp.IdentityNumber == "" {
		return nil, nil
	}
	rows, err := q.FindPossibleDuplicateInvoices(ctx, db.FindPossibleDuplicateInvoicesParams{
		TaxpayerID: taxpayerID, IssueDate: Date(&inv.IssueDate), Direction: string(inv.Direction),
		CustomerTaxNumber: Text(cp.TaxNumber), CustomerIdentityNumber: Text(cp.IdentityNumber),
		PayableMin: Num(inv.Amounts.PayableAmount.Sub(payableTolerance)),
		PayableMax: Num(inv.Amounts.PayableAmount.Add(payableTolerance)),
		ExcludeID:  invoiceID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Uuid)
	}
	return out, nil
}

func replaceContent(ctx context.Context, tx pgx.Tx, taxpayerID, invoiceID int64, inv model.Invoice, ublAttachmentID int64) (map[int]int64, error) {
	q := db.New(tx)
	customerID, err := findOrCreateCustomer(ctx, tx, taxpayerID, inv)
	if err != nil {
		return nil, fmt.Errorf("repository: customer: %w", err)
	}
	p, err := invoiceParams(taxpayerID, inv, customerID, ublAttachmentID)
	if err != nil {
		return nil, err
	}
	n, err := q.ReplaceInvoiceContent(ctx, db.ReplaceInvoiceContentParams{
		TaxpayerID: taxpayerID, ID: invoiceID,
		SourceSystem: p.SourceSystem, SourceID: p.SourceID, CaptureMethod: p.CaptureMethod, Ettn: p.Ettn,
		InvoiceNumber: p.InvoiceNumber, Direction: p.Direction, Type: p.Type,
		InvoiceTypeCodeRaw: p.InvoiceTypeCodeRaw, Profile: p.Profile, ProfileRaw: p.ProfileRaw, DocumentClass: p.DocumentClass,
		IssueDate: p.IssueDate, IssueTime: p.IssueTime, PaymentDueDate: p.PaymentDueDate, Period: p.Period,
		CustomerID: p.CustomerID, CustomerTaxNumber: p.CustomerTaxNumber, CustomerIdentityNumber: p.CustomerIdentityNumber,
		CustomerName: p.CustomerName, Currency: p.Currency, ExchangeRate: p.ExchangeRate,
		LineExtensionAmount: p.LineExtensionAmount, AllowanceTotalAmount: p.AllowanceTotalAmount,
		TaxExclusiveAmount: p.TaxExclusiveAmount, TaxAmount: p.TaxAmount, TaxInclusiveAmount: p.TaxInclusiveAmount,
		WithholdingAmount: p.WithholdingAmount, ExciseAmount: p.ExciseAmount, CommunicationsTaxAmount: p.CommunicationsTaxAmount,
		IncomeTaxWithholdingAmount: p.IncomeTaxWithholdingAmount, IncomeTaxWithholdingRate: p.IncomeTaxWithholdingRate,
		PayableAmount: p.PayableAmount, PayableAmountTry: p.PayableAmountTry,
		GlobalTaxExemptionCode: p.GlobalTaxExemptionCode, OriginalInvoiceNumber: p.OriginalInvoiceNumber,
		OriginalInvoiceIssueDate: p.OriginalInvoiceIssueDate, OrderReference: p.OrderReference, Notes: p.Notes,
		HasItems: p.HasItems, TaxSummarySource: p.TaxSummarySource, UblAttachmentID: p.UblAttachmentID,
		RawPayload: p.RawPayload, ContentHash: p.ContentHash,
	})
	if err != nil {
		return nil, fmt.Errorf("repository: replace invoice: %w", err)
	}
	if n != 1 {
		return nil, fmt.Errorf("repository: replace invoice: %d rows", n)
	}
	if err := q.DeleteInvoiceChildren(ctx, db.DeleteInvoiceChildrenParams{TaxpayerID: taxpayerID, InvoiceID: invoiceID}); err != nil {
		return nil, err
	}
	return insertChildren(ctx, q, taxpayerID, invoiceID, inv)
}

func insertChildren(ctx context.Context, q *db.Queries, taxpayerID, invoiceID int64, inv model.Invoice) (map[int]int64, error) {
	itemIDs := make(map[int]int64, len(inv.Items))
	for _, it := range inv.Items {
		p, err := itemParams(taxpayerID, invoiceID, it)
		if err != nil {
			return nil, err
		}
		id, err := q.InsertInvoiceItem(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("repository: item %d: %w", it.LineNo, err)
		}
		itemIDs[it.LineNo] = id
	}
	for _, s := range inv.TaxSummaries {
		if err := q.InsertInvoiceTaxSummary(ctx, db.InsertInvoiceTaxSummaryParams{
			TaxpayerID: taxpayerID, InvoiceID: invoiceID,
			TaxRate: Num(s.TaxRate), Base: Num(s.Base), Vat: Num(s.VAT), Withholding: Num(s.Withholding),
			WithholdingCode: Text(s.WithholdingCode), TaxExemptionCode: Text(s.TaxExemptionCode),
		}); err != nil {
			return nil, fmt.Errorf("repository: tax summary: %w", err)
		}
	}
	return itemIDs, nil
}

func upsertSource(ctx context.Context, q *db.Queries, taxpayerID, invoiceID int64, inv model.Invoice, priority int, rule string, attachmentID int64) error {
	att := pgtype.Int8{}
	if attachmentID > 0 {
		att = pgtype.Int8{Int64: attachmentID, Valid: true}
	}
	_, err := q.UpsertInvoiceSource(ctx, db.UpsertInvoiceSourceParams{
		TaxpayerID: taxpayerID, InvoiceID: invoiceID, SourceSystem: inv.SourceSystem, SourceID: inv.SourceID,
		CaptureMethod: inv.CaptureMethod, Priority: int16(priority), MatchRule: rule, //nolint:gosec // 1..3
		ContentHash: Text(inv.ContentHash), AttachmentID: att, RawPayload: []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("repository: invoice source: %w", err)
	}
	return nil
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
	raw, err := sealCounterparty(cp)
	if err != nil {
		return pgtype.Int8{}, err
	}
	if err := q.UpsertCustomerSourceLink(ctx, db.UpsertCustomerSourceLinkParams{
		TaxpayerID: taxpayerID, CustomerID: c.ID, SourceSystem: inv.SourceSystem, SourceID: sourceID, RawPayload: raw,
	}); err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: c.ID, Valid: true}, nil
}

func insertCustomer(ctx context.Context, tx pgx.Tx, taxpayerID int64, inv model.Invoice, typ, sourceID string) (db.Customer, error) {
	cp := inv.Counterparty
	raw, err := sealCounterparty(cp)
	if err != nil {
		return db.Customer{}, err
	}
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
	if p.RawPayload, err = secrets.SealField(secrets.PurposeInvoiceRawPayload, p.RawPayload); err != nil {
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

// sealCounterparty is the customer raw_payload: the counterparty as parsed,
// sealed with the field key (#168).
func sealCounterparty(cp model.Party) ([]byte, error) {
	raw, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}
	return secrets.SealField(secrets.PurposeCustomerRawPayload, raw)
}
