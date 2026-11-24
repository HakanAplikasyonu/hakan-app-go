package easytrade

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// fakeReceiptReader answers by image size (each fake receipt has its own)
// and counts the reads.
type fakeReceiptReader struct {
	answers map[int]string
	calls   int
}

func (f *fakeReceiptReader) Read(_ context.Context, req extract.Request) (extract.Result, error) {
	f.calls++
	return extract.Parse(f.answers[len(req.Image)])
}

type noStore struct{}

func (noStore) Upload(context.Context, storage.File, string) error { return nil }

func grayPNG(t *testing.T, n int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, n, n))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Fişten KDV oranı okuma (#780), oluşturmadan Luca Excel'ine: Easytrade
// BİM fişini tek %1 oranla gönderir; fiş görselinden %1 ve %20 okunur,
// toplam tuttuğu için okunan satırlar yazılır ve Luca fişinde her oran
// kendi 191 satırına gider. Toplamı tutmayan fişte Easytrade'in tek oranı
// kalır. Aynı fiş ikinci kez okunmaz. Uydurma veriler, sahte sunucular.
func TestReceiptRatesToLucaAgainstPostgres(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000025")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	var imp int64
	if err := d.Pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('c', 64)) RETURNING id`, org, eng).Scan(&imp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf) VALUES
		($1, $2, $3, '770.01', 'GENEL GİDERLER', '770', 2, true),
		($1, $2, $3, '191.01', 'İNDİRİLECEK KDV %20', '191', 2, true),
		($1, $2, $3, '191.02', 'İNDİRİLECEK KDV %1', '191', 2, true),
		($1, $2, $3, '100.01', 'MERKEZ KASA', '100', 2, true)`, org, eng, imp); err != nil {
		t.Fatal(err)
	}

	bim, other := grayPNG(t, 3), grayPNG(t, 5)
	imgs := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/901.png":
			_, _ = w.Write(bim)
		case "/r/902.png":
			_, _ = w.Write(other)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer imgs.Close()
	f := &testfakes.Easytrade{Invoices: map[string]map[string]any{}, BranchOf: map[string]string{}, License: true, Expenses: map[string]map[string]any{
		"901": {"id": 901, "expense_date": "2026-10-02", "amount": "160.00", "tax_rate": map[string]any{"value": "1_kdv"},
			"description": "BİM market", "receipt_url": imgs.URL + "/r/901.png?sig=a"},
		"902": {"id": 902, "expense_date": "2026-10-03", "amount": "300.00", "tax_rate": map[string]any{"value": "20_kdv"},
			"description": "kırtasiye", "receipt_url": imgs.URL + "/r/902.png?sig=a"},
		"903": {"id": 903, "expense_date": "2026-10-04", "amount": "120.00", "tax_rate": map[string]any{"value": "20_kdv"}, "description": "çay"},
	}}
	srv := testfakes.Serve(t, f)
	reader := &fakeReceiptReader{answers: map[int]string{
		len(bim): `{"kind":"expense_receipt","kind_confidence":0.9,"fields":{"total_amount":{"value":"160,00","confidence":0.9},
"vat_1_total":{"value":"100,00","confidence":0.9},"vat_1_tax":{"value":"0,99","confidence":0.9},
"vat_20_total":{"value":"60,00","confidence":0.9},"vat_20_tax":{"value":"10,00","confidence":0.9}}}`,
		len(other): `{"kind":"expense_receipt","kind_confidence":0.9,"fields":{"total_amount":{"value":"90,00","confidence":0.9},
"vat_20_total":{"value":"90,00","confidence":0.9},"vat_20_tax":{"value":"15,00","confidence":0.9}}}`,
	}}
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	var queued []uuid.UUID
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), srv.URL, srv.Client()).
		WithReceiptReader(reader).
		WithReceiptQueue(func(_ context.Context, _ int64, id uuid.UUID) error { queued = append(queued, id); return nil })
	svc.interval = 0
	svc.receiptHTTP = imgs.Client()

	c, err := svc.Connect(ctx, tp, "a@b.c", "pw", "42")
	if err != nil {
		t.Fatal(err)
	}
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	if _, err := svc.Sync(ctx, tp, c.UUID, from, to); err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0] != c.UUID {
		t.Fatalf("receipt pass not queued: %v", queued)
	}
	type line struct{ rate, base, tax string }
	linesOf := func(sourceID string) []line {
		t.Helper()
		rows, err := d.Pool.Query(ctx, `SELECT l.tax_rate::numeric(5,0)::text, l.tax_base::numeric(18,2)::text, l.tax_amount::numeric(18,2)::text
			FROM expense_vat_lines l JOIN expenses e ON e.taxpayer_id = l.taxpayer_id AND e.id = l.expense_id
			WHERE e.taxpayer_id = $1 AND e.source_id = $2 ORDER BY l.tax_rate`, tp, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []line
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.rate, &l.base, &l.tax); err != nil {
				t.Fatal(err)
			}
			out = append(out, l)
		}
		return out
	}
	// Okumadan önce: Easytrade'in tek oranı.
	if got := linesOf("901"); len(got) != 1 || got[0].rate != "1" {
		t.Fatalf("before read: %+v", got)
	}

	rep, err := svc.ReadReceipts(ctx, tp, c.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Read != 1 || rep.Mismatch != 1 || reader.calls != 2 {
		t.Fatalf("pass %+v, reads %d", rep, reader.calls)
	}
	if got := linesOf("901"); len(got) != 2 || got[0] != (line{"1", "99.01", "0.99"}) || got[1] != (line{"20", "50.00", "10.00"}) {
		t.Fatalf("read lines: %+v", got)
	}
	var tax string
	var rateNull bool
	_ = d.Pool.QueryRow(ctx, `SELECT tax_amount::numeric(18,2)::text, tax_rate IS NULL FROM expenses WHERE taxpayer_id = $1 AND source_id = '901'`, tp).Scan(&tax, &rateNull)
	if tax != "10.99" || !rateNull {
		t.Fatalf("expense VAT %s, single rate cleared %v", tax, rateNull)
	}
	if got := linesOf("902"); len(got) != 1 || got[0].rate != "20" || got[0].tax != "50.00" {
		t.Fatalf("mismatch keeps Easytrade's rate: %+v", got)
	}

	// Aynı fiş ikinci kez okunmaz; yeni imzalı URL ya da değişen açıklama
	// ile gelen senkron okunan satırları korur.
	if _, err := svc.ReadReceipts(ctx, tp, c.UUID); err != nil || reader.calls != 2 {
		t.Fatalf("read again: %d %v", reader.calls, err)
	}
	f.Expenses["901"]["receipt_url"] = imgs.URL + "/r/901.png?sig=b"
	f.Expenses["901"]["description"] = "BİM market alışverişi"
	svc.now = func() time.Time { return time.Now().Add(SyncThrottle + time.Minute) }
	r2, err := svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil || r2.ExpensesUpdated != 1 {
		t.Fatalf("second sync %+v %v", r2, err)
	}
	if _, err := svc.ReadReceipts(ctx, tp, c.UUID); err != nil || reader.calls != 2 {
		t.Fatalf("after resync: %d %v", reader.calls, err)
	}
	if got := linesOf("901"); len(got) != 2 {
		t.Fatalf("lines after resync: %+v", got)
	}

	// Ekranda: "fişten okundu" / "fiş okunamadı / tutmadı".
	rsvc := receiptsusecase.New(d.Pool, noStore{}, nil, nil, nil, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	page, err := rsvc.Expenses(ctx, sc, receiptsusecase.ExpenseFilter{Period: "2026-10", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, v := range page.Items {
		s := "-"
		if v.ReceiptRead != nil {
			s = *v.ReceiptRead
		}
		status[v.Amount.StringFixed(2)] = s
		if v.Amount.StringFixed(2) == "160.00" && (len(v.VatLines) != 2 || len(v.ReviewFields) != 0 && contains(v.ReviewFields, "tax_rate")) {
			t.Fatalf("view %+v", v)
		}
	}
	if status["160.00"] != "read" || status["300.00"] != "mismatch" || status["120.00"] != "-" {
		t.Fatalf("receipt_read %+v", status)
	}

	// Luca Excel: BİM fişinin her oranı kendi 191 satırında.
	lf, err := rsvc.ExportExpensesLuca(ctx, sc, user, receiptsusecase.LucaExportInput{Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenReader(bytes.NewReader(lf.Data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := x.GetRows(x.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	var first [][2]string // the first voucher (2 Oct, the BİM receipt)
	for _, r := range rows[1:] {
		for len(r) < 9 {
			r = append(r, "")
		}
		if r[0] == "1" {
			first = append(first, [2]string{r[3], r[7]})
		}
	}
	want := map[string]string{"191.02": "0.99", "191.01": "10.00", "770.01": "149.01"}
	n191 := 0
	for _, r := range first {
		if w, ok := want[r[0]]; ok {
			if r[1] != w {
				t.Errorf("%s debit %s, want %s", r[0], r[1], w)
			}
			delete(want, r[0])
		}
		if len(r[0]) >= 3 && r[0][:3] == "191" {
			n191++
		}
	}
	if len(want) != 0 || n191 != 2 {
		t.Fatalf("voucher 1 %+v, missing %v", first, want)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
