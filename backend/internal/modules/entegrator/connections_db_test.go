package entegrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type queued struct {
	tp   int64
	id   uuid.UUID
	kind string
}

// Kabul (#518 A1): ofis entegratör seçer, web servis kullanıcısını girer;
// kimlik kasaya gider (satırda yok), ilk çekim (backfill) kuyruğa düşer.
// Aynı entegratöre ikinci bağlantı açılmaz; yeni kimlik eski hatayı siler.
func TestConnectionsSetup(t *testing.T) {
	ctx := context.Background()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	d.Engagement(t, org, tp)
	kr, err := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	vault := secrets.New(kr, secrets.NewPGStore(db.New(d.Pool)))
	var jobs []queued
	svc := NewConnections(d.Pool, vault, func(_ context.Context, tp int64, id uuid.UUID, kind string) error {
		jobs = append(jobs, queued{tp, id, kind})
		return nil
	})

	if _, err := svc.Create(ctx, tp, "yok", ""); !errors.Is(err, ErrUnknownIntegrator) {
		t.Fatalf("unknown integrator: %v", err)
	}
	c, err := svc.Create(ctx, tp, "izibiz", "test")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != "draft" || c.IntegratorName != "İzibiz" || c.Environment != "test" || len(c.Fields) != 2 || len(jobs) != 0 {
		t.Fatalf("draft: %+v jobs=%v", c, jobs)
	}
	if _, err := svc.Create(ctx, tp, "izibiz", ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := svc.Sync(ctx, tp, c.UUID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("sync before credentials: %v", err)
	}
	if _, err := svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"username": "ofis"}); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("missing password: %v", err)
	}

	// A previous failure is cleared by new credentials.
	row, _ := db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: c.UUID})
	if _, err := db.New(d.Pool).SetSourceConnectionStatus(ctx, db.SetSourceConnectionStatusParams{
		TaxpayerID: tp, ID: row.ID, Status: "auth_failed", LastError: []byte(`{"kind":"auth","message":"401"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if v, _ := svc.Get(ctx, tp, c.UUID); v.State != "auth_failed" || v.Error == nil || v.Error.Message != "401" {
		t.Fatalf("failed view: %+v", v)
	}

	c, err = svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"username": " ofis ", "password": "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if c.State != "waiting" || !c.HasCredentials || c.Error != nil {
		t.Fatalf("after credentials: %+v", c)
	}
	if len(jobs) != 1 || jobs[0] != (queued{tp, c.UUID, RunBackfill}) {
		t.Fatalf("jobs: %v", jobs)
	}
	row, _ = db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: c.UUID})
	if strings.Contains(string(row.Meta), "s3cret") || !strings.HasPrefix(row.CredentialsRef.String, "vault:") {
		t.Fatalf("credentials leaked to the row: meta=%s ref=%s", row.Meta, row.CredentialsRef.String)
	}
	var got Credentials
	if err := secrets.GetJSON(ctx, vault, CredentialsPurpose, secrets.Ref(row.CredentialsRef.String), &got); err != nil {
		t.Fatal(err)
	}
	if got.Username != "ofis" || got.Password != "s3cret" || !got.Test {
		t.Fatalf("vault: %+v", got)
	}

	// Re-entering replaces the same secret.
	ref := row.CredentialsRef.String
	if _, err := svc.SetCredentials(ctx, tp, c.UUID, map[string]string{"username": "ofis", "password": "yeni"}); err != nil {
		t.Fatal(err)
	}
	row, _ = db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: c.UUID})
	if row.CredentialsRef.String != ref {
		t.Fatalf("ref changed: %s → %s", ref, row.CredentialsRef.String)
	}
	_ = secrets.GetJSON(ctx, vault, CredentialsPurpose, secrets.Ref(ref), &got)
	if got.Password != "yeni" {
		t.Fatalf("replace: %+v", got)
	}

	if _, err := svc.Sync(ctx, tp, c.UUID); err != nil {
		t.Fatal(err)
	}
	if jobs[len(jobs)-1].kind != RunManual {
		t.Fatalf("manual: %v", jobs)
	}

	// İşNet: no user, first sync at once. Sovos options go to Options.
	isnet, err := svc.Create(ctx, tp, "isnet", "")
	if err != nil {
		t.Fatal(err)
	}
	if isnet.State != "waiting" || !isnet.IPFiltered || jobs[len(jobs)-1] != (queued{tp, isnet.UUID, RunBackfill}) {
		t.Fatalf("isnet: %+v %v", isnet, jobs)
	}
	sv, _ := svc.Create(ctx, tp, "sovos", "")
	if _, err := svc.SetCredentials(ctx, tp, sv.UUID, map[string]string{"username": "u", "password": "p", "sender_alias": "urn:mail:gb"}); err != nil {
		t.Fatal(err)
	}
	row, _ = db.New(d.Pool).GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: tp, Uuid: sv.UUID})
	got = Credentials{}
	_ = secrets.GetJSON(ctx, vault, CredentialsPurpose, secrets.Ref(row.CredentialsRef.String), &got)
	if got.Options["sender_alias"] != "urn:mail:gb" || got.Test {
		t.Fatalf("sovos: %+v", got)
	}

	list, err := svc.List(ctx, tp)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %d %v", len(list), err)
	}

	// Another taxpayer's connection is not found.
	other := d.Taxpayer(t, "legal", "4840847211")
	if _, err := svc.Get(ctx, other, c.UUID); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("other taxpayer: %v", err)
	}

	// No vault: nothing written. No queue: set up, but no sync to start.
	if _, err := NewConnections(d.Pool, nil, nil).SetCredentials(ctx, tp, c.UUID, nil); !errors.Is(err, ErrNoVault) {
		t.Fatalf("no vault: %v", err)
	}
	if _, err := NewConnections(d.Pool, vault, nil).Sync(ctx, tp, c.UUID); !errors.Is(err, ErrNoQueue) {
		t.Fatalf("no queue: %v", err)
	}
	if v, err := NewConnections(d.Pool, vault, nil).SetCredentials(ctx, tp, c.UUID, map[string]string{"username": "ofis", "password": "x"}); err != nil || v.State != "waiting" {
		t.Fatalf("credentials without queue: %+v %v", v, err)
	}
}

func TestCatalogFieldsMatchOpeners(t *testing.T) {
	for _, i := range Catalog() {
		if _, ok := integrator(i.ID); !ok {
			t.Fatal(i.ID)
		}
		if i.IPFiltered != (len(i.Fields) == 0) {
			t.Errorf("%s: ip_filtered=%v fields=%d", i.ID, i.IPFiltered, len(i.Fields))
		}
	}
	if s := State("draft", "pending", false, false); s != "draft" {
		t.Fatal(s)
	}
	if s := State("credentials_entered", "pending", true, false); s != "waiting" {
		t.Fatal(s)
	}
}
