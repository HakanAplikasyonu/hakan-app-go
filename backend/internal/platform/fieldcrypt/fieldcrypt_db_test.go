package fieldcrypt_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/fieldcrypt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func testKey(b byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }

const marker = "GIZLI-KARSI-TARAF-ACIKLAMASI"

// dump is every raw payload of the bank tables as text, as a database dump
// would show it.
func dump(t *testing.T, ctx context.Context, d *testdb.DB) string {
	t.Helper()
	var s string
	if err := d.Pool.QueryRow(ctx, `SELECT COALESCE(string_agg(raw_payload::text, '|'), '') FROM (
		SELECT raw_payload FROM bank_transactions UNION ALL SELECT raw_payload FROM bank_transaction_sources) x`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestBankPayloadEncryptionMigration: plaintext rows written before the
// field key existed are sealed by Run(Encrypt), re-wrapped by Run(Rotate),
// returned to plaintext by Run(Decrypt); new rows are sealed on write.
func TestBankPayloadEncryptionMigration(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	t.Cleanup(func() { secrets.InstallFieldCipher(nil) })
	tp := d.Taxpayer(t, "legal", "9000000017")
	q := db.New(d.Pool)
	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{
		TaxpayerID: tp, Type: "bank_account", Name: "Ziraat TL", Currency: "TRY",
		BankID: pgtype.Text{String: "ziraat", Valid: true},
		Iban:   pgtype.Text{String: "TR330006100519786457841326", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := banking.NewService(d.Pool, nil)
	txn := func(id string, day int) banking.Transaction {
		return banking.Transaction{SourceSystem: "bank_ziraat", SourceID: id, CaptureMethod: banking.CaptureAPI,
			TransactionDate: time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC), Direction: banking.DirOut,
			Amount: "10", Description: "masraf", Raw: map[string]any{"note": marker}}
	}

	// 1. Before encryption: plaintext in the database.
	secrets.InstallFieldCipher(nil)
	if _, err := svc.Ingest(ctx, tp, box.ID, nil, []banking.Transaction{txn("A", 1)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dump(t, ctx, d), marker) {
		t.Fatal("setup: expected plaintext rows")
	}

	enc := config.EncryptionConfig{Key: "test-legacy-key-32-bytes-long!!!", FieldKey: testKey(5)}
	fc, err := fieldcrypt.Install(enc)
	if err != nil || fc == nil {
		t.Fatalf("install: %v", err)
	}
	// 2. New rows are sealed on write.
	if _, err := svc.Ingest(ctx, tp, box.ID, nil, []banking.Transaction{txn("B", 2)}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(dump(t, ctx, d), marker); n != 2 {
		t.Fatalf("after sealed write: %d plaintext copies, want 2 (only row A)", n)
	}

	// 3. Forward migration seals the old rows.
	only := []string{"bank_transactions", "bank_transaction_sources"}
	rep, err := fieldcrypt.Run(ctx, d.Pool, fc, fieldcrypt.Encrypt, 1, only)
	if err != nil || rep.Failed() != 0 {
		t.Fatalf("encrypt: %+v %v", rep, err)
	}
	if strings.Contains(dump(t, ctx, d), marker) {
		t.Fatal("plaintext left after encrypt")
	}
	if rep, _ := fieldcrypt.Run(ctx, d.Pool, fc, fieldcrypt.Encrypt, 1, only); rep.Columns[0].Changed != 0 {
		t.Fatalf("encrypt is not idempotent: %+v", rep)
	}
	statusKEKs := func() map[string]int64 {
		counts, err := fieldcrypt.Status(ctx, d.Pool)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]int64{}
		for _, c := range counts {
			if c.Column == "bank_transactions.raw_payload" {
				m[c.KEK] = c.Count
			}
		}
		return m
	}
	if m := statusKEKs(); m["field"] != 2 || m[""] != 0 {
		t.Fatalf("status after encrypt = %v", m)
	}

	// 4. Rotation to a new key, then the old key can go.
	enc.FieldKeyring, enc.FieldActiveKeyID = "f2027:"+testKey(6), "f2027"
	rotated, err := fieldcrypt.FromConfig(enc)
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := fieldcrypt.Run(ctx, d.Pool, rotated, fieldcrypt.Rotate, 1, only); err != nil || rep.Columns[0].Changed != 2 {
		t.Fatalf("rotate: %+v %v", rep, err)
	}
	if m := statusKEKs(); m["f2027"] != 2 || m["field"] != 0 {
		t.Fatalf("status after rotate = %v", m)
	}
	onlyNew, err := fieldcrypt.FromConfig(config.EncryptionConfig{FieldKeyring: "f2027:" + testKey(6)})
	if err != nil {
		t.Fatal(err)
	}

	// 5. Import undo copies a source payload onto its movement: still opens.
	var raw []byte
	if err := d.Pool.QueryRow(ctx, `SELECT raw_payload FROM bank_transactions ORDER BY id LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if plain, err := onlyNew.Open(secrets.PurposeBankRawPayload, raw); err != nil || !bytes.Contains(plain, []byte(marker)) {
		t.Fatalf("open with the new key only: %s %v", plain, err)
	}

	// 6. Reverse migration.
	if rep, err := fieldcrypt.Run(ctx, d.Pool, onlyNew, fieldcrypt.Decrypt, 1, only); err != nil || rep.Failed() != 0 {
		t.Fatalf("decrypt: %+v %v", rep, err)
	}
	if n := strings.Count(dump(t, ctx, d), marker); n != 4 {
		t.Fatalf("after decrypt: %d plaintext copies, want 4", n)
	}
	if m := statusKEKs(); m[""] != 2 {
		t.Fatalf("status after decrypt = %v", m)
	}
}

func TestFromConfigRefusesAppKey(t *testing.T) {
	appKey := testKey(3)
	if _, err := fieldcrypt.FromConfig(config.EncryptionConfig{Key: appKey, FieldKey: appKey}); err == nil {
		t.Fatal("field key equal to APP_ENCRYPTION_KEY must be refused")
	}
	if fc, err := fieldcrypt.FromConfig(config.EncryptionConfig{Key: appKey}); fc != nil || err != nil {
		t.Fatalf("no field key: %v %v", fc, err)
	}
}
