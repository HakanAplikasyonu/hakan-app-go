package taxpayerportal

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/reviewlink"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// A fuel receipt read with a low total confidence and an unregistered plate.
const lowConfidenceAnswer = `{"kind":"fuel_receipt","kind_confidence":0.95,"fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.92},
"vendor_name":{"value":"Petrol A.Ş.","confidence":0.9},"total_amount":{"value":"1.200,00","confidence":0.55},
"tax_amount":{"value":"200,00","confidence":0.9},"vehicle_plate":{"value":"34ABC123","confidence":0.9}}}`

type lowReader struct{}

func (lowReader) Read(context.Context, extract.Request) (extract.Result, error) {
	r, err := extract.Parse(lowConfidenceAnswer)
	r.Model = "claude-test"
	return r, err
}

// TestTaxpayerWritesReachOfficeQueue (issue #281): end to end through the
// mükellef panel with the production request scope (rls.Middleware in
// taxpayer scope, real review queue and data warnings). A receipt photo's
// low-confidence fields and the taxpayer's correction land in the office's
// review queue, the plate warning is written for the office, and the
// request answers without a row level security error.
func TestTaxpayerWritesReachOfficeQueue(t *testing.T) {
	e := setup(t)
	middleware.SetTaxpayerScope(rls.Middleware(e.d.Pool, database.VisibleTaxpayers(e.d.Pool), nil))
	t.Cleanup(func() { middleware.SetTaxpayerScope(nil) })

	q := database.NewQueries(e.d.Pool)
	reviewer := reviewlink.NewReviewer(reviewusecase.New(reviewrepository.New(e.d.Pool, q)))
	warnings := warningsengine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), lowReader{}, reviewer, warnings, nil)
	e.h.WithReceipts(svc)

	body, ct := receiptForm(t, "fis-281")
	rec := e.serve(e.req(t, http.MethodPost, "/v1/taxpayer/receipts", body, ct))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	var first map[string]any
	decode(t, rec, &first)
	id := first["uuid"].(string)
	// #780: the read runs after the request's transaction commits, in the
	// office scope narrowed to the taxpayer (as rls.ForOffice gives it).
	waitReads(t, svc)
	rec = e.do(t, "GET", "/v1/taxpayer/receipts/"+id, e.tpA, nil, "")
	var read map[string]any
	decode(t, rec, &read)
	if read["status"] != "draft" {
		t.Fatalf("after the background read = %d %v", rec.Code, read)
	}

	rec = e.do(t, "POST", "/v1/taxpayer/receipts/"+id+"/corrections", e.tpA,
		bytes.NewBufferString(`{"corrections":[{"field":"total_amount","value":"1.250,00","note":"fişte 1.250 yazıyor"}]}`), "application/json")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("corrections = %d %s", rec.Code, rec.Body)
	}

	ctx := context.Background()
	count := func(sql string) int {
		t.Helper()
		var n int
		err := rls.System(ctx, e.d.Pool, "test: read back", func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, sql, e.orgA, e.engA).Scan(&n)
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	// The OCR field (confidence 0.55) and the taxpayer's correction.
	if n := count(`SELECT count(*) FROM suggestions WHERE organization_id = $1 AND engagement_id = $2`); n != 2 {
		t.Fatalf("office review queue has %d suggestions, want 2", n)
	}
	if n := count(`SELECT count(*) FROM data_warnings WHERE organization_id = $1 AND engagement_id = $2 AND code = 'PLATE_NOT_REGISTERED'`); n != 1 {
		t.Fatalf("plate warnings = %d, want 1", n)
	}

	// An e-invoice file from the panel is read for the office (no queue:
	// processed in the request, as the worker would, #166 Process).
	b, err := os.ReadFile("../invoices/ublmap/testdata/temelfatura-tevkifat.xml")
	if err != nil {
		t.Fatal(err)
	}
	var ubl bytes.Buffer
	mw := multipart.NewWriter(&ubl)
	fw, _ := mw.CreateFormFile("file", "fatura.xml")
	_, _ = fw.Write(b)
	_ = mw.Close()
	rec = e.do(t, "POST", "/v1/taxpayer/uploads", e.tpA, &ubl, mw.FormDataContentType())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ubl upload = %d %s", rec.Code, rec.Body)
	}
	var job upload.Job
	decode(t, rec, &job)
	if job.Status != "done" || job.Report == nil || job.Report.Added != 1 {
		t.Fatalf("ubl job: %+v", job)
	}
}
