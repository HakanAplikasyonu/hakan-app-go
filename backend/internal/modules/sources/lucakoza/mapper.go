package lucakoza

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// SourceSystem of everything read from Luca Koza.
const SourceSystem = "luca_koza"

// Koza's invoice movement types (parUstHareketTuru).
const (
	MovePurchase       = 16 // alım
	MoveSalesReturn    = 17 // satış iade
	MoveSales          = 18 // satış
	MovePurchaseReturn = 19 // alım iade
)

// movements are read in this order, one list call each.
var movements = []int{MovePurchase, MoveSalesReturn, MoveSales, MovePurchaseReturn}

// Excluded reasons (sources.Report.Excluded).
const (
	SkipUnknownMovement = "unknown_movement_type"
	SkipNoID            = "no_id"
	SkipNoDate          = "no_issue_date"
	SkipOutsideRange    = "outside_range"
	SkipNoAmounts       = "no_amounts"
	SkipRepeated        = "repeated"
)

var hundred = decimal.NewFromInt(100)

// kind is a movement type's place in the common model, as the other
// sources map it (KolayBi): a sales return is an outgoing IADE that lowers
// sales, a purchase return an incoming IADE that lowers purchases.
func kind(movement int) (model.Direction, string, bool) {
	switch movement {
	case MoveSales:
		return model.DirectionOutgoing, model.TypeSatis, true
	case MoveSalesReturn:
		return model.DirectionOutgoing, model.TypeIade, true
	case MovePurchase:
		return model.DirectionIncoming, model.TypeAlis, true
	case MovePurchaseReturn:
		return model.DirectionIncoming, model.TypeIade, true
	}
	return "", "", false
}

// Read lists the four movement types for [from, to] and maps them.
func (c *Client) Read(ctx context.Context, from, to time.Time) (sources.Batch, error) {
	b := sources.Batch{Excluded: sources.Excluded{}}
	seen := map[string]bool{}
	for _, mv := range movements {
		list, err := c.Invoices(ctx, mv, from, to)
		if err != nil {
			return b, err
		}
		for _, r := range list {
			inv, ws, skip := mapInvoice(mv, r, from, to)
			if skip == "" && seen[inv.SourceID] {
				skip = SkipRepeated // Koza ignored the movement filter
			}
			if skip != "" {
				b.Excluded[skip]++
				continue
			}
			seen[inv.SourceID] = true
			b.Invoices = append(b.Invoices, inv)
			b.Warn(inv.SourceID, ws...)
		}
	}
	return b, nil
}

// movementOf is the record's own movement type when it carries one, else the
// type the list was asked for. ok is false for a type that is not a number.
func movementOf(asked int, r obj) (int, bool) {
	s := r.str("parUstHareketTuru", "ustHareketTuru")
	if s == "" {
		return asked, true
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func hash(o obj) string {
	b, _ := json.Marshal(o)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// mapInvoice maps one Koza invoice header with its lines. A field Koza did
// not send is left empty and flagged, never guessed; an unknown movement
// type or a record without id / date / amounts is skipped with a reason.
func mapInvoice(asked int, r obj, from, to time.Time) (model.Invoice, []model.Warning, string) {
	var inv model.Invoice
	var ws []model.Warning
	mv, ok := movementOf(asked, r)
	if !ok {
		return inv, nil, SkipUnknownMovement
	}
	dir, typ, ok := kind(mv)
	if !ok {
		return inv, nil, SkipUnknownMovement
	}
	id := r.str("ssFaturaBaslikId")
	if id == "" {
		return inv, nil, SkipNoID
	}
	v, _ := r.get("belgeTarihi")
	issue, ok := parseDate(v)
	if !ok {
		return inv, nil, SkipNoDate
	}
	if issue.Before(from) || issue.After(to) {
		return inv, nil, SkipOutsideRange
	}
	inv.SourceSystem, inv.SourceID, inv.CaptureMethod = SourceSystem, id, model.CaptureAPI
	inv.Direction, inv.Type, inv.DocumentClass = dir, typ, "invoice"
	inv.InvoiceNumber = model.NormalizeInvoiceNumber(r.str("belgeSeriNo", "belgeNo"))
	inv.UUID = r.str("ettn", "uuid")
	inv.IssueDate, inv.Period = issue, issue.Format("2006-01")
	inv.Currency = model.NormalizeCurrency(r.str("paraBirimKod", "dovizKod", "paraBirimi"))
	if inv.Currency == "" {
		inv.Currency = "TRY"
	}
	if x, ok := r.dec("dovizKuru", "kurBedeli", "kur"); ok && x.IsPositive() && inv.Currency != "TRY" {
		inv.ExchangeRate = &x
	}
	inv.Notes = r.str("aciklama")

	cari := r.sub("cari", "finansalNesne")
	inv.Counterparty = model.Party{Name: cari.str("cariTanim", "tanim", "unvan"), TaxOffice: cari.str("vergiDairesi")}
	rawID := cari.str("vergiKimlikNo", "tcKimlikNo")
	if vkn, tckn, ok := model.ClassifyTaxID(rawID); ok {
		inv.Counterparty.TaxNumber, inv.Counterparty.IdentityNumber = vkn, tckn
	} else {
		reason := "missing"
		if rawID != "" {
			reason = "invalid"
		}
		ws = append(ws, model.Warning{Code: model.WarnInvalidTaxID, EntityType: model.EntityInvoice,
			Params: map[string]any{"field": "customer_tax_number", "value": rawID, "reason": reason, "source": SourceSystem}})
	}
	if inv.Counterparty.Name == "" {
		ws = append(ws, model.Warning{Code: model.WarnInvalidTaxID, EntityType: model.EntityInvoice,
			Params: map[string]any{"field": "customer_name", "reason": "missing", "source": SourceSystem}})
	}

	for i, ln := range r.list("detayList", "faturaDetayList", "ftrSsFaturaDetayList", "lines", "kalemler", "detaylar") {
		inv.Items = append(inv.Items, mapLine(i+1, ln))
	}
	a := &inv.Amounts
	for _, it := range inv.Items {
		a.LineExtensionAmount = a.LineExtensionAmount.Add(it.LineExtensionAmount)
		a.AllowanceTotalAmount = a.AllowanceTotalAmount.Add(it.AllowanceTotalAmount)
		a.TaxExclusiveAmount = a.TaxExclusiveAmount.Add(it.TaxExclusiveAmount)
		a.TaxAmount = a.TaxAmount.Add(it.TotalTaxAmount)
		a.WithholdingAmount = a.WithholdingAmount.Add(it.WithholdingAmount)
	}
	if len(inv.Items) > 0 {
		inv.HasItems, inv.TaxSummarySource = true, model.TaxSummaryFromItems
		inv.TaxSummaries = summaries(inv.Items)
	} else {
		// No lines: the header's totals, flagged as an estimate.
		net, okN := r.dec("netTutar", "toplamNetTutar", "matrah")
		vat, _ := r.dec("kdvTutar", "toplamKdvTutar")
		w, _ := r.dec("tevkifatTutar", "toplamTevkifatTutar")
		if !okN || !net.IsPositive() {
			return model.Invoice{}, nil, SkipNoAmounts
		}
		a.LineExtensionAmount, a.TaxExclusiveAmount, a.TaxAmount, a.WithholdingAmount = net, net, vat, w
		inv.TaxSummarySource = model.TaxSummaryFromHeaderEstimate
		rate := model.NearestVATRate(vat.Div(net).Mul(hundred))
		inv.TaxSummaries = []model.TaxSummary{{TaxRate: rate, Base: net, VAT: vat, Withholding: w}}
		ws = append(ws, model.Warning{Code: model.WarnHeaderEstimate, EntityType: model.EntityInvoice, Params: map[string]any{"source": SourceSystem}})
	}
	a.TaxInclusiveAmount = a.TaxExclusiveAmount.Add(a.TaxAmount)
	a.PayableAmount = a.TaxInclusiveAmount.Sub(a.WithholdingAmount)
	a.PayableAmountTRY = a.PayableAmount
	if inv.ExchangeRate != nil {
		a.PayableAmountTRY = a.PayableAmount.Mul(*inv.ExchangeRate).Round(2)
	}
	inv.ContentHash = hash(r)
	inv.RawPayload = map[string]any{"luca_koza": map[string]any(r)}
	return inv, ws, ""
}

// mapLine maps one invoice line. netTutar is the VAT-exclusive amount; when
// Koza leaves it out it is miktar × birimFiyat. A missing kdvTutar is
// net × kdvOran. The withholding rate is tevkifatTutar / kdvTutar.
func mapLine(no int, ln obj) model.InvoiceItem {
	qty, _ := ln.dec("miktar")
	unit, _ := ln.dec("birimFiyat")
	gross := qty.Mul(unit).Round(2)
	net, ok := ln.dec("netTutar")
	if !ok {
		net = gross
	}
	if gross.IsZero() {
		gross = net
	}
	rate, _ := ln.dec("kdvOran")
	rate = model.NormalizePercent(rate)
	vat, ok := ln.dec("kdvTutar")
	if !ok {
		vat = net.Mul(rate).Div(hundred).Round(2)
	}
	w, _ := ln.dec("tevkifatTutar")
	var wRate decimal.Decimal
	if w.IsPositive() && vat.IsPositive() {
		wRate = w.Div(vat).Mul(hundred).Round(0)
	}
	disc := decimal.Zero
	if gross.GreaterThan(net) {
		disc = gross.Sub(net)
	}
	name := ln.str("aciklama", "kartAdi", "stokKartAdi", "urunAdi", "kartKod")
	return model.InvoiceItem{
		LineNo: no, Name: name, Description: ln.str("aciklama"), ProductCode: ln.str("kartKod", "stokKartKodu"),
		Quantity: qty, UnitPrice: unit, LineExtensionAmount: gross, OwnAllowanceAmount: disc, AllowanceTotalAmount: disc,
		TaxExclusiveAmount: net, TaxRate: rate, TotalTaxAmount: vat, TaxInclusiveAmount: net.Add(vat),
		WithholdingRate: wRate, WithholdingAmount: w, LineType: model.LineUnknown,
	}
}

func summaries(its []model.InvoiceItem) []model.TaxSummary {
	by := map[string]*model.TaxSummary{}
	var keys []string
	for _, it := range its {
		k := it.TaxRate.String()
		s, ok := by[k]
		if !ok {
			s = &model.TaxSummary{TaxRate: it.TaxRate}
			by[k] = s
			keys = append(keys, k)
		}
		s.Base, s.VAT, s.Withholding = s.Base.Add(it.TaxExclusiveAmount), s.VAT.Add(it.TotalTaxAmount), s.Withholding.Add(it.WithholdingAmount)
	}
	sort.Strings(keys)
	out := make([]model.TaxSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, *by[k])
	}
	return out
}
