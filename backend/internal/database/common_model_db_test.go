package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

func num(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestCommonDataModelRoundTrip needs a migrated database (TEST_DATABASE_URL).
// It runs inside a transaction that is rolled back.
func TestCommonDataModelRoundTrip(t *testing.T) {
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	var tp1, tp2, orgID, engID int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tx.QueryRow(ctx, `INSERT INTO taxpayers DEFAULT VALUES RETURNING id`).Scan(&tp1))
	must(tx.QueryRow(ctx, `INSERT INTO taxpayers DEFAULT VALUES RETURNING id`).Scan(&tp2))
	must(tx.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ('w2-test-'||gen_random_uuid(), 'Test') RETURNING id`).Scan(&orgID))
	must(tx.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, orgID, tp1).Scan(&engID))

	conn, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: tp1, SourceSystem: "entegrator_uyumsoft", SetupState: "draft", Meta: []byte(`{}`),
		CredentialsRef: pgtype.Text{String: "vault:" + uuid.NewString(), Valid: true},
	})
	must(err)

	cust, err := q.InsertCustomer(ctx, db.InsertCustomerParams{
		TaxpayerID: tp1, SourceSystem: "manual_upload", SourceID: "9205121120", CaptureMethod: "ubl_upload",
		Type: "customer", Name: "BBB Limited", TaxNumber: pgtype.Text{String: "9205121120", Valid: true}, RawPayload: []byte(`{}`),
	})
	must(err)
	// Same VKN again for the same taxpayer: rejected (01 §4.3 merge rule).
	must(savepoint(ctx, tx))
	_, err = q.InsertCustomer(ctx, db.InsertCustomerParams{
		TaxpayerID: tp1, SourceSystem: "parasut", SourceID: "77", CaptureMethod: "api",
		Type: "customer", Name: "BBB", TaxNumber: pgtype.Text{String: "9205121120", Valid: true}, RawPayload: []byte(`{}`),
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate VKN: %v", err)
	}
	must(rollbackTo(ctx, tx))

	ettn := uuid.New()
	params := db.InsertInvoiceParams{
		TaxpayerID: tp1, SourceSystem: "manual_upload", SourceID: ettn.String(), CaptureMethod: "ubl_upload",
		Ettn: pgtype.UUID{Bytes: ettn, Valid: true}, InvoiceNumber: "GIB2026000000001", Direction: "outgoing", Type: "SATIS",
		DocumentClass: "invoice", StatusNormalized: "active", Period: "2026-10",
		IssueDate: pgtype.Date{Valid: true}, Currency: "TRY",
		CustomerID: pgtype.Int8{Int64: cust.ID, Valid: true}, CustomerTaxNumber: pgtype.Text{String: "9205121120", Valid: true},
		LineExtensionAmount: num(t, "100.1234"), AllowanceTotalAmount: num(t, "0"), TaxExclusiveAmount: num(t, "100.1234"),
		TaxAmount: num(t, "20.0247"), TaxInclusiveAmount: num(t, "120.1481"), WithholdingAmount: num(t, "0"),
		ExciseAmount: num(t, "0"), CommunicationsTaxAmount: num(t, "0"), IncomeTaxWithholdingAmount: num(t, "0"),
		PayableAmount: num(t, "120.1481"), HasItems: true, TaxSummarySource: "items", RawPayload: []byte(`{}`),
	}
	must(params.IssueDate.Scan("2026-10-15"))
	inv, err := q.InsertInvoice(ctx, params)
	must(err)
	must(q.InsertInvoiceItem(ctx, db.InsertInvoiceItemParams{
		TaxpayerID: tp1, InvoiceID: inv.ID, LineNo: 1, Name: "Kalem", Quantity: num(t, "1.5"), UnitPrice: num(t, "66.748933"),
		LineExtensionAmount: num(t, "100.1234"), OwnAllowanceAmount: num(t, "0"), AllowanceTotalAmount: num(t, "0"),
		TaxExclusiveAmount: num(t, "100.1234"), TaxRate: num(t, "20"), TotalTaxAmount: num(t, "20.0247"),
		TaxInclusiveAmount: num(t, "120.1481"), WithholdingRate: num(t, "0"), WithholdingAmount: num(t, "0"),
		ExciseRate: num(t, "0"), ExciseAmount: num(t, "0"), CommunicationsTaxRate: num(t, "0"), CommunicationsTaxAmount: num(t, "0"),
		IncomeTaxWithholdingAmount: num(t, "0"), LineType: "unknown", TaxBreakdowns: []byte(`[]`),
	}))
	must(q.InsertInvoiceTaxSummary(ctx, db.InsertInvoiceTaxSummaryParams{
		TaxpayerID: tp1, InvoiceID: inv.ID, TaxRate: num(t, "20"), Base: num(t, "100.1234"), Vat: num(t, "20.0247"), Withholding: num(t, "0"),
	}))

	// Lossless NUMERIC(18,4).
	var got string
	must(tx.QueryRow(ctx, `SELECT tax_amount::text FROM invoices WHERE id = $1`, inv.ID).Scan(&got))
	if got != "20.0247" {
		t.Fatalf("tax_amount = %s", got)
	}
	// Same ETTN from a second source is allowed (01 §6: not unique).
	params.SourceSystem, params.CaptureMethod = "parasut", "api"
	_, err = q.InsertInvoice(ctx, params)
	must(err)
	found, err := q.FindInvoicesByETTN(ctx, db.FindInvoicesByETTNParams{TaxpayerID: tp1, Ettn: pgtype.UUID{Bytes: ettn, Valid: true}})
	must(err)
	if len(found) != 2 {
		t.Fatalf("by ettn = %d", len(found))
	}
	// Other taxpayer sees nothing.
	other, err := q.FindInvoicesByETTN(ctx, db.FindInvoicesByETTNParams{TaxpayerID: tp2, Ettn: pgtype.UUID{Bytes: ettn, Valid: true}})
	must(err)
	if len(other) != 0 {
		t.Fatal("cross-taxpayer leak")
	}
	// Same source key twice: rejected.
	must(savepoint(ctx, tx))
	_, err = q.InsertInvoice(ctx, params)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate source key: %v", err)
	}
	must(rollbackTo(ctx, tx))

	// An item cannot point at another taxpayer's invoice (composite FK).
	must(savepoint(ctx, tx))
	err = q.InsertInvoiceItem(ctx, db.InsertInvoiceItemParams{
		TaxpayerID: tp2, InvoiceID: inv.ID, LineNo: 9, Quantity: num(t, "0"), UnitPrice: num(t, "0"),
		LineExtensionAmount: num(t, "0"), OwnAllowanceAmount: num(t, "0"), AllowanceTotalAmount: num(t, "0"),
		TaxExclusiveAmount: num(t, "0"), TaxRate: num(t, "0"), TotalTaxAmount: num(t, "0"), TaxInclusiveAmount: num(t, "0"),
		WithholdingRate: num(t, "0"), WithholdingAmount: num(t, "0"), ExciseRate: num(t, "0"), ExciseAmount: num(t, "0"),
		CommunicationsTaxRate: num(t, "0"), CommunicationsTaxAmount: num(t, "0"), IncomeTaxWithholdingAmount: num(t, "0"),
		LineType: "unknown", TaxBreakdowns: []byte(`[]`),
	})
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("cross-taxpayer item: %v", err)
	}
	must(rollbackTo(ctx, tx))

	// Attachments dedupe on sha256.
	sha := "aa00000000000000000000000000000000000000000000000000000000000000"
	a1, err := q.UpsertAttachment(ctx, db.UpsertAttachmentParams{TaxpayerID: tp1, Kind: "ubl_xml", StorageKey: "k1", Sha256: sha, SizeBytes: 10})
	must(err)
	a2, err := q.UpsertAttachment(ctx, db.UpsertAttachmentParams{TaxpayerID: tp1, Kind: "ubl_xml", StorageKey: "k2", Sha256: sha, SizeBytes: 10})
	must(err)
	if !a1.Inserted || a2.Inserted || a1.ID != a2.ID || a2.StorageKey != "k1" {
		t.Fatalf("attachment dedupe: %+v %+v", a1, a2)
	}

	// One open warning per (entity, code).
	w := db.UpsertOpenDataWarningParams{OrganizationID: orgID, EngagementID: engID, TaxpayerID: tp1,
		Period: pgtype.Text{String: "2026-10", Valid: true}, EntityType: "invoice", EntityID: pgtype.Int8{Int64: inv.ID, Valid: true},
		Code: "TAX_TOTAL_MISMATCH", Severity: "warning", Params: []byte(`{"a":1}`), Producer: "ubl_parser"}
	w1, err := q.UpsertOpenDataWarning(ctx, w)
	must(err)
	w.Params = []byte(`{"a":2}`)
	w2, err := q.UpsertOpenDataWarning(ctx, w)
	must(err)
	if w1.ID != w2.ID || string(w2.Params) != `{"a": 2}` {
		t.Fatalf("warning upsert: %d %d %s", w1.ID, w2.ID, w2.Params)
	}

	run, err := q.StartSyncRun(ctx, db.StartSyncRunParams{TaxpayerID: tp1, ConnectionID: conn.ID, Kind: "manual"})
	must(err)
	n, err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{TaxpayerID: tp1, ID: run.ID, Status: "success", Counters: []byte(`{"read":1}`)})
	must(err)
	if n != 1 {
		t.Fatal("sync run not finished")
	}
}

func savepoint(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SAVEPOINT sp")
	return err
}

// rollbackTo clears the aborted state left by an expected failure.
func rollbackTo(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp")
	return err
}
