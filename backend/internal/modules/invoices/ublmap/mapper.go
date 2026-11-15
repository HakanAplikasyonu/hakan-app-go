// Package ublmap maps a UBL-TR invoice (go-ubltr generated types) to the
// common invoice model (kaynak paket `31`). It does not validate against XSD
// or Schematron (that is the upload / sync step) and it does not touch the
// database.
package ublmap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

// Tolerances from `31` §5.1 and §6.
var (
	lineTolerance  = decimal.RequireFromString("0.01")
	totalTolerance = decimal.RequireFromString("0.05")
)

// DefaultRetailTCKN is the generic end-consumer id on e-Arşiv invoices (`31` §4).
const DefaultRetailTCKN = "11111111111"

// Options describes where the XML came from.
type Options struct {
	// OwnTaxID is the taxpayer's VKN / TCKN from the connection record, never
	// from the request (`32` §2). Required.
	OwnTaxID string
	// Direction, when the source knows it (an inbox vs outbox listing). Empty:
	// derived from which side carries OwnTaxID.
	Direction model.Direction
	// SourceSystem and CaptureMethod default to manual_upload / ubl_upload.
	SourceSystem  string
	CaptureMethod string
}

// Result is a mapped invoice plus the warnings raised while mapping.
type Result struct {
	Invoice  model.Invoice
	Warnings []model.Warning
}

// PartyMismatchError means the taxpayer's own side of the invoice does not
// carry OwnTaxID; the invoice must not be stored (`31` §4, PARTY_MISMATCH).
type PartyMismatchError struct {
	OwnTaxID    string
	SupplierIDs []string
	CustomerIDs []string
	Direction   model.Direction
}

func (e *PartyMismatchError) Error() string {
	return fmt.Sprintf("ublmap: %s: own tax id is on neither the expected side of the invoice", model.WarnPartyMismatch)
}

// Warning returns the PARTY_MISMATCH warning for this error.
func (e *PartyMismatchError) Warning() model.Warning {
	return model.Warning{Code: model.WarnPartyMismatch, EntityType: model.EntityInvoice, Params: map[string]any{
		"direction":    string(e.Direction),
		"supplier_ids": e.SupplierIDs,
		"customer_ids": e.CustomerIDs,
	}}
}

var (
	ErrNoOwnTaxID  = errors.New("ublmap: OwnTaxID is required")
	ErrInvalidETTN = errors.New("ublmap: cbc:UUID is not a valid ETTN")
	ErrIssueDate   = errors.New("ublmap: cbc:IssueDate is not a valid date")
)

// MapXML unmarshals one Invoice XML document and maps it.
func MapXML(data []byte, opt Options) (*Result, error) {
	var inv invoice.Invoice
	if err := ubltr.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("ublmap: unmarshal: %w", err)
	}
	return Map(&inv, data, opt)
}

// Map maps a parsed invoice. raw is the original document bytes (for
// content_hash); it may be nil when the caller hashes elsewhere.
func Map(inv *invoice.Invoice, raw []byte, opt Options) (*Result, error) {
	own := model.DigitsOnly(opt.OwnTaxID)
	if own == "" {
		return nil, ErrNoOwnTaxID
	}
	m := &mapper{inv: inv, res: &Result{}}
	if err := m.header(raw, opt, own); err != nil {
		return nil, err
	}
	m.headerTaxes()
	if len(inv.InvoiceLine) > 0 {
		m.lines()
		m.summaryFromItems()
		m.checks()
	} else {
		m.headerEstimate()
	}
	return m.res, nil
}

type mapper struct {
	inv *invoice.Invoice
	res *Result

	headerVAT          decimal.Decimal
	headerVATByRate    map[string]decimal.Decimal
	headerVATSubtotals []cac.TaxSubtotalType
	unknownCodes       map[string]bool
}

func (m *mapper) warn(code string, params map[string]any) {
	m.res.Warnings = append(m.res.Warnings, model.Warning{Code: code, EntityType: model.EntityInvoice, Params: params})
}

func (m *mapper) warnLine(code string, lineNo int, params map[string]any) {
	m.res.Warnings = append(m.res.Warnings, model.Warning{Code: code, EntityType: model.EntityInvoiceItem, LineNo: lineNo, Params: params})
}

func (m *mapper) unknownTaxCode(kind, code string) {
	if m.unknownCodes == nil {
		m.unknownCodes = map[string]bool{}
	}
	key := kind + ":" + code
	if m.unknownCodes[key] {
		return
	}
	m.unknownCodes[key] = true
	m.warn(model.WarnUnknownTaxCode, map[string]any{"kind": kind, "code": code})
}

// --- header -----------------------------------------------------------------

var typeMap = map[string]string{
	"SATIS":        model.TypeSatis,
	"IADE":         model.TypeIade,
	"TEVKIFAT":     model.TypeTevkifat,
	"ISTISNA":      model.TypeIstisna,
	"OZELMATRAH":   model.TypeOzelMatrah,
	"IHRACKAYITLI": model.TypeIhracKayitli,
	"SGK":          model.TypeSGK,
	"TEVKIFATIADE": model.TypeIade,
}

var profileMap = map[string]string{
	"TEMELFATURA":        model.ProfileTemel,
	"TICARIFATURA":       model.ProfileTicari,
	"EARSIVFATURA":       model.ProfileEArsiv,
	"IHRACAT":            model.ProfileIhracat,
	"YOLCUBERABERFATURA": model.ProfileYolcuBeraber,
	"KAMU":               model.ProfileKamu,
	"ILAC_TIBBICIHAZ":    model.ProfileIlacTibbiCihaz,
}

func (m *mapper) header(raw []byte, opt Options, own string) error {
	inv := m.inv
	out := &m.res.Invoice
	out.SourceSystem = firstNonEmpty(opt.SourceSystem, "manual_upload")
	out.CaptureMethod = firstNonEmpty(opt.CaptureMethod, model.CaptureUBLUpload)
	out.DocumentClass = "invoice"
	out.RawPayload = map[string]any{}

	ettn, err := uuid.Parse(strings.TrimSpace(inv.UUID.Value))
	if err != nil {
		return ErrInvalidETTN
	}
	out.UUID = ettn.String()
	out.SourceID = out.UUID
	out.InvoiceNumber = model.NormalizeInvoiceNumber(inv.ID.Value)

	issue, err := parseDate(inv.IssueDate.Value)
	if err != nil {
		return ErrIssueDate
	}
	out.IssueDate = issue
	out.Period = issue.Format("2006-01")
	if inv.IssueTime != nil {
		out.IssueTime = strings.TrimSpace(inv.IssueTime.Value)
	}

	if err := m.direction(opt, own); err != nil {
		return err
	}

	// Type (`31` §3.1).
	rawType := strings.ToUpper(strings.TrimSpace(inv.InvoiceTypeCode.Value))
	out.TypeCodeRaw = rawType
	if t, ok := typeMap[rawType]; ok {
		out.Type = t
	} else {
		out.Type = model.TypeSatis
		if out.Direction == model.DirectionIncoming {
			out.Type = model.TypeAlis
		}
		m.warn(model.WarnUnmappedTypeCode, map[string]any{"value": rawType})
	}
	if out.Type == model.TypeSatis && out.Direction == model.DirectionIncoming {
		out.Type = model.TypeAlis
	}

	// Profile (`31` §3.2). UBL never yields a nil profile.
	rawProfile := strings.ToUpper(strings.TrimSpace(inv.ProfileID.Value))
	out.ProfileRaw = rawProfile
	p, ok := profileMap[rawProfile]
	if !ok {
		p = model.ProfileOzel
		m.warn(model.WarnUnmappedProfile, map[string]any{"value": rawProfile})
	}
	out.Profile = &p

	out.Currency = model.NormalizeCurrency(inv.DocumentCurrencyCode.Value)
	if inv.PricingExchangeRate != nil {
		if r, err := dec(inv.PricingExchangeRate.CalculationRate.Value); err == nil && r.IsPositive() {
			out.ExchangeRate = &r
		}
	}

	var notes []string
	for _, n := range inv.Note {
		if v := strings.TrimSpace(n.Value); v != "" {
			notes = append(notes, v)
		}
	}
	out.Notes = strings.Join(notes, "\n")

	for _, br := range inv.BillingReference {
		if br.InvoiceDocumentReference == nil {
			continue
		}
		out.OriginalInvoiceNumber = model.NormalizeInvoiceNumber(br.InvoiceDocumentReference.ID.Value)
		if d, err := parseDate(br.InvoiceDocumentReference.IssueDate.Value); err == nil {
			out.OriginalInvoiceIssueDate = &d
		}
		break
	}
	if or := inv.OrderReference; or != nil {
		out.OrderReference = map[string]any{"id": strings.TrimSpace(or.ID.Value)}
		if v := strings.TrimSpace(or.IssueDate.Value); v != "" {
			out.OrderReference["issue_date"] = v
		}
	}
	if len(inv.DespatchDocumentReference) > 0 {
		var refs []map[string]string
		for _, d := range inv.DespatchDocumentReference {
			refs = append(refs, map[string]string{"id": strings.TrimSpace(d.ID.Value), "issue_date": strings.TrimSpace(d.IssueDate.Value)})
		}
		out.RawPayload["despatch_references"] = refs
	}
	for _, pm := range inv.PaymentMeans {
		if pm.PaymentDueDate == nil {
			continue
		}
		if d, err := parseDate(pm.PaymentDueDate.Value); err == nil {
			out.PaymentDueDate = &d
			break
		}
	}

	// Totals (`31` §2).
	lmt := inv.LegalMonetaryTotal
	out.Amounts.LineExtensionAmount = decOrZero(lmt.LineExtensionAmount.Value)
	out.Amounts.TaxExclusiveAmount = decOrZero(lmt.TaxExclusiveAmount.Value)
	out.Amounts.TaxInclusiveAmount = decOrZero(lmt.TaxInclusiveAmount.Value)
	out.Amounts.PayableAmount = decOrZero(lmt.PayableAmount.Value)
	if lmt.AllowanceTotalAmount != nil {
		out.Amounts.AllowanceTotalAmount = decOrZero(lmt.AllowanceTotalAmount.Value)
	}
	if lmt.ChargeTotalAmount != nil {
		out.RawPayload["charge_total_amount"] = strings.TrimSpace(lmt.ChargeTotalAmount.Value)
	}
	if lmt.PayableRoundingAmount != nil {
		out.RawPayload["payable_rounding_amount"] = strings.TrimSpace(lmt.PayableRoundingAmount.Value)
	}
	for _, wt := range inv.WithholdingTaxTotal {
		out.Amounts.WithholdingAmount = out.Amounts.WithholdingAmount.Add(decOrZero(wt.TaxAmount.Value))
		for _, st := range wt.TaxSubtotal {
			if code := taxTypeCode(st); !codelist.ValidWithholdingTax(code) {
				m.unknownTaxCode("withholding", code)
			}
		}
	}
	out.Amounts.PayableAmountTRY = out.Amounts.PayableAmount
	if out.Currency != "TRY" && out.ExchangeRate != nil {
		out.Amounts.PayableAmountTRY = out.Amounts.PayableAmount.Mul(*out.ExchangeRate).Round(4)
	}

	if raw != nil {
		sum := sha256.Sum256(raw)
		out.ContentHash = hex.EncodeToString(sum[:])
	}
	return nil
}

// direction decides outgoing / incoming and checks the taxpayer's own side
// (`31` §4).
func (m *mapper) direction(opt Options, own string) error {
	supplier := &m.inv.AccountingSupplierParty.Party
	customer := &m.inv.AccountingCustomerParty.Party
	sIDs, cIDs := partyTaxIDs(supplier), partyTaxIDs(customer)
	onSupplier, onCustomer := contains(sIDs, own), contains(cIDs, own)

	dir := opt.Direction
	switch dir {
	case model.DirectionOutgoing:
		if !onSupplier {
			return &PartyMismatchError{OwnTaxID: own, SupplierIDs: sIDs, CustomerIDs: cIDs, Direction: dir}
		}
	case model.DirectionIncoming:
		if !onCustomer {
			return &PartyMismatchError{OwnTaxID: own, SupplierIDs: sIDs, CustomerIDs: cIDs, Direction: dir}
		}
	default:
		switch {
		case onSupplier:
			dir = model.DirectionOutgoing
		case onCustomer:
			dir = model.DirectionIncoming
		default:
			return &PartyMismatchError{OwnTaxID: own, SupplierIDs: sIDs, CustomerIDs: cIDs}
		}
	}
	m.res.Invoice.Direction = dir
	counter := customer
	if dir == model.DirectionIncoming {
		counter = supplier
	}
	m.res.Invoice.Counterparty = m.party(counter)
	return nil
}

// partyTaxIDs returns the normalised VKN / TCKN identifications of a party.
func partyTaxIDs(p *cac.PartyType) []string {
	var out []string
	for _, id := range p.PartyIdentification {
		switch strings.ToUpper(strings.TrimSpace(id.ID.SchemeID)) {
		case "VKN", "TCKN":
			if d := model.DigitsOnly(id.ID.Value); d != "" {
				out = append(out, d)
			}
		}
	}
	return out
}

func (m *mapper) party(p *cac.PartyType) model.Party {
	var out model.Party
	var others []map[string]string
	for _, id := range p.PartyIdentification {
		scheme := strings.ToUpper(strings.TrimSpace(id.ID.SchemeID))
		if scheme != "VKN" && scheme != "TCKN" {
			others = append(others, map[string]string{"scheme": id.ID.SchemeID, "id": strings.TrimSpace(id.ID.Value)})
			continue
		}
		vkn, tckn, ok := model.ClassifyTaxID(id.ID.Value)
		if !ok {
			m.warn(model.WarnInvalidTaxID, map[string]any{"scheme": scheme, "value": strings.TrimSpace(id.ID.Value)})
			continue
		}
		if vkn != "" && out.TaxNumber == "" {
			out.TaxNumber = vkn
		}
		if tckn != "" && out.IdentityNumber == "" {
			out.IdentityNumber = tckn
		}
	}
	if len(others) > 0 {
		m.res.Invoice.RawPayload["counterparty_identifications"] = others
	}
	if p.PartyName != nil {
		out.Name = strings.TrimSpace(p.PartyName.Name.Value)
	}
	if p.Person != nil {
		out.FirstName = strings.TrimSpace(p.Person.FirstName.Value)
		out.LastName = strings.TrimSpace(p.Person.FamilyName.Value)
		if out.Name == "" {
			out.Name = strings.TrimSpace(out.FirstName + " " + out.LastName)
		}
	}
	out.LegalType = "corporate"
	if out.TaxNumber == "" && out.IdentityNumber != "" {
		out.LegalType = "individual"
	}
	if p.PartyTaxScheme != nil && p.PartyTaxScheme.TaxScheme.Name != nil {
		out.TaxOffice = strings.TrimSpace(p.PartyTaxScheme.TaxScheme.Name.Value)
	}
	a := p.PostalAddress
	var street []string
	if a.StreetName != nil {
		street = append(street, strings.TrimSpace(a.StreetName.Value))
	}
	if a.BuildingName != nil {
		street = append(street, strings.TrimSpace(a.BuildingName.Value))
	}
	if a.BuildingNumber != nil {
		street = append(street, strings.TrimSpace(a.BuildingNumber.Value))
	}
	out.Address = strings.TrimSpace(strings.Join(street, " "))
	out.City = strings.TrimSpace(a.CityName.Value)
	out.District = strings.TrimSpace(a.CitySubdivisionName.Value)
	if a.PostalZone != nil {
		out.PostalCode = strings.TrimSpace(a.PostalZone.Value)
	}
	out.Country = strings.TrimSpace(a.Country.Name.Value)
	if c := p.Contact; c != nil {
		if c.ElectronicMail != nil {
			out.Email = strings.TrimSpace(c.ElectronicMail.Value)
		}
		if c.Telephone != nil {
			out.Phone = strings.TrimSpace(c.Telephone.Value)
		}
	}
	out.IsDefaultRetail = out.IdentityNumber == DefaultRetailTCKN
	return out
}

// --- taxes ------------------------------------------------------------------

func taxTypeCode(st cac.TaxSubtotalType) string {
	if st.TaxCategory.TaxScheme.TaxTypeCode == nil {
		return ""
	}
	return strings.TrimSpace(st.TaxCategory.TaxScheme.TaxTypeCode.Value)
}

func exemptionCode(st cac.TaxSubtotalType) string {
	if st.TaxCategory.TaxExemptionReasonCode == nil {
		return ""
	}
	return strings.TrimSpace(st.TaxCategory.TaxExemptionReasonCode.Value)
}

func percent(st cac.TaxSubtotalType) decimal.Decimal {
	if st.Percent == nil {
		return decimal.Zero
	}
	return model.NormalizePercent(decOrZero(st.Percent.Value))
}

// headerTaxes reads the invoice-level cac:TaxTotal (`31` §2): only 0015
// subtotals feed amounts.tax_amount; ÖTV / ÖİV / stopaj go to their fields.
func (m *mapper) headerTaxes() {
	out := &m.res.Invoice
	m.headerVATByRate = map[string]decimal.Decimal{}
	var codes []string
	for _, tt := range m.inv.TaxTotal {
		for _, st := range tt.TaxSubtotal {
			code := taxTypeCode(st)
			amt := decOrZero(st.TaxAmount.Value)
			if !codelist.ValidTax(code) {
				m.unknownTaxCode("tax", code)
				continue
			}
			switch taxField(code) {
			case fieldVAT:
				m.headerVAT = m.headerVAT.Add(amt)
				key := percent(st).String()
				m.headerVATByRate[key] = m.headerVATByRate[key].Add(amt)
				m.headerVATSubtotals = append(m.headerVATSubtotals, st)
				codes = append(codes, exemptionCode(st))
			case fieldExcise:
				out.Amounts.ExciseAmount = out.Amounts.ExciseAmount.Add(amt)
			case fieldCommunicationsTax:
				out.Amounts.CommunicationsTaxAmount = out.Amounts.CommunicationsTaxAmount.Add(amt)
			case fieldIncomeTaxWithholding:
				out.Amounts.IncomeTaxWithholdingAmount = out.Amounts.IncomeTaxWithholdingAmount.Add(amt)
				if out.IncomeTaxWithholdingRate == nil && st.Percent != nil {
					r := percent(st)
					out.IncomeTaxWithholdingRate = &r
				}
			}
		}
	}
	out.Amounts.TaxAmount = m.headerVAT
	// Invoice-wide exemption: every VAT subtotal carries the same code (`31` §5.2).
	if len(codes) > 0 && codes[0] != "" {
		same := true
		for _, c := range codes[1:] {
			if c != codes[0] {
				same = false
				break
			}
		}
		if same {
			out.GlobalTaxExemptionCode = codes[0]
		}
	}
}

// --- lines ------------------------------------------------------------------

func (m *mapper) lines() {
	out := &m.res.Invoice
	out.HasItems = true
	out.TaxSummarySource = model.TaxSummaryFromItems
	for i, l := range m.inv.InvoiceLine {
		out.Items = append(out.Items, m.line(i, l))
	}
}

func (m *mapper) line(idx int, l cac.InvoiceLineType) model.InvoiceItem {
	it := model.InvoiceItem{LineNo: idx + 1, LineType: model.LineUnknown, TaxBreakdowns: []model.TaxBreakdown{}}
	if n, err := strconv.Atoi(strings.TrimSpace(l.ID.Value)); err == nil && n > 0 {
		it.LineNo = n
	}
	it.Name = strings.TrimSpace(l.Item.Name.Value)
	if l.Item.Description != nil {
		it.Description = strings.TrimSpace(l.Item.Description.Value)
	}
	if l.Item.SellersItemIdentification != nil {
		it.ProductCode = strings.TrimSpace(l.Item.SellersItemIdentification.ID.Value)
	}
	it.Quantity = decOrZero(l.InvoicedQuantity.Value)
	it.UnitCode = strings.TrimSpace(l.InvoicedQuantity.UnitCode)
	it.UnitPrice = decOrZero(l.Price.PriceAmount.Value)
	it.LineExtensionAmount = it.Quantity.Mul(it.UnitPrice).Round(4)
	for _, ac := range l.AllowanceCharge {
		if !isTrue(ac.ChargeIndicator.Value) {
			it.OwnAllowanceAmount = it.OwnAllowanceAmount.Add(decOrZero(ac.Amount.Value))
		}
	}
	it.AllowanceTotalAmount = it.OwnAllowanceAmount
	it.TaxExclusiveAmount = decOrZero(l.LineExtensionAmount.Value)

	var vatTaxable *decimal.Decimal
	vatSeen := false
	if l.TaxTotal != nil {
		for _, st := range l.TaxTotal.TaxSubtotal {
			code := taxTypeCode(st)
			amt := decOrZero(st.TaxAmount.Value)
			it.TaxBreakdowns = append(it.TaxBreakdowns, breakdown("tax", st))
			if !codelist.ValidTax(code) {
				m.unknownTaxCode("tax", code)
				continue
			}
			switch taxField(code) {
			case fieldVAT:
				it.TotalTaxAmount = it.TotalTaxAmount.Add(amt)
				if !vatSeen {
					vatSeen = true
					it.TaxRate = percent(st)
					it.TaxExemptionCode = exemptionCode(st)
				}
				if st.TaxableAmount != nil {
					v := decOrZero(st.TaxableAmount.Value)
					if vatTaxable != nil {
						v = v.Add(*vatTaxable)
					}
					vatTaxable = &v
				}
			case fieldExcise:
				it.ExciseAmount = it.ExciseAmount.Add(amt)
				it.ExciseRate = percent(st)
			case fieldCommunicationsTax:
				it.CommunicationsTaxAmount = it.CommunicationsTaxAmount.Add(amt)
				it.CommunicationsTaxRate = percent(st)
			case fieldIncomeTaxWithholding:
				it.IncomeTaxWithholdingAmount = it.IncomeTaxWithholdingAmount.Add(amt)
			}
		}
	}
	for _, wt := range l.WithholdingTaxTotal {
		for _, st := range wt.TaxSubtotal {
			code := taxTypeCode(st)
			it.TaxBreakdowns = append(it.TaxBreakdowns, breakdown("withholding", st))
			if !codelist.ValidWithholdingTax(code) {
				m.unknownTaxCode("withholding", code)
			}
			it.WithholdingAmount = it.WithholdingAmount.Add(decOrZero(st.TaxAmount.Value))
			if it.WithholdingCode == "" {
				it.WithholdingCode = code
				it.WithholdingRate = percent(st)
			}
		}
	}

	// Special base (801–811) and export-registered (701–704) codes sit in the
	// KDV subtotal's TaxExemptionReasonCode (`31` §5.2).
	if c := it.TaxExemptionCode; c != "" {
		switch {
		case codelist.ValidExemptionOzelMatrah(c):
			it.SpecialTaxBaseCode = c
			if vatTaxable != nil {
				v := *vatTaxable
				it.SpecialTaxBaseAmount = &v
			}
		case codelist.ValidExemptionIhrac(c):
			it.ExportRegisteredCode = c
		}
	}
	for _, d := range l.Delivery {
		if d.Shipment == nil {
			continue
		}
		for _, g := range d.Shipment.GoodsItem {
			if g.RequiredCustomsID != nil && it.GTIPCode == "" {
				it.GTIPCode = strings.TrimSpace(g.RequiredCustomsID.Value)
			}
		}
	}

	it.TaxInclusiveAmount = it.TaxExclusiveAmount.Add(it.TotalTaxAmount).Add(it.ExciseAmount).Add(it.CommunicationsTaxAmount)

	// LineExtensionAmount must equal the KDV TaxableAmount (`31` §5.1). ÖTV and
	// ÖİV are part of the KDV base, so they are added before comparing.
	if vatTaxable != nil && it.SpecialTaxBaseCode == "" {
		expected := it.TaxExclusiveAmount.Add(it.ExciseAmount).Add(it.CommunicationsTaxAmount)
		if expected.Sub(*vatTaxable).Abs().GreaterThan(lineTolerance) {
			m.warnLine(model.WarnLineTaxableMismatch, it.LineNo, map[string]any{
				"expected": expected.StringFixed(2), "actual": vatTaxable.StringFixed(2),
			})
		}
	}
	return it
}

func breakdown(kind string, st cac.TaxSubtotalType) model.TaxBreakdown {
	b := model.TaxBreakdown{Kind: kind, TaxTypeCode: taxTypeCode(st), TaxAmount: strings.TrimSpace(st.TaxAmount.Value), Currency: st.TaxAmount.CurrencyID}
	if st.TaxableAmount != nil {
		b.TaxableAmount = strings.TrimSpace(st.TaxableAmount.Value)
	}
	if st.Percent != nil {
		b.Percent = strings.TrimSpace(st.Percent.Value)
	}
	if st.TaxCategory.TaxScheme.Name != nil {
		b.Name = strings.TrimSpace(st.TaxCategory.TaxScheme.Name.Value)
	}
	b.ExemptionCode = exemptionCode(st)
	if st.TaxCategory.TaxExemptionReason != nil {
		b.Exemption = strings.TrimSpace(st.TaxCategory.TaxExemptionReason.Value)
	}
	return b
}

// --- summary and checks ---------------------------------------------------

type summaryKey struct{ rate, wcode, excode string }

// summaryFromItems implements `31` §6: GROUP BY rate, withholding code, exemption code.
func (m *mapper) summaryFromItems() {
	out := &m.res.Invoice
	groups := map[summaryKey]*model.TaxSummary{}
	var order []summaryKey
	for _, it := range out.Items {
		k := summaryKey{it.TaxRate.String(), it.WithholdingCode, it.TaxExemptionCode}
		g, ok := groups[k]
		if !ok {
			g = &model.TaxSummary{TaxRate: it.TaxRate, WithholdingCode: it.WithholdingCode, TaxExemptionCode: it.TaxExemptionCode}
			groups[k] = g
			order = append(order, k)
		}
		g.Base = g.Base.Add(it.TaxExclusiveAmount)
		g.VAT = g.VAT.Add(it.TotalTaxAmount)
		g.Withholding = g.Withholding.Add(it.WithholdingAmount)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := groups[order[i]], groups[order[j]]
		if !a.TaxRate.Equal(b.TaxRate) {
			return a.TaxRate.LessThan(b.TaxRate)
		}
		if a.WithholdingCode != b.WithholdingCode {
			return a.WithholdingCode < b.WithholdingCode
		}
		return a.TaxExemptionCode < b.TaxExemptionCode
	})
	out.TaxSummaries = make([]model.TaxSummary, 0, len(order))
	for _, k := range order {
		out.TaxSummaries = append(out.TaxSummaries, *groups[k])
	}
}

func (m *mapper) checks() {
	out := &m.res.Invoice
	var lineVAT, lineBase decimal.Decimal
	lineVATByRate := map[string]decimal.Decimal{}
	for _, it := range out.Items {
		lineVAT = lineVAT.Add(it.TotalTaxAmount)
		lineBase = lineBase.Add(it.TaxExclusiveAmount)
		k := it.TaxRate.String()
		lineVATByRate[k] = lineVATByRate[k].Add(it.TotalTaxAmount)
	}
	if exceeds(lineVAT, m.headerVAT) {
		m.warn(model.WarnTaxTotalMismatch, map[string]any{"check": "vat", "expected": m.headerVAT.StringFixed(2), "actual": lineVAT.StringFixed(2)})
	}
	// Invoice-level allowances / charges sit between Σ lines and TaxExclusiveAmount.
	expectedBase := lineBase
	for _, ac := range m.inv.AllowanceCharge {
		if isTrue(ac.ChargeIndicator.Value) {
			expectedBase = expectedBase.Add(decOrZero(ac.Amount.Value))
		} else {
			expectedBase = expectedBase.Sub(decOrZero(ac.Amount.Value))
		}
	}
	if exceeds(expectedBase, out.Amounts.TaxExclusiveAmount) {
		m.warn(model.WarnBaseTotalMismatch, map[string]any{"expected": out.Amounts.TaxExclusiveAmount.StringFixed(2), "actual": expectedBase.StringFixed(2)})
	}
	// TaxInclusive − withholding (+ rounding) = Payable. Stopaj is also
	// accepted as a deduction (`05` PAYABLE_MISMATCH).
	rounding := decimal.Zero
	if r := m.inv.LegalMonetaryTotal.PayableRoundingAmount; r != nil {
		rounding = decOrZero(r.Value)
	}
	payable := out.Amounts.TaxInclusiveAmount.Sub(out.Amounts.WithholdingAmount).Add(rounding)
	if exceeds(payable, out.Amounts.PayableAmount) &&
		exceeds(payable.Sub(out.Amounts.IncomeTaxWithholdingAmount), out.Amounts.PayableAmount) {
		m.warn(model.WarnPayableMismatch, map[string]any{"expected": out.Amounts.PayableAmount.StringFixed(2), "actual": payable.StringFixed(2)})
	}
	rates := map[string]bool{}
	for k := range m.headerVATByRate {
		rates[k] = true
	}
	for k := range lineVATByRate {
		rates[k] = true
	}
	keys := make([]string, 0, len(rates))
	for k := range rates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if exceeds(lineVATByRate[k], m.headerVATByRate[k]) {
			m.warn(model.WarnTaxTotalMismatch, map[string]any{"check": "by_rate", "rate": k,
				"expected": m.headerVATByRate[k].StringFixed(2), "actual": lineVATByRate[k].StringFixed(2)})
		}
	}
}

// headerEstimate handles an invoice without lines (`31` §5.2, `01` §4.6 rule 3).
func (m *mapper) headerEstimate() {
	out := &m.res.Invoice
	out.HasItems = false
	out.Items = []model.InvoiceItem{}
	out.TaxSummarySource = model.TaxSummaryFromHeaderEstimate
	out.TaxSummaries = []model.TaxSummary{}
	for _, st := range m.headerVATSubtotals {
		base := decimal.Zero
		if st.TaxableAmount != nil {
			base = decOrZero(st.TaxableAmount.Value)
		}
		out.TaxSummaries = append(out.TaxSummaries, model.TaxSummary{
			TaxRate: percent(st), Base: base, VAT: decOrZero(st.TaxAmount.Value), TaxExemptionCode: exemptionCode(st),
		})
	}
	if len(out.TaxSummaries) == 0 {
		rate := decimal.Zero
		if out.Amounts.TaxExclusiveAmount.IsPositive() {
			rate = model.NearestVATRate(out.Amounts.TaxAmount.Div(out.Amounts.TaxExclusiveAmount).Mul(decimal.NewFromInt(100)))
		}
		out.TaxSummaries = append(out.TaxSummaries, model.TaxSummary{TaxRate: rate, Base: out.Amounts.TaxExclusiveAmount, VAT: out.Amounts.TaxAmount})
	}
	if w := out.Amounts.WithholdingAmount; w.IsPositive() && len(out.TaxSummaries) == 1 {
		out.TaxSummaries[0].Withholding = w
	}
	m.warn(model.WarnHeaderEstimate, nil)
}

// --- helpers ----------------------------------------------------------------

func exceeds(a, b decimal.Decimal) bool { return a.Sub(b).Abs().GreaterThan(totalTolerance) }

func dec(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(s)
}

// decOrZero parses a UBL amount. XSD guarantees the lexical form; an
// unparsable value (non-validated input) becomes zero and the totals checks
// flag the invoice.
func decOrZero(s string) decimal.Decimal {
	d, err := dec(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) > 10 { // tolerate a zone suffix such as 2026-10-01+03:00
		s = s[:10]
	}
	return time.Parse("2006-01-02", s)
}

func isTrue(s string) bool {
	s = strings.TrimSpace(s)
	return s == "true" || s == "1"
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
