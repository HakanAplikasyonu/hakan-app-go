package entegrator

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// isnetConnection opens an İşNet connection (test environment) of a fresh
// taxpayer; İşNet needs no stored user, so there is no vault.
func isnetConnection(t *testing.T) (*Connections, int64, uuid.UUID) {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	d.Engagement(t, org, tp)
	svc := NewConnections(d.Pool, nil, nil)
	c, err := svc.Create(context.Background(), tp, "isnet", "test")
	if err != nil {
		t.Fatal(err)
	}
	return svc, tp, c.UUID
}

// "Bağlantıyı test et" İşNet'te (32 §2, §9): kullanıcı yok; çağrı
// mükellefin VKN'siyle yalnızca SearchAllInvoice yapar. Yanıt gelmezse
// (tanımsız IP) sonuç "permission:ip-not-allowlisted" olur, test ortamında
// da. Result=Failed ham metni sonuca girmez.
func TestIsnetConnectionTest(t *testing.T) {
	ctx := context.Background()
	svc, tp, id := isnetConnection(t)

	fake := &fakeIsnet{}
	ok := httptest.NewServer(fake)
	t.Cleanup(ok.Close)
	svc.WithTest(ok.Client(), map[string]string{"isnet": ok.URL})
	// Entered values are ignored: İşNet has no user.
	res, err := svc.Test(ctx, tp, id, map[string]string{"username": "yok"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Kind != "" || res.TestedAt.IsZero() {
		t.Fatalf("ok: %+v", res)
	}
	fake.mu.Lock()
	if len(fake.actions) != 1 || !strings.HasSuffix(fake.actions[0], "/SearchAllInvoice") || len(fake.vkns) != 1 || fake.vkns[0] != ownVKN {
		t.Errorf("calls %v vkns %v", fake.actions, fake.vkns)
	}
	fake.mu.Unlock()

	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>
<SearchAllInvoiceResponse xmlns="http://tempuri.org/"><SearchAllInvoiceResult xmlns:a="http://schemas.datacontract.org/2004/07/EInvoice.Service.Model" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
<a:ErrorMessage>Beklenmeyen sunucu hatasi ayrinti-xyz</a:ErrorMessage><a:Invoices/><a:Result>Failed</a:Result></SearchAllInvoiceResult></SearchAllInvoiceResponse></s:Body></s:Envelope>`)
	}))
	t.Cleanup(failed.Close)
	svc.WithTest(failed.Client(), map[string]string{"isnet": failed.URL})
	res, err = svc.Test(ctx, tp, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Kind != TestUnknown || res.Code != "unknown:Failed" {
		t.Fatalf("failed: %+v", res)
	}

	// İşNet drops calls from IPs it does not know: no answer at all.
	silent := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(silent.Close)
	svc.WithTest(&http.Client{Timeout: 300 * time.Millisecond}, map[string]string{"isnet": silent.URL})
	res, err = svc.Test(ctx, tp, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Kind != TestPermission || res.Code != "permission:ip-not-allowlisted" {
		t.Fatalf("no answer: %+v", res)
	}
}
