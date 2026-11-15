package webservice_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice/vakifbank"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// fakeBank answers like the Vakıfbank test system: the fixture, or a result code.
type fakeBank struct {
	code     string // "" → fixture
	requests int
	lastBody string
}

func (f *fakeBank) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests++
	var b bytes.Buffer
	_, _ = b.ReadFrom(r.Body)
	f.lastBody = b.String()
	if f.code == "" {
		fx, _ := os.ReadFile("vakifbank/testdata/getirhareket.xml")
		_, _ = w.Write(fx)
		return
	}
	_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetirHareketResponse><GetirHareketResult>` +
		`<IslemKodu>` + f.code + `</IslemKodu><IslemAciklamasi>x</IslemAciklamasi></GetirHareketResult></GetirHareketResponse></s:Body></s:Envelope>`))
}

func setup(t *testing.T, taxID string) (*webservice.Service, *fakeBank, *testdb.DB, int64, *time.Time) {
	t.Helper()
	d := testdb.Open(t)
	tp := d.Taxpayer(t, "legal", taxID)
	fb := &fakeBank{}
	srv := httptest.NewServer(fb)
	t.Cleanup(srv.Close)
	kr, err := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	vault := secrets.New(kr, secrets.NewPGStore(db.New(d.Pool)))
	reg := webservice.Registry{"vakifbank": func(string) webservice.Adapter { return vakifbank.New(srv.URL, srv.Client()) }}
	svc := webservice.NewService(d.Pool, vault, banking.NewService(d.Pool, nil), reg, webservice.Options{
		EgressIPs: []string{"203.0.113.10", "203.0.113.11"}, TechContact: "Entegrasyon ekibi, 0212 000 00 00, banka@example.com",
	})
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	webservice.SetClock(svc, func() time.Time { return now })
	return svc, fb, d, tp, &now
}

// TestConnectionFlow walks 20 §5.1 against the fake test system: instruction,
// branch, credentials, verification, then movements (20 §9 madde 1, 2, 5).
func TestConnectionFlow(t *testing.T) {
	svc, fb, d, tp, now := setup(t, "9000000017")
	ctx := context.Background()

	if _, err := svc.Create(ctx, tp, "garanti", "", nil); err != webservice.ErrUnknownBank {
		t.Fatalf("unknown bank: %v", err)
	}
	if _, err := svc.Create(ctx, tp, "vakifbank", "", []string{"TR00 1234"}); err == nil {
		t.Fatal("bad IBAN accepted")
	}
	c, err := svc.Create(ctx, tp, "vakifbank", "test", []string{"TR59 0001 5001 5800 7301 2345 67"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "draft" || c.BankName != "Vakıfbank" || len(c.Fields) != 3 || c.IBANs[0] != "TR590001500158007301234567" {
		t.Fatalf("created: %+v", c)
	}

	pdf, name, err := svc.Instruction(ctx, tp, c.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("instruction: %q %d bytes", name, len(pdf))
	}
	if c, _ = svc.Get(ctx, tp, c.UUID); c.SetupState != "form_ready" {
		t.Fatalf("after instruction: %s", c.SetupState)
	}
	if c, err = svc.MarkSubmitted(ctx, tp, c.UUID); err != nil || c.SetupState != "submitted_to_bank" || c.SubmittedAt == nil {
		t.Fatalf("submitted: %+v %v", c, err)
	}

	if _, err := svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"customer_no": "1"}); err == nil {
		t.Fatal("incomplete credentials accepted")
	}
	c, err = svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"customer_no": "123456789", "username": "atlas_ws", "password": "s3cret!"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "active" || c.Status != "active" || !c.HasCredentials || c.Error != nil || c.VerifiedAt == nil {
		t.Fatalf("verified: %+v", c)
	}
	// Verification is a one-day query for today (Türkiye time).
	if !strings.Contains(fb.lastBody, "<dto:SorguBaslangicTarihi>2026-11-04 00:00<") {
		t.Errorf("verify window: %s", fb.lastBody)
	}
	// The password is in the vault, never in source_connections.
	var metaText, ref string
	if err := d.Pool.QueryRow(ctx, `SELECT meta::text, credentials_ref FROM source_connections WHERE uuid = $1`, c.UUID).Scan(&metaText, &ref); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metaText, "s3cret") || !strings.HasPrefix(ref, "vault:") {
		t.Fatalf("credentials leaked: %s %s", metaText, ref)
	}
	// The bank account became a cash box linked to the connection.
	var boxes int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM cash_boxes WHERE taxpayer_id = $1 AND iban = 'TR590001500158007301234567'
		AND bank_id = 'vakifbank' AND bank_connection_id IS NOT NULL`, tp).Scan(&boxes)
	if boxes != 1 {
		t.Fatalf("cash boxes: %d", boxes)
	}

	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	res, err := svc.Sync(ctx, tp, c.UUID, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if res.Windows != 1 || res.Accounts != 1 || res.Inserted != 2 {
		t.Fatalf("sync: %+v", res)
	}
	var in, out string
	if err := d.Pool.QueryRow(ctx, `SELECT
		  (SELECT kind_detail FROM bank_transactions WHERE taxpayer_id = $1 AND debit_credit = 'A'),
		  (SELECT counterparty_name FROM bank_transactions WHERE taxpayer_id = $1 AND debit_credit = 'B')`, tp).Scan(&in, &out); err != nil {
		t.Fatal(err)
	}
	if out != "KALEM KIRTASIYE" || in == "" {
		t.Fatalf("stored: in=%q out=%q", in, out)
	}

	if _, err := svc.Sync(ctx, tp, c.UUID, day, day); err != webservice.ErrThrottled {
		t.Fatalf("throttle: %v", err)
	}
	*now = now.Add(webservice.SyncThrottle)
	res, err = svc.Sync(ctx, tp, c.UUID, day, day)
	if err != nil || res.Inserted != 0 || res.Skipped != 2 {
		t.Fatalf("second sync must not duplicate: %+v %v", res, err)
	}
	if res.Connection.LastSyncAt == nil {
		t.Fatal("last sync not recorded")
	}
	if _, err := svc.Sync(ctx, tp, c.UUID, day.AddDate(-2, 0, 0), day); err == nil {
		t.Fatal("range beyond 365 days accepted")
	}
}

// TestRefusals: the bank's codes become the reason the taxpayer reads (20 §5.4).
func TestRefusals(t *testing.T) {
	svc, fb, _, tp, _ := setup(t, "9000000017")
	ctx := context.Background()
	c, err := svc.Create(ctx, tp, "vakifbank", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := map[string]string{"customer_no": "1", "username": "u", "password": "p"}

	fb.code = "VBH0009"
	c, err = svc.SetCredentials(ctx, tp, c.UUID, creds)
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "failed" || c.Status != "auth_failed" || c.Error == nil || c.Error.Reason != webservice.ReasonIPRejected ||
		c.Error.Code != "VBH0009" || !strings.Contains(c.Error.Message, "IP") {
		t.Fatalf("ip rejected: %+v", c)
	}
	if _, err := svc.Sync(ctx, tp, c.UUID, time.Now(), time.Now()); err != webservice.ErrBadState {
		t.Fatalf("sync on failed connection: %v", err)
	}

	// Corrected at the branch: the same credentials now pass.
	fb.code = ""
	if c, err = svc.SetCredentials(ctx, tp, c.UUID, creds); err != nil || c.SetupState != "active" || c.Error != nil {
		t.Fatalf("retry: %+v %v", c, err)
	}
}

// TestOtherCompanysAccounts: the bank lists accounts of another VKN → refused (22 §1).
func TestOtherCompanysAccounts(t *testing.T) {
	svc, _, d, tp, _ := setup(t, "9000000025")
	ctx := context.Background()
	c, _ := svc.Create(ctx, tp, "vakifbank", "", nil)
	c, err := svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"customer_no": "1", "username": "u", "password": "p"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "failed" || c.Error == nil || c.Error.Reason != "tax_id_mismatch" {
		t.Fatalf("mismatch: %+v", c)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM cash_boxes WHERE taxpayer_id = $1`, tp).Scan(&n)
	if n != 0 {
		t.Fatalf("cash boxes created for a foreign account: %d", n)
	}
}

func TestWindows(t *testing.T) {
	loc := time.FixedZone("TRT", 3*3600)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 10, 20, 23, 59, 0, 0, loc)
	ws := webservice.Windows(from, to, 7)
	if len(ws) != 3 || !ws[0].End.Equal(time.Date(2026, 10, 7, 23, 59, 0, 0, loc)) ||
		!ws[1].Start.Equal(time.Date(2026, 10, 8, 0, 0, 0, 0, loc)) || !ws[2].End.Equal(to) {
		t.Fatalf("%+v", ws)
	}
	if len(webservice.Windows(from, to, 0)) != 1 || webservice.Windows(to, from, 7) != nil {
		t.Fatal("edge cases")
	}
}
