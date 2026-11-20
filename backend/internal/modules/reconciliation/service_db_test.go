package reconciliation

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #389: Eylül ve Ekim mizanı → Ekim hareketi ile kaynakların karşılaştırması,
// fişlenmemiş banka hareketi açıklaması, banka bakiyesi, karar taşıma,
// inceleme kuyruğu ve engagement yalıtımı.
func TestReconciliationRunAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tag := time.Now().Format("150405.000000")
	var org, tp, eng, user, otherEng, otherTp int64
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'M') RETURNING id`, "mt-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '4730074556', 'Atlas') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '1234567890', 'Başka') RETURNING id`).Scan(&otherTp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, otherTp).Scan(&otherEng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))

	mizan := func(period, sha string, lines [][3]string) {
		var imp int64
		must(pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256, period, file_name)
			VALUES ($1, $2, 'xlsx', 'trial_balance', repeat($3, 64), $4, $5) RETURNING id`, org, eng, sha, period, "mizan-"+period+".xlsx").Scan(&imp))
		_, err := pool.Exec(ctx, `INSERT INTO trial_balance_lines (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf, debit, credit, balance)
			VALUES ($1, $2, $3, '102', 'BANKALAR', '102', 1, false, 0, 0, 0)`, org, eng, imp)
		must(err)
		for _, l := range lines {
			_, err := pool.Exec(ctx, `INSERT INTO trial_balance_lines (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf, debit, credit, balance)
				VALUES ($1, $2, $3, $4::text, $4::text, left($4::text, 3), 2, true, $5::numeric, $6::numeric, $5::numeric - $6::numeric)`, org, eng, imp, l[0], l[1], l[2])
			must(err)
		}
	}
	mizan("2026-09", "a", [][3]string{{"102.01", "10000", "2000"}, {"600.01", "0", "5000"}, {"770.01", "1000", "0"}})
	mizan("2026-10", "b", [][3]string{{"102.01", "14000", "2500"}, {"600.01", "0", "9000"}, {"770.01", "1345", "0"}, {"335.01", "0", "800"}})

	var box int64
	must(pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Banka TL', 'TRY') RETURNING id`, tp).Scan(&box))
	bank := func(src, day, dc, amount string, balance any) int64 {
		var id int64
		must(pool.QueryRow(ctx, `
			INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
				booked_at, transaction_date, debit_credit, amount, currency, description, period, counterparty_name, balance_after)
			VALUES ($1, $2, 'bank_test', $3, 'api', $4::date, $4::date, $5, $6, 'TRY', 'açıklama', '2026-10', 'ACME LTD', $7)
			RETURNING id`, tp, box, src, day, dc, amount, balance).Scan(&id))
		return id
	}
	in1 := bank("t1", "2026-10-03", "A", "3000", nil)
	out1 := bank("t2", "2026-10-04", "B", "500", nil)
	bank("t3", "2026-10-20", "A", "1000", "11500")

	var draft int64
	must(pool.QueryRow(ctx, `INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, status, approved_at)
		VALUES ($1, $2, $3, '2026-10', 1, '2026-10-31', 'exported', NOW()) RETURNING id`, org, eng, tp).Scan(&draft))
	line := func(no int, acc, debit, credit, kind, src string) {
		_, err := pool.Exec(ctx, `INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, debit, credit, source_kind, source_id)
			VALUES ($1, $2, $3, $4, $5, $6::numeric, $7::numeric, NULLIF($8, ''), NULLIF($9, ''))`, org, eng, draft, no, acc, debit, credit, kind, src)
		must(err)
	}
	line(1, "102.01", "3000", "0", "bank_transaction", itoa(in1))
	line(2, "102.01", "0", "500", "bank_transaction", itoa(out1))
	line(3, "120.01", "4000", "0", "invoice", "999")
	line(4, "600.01", "0", "4000", "invoice", "999")
	// Eylül fişi pencereye girmez.
	var sep int64
	must(pool.QueryRow(ctx, `INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date)
		VALUES ($1, $2, $3, '2026-09', 1, '2026-09-30') RETURNING id`, org, eng, tp).Scan(&sep))
	_, err := pool.Exec(ctx, `INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, debit, credit)
		VALUES ($1, $2, $3, 1, '770.01', 99, 0)`, org, eng, sep)
	must(err)

	reviewSvc := reviewusecase.New(reviewrepository.New(pool, db.New(pool))).Register(SuggestionKind, Applier{})
	svc := New(database.Ambient{Pool: pool}, reviewSvc)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	if _, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-08"}); !errors.Is(err, ErrNoTrialBalance) {
		t.Fatalf("no trial balance: %v", err)
	}
	if _, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-13"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad period: %v", err)
	}
	det, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-10"})
	must(err)
	run := det.Run
	if run.WindowFrom != "2026-10" || run.WindowTo != "2026-10" || run.Summary.Cumulative || run.TrialBalance == nil || run.TrialBalance.FileName != "mizan-2026-10.xlsx" {
		t.Fatalf("run %+v", run)
	}
	if b := run.Summary.Bank; b == nil || !b.Matched || b.LedgerBalance != "11500.00" || b.StatementBalance != "11500.00" {
		t.Fatalf("bank %+v", run.Summary.Bank)
	}
	if run.Summary.UnpostedCount != 1 || run.Summary.Unposted[KindBank].Amount != "1000.00" {
		t.Fatalf("unposted %+v", run.Summary)
	}
	items := map[string]ItemView{}
	for _, it := range det.Items {
		items[it.MainCode] = it
	}
	want := map[string]string{"102": ResultDifference, "120": ResultMissingInLedger, "600": ResultMatched, "770": ResultMissingInSources, "335": ResultMissingInSources}
	for code, res := range want {
		if items[code].Result != res {
			t.Errorf("%s result %q, want %q (%+v)", code, items[code].Result, res, items[code])
		}
	}
	if len(det.Items) != len(want) {
		t.Fatalf("items %d: %+v", len(det.Items), det.Items)
	}
	b := items["102"]
	if b.Difference != "1000.00" || *b.SuggestionCode != SuggestUnposted || len(b.Unposted) != 1 || b.Unposted[0].Label != "ACME LTD" || b.Status != StatusOpen {
		t.Fatalf("102 %+v", b)
	}
	if items["600"].Status != StatusResolved || items["770"].LedgerNet != "345.00" {
		t.Fatalf("600 / 770 %+v %+v", items["600"], items["770"])
	}

	// Karar: açıklama notsuz olmaz; notla olur ve aynı farkla yeniden
	// çalıştırmada korunur.
	if _, err := svc.Decide(ctx, sc, items["770"].UUID, user, DecisionInput{Status: StatusExplained}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("explained without note: %v", err)
	}
	note := "  Kırtasiye masrafı Luca'ya elle girildi  "
	v, err := svc.Decide(ctx, sc, items["770"].UUID, user, DecisionInput{Status: StatusExplained, Note: &note})
	must(err)
	if v.Status != StatusExplained || *v.Note != strings.TrimSpace(note) || v.DecidedAt == nil {
		t.Fatalf("decided %+v", v)
	}
	if _, err := svc.SendToReview(ctx, sc, items["600"].UUID, user, nil); !errors.Is(err, ErrMatched) {
		t.Fatalf("matched to review: %v", err)
	}

	// İnceleme kuyruğu: gönder → bekliyor → kabul → çözüldü, notu öneri metni.
	v, err = svc.SendToReview(ctx, sc, b.UUID, user, nil)
	must(err)
	if v.Status != StatusInReview || v.Review == nil || v.Review.Status != string(reviewusecase.StatusPending) {
		t.Fatalf("in review %+v", v)
	}
	if _, err := svc.Decide(ctx, sc, b.UUID, user, DecisionInput{Status: StatusResolved}); !errors.Is(err, ErrInReview) {
		t.Fatalf("decide while in review: %v", err)
	}
	if _, err := svc.SendToReview(ctx, sc, b.UUID, user, nil); !errors.Is(err, ErrInReview) {
		t.Fatalf("send twice: %v", err)
	}
	sub := reviewusecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	sug, _, err := reviewSvc.Get(ctx, sub, v.Review.UUID)
	must(err)
	if sug.Input.Kind != SuggestionKind || sug.Input.Period != "2026-10" || !strings.Contains(sug.Input.Rationale, "102") || sug.Input.Confidence.GreaterThan(decimal.NewFromFloat(0.5)) {
		t.Fatalf("suggestion %+v", sug.Input)
	}
	if _, err := reviewSvc.Accept(ctx, sub, v.Review.UUID, user, "tamam"); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Detail(ctx, sc, run.UUID.String(), "")
	must(err)
	for _, it := range again.Items {
		if it.MainCode == "102" && (it.Status != StatusResolved || it.Note == nil || !strings.Contains(*it.Note, "fişlenmemiş")) {
			t.Fatalf("accepted %+v", it)
		}
	}
	if _, err := reviewSvc.Revert(ctx, sub, v.Review.UUID, user, "geri"); err != nil {
		t.Fatal(err)
	}
	again, err = svc.Detail(ctx, sc, "latest", "2026-10")
	must(err)
	for _, it := range again.Items {
		if it.MainCode == "102" && it.Status != StatusOpen {
			t.Fatalf("reverted %+v", it)
		}
	}

	// Yeniden çalıştırma: 770'in kararı ve notu taşınır.
	det2, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-10", Tolerance: ptr("0,50")})
	must(err)
	if det2.Run.UUID == run.UUID || det2.Run.Tolerance != "0.50" {
		t.Fatalf("second run %+v", det2.Run)
	}
	for _, it := range det2.Items {
		if it.MainCode == "770" && (it.Status != StatusExplained || it.Note == nil) {
			t.Fatalf("carried %+v", it)
		}
	}
	runs, err := svc.Runs(ctx, sc, "")
	must(err)
	if len(runs) != 2 || runs[0].UUID != det2.Run.UUID {
		t.Fatalf("runs %+v", runs)
	}

	// Eylül: önceki mizan yok → yıl başından birikimli, açılış açıklaması.
	sepRun, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-09"})
	must(err)
	if !sepRun.Run.Summary.Cumulative || sepRun.Run.WindowFrom != "2026-01" {
		t.Fatalf("september %+v", sepRun.Run)
	}

	// Dönem başına son koşu tek sorguda (#639): karşılaştırılmamış ay nil.
	byPeriod, err := svc.LatestByPeriod(ctx, sc, "2026-08", "2026-10")
	must(err)
	if len(byPeriod) != 3 || byPeriod["2026-08"] != nil || byPeriod["2026-09"] == nil || byPeriod["2026-09"].UUID != sepRun.Run.UUID ||
		byPeriod["2026-10"] == nil || byPeriod["2026-10"].UUID != det2.Run.UUID {
		t.Fatalf("latest by period %+v", byPeriod)
	}

	// Koşusu olmayan dönem: boş durum ErrNoRun (handler 200 + run null döner, #639).
	if _, err := svc.Detail(ctx, sc, "latest", "2025-01"); !errors.Is(err, ErrNoRun) || !errors.Is(err, ErrNotFound) {
		t.Fatalf("no run: %v", err)
	}
	if _, err := svc.Counterparties(ctx, sc, "latest", "2025-01"); !errors.Is(err, ErrNoRun) {
		t.Fatalf("no run counterparties: %v", err)
	}

	// Başka engagement görmez.
	osc := engagementctx.Scope{ID: otherEng, OrganizationID: org, TaxpayerID: otherTp}
	if other, err := svc.LatestByPeriod(ctx, osc, "2026-08", "2026-10"); err != nil || other["2026-10"] != nil {
		t.Fatalf("other engagement latest by period: %v %+v", err, other)
	}
	if _, err := svc.Detail(ctx, osc, run.UUID.String(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other engagement: %v", err)
	}
	if _, err := svc.Decide(ctx, osc, items["770"].UUID, user, DecisionInput{Status: StatusIgnored}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other engagement decide: %v", err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func ptr(s string) *string { return &s }
