package secrets

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// TestPGStoreDumpAndRotation needs a migrated database:
//
//	TEST_DATABASE_URL=postgres://… go test ./internal/platform/secrets/ -run PG
func TestPGStoreDumpAndRotation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPGStore(db.New(pool))

	const user, pass = "pg-test-kullanici", "pg-test-parola-XYZ"
	v1 := New(mustKeyring(t, "", ""), store)
	ref, err := PutJSON(ctx, v1, "source_connection", creds{Username: user, Password: pass})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := refID(ref)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM secret_records WHERE uuid = $1", id) })

	// "Database dump": every column of the row as text, plus bytea as escape text.
	var dump string
	if err := pool.QueryRow(ctx,
		`SELECT row_to_json(s)::text || encode(wrapped_dek, 'escape') || encode(ciphertext, 'escape')
		 FROM secret_records s WHERE uuid = $1`, id).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, user) || strings.Contains(dump, pass) {
		t.Fatal("plaintext visible in database row")
	}

	v2 := New(mustKeyring(t, "k2:"+key(2), "k2"), store)
	if _, err := v2.Rotate(ctx, 50); err != nil {
		t.Fatal(err)
	}
	var kek string
	if err := pool.QueryRow(ctx, "SELECT kek_id FROM secret_records WHERE uuid = $1", id).Scan(&kek); err != nil || kek != "k2" {
		t.Fatalf("kek after rotation = %q (%v)", kek, err)
	}
	onlyK2, _ := ParseKeyring("k2:"+key(2), "k2", "")
	var got creds
	if err := GetJSON(ctx, New(onlyK2, store), "source_connection", ref, &got); err != nil || got.Password != pass {
		t.Fatalf("read after rotation: %v", err)
	}
}
