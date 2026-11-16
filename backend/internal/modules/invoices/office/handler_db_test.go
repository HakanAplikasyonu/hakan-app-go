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
