package entegrator

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// "Bağlantıyı test et" EDM'de (#780): girilen kullanıcıyla Login + tek
// başlık listesi. Doğru kullanıcı → ok; yanlış şifre → auth (EDM kodu
// 2001); servis kapalı / ulaşılamıyor → unavailable; yanıt gecikirse →
// timeout. Sonuçta ham hata metni ve şifre yok; bağlantı satırı değişmez.
func TestEDMConnectionTest(t *testing.T) {
	ctx := context.Background()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	d.Engagement(t, org, tp)
	svc := NewConnections(d.Pool, nil, nil)
	c, err := svc.Create(ctx, tp, "edm", "test")
	if err != nil {
		t.Fatal(err)
	}
	before, err := db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: c.UUID})
	if err != nil {
		t.Fatal(err)
	}
	good := map[string]string{"username": testfakes.EDMUsername, "password": testfakes.EDMPassword}
	var codes []string

	// No stored user and nothing entered: nothing to test.
	if _, err := svc.Test(ctx, tp, c.UUID, nil); !errors.Is(err, ErrNotReady) {
		t.Fatalf("no credentials: %v", err)
	}

	f := testfakes.EDMSample()
	srv := testfakes.Serve(t, f)
	svc.WithTest(srv.Client(), map[string]string{"edm": srv.URL})
	res, err := svc.Test(ctx, tp, c.UUID, good)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || f.Logins != 1 || f.Logouts != 1 {
		t.Fatalf("ok: %+v logins=%d logouts=%d", res, f.Logins, f.Logouts)
	}

	const wrong = "yanlis-sifre-987"
	res, err = svc.Test(ctx, tp, c.UUID, map[string]string{"username": testfakes.EDMUsername, "password": wrong})
	if err != nil {
		t.Fatal(err)
	}
	codes = append(codes, res.Code)
	if res.OK || res.Kind != TestAuth || !strings.HasPrefix(res.Code, "auth:2001") {
		t.Fatalf("wrong password: %+v", res)
	}

	f.Down = true
	res, err = svc.Test(ctx, tp, c.UUID, good)
	if err != nil {
		t.Fatal(err)
	}
	codes = append(codes, res.Code)
	if res.OK || res.Kind != TestUnavailable || !strings.Contains(res.Code, "503") {
		t.Fatalf("service down: %+v", res)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	svc.WithTest(&http.Client{Timeout: 5 * time.Second}, map[string]string{"edm": gone.URL})
	res, err = svc.Test(ctx, tp, c.UUID, good)
	if err != nil {
		t.Fatal(err)
	}
	codes = append(codes, res.Code)
	if res.OK || res.Kind != TestUnavailable {
		t.Fatalf("unreachable: %+v", res)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	svc.WithTest(&http.Client{Timeout: 300 * time.Millisecond}, map[string]string{"edm": slow.URL})
	res, err = svc.Test(ctx, tp, c.UUID, good)
	if err != nil {
		t.Fatal(err)
	}
	codes = append(codes, res.Code)
	if res.OK || res.Kind != TestTimeoutKind {
		t.Fatalf("slow: %+v", res)
	}

	for _, code := range codes {
		for _, leak := range []string{wrong, testfakes.EDMPassword, "şifre", "http://", "127.0.0.1"} {
			if strings.Contains(code, leak) {
				t.Errorf("code leaks %q: %s", leak, code)
			}
		}
	}
	after, err := db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: c.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != before.Status || after.SetupState != before.SetupState || after.CredentialsRef.Valid || string(after.LastError) != string(before.LastError) {
		t.Fatalf("test changed the connection: %+v → %+v", before, after)
	}
}
