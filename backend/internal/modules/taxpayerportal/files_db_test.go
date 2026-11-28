package taxpayerportal

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// #780 mobil belge görüntüleyici: fatura PDF'i yalnızca kendi şirketinin
// faturası için iner; başka şirketin faturası ve başka şirket başlığı kapalı.
func TestTaxpayerInvoicePDF(t *testing.T) {
	e := setup(t)
	rec := e.do(t, "GET", "/v1/taxpayer/invoices/"+e.invA.String()+"/pdf", e.tpA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own invoice pdf: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type %q", ct)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", rec.Body.Bytes()[:min(8, rec.Body.Len())])
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/invoices/"+e.invB.String()+"/pdf", e.tpA, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("other company's invoice pdf: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/invoices/"+e.invB.String()+"/pdf", e.tpB, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("other company header: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/invoices/not-a-uuid/pdf", e.tpA, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("bad uuid: %d", rec.Code)
	}
}

// Fiş fotoğrafı: bilinmeyen fiş 404, başka şirket başlığı 403.
func TestTaxpayerReceiptImageScoped(t *testing.T) {
	e := setup(t)
	if rec := e.do(t, "GET", "/v1/taxpayer/receipts/"+uuid.NewString()+"/image", e.tpA, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown receipt: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, "GET", "/v1/taxpayer/receipts/"+uuid.NewString()+"/image", e.tpB, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("other company header: %d", rec.Code)
	}
}
