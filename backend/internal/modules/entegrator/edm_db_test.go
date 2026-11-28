package entegrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// edmEnv is the sync env with the EDM fake behind the real EDM client.
func edmEnv(t *testing.T, cfg Config) (*env, *testfakes.EDM) {
	t.Helper()
	e := setup(t, cfg)
	f := &testfakes.EDM{}
	srv := testfakes.Serve(t, f)
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("edm", provider.Config{Username: testfakes.EDMUsername, Password: testfakes.EDMPassword, Endpoint: srv.URL, VKN: own})
	})
	return e, f
}

// edmAdd puts an invoice into the fake: dir IN (taxpayer is the customer)
// or OUT, created at cdate (Türkiye time, no zone as EDM sends it).
func edmAdd(t *testing.T, f *testfakes.EDM, n int, dir, cdate string) string {
	t.Helper()
	xml, ettn := invoiceXML(t, n, dir == "IN", "")
	f.Invoices = append(f.Invoices, testfakes.EDMInvoice{
		UUID: strings.ToUpper(ettn), ID: fmt.Sprintf("TST2026%09d", n), Direction: dir,
		Sender: "4840847211", Receiver: ownVKN, Supplier: "Tedarikçi A.Ş.", Customer: "Mükellef",
		IssueDate: cdate[:10], CDate: cdate, Amount: "1180.00", Status: "RECEIVE - SUCCEED", UBL: xml,
	})
	return ettn
}

func indexOf(calls []string, pred func(string) bool) int {
	for i, c := range calls {
		if pred(c) {
			return i
		}
	}
	return -1
}

// EDM ilk bağlanma (#780, 35): son iki ay (bu ay + geçen ay) önce listelenip
// UBL'leri indirilir, eski aylar sonra; sayfalama OFFSET ile ilerler; ikinci
// çalışma aynı faturaları yeniden yazmaz (çift kayıt yok); oturum kapatılır.
func TestEDMBackfillAgainstFake(t *testing.T) {
	ctx := context.Background()
	e, f := edmEnv(t, Config{PageSize: 2})
	e.now = mustTime(t, "2026-11-15T10:00:00+03:00")
	var nov []string
	for i := range 3 {
		nov = append(nov, edmAdd(t, f, 100+i, "IN", fmt.Sprintf("2026-11-0%dT09:00:00.123", i+2)))
	}
	out := edmAdd(t, f, 200, "OUT", "2026-11-04T12:00:00")
	f.Invoices[len(f.Invoices)-1].Status = "SEND - SUCCEED"
	oct := edmAdd(t, f, 110, "IN", "2026-10-01T00:00:01")
	old := edmAdd(t, f, 120, "IN", "2025-03-10T14:30:00")
	// Outside the backfill (before January 1 of last year).
	edmAdd(t, f, 130, "IN", "2024-12-31T23:00:00")

	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 6 || rep.Counters.UBLFetched != 6 || rep.Counters.UBLFailed != 0 {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 6 {
		t.Fatalf("invoices = %d", n)
	}
	for _, ettn := range append(nov, oct, old) {
		_ = e.doc(t, strings.ToLower(ettn), "incoming")
	}
	_ = e.doc(t, strings.ToLower(out), "outgoing")

	// The three November invoices fill two pages (LIMIT 2, OFFSET 2).
	if indexOf(f.Calls, func(c string) bool { return c == "list IN 2026-11-01T00:00:00+03:00 2" }) < 0 {
		t.Errorf("second page of November not listed: %v", f.Calls)
	}
	// Recent months first: the October UBL is in before any older month is listed.
	fetchOct := indexOf(f.Calls, func(c string) bool { return strings.EqualFold(c, "fetch "+oct) })
	firstOld := indexOf(f.Calls, func(c string) bool {
		return strings.HasPrefix(c, "list ") && !strings.Contains(c, " 2026-11-") && !strings.Contains(c, " 2026-10-")
	})
	if fetchOct < 0 || firstOld < 0 || fetchOct > firstOld {
		t.Errorf("recent months not first (fetch oct #%d, first old list #%d): %v", fetchOct, firstOld, f.Calls)
	}
	if first := indexOf(f.Calls, func(c string) bool { return strings.HasPrefix(c, "list ") }); first < 0 ||
		!strings.Contains(f.Calls[first], " 2026-11-01T00:00:00+03:00 ") {
		t.Errorf("first listing is not the current month: %v", f.Calls)
	}
	if f.Logins != 1 || f.Logouts != 1 {
		t.Errorf("one session per run: logins=%d logouts=%d", f.Logins, f.Logouts)
	}

	// Running again lists the same documents and writes nothing new.
	f.Calls = nil
	rep, err = e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.Listed != 6 || rep.Counters.New != 0 || rep.Counters.UBLFetched != 0 {
		t.Fatalf("second run %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 6 {
		t.Fatalf("invoices after second run = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM entegrator_documents WHERE taxpayer_id = $1`); n != 6 {
		t.Fatalf("documents after second run = %d", n)
	}
}

// Yanlış EDM şifresi ilk çekimi durdurur, bağlantı auth_failed olur; hata
// kaydında şifre yok.
func TestEDMWrongPasswordMarksAuthFailed(t *testing.T) {
	yanlisDeger := "hatali-deger-x"
	ctx := context.Background()
	e := setup(t, Config{})
	f := &testfakes.EDM{}
	srv := testfakes.Serve(t, f)
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) {
		return Open("edm", provider.Config{Username: testfakes.EDMUsername, Password: yanlisDeger, Endpoint: srv.URL})
	})
	edmAdd(t, f, 1, "IN", "2026-11-02T09:00:00")
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill); provider.KindOf(err) != provider.KindAuth {
		t.Fatalf("got %v", err)
	}
	row, err := e.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: e.tp, Uuid: e.conn.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "auth_failed" {
		t.Fatalf("status %s", row.Status)
	}
	if strings.Contains(string(row.LastError), yanlisDeger) {
		t.Fatalf("last_error leaks the password: %s", row.LastError)
	}
	if f.Logins != 0 {
		t.Fatalf("logins %d", f.Logins)
	}
}
