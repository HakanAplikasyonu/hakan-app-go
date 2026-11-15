package taxpayerportal

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type env struct {
	d     *testdb.DB
	mux   *http.ServeMux
	userA int64
	tpA   uuid.UUID
	tpB   uuid.UUID
	tpAID int64
	tpBID int64
	engA  int64
	orgA  int64
	invB  uuid.UUID
	invA  uuid.UUID
}

const vknA, vknB = "1288331521", "9205121120"

func tpUUID(t *testing.T, d *testdb.DB, id int64) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	if err := d.Pool.QueryRow(context.Background(), "SELECT uuid FROM taxpayers WHERE id = $1", id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

// saveInvoice stores a go-ubltr sample invoice for taxpayer tp (own VKN own).
func saveInvoice(t *testing.T, d *testdb.DB, tp int64, own string) uuid.UUID {
	t.Helper()
	b, err := os.ReadFile("../invoices/ublmap/testdata/temelfatura-satis.xml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ublmap.MapXML(b, ublmap.Options{OwnTaxID: own})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := repository.SaveInvoice(ctx, tx, tp, r.Invoice, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return res.InvoiceUUID
}

func setup(t *testing.T) env {
	t.Helper()
	d := testdb.Open(t)
	e := env{d: d}
	e.orgA = d.Organization(t)
	e.userA = d.User(t)
	e.tpAID = d.Taxpayer(t, "legal", vknA)
	e.tpBID = d.Taxpayer(t, "legal", vknB)
	e.engA = d.Engagement(t, e.orgA, e.tpAID)
	e.tpA, e.tpB = tpUUID(t, d, e.tpAID), tpUUID(t, d, e.tpBID)
	if _, err := d.Pool.Exec(context.Background(), "INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')", e.tpAID, e.userA); err != nil {
		t.Fatal(err)
	}
	// The sample is issued by 1288331521 to a TCKN buyer: outgoing for A.
	e.invA = saveInvoice(t, d, e.tpAID, vknA)
	// Taxpayer B has its own invoice (the same file read from B's side would
	// not match B's VKN, so use a different sample where B is the buyer).
	b, _ := os.ReadFile("../invoices/ublmap/testdata/ticarifatura-satis.xml")
	r, err := ublmap.MapXML(b, ublmap.Options{OwnTaxID: vknB})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, _ := d.Pool.Begin(ctx)
	res, err := repository.SaveInvoice(ctx, tx, e.tpBID, r.Invoice, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	e.invB = res.InvoiceUUID

	members := taxpayers.NewMembers(taxpayers.NewMemberPostgres(d.Pool), nil, taxpayers.InvitationConfig{}, nil)
	ups := upload.New(d.Pool, storage.NewMemory(), nil, nil)
	h := NewHandler(db.New(d.Pool), ups)
	e.mux = http.NewServeMux()
	fakeAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: e.userA})))
		})
	}
	mount(e.mux, h, fakeAuth, middleware.RequireTaxpayer(members))
	return e
}

func (e env) do(t *testing.T, method, path string, tp uuid.UUID, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("X-Taxpayer", tp.String())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatal(err)
	}
}

// Kabul: mükellef yalnızca kendi şirketini görüyor.
func TestTaxpayerSeesOnlyOwnCompany(t *testing.T) {
	e := setup(t)
	rec := e.do(t, "GET", "/v1/taxpayer/invoices", e.tpA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own list: %d %s", rec.Code, rec.Body.String())
	}
	var page invoicePage
	decode(t, rec, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].UUID != e.invA || page.Items[0].Payable != "492.00" {
		t.Fatalf("own invoices: %+v", page)
	}
	// Another company's invoice through the own header: not found.
	if rec := e.do(t, "GET", "/v1/taxpayer/invoices/"+e.invB.String(), e.tpA, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("other company's invoice via own header: %d", rec.Code)
	}
	// The other company's header: 403 for every route.
	for _, path := range []string{"/v1/taxpayer/invoices", "/v1/taxpayer/invoices/" + e.invB.String(), "/v1/taxpayer/receipts", "/v1/taxpayer/uploads"} {
		if rec := e.do(t, "GET", path, e.tpB, nil, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s with other company header: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	// Own invoice detail.
	rec = e.do(t, "GET", "/v1/taxpayer/invoices/"+e.invA.String(), e.tpA, nil, "")
	var detail InvoiceDetail
	decode(t, rec, &detail)
	if rec.Code != http.StatusOK || len(detail.Items) != 2 || len(detail.TaxSummary) != 2 || len(detail.Sources) != 1 {
		t.Fatalf("detail: %d %+v", rec.Code, detail)
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/invoices?direction=sideways", e.tpA, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad direction: %d", rec.Code)
	}
}

func TestTaxpayerUploadUsesActiveEngagement(t *testing.T) {
	e := setup(t)
	b, _ := os.ReadFile("../invoices/ublmap/testdata/temelfatura-tevkifat.xml")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "fatura.xml")
	_, _ = fw.Write(b)
	_ = mw.Close()
	rec := e.do(t, "POST", "/v1/taxpayer/uploads", e.tpA, &body, mw.FormDataContentType())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var job upload.Job
	decode(t, rec, &job)
	if job.Status != "done" || job.Report == nil || job.Report.Added != 1 {
		t.Fatalf("job: %+v", job)
	}
	var engID int64
	if err := e.d.Pool.QueryRow(context.Background(), "SELECT engagement_id FROM ubl_upload_jobs WHERE uuid = $1", job.UUID).Scan(&engID); err != nil || engID != e.engA {
		t.Fatalf("job engagement = %d (%v), want %d", engID, err, e.engA)
	}
	rec = e.do(t, "GET", "/v1/taxpayer/uploads", e.tpA, nil, "")
	var list uploadPage
	decode(t, rec, &list)
	if len(list.Items) != 1 || list.Items[0].UUID != job.UUID {
		t.Fatalf("uploads: %+v", list)
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/uploads/"+job.UUID.String(), e.tpA, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("get upload: %d", rec.Code)
	}
	// Without an active office there is nobody to process the file.
	e.d.EndEngagement(t, e.engA)
	body.Reset()
	mw = multipart.NewWriter(&body)
	fw, _ = mw.CreateFormFile("file", "fatura.xml")
	_, _ = fw.Write(b)
	_ = mw.Close()
	if rec := e.do(t, "POST", "/v1/taxpayer/uploads", e.tpA, &body, mw.FormDataContentType()); rec.Code != http.StatusConflict {
		t.Fatalf("upload without office: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReceiptsList(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var att int64
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO attachments (taxpayer_id, kind, storage_key, sha256, size_bytes)
		VALUES ($1, 'image', 'k', repeat('a', 64), 1) RETURNING id`, e.tpAID).Scan(&att); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO receipt_captures (organization_id, engagement_id, taxpayer_id, attachment_id, media_type, kind, status, fields)
		VALUES ($1, $2, $3, $4, 'image/jpeg', 'fuel_receipt', 'draft', '{"vendor_name":{"value":"Akaryakıt A.Ş."},"total_amount":{"value":"1250.00"}}')`,
		e.orgA, e.engA, e.tpAID, att); err != nil {
		t.Fatal(err)
	}
	rec := e.do(t, "GET", "/v1/taxpayer/receipts", e.tpA, nil, "")
	var page receiptPage
	decode(t, rec, &page)
	if rec.Code != http.StatusOK || page.Total != 1 || page.Items[0].VendorName != "Akaryakıt A.Ş." || page.Items[0].TotalAmount != "1250.00" {
		t.Fatalf("receipts: %d %+v", rec.Code, page)
	}
}
