package office

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownVKN = "1288331521"

type env struct {
	d     *testdb.DB
	q     *db.Queries
	h     *Handler
	scope engagementctx.Scope
	inv   string
	xml   []byte
}

func setup(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	eng := d.Engagement(t, org, tp)
	q := db.New(d.Pool)

	xml, err := os.ReadFile("../ublmap/testdata/temelfatura-satis.xml")
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := ublmap.MapXML(xml, ublmap.Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt := db.New(tx)
	att, err := qt.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: tp, Kind: "ubl_xml", StorageKey: "taxpayers/x/ubl/a.xml",
		Sha256: strings.Repeat("a", 64), SizeBytes: int64(len(xml)),
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.SaveInvoice(ctx, tx, tp, mapped.Invoice, att.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qt.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
		OrganizationID: org, EngagementID: eng, TaxpayerID: tp, Period: repository.Text(mapped.Invoice.Period),
		EntityType: "invoice", EntityID: pgtype.Int8{Int64: saved.InvoiceID, Valid: true},
		Code: "TAX_TOTAL_MISMATCH", Severity: "warning", Params: []byte(`{"diff":"0.02"}`), Producer: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var started pgtype.Timestamptz
	if err := d.Pool.QueryRow(ctx, `SELECT started_at FROM engagements WHERE id = $1`, eng).Scan(&started); err != nil {
		t.Fatal(err)
	}
	download := func(_ *http.Request, path string) (io.ReadCloser, int64, error) {
		if path != "taxpayers/x/ubl/a.xml" {
			t.Errorf("download path %q", path)
		}
		return io.NopCloser(strings.NewReader(string(xml))), int64(len(xml)), nil
	}
	return env{
		d: d, q: q, h: NewHandler(q, download),
		scope: engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp, StartedAt: started.Time},
		inv:   saved.InvoiceUUID.String(), xml: xml,
	}
}

func (e env) call(t *testing.T, fn http.HandlerFunc, path string, pv map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range pv {
		r.SetPathValue(k, v)
	}
	r = r.WithContext(engagementctx.WithScope(r.Context(), e.scope))
	w := httptest.NewRecorder()
	fn(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct{ Data T }
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return env.Data
}

// Ofis paneli (#142): liste, detay (satırlar, KDV özeti, uyarılar) ve UBL.
func TestOfficeInvoiceViews(t *testing.T) {
	e := setup(t)

	w := e.call(t, e.h.list, "/x?direction=outgoing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list %d %s", w.Code, w.Body)
	}
	p := decode[page](t, w)
	if p.Total != 1 || len(p.Items) != 1 || p.Items[0].UUID.String() != e.inv || p.Items[0].OpenWarnings != 1 {
		t.Fatalf("list = %+v", p)
	}
	if p.Items[0].Payable == "" || p.Items[0].CounterpartyName == "" {
		t.Fatalf("row fields missing: %+v", p.Items[0])
	}
	if w := e.call(t, e.h.list, "/x?direction=incoming", nil); decode[page](t, w).Total != 0 {
		t.Fatalf("incoming filter: %s", w.Body)
	}
	if w := e.call(t, e.h.list, "/x?direction=sideways", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad direction = %d", w.Code)
	}

	w = e.call(t, e.h.get, "/x", map[string]string{"invoiceUuid": e.inv})
	if w.Code != http.StatusOK {
		t.Fatalf("get %d %s", w.Code, w.Body)
	}
	d := decode[Detail](t, w)
	if len(d.Items) == 0 || len(d.TaxSummary) == 0 || len(d.Sources) == 0 || !d.HasUBL {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.Warnings) != 1 || d.Warnings[0].Code != "TAX_TOTAL_MISMATCH" || d.Warnings[0].Params["diff"] != "0.02" {
		t.Fatalf("warnings = %+v", d.Warnings)
	}

	w = e.call(t, e.h.ubl, "/x", map[string]string{"invoiceUuid": e.inv})
	if w.Code != http.StatusOK || w.Body.String() != string(e.xml) || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/xml") {
		t.Fatalf("ubl %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	if w := e.call(t, e.h.get, "/x", map[string]string{"invoiceUuid": "00000000-0000-0000-0000-000000000000"}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown invoice = %d", w.Code)
	}
}

// Önceki ofisin dönemine ait fatura, mükellef paylaşmadıkça yeni ofise
// görünmez (engagement_document_visible, #8).
func TestOfficeSeesOnlyVisibleInvoices(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	prevOrg := e.d.Organization(t)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO engagements (organization_id, taxpayer_id, status, started_at, ended_at)
		VALUES ($1, $2, 'ended', '2020-01-01', '2026-10-01')`, prevOrg, e.scope.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Pool.Exec(ctx, `UPDATE engagements SET started_at = '2026-10-01' WHERE id = $1`, e.scope.ID); err != nil {
		t.Fatal(err)
	}
	// The sample invoice is dated 2026-03-01, before this office started.
	if w := e.call(t, e.h.list, "/x", nil); decode[page](t, w).Total != 0 {
		t.Fatalf("earlier office's invoice listed: %s", w.Body)
	}
	if w := e.call(t, e.h.get, "/x", map[string]string{"invoiceUuid": e.inv}); w.Code != http.StatusNotFound {
		t.Fatalf("earlier office's invoice detail = %d", w.Code)
	}
	if w := e.call(t, e.h.ubl, "/x", map[string]string{"invoiceUuid": e.inv}); w.Code != http.StatusNotFound {
		t.Fatalf("earlier office's invoice UBL = %d", w.Code)
	}
}

// İmleçli sayfalama (#182): imleçle gezilen sayfalar OFFSET sırasıyla aynı,
// eksiksiz ve tekrarsızdır; aynı tarihli faturalar id ile ayrılır.
func TestOfficeInvoiceCursorPagination(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// 11 more invoices, three per date, so ties on issue_date are crossed.
	if _, err := e.d.Pool.Exec(ctx, `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
			direction, type, issue_date, period, currency, tax_summary_source)
		SELECT $1, 'test', 'cur-' || g, 'api', 'CUR' || g, 'outgoing', 'SATIS',
			DATE '2025-03-01' + (g / 3), '2025-03', 'TRY', 'header_estimate'
		FROM generate_series(1, 11) g`, e.scope.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	all := decode[page](t, e.call(t, e.h.list, "/x?limit=100", nil))
	if all.Total != 12 || len(all.Items) != 12 || all.NextCursor != nil {
		t.Fatalf("offset page = total %d items %d next %v", all.Total, len(all.Items), all.NextCursor)
	}

	var got []string
	path := "/x?limit=5"
	for i := 0; i < 10; i++ {
		p := decode[page](t, e.call(t, e.h.list, path, nil))
		if p.Total != 12 {
			t.Fatalf("total on page %d = %d", i, p.Total)
		}
		for _, it := range p.Items {
			got = append(got, it.UUID.String())
		}
		if p.NextCursor == nil {
			break
		}
		path = "/x?limit=5&cursor=" + *p.NextCursor
	}
	if len(got) != 12 {
		t.Fatalf("cursor pages returned %d rows", len(got))
	}
	for i, it := range all.Items {
		if got[i] != it.UUID.String() {
			t.Fatalf("row %d: cursor %s, offset %s", i, got[i], it.UUID)
		}
	}
	// The warning count is kept on cursor pages too.
	warned := 0
	for _, it := range all.Items {
		warned += int(it.OpenWarnings)
	}
	if warned != 1 {
		t.Fatalf("open warnings = %d", warned)
	}
	if w := e.call(t, e.h.list, "/x?cursor=bozuk", nil); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_CURSOR") {
		t.Fatalf("bad cursor = %d %s", w.Code, w.Body)
	}
}

// Eksik kur (#571): dövizli faturada kaynakta kur yoksa ofis girer; KDV ve
// TL karşılığı onu kullanır. Kuru olan, TL ya da kilitli döneme ait fatura
// reddedilir.
func TestOfficeSetsMissingExchangeRate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	put := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body))
		r.SetPathValue("invoiceUuid", e.inv)
		r = r.WithContext(engagementctx.WithScope(r.Context(), e.scope))
		w := httptest.NewRecorder()
		e.h.setExchangeRate(w, r)
		return w
	}
	if w := put(`{"rate":"38,5"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "NOT_FOREIGN_CURRENCY") {
		t.Fatalf("TL invoice: %d %s", w.Code, w.Body)
	}
	var period string
	if err := e.d.Pool.QueryRow(ctx, `
		UPDATE invoices SET currency = 'USD', exchange_rate = NULL, payable_amount_try = NULL
		WHERE uuid = $1 RETURNING period`, e.inv).Scan(&period); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"rate":"0"}`, `{"rate":"-1"}`, `{"rate":"abc"}`, `{"rate":"1.1234567"}`, `{"kur":"38"}`} {
		if w := put(bad); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, w.Code, w.Body)
		}
	}

	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period)
		VALUES ($1, $2, $3, $4)`, e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID, period); err != nil {
		t.Fatal(err)
	}
	if w := put(`{"rate":"38,5"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "PERIOD_LOCKED") {
		t.Fatalf("locked period: %d %s", w.Code, w.Body)
	}
	if _, err := e.d.Pool.Exec(ctx, `DELETE FROM period_locks WHERE engagement_id = $1`, e.scope.ID); err != nil {
		t.Fatal(err)
	}

	w := put(`{"rate":"38,5"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	var rate, payable, payableTRY string
	if err := e.d.Pool.QueryRow(ctx, `SELECT exchange_rate::text, payable_amount::text, payable_amount_try::text
		FROM invoices WHERE uuid = $1`, e.inv).Scan(&rate, &payable, &payableTRY); err != nil {
		t.Fatal(err)
	}
	want := decimal.RequireFromString(payable).Mul(decimal.RequireFromString("38.5")).Round(2)
	if !decimal.RequireFromString(rate).Equal(decimal.RequireFromString("38.5")) || !decimal.RequireFromString(payableTRY).Equal(want) {
		t.Fatalf("stored rate %s payable_try %s (payable %s)", rate, payableTRY, payable)
	}
	if w := put(`{"rate":"40"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "EXCHANGE_RATE_SET") {
		t.Fatalf("second set: %d %s", w.Code, w.Body)
	}
}
