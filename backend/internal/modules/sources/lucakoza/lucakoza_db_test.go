package lucakoza

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// The office setting, the connection and a sync against the fake Koza:
// off by default (409 in the handler), the password never stored, the
// four movement types written as invoices.
func TestServiceAgainstFake(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "9000000017")
	f := testfakes.LucaKozaSample()
	srv := testfakes.Serve(t, f)
	svc := NewService(d.Pool, srv.Client())
	cr := Credentials{BaseURL: srv.URL + "/LucaKoza", OrgCode: testfakes.LucaKozaOrgCode,
		Username: testfakes.LucaKozaUser, Password: testfakes.LucaKozaPassword}

	if st, err := svc.Settings(ctx, org); err != nil || st.Enabled {
		t.Fatalf("default setting: %+v %v, want off", st, err)
	}
	if _, _, err := svc.Connect(ctx, org, tp, cr, ""); !errors.Is(err, ErrOff) {
		t.Fatalf("connect while off: %v", err)
	}
	if _, err := svc.List(ctx, org, tp); !errors.Is(err, ErrOff) {
		t.Fatalf("list while off: %v", err)
	}
	if f.Logins != 0 {
		t.Fatal("Koza was called while the setting is off")
	}
	if _, err := svc.SaveSettings(ctx, org, 0, true); err != nil {
		t.Fatal(err)
	}

	bad := cr
	bad.Password = "yanlis"
	var kz *KozaError
	if _, _, err := svc.Connect(ctx, org, tp, bad, ""); !errors.As(err, &kz) || kz.Message != "Kullanıcı adı veya şifre hatalı." {
		t.Fatalf("wrong password: %v", err)
	}
	c, cos, err := svc.Connect(ctx, org, tp, cr, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cos) != 2 || c.SetupState != "credentials_entered" || len(c.Companies) != 2 {
		t.Fatalf("two companies must be chosen: %+v", c)
	}
	if _, err := svc.SelectCompany(ctx, org, tp, c.UUID, "999"); !errors.Is(err, ErrUnknownCo) {
		t.Fatalf("foreign company: %v", err)
	}
	if c, err = svc.SelectCompany(ctx, org, tp, c.UUID, testfakes.LucaKozaCompanyID); err != nil || c.SetupState != "active" || c.CompanyName != "Atlas Gıda - Merkez" {
		t.Fatalf("select: %+v %v", c, err)
	}
	var row string
	var ref *string
	_ = d.Pool.QueryRow(ctx, `SELECT row_to_json(s)::text, credentials_ref FROM source_connections s WHERE uuid = $1`, c.UUID).Scan(&row, &ref)
	if strings.Contains(row, testfakes.LucaKozaPassword) || ref != nil {
		t.Fatal("the Koza password was stored")
	}
	if !strings.Contains(row, testfakes.LucaKozaUser) || !strings.Contains(row, testfakes.LucaKozaOrgCode) {
		t.Fatalf("the non-secret settings are missing: %s", row)
	}

	from, to := oct()
	if _, err := svc.Sync(ctx, org, tp, c.UUID, from, to, ""); !errors.Is(err, ErrMissing) {
		t.Fatalf("sync without password: %v", err)
	}
	if _, err := svc.Sync(ctx, org, tp, c.UUID, from, to, "yanlis"); !errors.As(err, &kz) {
		t.Fatalf("sync with a wrong password: %v", err)
	}
	rep, err := svc.Sync(ctx, org, tp, c.UUID, from, to, testfakes.LucaKozaPassword)
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoicesCreated != 4 || rep.Excluded[SkipUnknownMovement] != 1 {
		t.Fatalf("report: %+v", rep)
	}
	var out, in, iade int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE direction = 'outgoing'), count(*) FILTER (WHERE direction = 'incoming'),
		count(*) FILTER (WHERE type = 'IADE') FROM invoices WHERE taxpayer_id = $1 AND source_system = 'luca_koza'`, tp).Scan(&out, &in, &iade)
	if out != 2 || in != 2 || iade != 2 {
		t.Fatalf("invoices: out %d in %d iade %d", out, in, iade)
	}
	var w string
	_ = d.Pool.QueryRow(ctx, `SELECT withholding_amount::numeric(18,2)::text FROM invoices WHERE taxpayer_id = $1 AND source_system = 'luca_koza' AND source_id = '1003'`, tp).Scan(&w)
	if w != "1000.00" {
		t.Fatalf("withholding = %q", w)
	}
	if _, err := svc.Sync(ctx, org, tp, c.UUID, from, to, testfakes.LucaKozaPassword); !errors.Is(err, ErrThrottle) {
		t.Fatalf("second sync at once: %v", err)
	}
	svc.now = func() time.Time { return time.Now().Add(SyncThrottle + time.Minute) }
	if rep, err = svc.Sync(ctx, org, tp, c.UUID, from, to, testfakes.LucaKozaPassword); err != nil || rep.InvoicesCreated != 0 {
		t.Fatalf("repeat sync: %+v %v", rep, err)
	}
	if len(f.Unknown) != 0 {
		t.Fatalf("unknown endpoints called: %v", f.Unknown)
	}
}
