// Package office serves the office panel's invoice views (issue #142):
// /v1/tenant/engagements/{engagementUuid}/invoices. An office sees the
// engagement's taxpayer invoices of its own period plus whatever the owner
// shared from before it (engagement_document_visible, issue #8).
package office

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exchangerates"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/pagecursor"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Downloader reads an attachment from the object store.
type Downloader func(r *http.Request, path string) (io.ReadCloser, int64, error)

// Handler serves the routes.
type Handler struct {
	q           *db.Queries
	download    Downloader
	activity    *activity.Recorder
	notifier    Notifier
	frontendURL string
}

// WithActivity records KDV indirim dönemi changes in the activity log (#640);
// rec may be nil.
func (h *Handler) WithActivity(rec *activity.Recorder) *Handler {
	h.activity = rec
	return h
}

// NewHandler builds the handler. download may be nil (no UBL preview).
func NewHandler(q *db.Queries, download Downloader) *Handler {
	return &Handler{q: q, download: download}
}

// RegisterRoutes mounts the routes behind the active-engagement chain. A nil
// resolver (tenancy off) or handler registers nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	chain := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(orgResolver),
			middleware.RequireTenantPermission(rbac.PermTenantInvoicesRead),
			middleware.RequireEngagement(engResolver),
		)
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/invoices"
	mux.Handle("GET "+base, chain(h.list))
	mux.Handle("GET "+base+"/{invoiceUuid}", chain(h.get))
	mux.Handle("GET "+base+"/{invoiceUuid}/ubl", chain(h.ubl))
	mux.Handle("GET "+base+"/{invoiceUuid}/pdf", chain(h.pdf))
	// Eksik kuru tamamlama (#571): fatura yazma yetkisi ister.
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(orgResolver),
			middleware.RequireTenantPermission(rbac.PermTenantInvoicesWrite),
			middleware.RequireEngagement(engResolver),
		)
	}
	mux.Handle("PUT "+base+"/{invoiceUuid}/exchange-rate", write(h.setExchangeRate))
	// Eksik / geçersiz VKN'nin ofiste tamamlanması (#669, INVALID_TAX_ID).
	mux.Handle("PUT "+base+"/{invoiceUuid}/counterparty-tax-id", write(h.setCounterpartyTaxID))
	// KDV indirim dönemi (#640): alış faturasının KDV'si sonraki bir aya alınır.
	mux.Handle("PUT "+base+"/{invoiceUuid}/vat-deduction-period", write(h.setVATDeductionPeriod))
	mux.Handle("DELETE "+base+"/{invoiceUuid}/vat-deduction-period", write(h.clearVATDeductionPeriod))
}

// maxExchangeRate bounds an office-entered rate (TL per unit).
var maxExchangeRate = decimal.NewFromInt(100000)

// setExchangeRate completes the missing exchange rate of a foreign-currency
// invoice (#571), or replaces an automatic TCMB rate (#780): KDV and the TL
// amount use it. An entered rate is never overwritten automatically. An
// invoice with a source or office rate, a TL invoice and a locked period
// are refused.
func (h *Handler) setExchangeRate(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var body struct {
		Rate string `json:"rate"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "body must be {\"rate\": \"…\"}")
		return
	}
	rate, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(body.Rate), ",", "."))
	if err != nil || !rate.IsPositive() || rate.GreaterThan(maxExchangeRate) || rate.Exponent() < -6 {
		response.Error(w, r, http.StatusBadRequest, "INVALID_RATE", "rate must be a positive number with at most 6 decimals")
		return
	}
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if inv.Currency == "TRY" {
		response.Error(w, r, http.StatusConflict, "NOT_FOREIGN_CURRENCY", "the invoice is in TL")
		return
	}
	// A TCMB rate filled automatically (#780) may be changed by hand; a
	// rate from the source or entered in the office may not.
	hasRate := inv.ExchangeRate.Valid && !repository.Dec(inv.ExchangeRate).IsZero()
	if !exchangerates.MayAutoFill(hasRate, inv.ExchangeRateSource.String) {
		response.Error(w, r, http.StatusConflict, "EXCHANGE_RATE_SET", "the invoice already has an exchange rate")
		return
	}
	if _, err := h.q.GetPeriodLock(r.Context(), db.GetPeriodLockParams{
		OrganizationID: s.OrganizationID, EngagementID: s.ID, Period: inv.Period,
	}); err == nil {
		response.Error(w, r, http.StatusConflict, "PERIOD_LOCKED", "the period is locked")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not check the period lock")
		return
	}
	n, err := h.q.SetMissingInvoiceExchangeRate(r.Context(), db.SetMissingInvoiceExchangeRateParams{
		Rate: repository.Num(rate), TaxpayerID: s.TaxpayerID, ID: inv.ID, SetBy: actorOf(r),
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not set the exchange rate")
		return
	}
	if n == 0 {
		response.Error(w, r, http.StatusConflict, "EXCHANGE_RATE_SET", "the invoice already has an exchange rate")
		return
	}
	h.get(w, r)
}

// setVATDeductionPeriod moves a purchase invoice's KDV deduction to a later
// month (#640). The document date and period stay; only the KDV indirim
// dönemi changes. KDVK 29/3: at the latest by the end of the calendar year
// following the invoice's year. Both the current and the new deduction month
// must be open.
func (h *Handler) setVATDeductionPeriod(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Period string `json:"period"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "body must be {\"period\": \"YYYY-MM\"}")
		return
	}
	target := strings.TrimSpace(body.Period)
	if _, err := time.Parse("2006-01", target); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_PERIOD", "period must be YYYY-MM")
		return
	}
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if err := VATDeductionAllowed(inv.Direction, inv.Period, target); err != "" {
		response.Error(w, r, http.StatusUnprocessableEntity, err, vatDeductionMessages[err])
		return
	}
	if target == DeductionPeriod(inv) {
		h.get(w, r)
		return
	}
	h.applyVATDeduction(w, r, inv, target)
}

// clearVATDeductionPeriod returns the KDV deduction to the document month.
func (h *Handler) clearVATDeductionPeriod(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if inv.Direction != "incoming" {
		response.Error(w, r, http.StatusUnprocessableEntity, "NOT_PURCHASE_INVOICE", vatDeductionMessages["NOT_PURCHASE_INVOICE"])
		return
	}
	if !inv.VatDeductionPeriod.Valid {
		h.get(w, r)
		return
	}
	h.applyVATDeduction(w, r, inv, "")
}

func (h *Handler) applyVATDeduction(w http.ResponseWriter, r *http.Request, inv db.Invoice, target string) {
	s := engagementctx.Must(r.Context())
	from := DeductionPeriod(inv)
	to := target
	if to == "" {
		to = inv.Period
	}
	for _, p := range []string{from, to} {
		if _, err := h.q.GetPeriodLock(r.Context(), db.GetPeriodLockParams{
			OrganizationID: s.OrganizationID, EngagementID: s.ID, Period: p,
		}); err == nil {
			response.Error(w, r, http.StatusConflict, "PERIOD_LOCKED", "the period "+p+" is locked")
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			response.InternalErr(w, r, err, "could not check the period lock")
			return
		}
	}
	set := db.SetInvoiceVATDeductionPeriodParams{TaxpayerID: s.TaxpayerID, ID: inv.ID}
	if target != "" {
		set.VatPeriod = pgtype.Text{String: target, Valid: true}
		set.SetBy = actorOf(r)
	}
	n, err := h.q.SetInvoiceVATDeductionPeriod(r.Context(), set)
	if err != nil {
		response.InternalErr(w, r, err, "could not set the KDV deduction period")
		return
	}
	if n == 0 {
		response.NotFound(w, r, "invoice not found")
		return
	}
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal > 0 {
		action := "invoice.vat_deduction_period.set"
		if target == "" {
			action = "invoice.vat_deduction_period.clear"
		}
		uid, invUUID := p.UserInternal, inv.Uuid
		h.activity.Record(r.Context(), &uid, action, "invoice", &invUUID, map[string]any{
			"engagement_uuid": s.UUID.String(), "document_period": inv.Period, "from": from, "to": to,
		}, r)
	}
	if target != "" {
		var actorID int64
		if p, ok := authctx.PrincipalFrom(r.Context()); ok {
			actorID = p.UserInternal
		}
		h.notifyLatePurchase(r.Context(), s, inv, actorID, target)
	}
	h.get(w, r)
}

// DeductionPeriod is the month whose KDV return deducts the invoice's KDV:
// the KDV indirim dönemi when set (#640), otherwise the document month.
func DeductionPeriod(inv db.Invoice) string {
	if inv.VatDeductionPeriod.Valid && inv.VatDeductionPeriod.String != "" {
		return inv.VatDeductionPeriod.String
	}
	return inv.Period
}

var vatDeductionMessages = map[string]string{
	"NOT_PURCHASE_INVOICE":     "only a purchase invoice has a KDV deduction period",
	"VAT_PERIOD_NOT_FORWARD":   "the KDV deduction period must be after the document month",
	"VAT_PERIOD_OUT_OF_WINDOW": "the KDV deduction period must be by the end of the year after the invoice year",
}

// VATDeductionAllowed checks a KDV indirim dönemi against the rules (#640):
// purchase invoices only, a later month than the document, and no later than
// December of the year after the document year (KDVK 29/3). It returns the
// error code, or "" when allowed. Both periods are YYYY-MM.
func VATDeductionAllowed(direction, docPeriod, target string) string {
	if direction != "incoming" {
		return "NOT_PURCHASE_INVOICE"
	}
	if target <= docPeriod {
		return "VAT_PERIOD_NOT_FORWARD"
	}
	docYear, err1 := strconv.Atoi(docPeriod[:4])
	targetYear, err2 := strconv.Atoi(target[:4])
	if err1 != nil || err2 != nil || targetYear > docYear+1 {
		return "VAT_PERIOD_OUT_OF_WINDOW"
	}
	return ""
}

// Row is a list row: the invoice plus its open warning count.
type Row struct {
	view.Invoice
	OpenWarnings int64 `json:"open_warnings"`
	// VATDeductionPeriod: the month the KDV is deducted in when moved
	// forward from the document month (#640); null = the document month.
	VATDeductionPeriod *string `json:"vat_deduction_period"`
}

type page struct {
	Items  []Row `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
	// NextCursor fetches the next page with ?cursor= (keyset pagination,
	// #182); null on the last page.
	NextCursor *string `json:"next_cursor"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	period := strings.TrimSpace(q.Get("period"))
	direction := strings.TrimSpace(q.Get("direction"))
	if direction != "" && direction != "outgoing" && direction != "incoming" {
		response.Error(w, r, http.StatusBadRequest, "INVALID_DIRECTION", "direction must be outgoing or incoming")
		return
	}
	var rows []db.ListEngagementInvoicesRow
	if c := strings.TrimSpace(q.Get("cursor")); c != "" {
		// Keyset page: offset is ignored; deep pages cost as little as the first.
		date, id, err := pagecursor.Decode(c)
		if err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_CURSOR", "cursor is not valid")
			return
		}
		offset = 0
		after, err := h.q.ListEngagementInvoicesAfter(r.Context(), db.ListEngagementInvoicesAfterParams{
			OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID,
			CursorDate: pgtype.Date{Time: date, Valid: true}, CursorID: id,
			Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
		for _, a := range after {
			rows = append(rows, db.ListEngagementInvoicesRow(a))
		}
	} else {
		var err error
		rows, err = h.q.ListEngagementInvoices(r.Context(), db.ListEngagementInvoicesParams{
			OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID,
			Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit, Off: offset,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
	}
	total, err := h.q.CountEngagementInvoices(r.Context(), db.CountEngagementInvoicesParams{
		TaxpayerID: s.TaxpayerID, EngagementID: s.ID, Period: repository.Text(period), Direction: repository.Text(direction),
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not count invoices")
		return
	}
	out := page{Items: make([]Row, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, row := range rows {
		out.Items = append(out.Items, Row{
			Invoice: view.FromInvoice(row.Invoice), OpenWarnings: row.OpenWarnings,
			VATDeductionPeriod: textPtr(row.Invoice.VatDeductionPeriod),
		})
	}
	if n := len(rows); n > 0 && n == int(limit) {
		next := pagecursor.Encode(rows[n-1].Invoice.IssueDate.Time, rows[n-1].Invoice.ID)
		out.NextCursor = &next
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Warning is a data warning on the invoice or one of its lines.
type Warning struct {
	UUID       uuid.UUID      `json:"uuid"`
	EntityType string         `json:"entity_type"`
	Code       string         `json:"code"`
	Severity   string         `json:"severity"`
	Status     string         `json:"status"`
	Params     map[string]any `json:"params"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Detail is the office invoice detail.
type Detail struct {
	view.Detail
	HasUBL   bool      `json:"has_ubl"`
	Warnings []Warning `json:"warnings"`
	// ExchangeRateEntry: who entered the missing rate in the office, and
	// when (#571); null when the rate came with the source.
	ExchangeRateEntry *RateEntry `json:"exchange_rate_entry"`
	// CounterpartyTaxIDEntry: who entered the missing VKN / TCKN in the
	// office, and when (#669); null when it came with the source.
	CounterpartyTaxIDEntry *RateEntry `json:"counterparty_tax_id_entry"`
	// VATDeductionPeriod: the KDV indirim dönemi when moved forward from the
	// document month (#640); null = the document month.
	VATDeductionPeriod *string `json:"vat_deduction_period"`
	// VATDeductionEntry: who moved the KDV deduction, and when (#640).
	VATDeductionEntry *RateEntry `json:"vat_deduction_entry"`
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid || t.String == "" {
		return nil
	}
	v := t.String
	return &v
}

// RateEntry is an office-entered exchange rate's author.
type RateEntry struct {
	SetByName string    `json:"set_by_name"`
	SetAt     time.Time `json:"set_at"`
}

func actorOf(r *http.Request) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (db.Invoice, bool) {
	s := engagementctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("invoiceUuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return db.Invoice{}, false
	}
	inv, err := h.q.GetEngagementInvoice(r.Context(), db.GetEngagementInvoiceParams{TaxpayerID: s.TaxpayerID, Uuid: id, EngagementID: s.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "invoice not found")
		return db.Invoice{}, false
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return db.Invoice{}, false
	}
	return inv, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	d, err := view.LoadDetail(r.Context(), h.q, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice details")
		return
	}
	ws, err := h.q.ListInvoiceWarnings(r.Context(), db.ListInvoiceWarningsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID, InvoiceID: pgtype.Int8{Int64: inv.ID, Valid: true},
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not load warnings")
		return
	}
	out := Detail{Detail: d, HasUBL: inv.UblAttachmentID.Valid && h.download != nil, Warnings: make([]Warning, 0, len(ws))}
	if e, err := h.q.InvoiceExchangeRateEntry(r.Context(), db.InvoiceExchangeRateEntryParams{TaxpayerID: s.TaxpayerID, ID: inv.ID}); err == nil {
		out.ExchangeRateEntry = &RateEntry{SetByName: e.SetByName, SetAt: e.ExchangeRateSetAt.Time}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not load the exchange rate entry")
		return
	}
	if e, err := h.q.InvoiceCounterpartyTaxIDEntry(r.Context(), db.InvoiceCounterpartyTaxIDEntryParams{TaxpayerID: s.TaxpayerID, ID: inv.ID}); err == nil {
		out.CounterpartyTaxIDEntry = &RateEntry{SetByName: e.SetByName, SetAt: e.CounterpartyTaxIDSetAt.Time}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not load the tax id entry")
		return
	}
	out.VATDeductionPeriod = textPtr(inv.VatDeductionPeriod)
	if e, err := h.q.InvoiceVATDeductionEntry(r.Context(), db.InvoiceVATDeductionEntryParams{TaxpayerID: s.TaxpayerID, ID: inv.ID}); err == nil {
		out.VATDeductionEntry = &RateEntry{SetByName: e.SetByName, SetAt: e.VatDeductionSetAt.Time}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not load the KDV deduction entry")
		return
	}
	for _, x := range ws {
		out.Warnings = append(out.Warnings, Warning{
			UUID: x.Uuid, EntityType: x.EntityType, Code: x.Code, Severity: x.Severity, Status: x.Status,
			Params: params(x.Params), CreatedAt: x.CreatedAt.Time,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ubl streams the stored UBL XML for the invoice preview.
func (h *Handler) ubl(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if !inv.UblAttachmentID.Valid || h.download == nil {
		response.NotFound(w, r, "no UBL stored for this invoice")
		return
	}
	att, err := h.q.GetInvoiceAttachment(r.Context(), db.GetInvoiceAttachmentParams{TaxpayerID: s.TaxpayerID, ID: inv.UblAttachmentID.Int64})
	if err != nil {
		response.InternalErr(w, r, err, "could not load attachment")
		return
	}
	body, size, err := h.download(r, att.StorageKey)
	if err != nil {
		response.InternalErr(w, r, err, "could not read UBL")
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.Copy(w, body)
}

func paging(l, o string) (limit, offset int32) {
	limit = 50
	if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(o); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	return limit, offset
}

func params(raw []byte) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}
