package parasut

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestConnectionFlow: authorize → single company bound → sync → throttle;
// tokens live in the vault and rotate (10 §1.2–1.3).
func TestConnectionFlow(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	f := &fakeParasut{}
	dataset(f)
	srv := f.server(t)
	kr, err := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	vault := secrets.New(kr, secrets.NewPGStore(db.New(d.Pool)))

	if _, err := NewService(d.Pool, vault, App{}, nil).Create(ctx, tp); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	app := App{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://portal.example/cb"}
	svc := NewService(d.Pool, vault, app, srv.Client())
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	c, err := svc.Create(ctx, tp)
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "draft" || !strings.Contains(c.AuthorizeURL, "client_id=cid") || !strings.Contains(c.AuthorizeURL, "state="+c.UUID.String()) {
		t.Fatalf("draft: %+v", c)
	}
	if _, err := svc.Sync(ctx, tp, c.UUID, now, now); !errors.Is(err, ErrBadState) {
		t.Fatalf("sync before authorize: %v", err)
	}
	c, cos, err := svc.Authorize(ctx, tp, c.UUID, "the-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(cos) != 1 || c.SetupState != "active" || c.CompanyID != "777" || c.CompanyName != "Atlas Gıda Ticaret A.Ş." || c.AuthorizeURL != "" {
		t.Fatalf("authorized: %+v %+v", c, cos)
	}
	var metaText, ref string
	_ = d.Pool.QueryRow(ctx, `SELECT meta::text, credentials_ref FROM source_connections WHERE uuid = $1`, c.UUID).Scan(&metaText, &ref)
	if strings.Contains(metaText, "acc") || strings.Contains(metaText, "ref1") || !strings.HasPrefix(ref, "vault:") {
		t.Fatalf("tokens outside the vault: %s %s", metaText, ref)
	}

	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	rep, err := svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoicesCreated == 0 || rep.ExpensesCreated != 2 {
		t.Fatalf("sync: %+v", rep)
	}
	if _, err := svc.Sync(ctx, tp, c.UUID, from, to); !errors.Is(err, ErrThrottled) {
		t.Fatalf("throttle: %v", err)
	}
	list, _ := svc.List(ctx, tp)
	if len(list) != 1 || list[0].LastSyncAt == nil || list[0].LastReport == nil || list[0].LastReport.ExpensesCreated != 2 {
		t.Fatalf("list: %+v", list)
	}
	if _, err := svc.SelectCompany(ctx, tp, c.UUID, "999"); !errors.Is(err, ErrUnknownCo) {
		t.Fatalf("foreign company: %v", err)
	}
}
