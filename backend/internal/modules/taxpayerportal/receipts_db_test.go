package taxpayerportal

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

type countingReader struct{ calls int }

const fuelAnswer = `{"kind":"fuel_receipt","kind_confidence":0.95,"fields":{
"date":{"value":"14.10.2026","confidence":0.95},"vendor_tax_number":{"value":"1234567890","confidence":0.92},
"vendor_name":{"value":"Petrol A.Ş.","confidence":0.9},"total_amount":{"value":"1.200,00","confidence":0.93},
"tax_amount":{"value":"200,00","confidence":0.9}}}`

func (c *countingReader) Read(context.Context, extract.Request) (extract.Result, error) {
	c.calls++
	r, err := extract.Parse(fuelAnswer)
	r.Model = "claude-test"
	return r, err
}

type recReviewer struct{ got []receiptsusecase.FieldReview }

func (r *recReviewer) SubmitField(_ context.Context, f receiptsusecase.FieldReview) error {
	r.got = append(r.got, f)
	return nil
}

func receiptForm(t *testing.T, tag string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("image", "fis.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), []byte(tag)...))
	_ = mw.WriteField("kind", "fuel_receipt")
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

// Mobil fiş yüklemesi (#199): yükleme taslak üretir, aynı Idempotency-Key ile
// tekrar yeni kayıt açmaz, geçmiş listelenir, düzeltme inceleme kuyruğuna
// mükellef girdisi olarak gider.
func TestTaxpayerReceiptUploadAndCorrections(t *testing.T) {
	e := setup(t)
	reader, rev := &countingReader{}, &recReviewer{}
	e.h.WithReceipts(receiptsusecase.New(e.d.Pool, storage.NewMemory(), reader, rev, nil, nil))

	post := func(key string) (int, map[string]any) {
		body, ct := receiptForm(t, "fis-1")
		req := e.req(t, http.MethodPost, "/v1/taxpayer/receipts", body, ct)
		req.Header.Set("Idempotency-Key", key)
		rec := e.serve(req)
		var out map[string]any
		decode(t, rec, &out)
		return rec.Code, out
	}
	code, first := post("cihaz-1")
	if code != http.StatusCreated || first["status"] != "draft" {
		t.Fatalf("upload = %d %v", code, first)
	}
	fields, _ := first["fields"].(map[string]any)
	if v, _ := fields["vendor_name"].(map[string]any); v["value"] != "Petrol A.Ş." {
		t.Fatalf("fields = %v", fields)
	}
	code, again := post("cihaz-1")
	if code != http.StatusOK || again["uuid"] != first["uuid"] || reader.calls != 1 {
		t.Fatalf("replay = %d %v (reads %d)", code, again["uuid"], reader.calls)
	}

	var list receiptPage
	decode(t, e.do(t, "GET", "/v1/taxpayer/receipts", e.tpA, nil, ""), &list)
	if list.Total != 1 || list.Items[0].UUID.String() != first["uuid"] {
		t.Fatalf("history = %+v", list)
	}

	id := first["uuid"].(string)
	rec := e.do(t, "POST", "/v1/taxpayer/receipts/"+id+"/corrections", e.tpA,
		bytes.NewBufferString(`{"corrections":[{"field":"total_amount","value":"1.250,00","note":"fişte 1.250 yazıyor"}]}`), "application/json")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("corrections = %d %s", rec.Code, rec.Body)
	}
	var got correctionResult
	decode(t, rec, &got)
	if len(got.Submitted) != 1 || got.Submitted[0] != "total_amount" {
		t.Fatalf("submitted = %+v", got)
	}
	var proposal *receiptsusecase.FieldReview
	for i := range rev.got {
		if rev.got[i].ProposedBy != 0 {
			proposal = &rev.got[i]
		}
	}
	if proposal == nil || proposal.ProposedBy != e.userA || proposal.TargetType != "expense" ||
		proposal.Value.Value != "1250.00" || proposal.Note != "fişte 1.250 yazıyor" || proposal.EngagementID != e.engA {
		t.Fatalf("proposal = %+v", proposal)
	}

	for body, want := range map[string]int{
		`{"corrections":[{"field":"password","value":"x"}]}`:       http.StatusBadRequest,
		`{"corrections":[{"field":"total_amount","value":"abc"}]}`: http.StatusBadRequest,
		`{"corrections":[]}`: http.StatusBadRequest,
		`{"corrections":[{"field":"date","value":"1.1.2026"}],"x":1}`: http.StatusBadRequest,
	} {
		if rec := e.do(t, "POST", "/v1/taxpayer/receipts/"+id+"/corrections", e.tpA, bytes.NewBufferString(body), "application/json"); rec.Code != want {
			t.Errorf("%s = %d, want %d", body, rec.Code, want)
		}
	}
	if rec := e.do(t, "POST", "/v1/taxpayer/receipts/"+uuid.NewString()+"/corrections", e.tpA,
		bytes.NewBufferString(`{"corrections":[{"field":"date","value":"01.01.2026"}]}`), "application/json"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown receipt = %d", rec.Code)
	}
	// Another company's panel cannot upload (no membership).
	body, ct := receiptForm(t, "fis-2")
	if rec := e.do(t, "POST", "/v1/taxpayer/receipts", e.tpB, body, ct); rec.Code != http.StatusForbidden {
		t.Fatalf("other company = %d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

func TestTaxpayerReceiptsUnavailableWithoutService(t *testing.T) {
	e := setup(t)
	body, ct := receiptForm(t, "x")
	if rec := e.do(t, "POST", "/v1/taxpayer/receipts", e.tpA, body, ct); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no service = %d", rec.Code)
	}
}
