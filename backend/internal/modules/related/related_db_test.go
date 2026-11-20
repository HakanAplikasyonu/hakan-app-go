package related

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestRelatedChain: fatura ↔ banka hareketi ↔ fiş ↔ uyarı ↔ inceleme zinciri
// her iki yönden okunur; izni olmayan tür gizlenir; başka mükellefin kaydı
// bulunamaz (#469).
func TestRelatedChain(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	tp := d.Taxpayer(t, "legal", "9000000017")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	sub := Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	svc := New(q)
	all := func(string) bool { return true }

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	today := time.Now().UTC()

	// Fatura ve satırı.
	var invID int64
	var invUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
		                      issue_date, period, currency, customer_name, payable_amount, tax_summary_source)
		VALUES ($1, 'manual_upload', 'ATL1', 'ubl_upload', 'ATL2026000000101', 'outgoing', 'SATIS',
		        $2::date, to_char($2::date, 'YYYY-MM'), 'TRY', 'Yıldız Market', 12000, 'items')
		RETURNING id, uuid`, tp, today).Scan(&invID, &invUUID))
	var itemID int64
	must(d.Pool.QueryRow(ctx, `INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no) VALUES ($1, $2, 1) RETURNING id`,
		tp, invID).Scan(&itemID))

	// Tahsilat hareketi ve eşleşme.
	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: tp, Type: "bank_account", Currency: "TRY",
		Iban: pgtype.Text{String: "TR760001000519786457841326", Valid: true}})
	must(err)
	_, err = banking.NewService(d.Pool, nil).Ingest(ctx, tp, box.ID, nil, []banking.Transaction{{
		SourceSystem: "bank_ziraat", SourceID: "a", CaptureMethod: banking.CaptureAPI, TransactionDate: today,
		Direction: banking.DirIn, Amount: "12000", Currency: "TRY", Description: "FAST ATL2026000000101 TAHSILAT",
	}})
	must(err)
	var btID int64
	var btUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `SELECT id, uuid FROM bank_transactions WHERE taxpayer_id = $1 AND source_id = 'a'`, tp).Scan(&btID, &btUUID))
	var amount pgtype.Numeric
	must(amount.Scan("12000"))
	_, err = q.InsertBankTransactionMatch(ctx, db.InsertBankTransactionMatchParams{
		TaxpayerID: tp, BankTransactionID: btID, TargetType: "invoice", TargetID: invID,
		Amount: amount, Score: 150, Method: "auto", Evidence: []byte(`{}`),
	})
	must(err)

	// Faturanın satırından oluşan fiş.
	var draftID int64
	var draftUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, description)
		VALUES ($1, $2, $3, to_char($4::date, 'YYYY-MM'), 7, $4::date, 'Satış faturası') RETURNING id, uuid`,
		org, eng, tp, today).Scan(&draftID, &draftUUID))
	_, err = d.Pool.Exec(ctx, `INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, debit, credit, source_kind, source_id)
		VALUES ($1, $2, $3, 1, '120.01', 12000, 0, 'invoice_item', $4::bigint::text), ($1, $2, $3, 2, '600.01', 0, 12000, 'invoice_item', $4::bigint::text)`,
		org, eng, draftID, itemID)
	must(err)

	// Satır uyarısı ve satırın inceleme önerisi.
	var warnUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id,
			code, severity, params, producer, status)
		VALUES ($1, $2, $3, 'invoice_item', $4, 'VAT_RATE_MISMATCH', 'warning', '{}', 'test', 'open') RETURNING uuid`,
		org, eng, tp, itemID).Scan(&warnUUID))
	_, err = d.Pool.Exec(ctx, `INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id,
			target_field, proposed_value, confidence, rationale, origin, rule_ref, status)
		VALUES ($1, $2, $3, 'account_code', 'invoice_item', $4, 'account_code', '"600.01"', 0.8, 'test', 'rule', 'R1', 'pending')`,
		org, eng, tp, itemID)
	must(err)
	// Reddedilen öneri artık bağ değil (#669): listede olmamalı.
	_, err = d.Pool.Exec(ctx, `INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id,
			target_field, proposed_value, confidence, rationale, origin, rule_ref, status)
		VALUES ($1, $2, $3, 'account_code', 'invoice_item', $4, 'vat_account_code', '"191.99"', 0.4, 'test', 'rule', 'R2', 'rejected')`,
		org, eng, tp, itemID)
	must(err)
	// Onaylanmamış karşı hesap önerisi tahmindir: "possible".
	_, err = d.Pool.Exec(ctx, `INSERT INTO bank_transaction_postings (organization_id, engagement_id, taxpayer_id, bank_transaction_id,
			account_code, account_name, status, confidence, source)
		VALUES ($1, $2, $3, $4, '780.01', 'BANKA MASRAFLARI', 'suggested', 0.6, 'kind_default')`,
		org, eng, tp, btID)
	must(err)

	has := func(res Result, kind, relation, id string) bool {
		for _, it := range res.Items {
			if it.Kind == kind && it.Relation == relation && (id == "" || (it.UUID != nil && *it.UUID == id)) {
				return true
			}
		}
		return false
	}

	// Fatura → ödeme, fiş, uyarı, inceleme.
	res, err := svc.List(ctx, sub, "invoice", invUUID, all)
	must(err)
	for _, want := range [][3]string{
		{"bank_transaction", "payment", btUUID.String()},
		{"journal_draft", "journal", draftUUID.String()},
		{"warning", "warning", warnUUID.String()},
		{"suggestion", "possible", ""},
	} {
		if !has(res, want[0], want[1], want[2]) {
			t.Errorf("invoice: missing %v in %+v", want, res.Items)
		}
	}
	for _, it := range res.Items {
		if it.Kind == "suggestion" && it.Status == "rejected" {
			t.Errorf("rejected suggestion listed: %+v", it)
		}
		if it.Kind == "bank_transaction" && (it.Amount == nil || *it.Amount != "12000.00") {
			t.Errorf("payment amount: %+v", it)
		}
	}

	// Banka hareketi → fatura; fiş → fatura; uyarı → fatura ve kural.
	res, err = svc.List(ctx, sub, "bank_transaction", btUUID, all)
	must(err)
	if !has(res, "invoice", "payment", invUUID.String()) || !has(res, "account", "possible", "") || has(res, "account", "counterparty_account", "") {
		t.Errorf("bank: %+v", res.Items)
	}
	res, err = svc.List(ctx, sub, "journal_draft", draftUUID, all)
	must(err)
	if !has(res, "invoice", "source", invUUID.String()) || len(res.Items) != 1 {
		t.Errorf("journal: %+v", res.Items)
	}
	res, err = svc.List(ctx, sub, "warning", warnUUID, all)
	must(err)
	if !has(res, "invoice", "source", invUUID.String()) || !has(res, "rule", "rule", "") {
		t.Errorf("warning: %+v", res.Items)
	}

	// Banka kontrolünün dönem uyarısı hareketi params içinde uuid ile anar.
	var bnkUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type,
			code, severity, params, producer, status)
		VALUES ($1, $2, $3, to_char($5::date, 'YYYY-MM'), 'period', 'BNK_UNINVOICED_COLLECTION', 'warning',
			jsonb_build_object('groups', jsonb_build_array(jsonb_build_object('transactions', jsonb_build_array($4::text)))),
			'control:bank', 'open') RETURNING uuid`,
		org, eng, tp, btUUID.String(), today).Scan(&bnkUUID))
	res, err = svc.List(ctx, sub, "bank_transaction", btUUID, all)
	must(err)
	if !has(res, "warning", "warning", bnkUUID.String()) {
		t.Errorf("bank warning: %+v", res.Items)
	}
	res, err = svc.List(ctx, sub, "warning", bnkUUID, all)
	must(err)
	if !has(res, "bank_transaction", "source", btUUID.String()) {
		t.Errorf("warning → bank: %+v", res.Items)
	}

	// Elle bağ (#710): kaynağı olmayan fişe fatura ve banka hareketi
	// bağlanır; iki yönde "manual" görünür; ikinci bağ eklenmez; bağ
	// kaldırılınca kaybolur. Başka mükellefin faturası bağlanamaz.
	var handID int64
	var handUUID uuid.UUID
	must(d.Pool.QueryRow(ctx, `INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, description)
		VALUES ($1, $2, $3, to_char($4::date, 'YYYY-MM'), 8, $4::date, 'Elle fiş') RETURNING id, uuid`,
		org, eng, tp, today).Scan(&handID, &handUUID))
	_, err = d.Pool.Exec(ctx, `INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, debit, credit)
		VALUES ($1, $2, $3, 1, '102.01', 500, 0), ($1, $2, $3, 2, '120.01', 0, 500)`, org, eng, handID)
	must(err)
	jsvc := journals.New(d.Pool, storage.NewMemory())
	scope := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	for i := 0; i < 2; i++ {
		if on, err := jsvc.SetManualLink(ctx, scope, handUUID, journals.LinkInvoice, invUUID, true); err != nil || !on {
			t.Fatalf("link invoice: %v %v", on, err)
		}
	}
	if _, err := jsvc.SetManualLink(ctx, scope, handUUID, journals.LinkBankTransaction, btUUID, true); err != nil {
		t.Fatal(err)
	}
	res, err = svc.List(ctx, sub, "journal_draft", handUUID, all)
	must(err)
	if !has(res, "invoice", "manual", invUUID.String()) || !has(res, "bank_transaction", "manual", btUUID.String()) || len(res.Items) != 2 {
		t.Errorf("hand draft: %+v", res.Items)
	}
	res, err = svc.List(ctx, sub, "invoice", invUUID, all)
	must(err)
	if !has(res, "journal_draft", "manual", handUUID.String()) {
		t.Errorf("invoice → hand draft: %+v", res.Items)
	}
	if on, err := jsvc.SetManualLink(ctx, scope, handUUID, journals.LinkInvoice, invUUID, false); err != nil || on {
		t.Fatalf("unlink: %v %v", on, err)
	}
	res, err = svc.List(ctx, sub, "journal_draft", handUUID, all)
	must(err)
	if has(res, "invoice", "manual", "") || !has(res, "bank_transaction", "manual", btUUID.String()) {
		t.Errorf("after unlink: %+v", res.Items)
	}
	var lines int
	must(d.Pool.QueryRow(ctx, `SELECT count(*) FROM journal_draft_lines WHERE journal_draft_id = $1 AND source_kind IS NULL`, handID).Scan(&lines))
	if lines != 2 {
		t.Errorf("lines changed: %d without source", lines)
	}
	tpX := d.Taxpayer(t, "legal", "9200000025")
	engX := d.Engagement(t, org, tpX)
	if _, err := jsvc.SetManualLink(ctx, engagementctx.Scope{ID: engX, OrganizationID: org, TaxpayerID: tpX}, handUUID,
		journals.LinkInvoice, invUUID, true); !errors.Is(err, journals.ErrNotFound) {
		t.Errorf("other engagement: %v", err)
	}

	// Banka izni olmayan hareketi görmez.
	res, err = svc.List(ctx, sub, "invoice", invUUID, func(p string) bool { return p != rbac.PermTenantBankRead })
	must(err)
	if has(res, "bank_transaction", "payment", "") {
		t.Errorf("bank item without permission: %+v", res.Items)
	}

	// Başka mükellefin engagement'ı bu faturayı bulamaz.
	tp2 := d.Taxpayer(t, "legal", "9100000021")
	eng2 := d.Engagement(t, org, tp2)
	_, err = svc.List(ctx, Subject{OrganizationID: org, EngagementID: eng2, TaxpayerID: tp2}, "invoice", invUUID, all)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("other taxpayer: %v", err)
	}
	// Bilinmeyen kayıt.
	if _, err := svc.List(ctx, sub, "expense", uuid.New(), all); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown expense: %v", err)
	}
}
