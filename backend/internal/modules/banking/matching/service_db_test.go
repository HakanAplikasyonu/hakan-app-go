package matching

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownIBAN = "TR760001000519786457841326"

// TestSampleMonth is the #21 acceptance: on a sample month the matches and
// the warnings are the expected ones.
func TestSampleMonth(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	tp := d.Taxpayer(t, "legal", "9000000017")
	org := d.Organization(t)
	user := d.User(t)
	eng := d.Engagement(t, org, tp)

	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: tp, Type: "bank_account", Currency: "TRY",
		Iban: pgtype.Text{String: ownIBAN, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	// Invoices of October 2026.
	inv := func(number, direction, vkn, name, payable string, day int) int64 {
		var id int64
		err := d.Pool.QueryRow(ctx, `
			INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
			                      issue_date, period, currency, customer_tax_number, customer_name, payable_amount, tax_summary_source)
			VALUES ($1, 'manual_upload', $2::text, 'ubl_upload', $2::text, $3, 'SATIS', make_date(2026, 10, $6), '2026-10', 'TRY', $4, $5, $7::numeric, 'items')
			RETURNING id`, tp, number, direction, vkn, name, day, payable).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	inv101 := inv("ATL2026000000101", "outgoing", "9100000021", "Yıldız Market Ltd. Şti.", "12000", 3)
	inv102 := inv("ATL2026000000102", "outgoing", "9100000021", "Yıldız Market Ltd. Şti.", "3000", 5)
	inv103 := inv("ATL2026000000103", "outgoing", "9100000021", "Yıldız Market Ltd. Şti.", "2000", 6)
	invKrt := inv("KRT2026000001207", "incoming", "9200000035", "Kalem Kırtasiye", "15600", 7)

	bank := banking.NewService(d.Pool, nil)
	mv := func(id string, day int, dir banking.Direction, amount, desc, vkn, tckn string) banking.Transaction {
		tax := vkn
		if tckn != "" {
			tax = tckn
		}
		return banking.Transaction{SourceSystem: "bank_ziraat", SourceID: id, CaptureMethod: banking.CaptureAPI,
			TransactionDate: on(day), Direction: dir, Amount: amount, Currency: "TRY", Description: desc, CounterpartyTaxID: tax}
	}
	_, err = bank.Ingest(ctx, tp, box.ID, nil, []banking.Transaction{
		mv("a", 3, banking.DirIn, "12000", "FAST ATL2026000000101 TAHSILAT", "9100000021", ""), // auto
		mv("b", 25, banking.DirIn, "5000", "FAST TOPLU ODEME", "9100000021", ""),               // combination 3000 + 2000
		mv("c", 9, banking.DirOut, "15600", "EFT ODEME", "9200000035", ""),                     // 40 + 30 + 6 = 76 → suggest
		mv("d", 12, banking.DirOut, "7000", "EFT ODEME", "9300000049", ""),                     // no invoice → undocumented
		mv("e", 14, banking.DirIn, "900", "FAST GELEN", "9500000066", ""),                      // no invoice → uninvoiced
		mv("f", 31, banking.DirIn, "150.25", "MEVDUAT FAIZI", "", ""),
		mv("g", 31, banking.DirOut, "22.54", "STOPAJ", "", ""),
		mv("h", 15, banking.DirOut, "1500", "SGK PRIM ODEMESI", "", ""),
		mv("i", 1, banking.DirOut, "22000", "EKIM KIRASI", "", "10000000146"),
		mv("j", 20, banking.DirOut, "50", "IDDAA", "", ""),
		mv("k", 21, banking.DirOut, "75", "XQZ", "", ""),
	})
	if err != nil {
		t.Fatal(err)
	}

	review := reviewusecase.New(reviewrepository.New(d.Pool, q)).Register(SuggestionKind, Applier{})
	warn := engine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
	svc := NewService(d.Pool, review, warn, Options{FeeTolerance: d25})
	sub := Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}

	res, err := svc.Run(ctx, sub, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// a, b, c, d, e (transfers) and i (rent) are matchable: 1 auto, 2 suggested, 3 none.
	if res.Examined != 6 || res.Auto != 1 || res.Suggested != 2 || res.Unmatched != 3 {
		t.Fatalf("run: %+v", res)
	}

	matches := func(srcID string) map[int64]string {
		out := map[int64]string{}
		rows, err := d.Pool.Query(ctx, `SELECT m.target_id, m.method FROM bank_transaction_matches m
			JOIN bank_transactions b ON b.id = m.bank_transaction_id WHERE b.source_id = $1 AND b.taxpayer_id = $2`, srcID, tp)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var method string
			_ = rows.Scan(&id, &method)
			out[id] = method
		}
		return out
	}
	if m := matches("a"); len(m) != 1 || m[inv101] != "auto" {
		t.Fatalf("a: %v", m)
	}

	// Suggestions in the review queue, below the auto threshold, pending.
	type sug struct {
		uuid   uuid.UUID
		target int64
		conf   string
	}
	var sugs []sug
	rows, err := d.Pool.Query(ctx, `SELECT uuid, target_id, confidence::text FROM suggestions
		WHERE engagement_id = $1 AND kind = 'bank_match' AND status = 'pending' ORDER BY target_id`, eng)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s sug
		_ = rows.Scan(&s.uuid, &s.target, &s.conf)
		sugs = append(sugs, s)
	}
	rows.Close()
	if len(sugs) != 2 {
		t.Fatalf("suggestions: %+v", sugs)
	}

	// Accept both: the combination closes 102 + 103, the purchase closes KRT.
	rsub := reviewusecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	for _, s := range sugs {
		if _, err := review.Accept(ctx, rsub, s.uuid, user, "kontrol edildi"); err != nil {
			t.Fatal(err)
		}
	}
	if m := matches("b"); len(m) != 2 || m[inv102] != "suggested_accepted" || m[inv103] != "suggested_accepted" {
		t.Fatalf("b: %v", m)
	}
	if m := matches("c"); len(m) != 1 || m[invKrt] != "suggested_accepted" {
		t.Fatalf("c: %v", m)
	}

	// Warnings of the period, after a second run (the accepted matches count).
	if _, err := svc.Run(ctx, sub, "2026-10"); err != nil {
		t.Fatal(err)
	}
	var codes []string
	wrows, err := d.Pool.Query(ctx, `SELECT code FROM data_warnings WHERE engagement_id = $1 AND status = 'open' ORDER BY code`, eng)
	if err != nil {
		t.Fatal(err)
	}
	for wrows.Next() {
		var c string
		_ = wrows.Scan(&c)
		codes = append(codes, c)
	}
	wrows.Close()
	want := []string{CodeInterest, CodeNonBusiness, CodeRentWithholding, CodeTaxPaid, CodeUnclassified, CodeUndocumentedPayment, CodeUninvoicedCollection}
	sort.Strings(want)
	if len(codes) != len(want) {
		t.Fatalf("warnings %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("warnings %v, want %v", codes, want)
		}
	}
	var params string
	if err := d.Pool.QueryRow(ctx, `SELECT params->>'total' FROM data_warnings WHERE engagement_id = $1 AND code = $2 AND status = 'open'`,
		eng, CodeUninvoicedCollection).Scan(&params); err != nil || params != "900.00" {
		t.Fatalf("uninvoiced total %q %v (the accepted combination must not count)", params, err)
	}

	// Reverting the KRT match brings the movement back to "suggested".
	var krtSug uuid.UUID
	for _, s := range sugs {
		var src string
		_ = d.Pool.QueryRow(ctx, `SELECT source_id FROM bank_transactions WHERE id = $1`, s.target).Scan(&src)
		if src == "c" {
			krtSug = s.uuid
		}
	}
	if _, err := review.Revert(ctx, rsub, krtSug, user, "yanlış fatura"); err != nil {
		t.Fatal(err)
	}
	if m := matches("c"); len(m) != 0 {
		t.Fatalf("revert left matches: %v", m)
	}

	// One movement by hand (#528): the undocumented payment "d" gets its
	// invoice later; matching it resolves the warning, undoing reopens it.
	txUUID := func(src string) uuid.UUID {
		var id uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM bank_transactions WHERE source_id = $1 AND taxpayer_id = $2`, src, tp).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	invUUID := func(id int64) uuid.UUID {
		var u uuid.UUID
		_ = d.Pool.QueryRow(ctx, `SELECT uuid FROM invoices WHERE id = $1`, id).Scan(&u)
		return u
	}
	late := invUUID(inv("XYZ2026000000500", "incoming", "9300000049", "Geç Tedarikçi", "7000", 11))
	// "c" was reverted above: the accountant matches it by hand to KRT.
	undocumented := func() string {
		var total string
		_ = d.Pool.QueryRow(ctx, `SELECT string_agg(params->>'total', ',') FROM data_warnings WHERE engagement_id = $1 AND code = $2 AND status = 'open'`,
			eng, CodeUndocumentedPayment).Scan(&total)
		return total
	}
	// A rerun may have queued a new suggestion for "c"; the manual match
	// supersedes it so it leaves the review queue (#529).
	var cID int64
	_ = d.Pool.QueryRow(ctx, `SELECT id FROM bank_transactions WHERE source_id = 'c' AND taxpayer_id = $1`, tp).Scan(&cID)
	pending, err := review.Submit(ctx, rsub, reviewusecase.Input{
		Kind: SuggestionKind, TargetType: "bank_transaction", TargetID: &cID, TargetField: "match",
		Value: []byte(`{}`), Confidence: decimal.RequireFromString("0.5"), Rationale: "yeniden önerildi",
		Provenance: reviewusecase.Provenance{Origin: reviewusecase.OriginRule, RuleRef: "bank_match:21-6.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view, err := svc.Match(ctx, sub, txUUID("c"), []uuid.UUID{invUUID(invKrt)}, user); err != nil || view.MatchStatus != "matched" {
		t.Fatalf("match c: %+v %v", view, err)
	}
	var pendingStatus string
	_ = d.Pool.QueryRow(ctx, `SELECT status FROM suggestions WHERE uuid = $1`, pending.UUID).Scan(&pendingStatus)
	if pendingStatus != "superseded" {
		t.Fatalf("manual match must supersede the pending suggestion, status %q", pendingStatus)
	}
	if got := undocumented(); got != "7000.00" {
		t.Fatalf("undocumented before matching d = %q", got)
	}
	view, err := svc.Transaction(ctx, sub, txUUID("d"))
	if err != nil || !view.Matchable || len(view.Matches) != 0 || len(view.Candidates) == 0 || view.Candidates[0].InvoiceUUID != late {
		t.Fatalf("view d: %+v %v", view, err)
	}
	if _, err := svc.Match(ctx, sub, txUUID("d"), []uuid.UUID{invUUID(inv101)}, user); !errors.Is(err, ErrInvalidInvoices) {
		t.Fatalf("sales invoice on a payment: %v", err)
	}
	view, err = svc.Match(ctx, sub, txUUID("d"), []uuid.UUID{late}, user)
	if err != nil || view.MatchStatus != "matched" || len(view.Matches) != 1 || view.Matches[0].Method != "manual" || view.Matches[0].Amount != "7000.00" {
		t.Fatalf("match d: %+v %v", view, err)
	}
	if got := undocumented(); got != "" {
		t.Fatalf("manual match must resolve the undocumented payment warning: %q", got)
	}
	view, err = svc.Unmatch(ctx, sub, txUUID("d"))
	if err != nil || view.MatchStatus != "unmatched" || len(view.Matches) != 0 || undocumented() != "7000.00" {
		t.Fatalf("unmatch d: %+v %v total=%q", view, err, undocumented())
	}
	// Interest is neither a collection nor a payment of an invoice.
	if view, err := svc.Transaction(ctx, sub, txUUID("f")); err != nil || view.Matchable || len(view.Candidates) != 0 {
		t.Fatalf("view f: %+v %v", view, err)
	}
	if _, err := svc.Match(ctx, sub, txUUID("f"), []uuid.UUID{late}, user); !errors.Is(err, ErrNotMatchable) {
		t.Fatalf("match f: %v", err)
	}
	if _, err := svc.Transaction(ctx, sub, uuid.New()); !errors.Is(err, ErrTransactionNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

var d25 = d("25")
