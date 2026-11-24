package easytrade

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// Fiş görselinin imzası her okumada değişir; masraf değişmiş sayılmaz,
// fişin anahtarı yol kısmıdır (#780). Uydurma veriler.
func TestExpenseReceiptKey(t *testing.T) {
	base := obj{"id": float64(901), "expense_date": "2026-10-02", "amount": "160.00", "tax_rate": map[string]any{"value": "1_kdv"}}
	plain, _ := mapExpense(base)
	if plain.ReceiptKey != "" {
		t.Fatalf("no receipt: %q", plain.ReceiptKey)
	}
	with := func(u string) obj {
		o := obj{}
		for k, v := range base {
			o[k] = v
		}
		o["receipt_url"] = u
		return o
	}
	a, _ := mapExpense(with("https://cdn.example.test/receipts/901.jpg?X-Signature=aaa&Expires=1"))
	b, _ := mapExpense(with("https://cdn.example.test/receipts/901.jpg?X-Signature=bbb&Expires=2"))
	c, _ := mapExpense(with("https://cdn.example.test/receipts/902.jpg?X-Signature=aaa"))
	if a.ReceiptKey != "https://cdn.example.test/receipts/901.jpg" || a.ReceiptKey != b.ReceiptKey {
		t.Fatalf("keys %q %q", a.ReceiptKey, b.ReceiptKey)
	}
	if a.Hash != b.Hash || a.Hash == c.Hash || a.Hash == plain.Hash {
		t.Fatalf("hashes %s %s %s %s", a.Hash, b.Hash, c.Hash, plain.Hash)
	}
	// Fişi olmayan masrafın özeti eskisiyle aynı (senkron her şeyi yeniden yazmaz).
	if plain.Hash != hash(base) {
		t.Fatal("hash of a detail without receipt changed")
	}
	nested, _ := mapExpense(func() obj {
		o := with("")
		delete(o, "receipt_url")
		o["receipt"] = map[string]any{"url": "https://cdn.example.test/r/9.png?sig=1"}
		return o
	}())
	if nested.ReceiptKey != "https://cdn.example.test/r/9.png" {
		t.Fatalf("nested %q", nested.ReceiptKey)
	}
}

// Okuyucunun cevabından oran satırları: KDV'si okunan her oran; tek oranlı
// fişte toplam ve KDV'den tek satır; KDV'si okunamayan oran yazılmaz.
func TestReceiptLines(t *testing.T) {
	res, err := extract.Parse(`{"kind":"expense_receipt","kind_confidence":0.9,"fields":{
"total_amount":{"value":"160,00","confidence":0.9},"tax_amount":{"value":"10,99","confidence":0.9},
"vat_1_total":{"value":"100,00","confidence":0.9},"vat_1_tax":{"value":"0,99","confidence":0.9},
"vat_20_total":{"value":"60,00","confidence":0.9},"vat_20_tax":{"value":"10,00","confidence":0.9},
"vat_10_total":{"value":"5,00","confidence":0.9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := ReceiptLines(res)
	if len(lines) != 2 || !lines[0].Rate.Equal(decimal.NewFromInt(1)) || lines[0].Base.String() != "99.01" ||
		!lines[1].Rate.Equal(decimal.NewFromInt(20)) || lines[1].Base.String() != "50" {
		t.Fatalf("lines %+v", lines)
	}
	if ReceiptOutcome(lines, decimal.RequireFromString("160")) != sources.ReceiptRead ||
		ReceiptOutcome(lines, decimal.RequireFromString("175")) != sources.ReceiptMismatch ||
		ReceiptOutcome(nil, decimal.RequireFromString("160")) != sources.ReceiptUnreadable {
		t.Fatal("outcome")
	}
	one, _ := extract.Parse(`{"kind":"expense_receipt","kind_confidence":0.9,"fields":{"total_amount":{"value":"120","confidence":0.9},
"tax_amount":{"value":"20","confidence":0.9},"tax_rate":{"value":"20","confidence":0.9}}}`)
	if l := ReceiptLines(one); len(l) != 1 || l[0].Base.String() != "100" {
		t.Fatalf("single %+v", l)
	}
	rateOnly, _ := extract.Parse(`{"kind":"expense_receipt","kind_confidence":0.9,"fields":{"total_amount":{"value":"120","confidence":0.9},"tax_rate":{"value":"20","confidence":0.9}}}`)
	if l := ReceiptLines(rateOnly); len(l) != 0 {
		t.Fatalf("derived from a rate: %+v", l)
	}
}

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Fiş indirme sınırları: yalnız https, görsel türü, boyut; kalıcı ret ile
// geçici hata ayrı.
func TestDownloadReceipt(t *testing.T) {
	img := pngBytes(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			_, _ = w.Write(img)
		case "/page.html":
			_, _ = w.Write([]byte("<html><body>giriş</body></html>"))
		case "/big.jpg":
			_, _ = w.Write(append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, MaxReceiptBytes)...))
		case "/gone.jpg":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("token sent to the receipt host")
		}
	}))
	defer srv.Close()
	s := &Service{hc: srv.Client(), base: "https://pro.example.test/api"}
	ctx := context.Background()
	data, media, err := s.downloadReceipt(ctx, srv.URL+"/ok.png?sig=1")
	if err != nil || media != "image/png" || !bytes.Equal(data, img) {
		t.Fatalf("ok: %s %v", media, err)
	}
	for _, p := range []string{"/page.html", "/big.jpg", "/gone.jpg"} {
		if _, _, err := s.downloadReceipt(ctx, srv.URL+p); !errors.Is(err, errReceiptRejected) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, _, err := s.downloadReceipt(ctx, srv.URL+"/busy.jpg"); err == nil || errors.Is(err, errReceiptRejected) {
		t.Errorf("busy: %v", err)
	}
	if _, _, err := s.downloadReceipt(ctx, strings.Replace(srv.URL, "https", "http", 1)+"/ok.png"); !errors.Is(err, errReceiptRejected) {
		t.Errorf("plain http: %v", err)
	}
}
