package coverage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Test IBANs (valid check digits, no real account).
const (
	ibanAkbank = "TR870004600099888000012345"
	ibanZiraat = "TR760001000519786457841326"
)

type world struct {
	d     *testdb.DB
	q     *db.Queries
	svc   *Service
	scope engagementctx.Scope
	now   time.Time
}

func setup(t *testing.T) world {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "9000000017")
	eng := d.Engagement(t, org, tp)
	var engUUID uuid.UUID
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, eng).Scan(&engUUID); err != nil {
		t.Fatal(err)
	}
	q := db.New(d.Pool)
	// The accounts are added "now" (created_at is the database clock), so
	// the service runs on the real clock too.
	now := time.Now()
	docs := doctasks.New(d.Pool, nil, nil, doctasks.Config{}, nil)
	return world{
		d: d, q: q, svc: New(q, docs).WithClock(func() time.Time { return now }), now: now,
		scope: engagementctx.Scope{ID: eng, UUID: engUUID, OrganizationID: org, TaxpayerID: tp},
	}
}

func (w world) box(t *testing.T, name, bank, iban, currency string) db.CashBox {
	t.Helper()
	p := db.InsertCashBoxParams{TaxpayerID: w.scope.TaxpayerID, Type: "bank_account", Name: name, Currency: currency,
		BankID: pgtype.Text{String: bank, Valid: true}}
	if iban != "" {
		p.Iban = pgtype.Text{String: iban, Valid: true}
	}
	b, err := w.q.InsertCashBox(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (w world) movement(t *testing.T, box db.CashBox, period string) {
	t.Helper()
	_, err := w.d.Pool.Exec(context.Background(), `INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id,
			capture_method, booked_at, transaction_date, debit_credit, amount, currency, description, period)
		VALUES ($1, $2, 'bank_test', $3, 'api', $4::date, $4::date, 'A', 100, $5, 'tahsilat', $6)`,
		w.scope.TaxpayerID, box.ID, box.Uuid.String()+period, period+"-10", box.Currency, period)
	if err != nil {
		t.Fatal(err)
	}
}

func (w world) setting(t *testing.T, on bool) {
	t.Helper()
	if _, err := w.d.Pool.Exec(context.Background(),
		`UPDATE organizations SET settings = COALESCE(settings, '{}'::jsonb) || jsonb_build_object('bank_statement_tracking', $2::bool) WHERE id = $1`,
		w.scope.OrganizationID, on); err != nil {
		t.Fatal(err)
	}
}

func months(a AccountStatus) map[string]MonthStatus {
	out := map[string]MonthStatus{}
	for _, m := range a.Months {
		out[m.Period] = m
	}
	return out
}

func find(t *testing.T, st Status, id uuid.UUID) AccountStatus {
	t.Helper()
	for _, a := range st.Items {
		if a.CashBoxUUID == id {
			return a
		}
	}
	t.Fatalf("account %s not in the status", id)
	return AccountStatus{}
}

func TestPeriodsAndTitle(t *testing.T) {
	p := Periods(time.Date(2026, 11, 23, 10, 0, 0, 0, istanbul))
	if len(p) != WindowMonths || p[0] != "2026-05" || p[5] != "2026-10" {
		t.Fatalf("periods = %v", p)
	}
	if p := Periods(time.Date(2026, 1, 1, 0, 30, 0, 0, istanbul)); p[5] != "2025-12" {
		t.Fatalf("january: %v", p)
	}
	cases := []struct{ got, want string }{
		{Title("akbank", "TRY", "", "Akbank TL", "2026-10", false), "Akbank TRY hesabı – Ekim 2026 ekstresi"},
		{Title("kuveyt_turk", "USD", ibanAkbank, "", "2026-09", true), "Kuveyt Türk USD hesabı (···2345) – Eylül 2026 ekstresi"},
		{Title("akbank", "TRY", "", "Maaş hesabı", "2026-10", true), "Akbank TRY hesabı (Maaş hesabı) – Ekim 2026 ekstresi"},
		{Title("", "EUR", "", "Kasa dışı", "2026-10", false), "Kasa dışı EUR hesabı – Ekim 2026 ekstresi"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("title %q, want %q", c.got, c.want)
		}
	}
}

// Monthly status: only closed months count, an account is expected from the
// month before it was added (or its first movement), a closed account is
// left out, and "geldi, hareket yok" turns a missing month received.
func TestMonthlyStatus(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	periods := Periods(w.now)
	last, prev, old := periods[5], periods[4], periods[1]

	akbank := w.box(t, "Akbank TL", "akbank", ibanAkbank, "TRY")
	w.movement(t, akbank, old)
	w.movement(t, akbank, prev)
	w.movement(t, akbank, last)
	ziraat := w.box(t, "Ziraat TL", "ziraat", "", "TRY")
	w.movement(t, ziraat, prev)
	fresh := w.box(t, "Kuveyt Dolar", "kuveyt_turk", "", "USD")
	closed := w.box(t, "Garanti TL", "garanti", "", "TRY")
	if _, err := w.d.Pool.Exec(ctx, `UPDATE cash_boxes SET deactivated_at = NOW() WHERE id = $1`, closed.ID); err != nil {
		t.Fatal(err)
	}
	// The current month is never expected, even with a movement.
	cur := w.now.In(istanbul).Format("2006-01")
	w.movement(t, ziraat, cur)

	st, err := w.svc.Status(ctx, w.scope)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || len(st.Periods) != WindowMonths || st.Periods[5] != last || len(st.Items) != 3 {
		t.Fatalf("status: %+v", st)
	}
	for _, a := range st.Items {
		if a.CashBoxUUID == closed.Uuid {
			t.Fatal("closed account listed")
		}
		for _, m := range a.Months {
			if m.Period == cur {
				t.Fatalf("current month in the status: %+v", m)
			}
		}
	}
	a := months(find(t, st, akbank.Uuid))
	if a[old].Status != StatusReceived || a[last].Status != StatusReceived || a[periods[0]].Status != StatusNotExpected ||
		a[periods[2]].Status != StatusMissing {
		t.Fatalf("akbank: %+v", a)
	}
	z := months(find(t, st, ziraat.Uuid))
	if z[prev].Status != StatusReceived || z[last].Status != StatusMissing || z[periods[3]].Status != StatusNotExpected {
		t.Fatalf("ziraat: %+v", z)
	}
	if z[last].Title != Title("ziraat", "TRY", "", "Ziraat TL", last, false) || z[last].Requested {
		t.Fatalf("ziraat title: %+v", z[last])
	}
	f := find(t, st, fresh.Uuid)
	if fm := months(f); fm[last].Status != StatusMissing || fm[prev].Status != StatusNotExpected || f.Missing != 1 {
		t.Fatalf("new account: %+v", f)
	}
	// Akbank misses periods[2] and [3] (expected since its first movement),
	// Ziraat last month, the new Kuveyt account last month.
	if st.Missing != 4 {
		t.Fatalf("missing = %d", st.Missing)
	}

	// Geldi, hareket yok.
	if err := w.svc.Mark(ctx, w.scope, 0, MarkInput{CashBoxUUID: ziraat.Uuid, Period: last, Received: true}); err != nil {
		t.Fatal(err)
	}
	st, _ = w.svc.Status(ctx, w.scope)
	if m := months(find(t, st, ziraat.Uuid))[last]; m.Status != StatusReceived || !m.Marked {
		t.Fatalf("marked: %+v", m)
	}
	if err := w.svc.Mark(ctx, w.scope, 0, MarkInput{CashBoxUUID: ziraat.Uuid, Period: last, Received: false}); err != nil {
		t.Fatal(err)
	}
	st, _ = w.svc.Status(ctx, w.scope)
	if m := months(find(t, st, ziraat.Uuid))[last]; m.Status != StatusMissing {
		t.Fatalf("unmarked: %+v", m)
	}
	if err := w.svc.Mark(ctx, w.scope, 0, MarkInput{CashBoxUUID: ziraat.Uuid, Period: cur, Received: true}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("current month mark: %v", err)
	}
	if err := w.svc.Mark(ctx, w.scope, 0, MarkInput{CashBoxUUID: uuid.New(), Period: last, Received: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account mark: %v", err)
	}

	// Office setting off: no status, no requests.
	w.setting(t, false)
	if st, err := w.svc.Status(ctx, w.scope); err != nil || st.Enabled || len(st.Items) != 0 {
		t.Fatalf("off: %+v %v", st, err)
	}
	if _, err := w.svc.Request(ctx, w.scope, 0, RequestInput{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("off request: %v", err)
	}
	if l, err := w.svc.Gaps(ctx, w.scope.OrganizationID, true, 0, 20, 0); err != nil || l.Enabled || len(l.Items) != 0 {
		t.Fatalf("off gaps: %+v %v", l, err)
	}
}

// One click asks for a missing statement with the ready-made title; asking
// again opens nothing new; the bulk request asks for the rest.
func TestRequestMissingStatement(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	periods := Periods(w.now)
	last, prev := periods[5], periods[4]
	akbank := w.box(t, "Akbank TL", "akbank", "", "TRY")
	w.movement(t, akbank, prev)
	kuveyt := w.box(t, "Kuveyt Dolar", "kuveyt_turk", "", "USD")
	w.movement(t, kuveyt, last)

	want := "Akbank TRY hesabı – " + missingdocs.PeriodText(last) + " ekstresi"
	two := 2
	res, err := w.svc.Request(ctx, w.scope, 0, RequestInput{
		Items: []RequestItem{{CashBoxUUID: akbank.Uuid, Period: last}}, DueDays: 7, RemindDaysBefore: &two, Channels: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 1 || res.Created[0].Title != want || res.Created[0].Period != last ||
		res.Created[0].Kind != doctasks.KindDocumentRequest {
		t.Fatalf("created: %+v", res)
	}
	st, _ := w.svc.Status(ctx, w.scope)
	if m := months(find(t, st, akbank.Uuid))[last]; !m.Requested || m.Title != want {
		t.Fatalf("requested: %+v", m)
	}
	// Again: the open request is returned, not a second one.
	res, err = w.svc.Request(ctx, w.scope, 0, RequestInput{Items: []RequestItem{{CashBoxUUID: akbank.Uuid, Period: last}}})
	if err != nil || len(res.Created) != 0 || len(res.Existing) != 1 || res.Existing[0].Title != want {
		t.Fatalf("again: %+v %v", res, err)
	}
	// A month whose statement arrived is not asked for.
	if _, err := w.svc.Request(ctx, w.scope, 0, RequestInput{Items: []RequestItem{{CashBoxUUID: kuveyt.Uuid, Period: last}}}); !errors.Is(err, ErrNotMissing) {
		t.Fatalf("received month: %v", err)
	}
	// Bulk: every missing month of the taxpayer (Kuveyt has none: its first
	// movement is last month and it was added this month).
	res, err = w.svc.Request(ctx, w.scope, 0, RequestInput{})
	if err != nil || len(res.Created) != 0 || len(res.Existing) != 1 {
		t.Fatalf("bulk: %+v %v", res, err)
	}
	var n int
	if err := w.d.Pool.QueryRow(ctx, `SELECT count(*) FROM document_tasks WHERE engagement_id = $1 AND title = $2 AND status = 'open'`,
		w.scope.ID, want).Scan(&n); err != nil || n != 1 {
		t.Fatalf("document tasks = %d %v", n, err)
	}
}

// The office to-do list pages: one row per taxpayer + account + month.
func TestGapsBounded(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	periods := Periods(w.now)
	for i, bank := range []string{"akbank", "ziraat", "kuveyt_turk"} {
		b := w.box(t, bank, bank, "", "TRY")
		w.movement(t, b, periods[i]) // expected from periods[i] on
	}
	all, err := w.svc.Gaps(ctx, w.scope.OrganizationID, true, 0, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	// akbank misses 5 months, ziraat 4, kuveyt 3.
	if !all.Enabled || all.Total != 12 || len(all.Items) != 12 {
		t.Fatalf("all: total %d, %d items", all.Total, len(all.Items))
	}
	for _, g := range all.Items {
		if g.EngagementUUID != w.scope.UUID || g.TaxpayerMissing != 12 || g.Title == "" || g.Requested {
			t.Fatalf("row: %+v", g)
		}
	}
	page, err := w.svc.Gaps(ctx, w.scope.OrganizationID, true, 0, 5, 10)
	if err != nil || page.Total != 12 || len(page.Items) != 2 {
		t.Fatalf("page: %+v %v", page, err)
	}
	if beyond, err := w.svc.Gaps(ctx, w.scope.OrganizationID, true, 0, 5, 50); err != nil || len(beyond.Items) != 0 {
		t.Fatalf("beyond: %+v %v", beyond, err)
	}
	// A member who is not assigned sees nothing.
	if mine, err := w.svc.Gaps(ctx, w.scope.OrganizationID, false, w.d.User(t), 20, 0); err != nil || len(mine.Items) != 0 {
		t.Fatalf("not assigned: %+v %v", mine, err)
	}
}

// Kapatılan (pasife alınan) hesap: kapatıldığı aya kadarki ekstreleri
// beklenir, sonrası beklenmez. Hiç hareketi olmayan ya da pencereden önce
// kapatılan pasif hesap listelenmez. Değerler uydurmadır.
func TestClosedAccountExpectedUntilClosing(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	periods := Periods(w.now)
	closeAt := func(b db.CashBox, period string) {
		t.Helper()
		if _, err := w.d.Pool.Exec(ctx, `UPDATE cash_boxes SET deactivated_at = ($2 || '-15 12:00:00+03')::timestamptz WHERE id = $1`,
			b.ID, period); err != nil {
			t.Fatal(err)
		}
	}
	closed := w.box(t, "Garanti TL", "garanti", "", "TRY")
	w.movement(t, closed, periods[1])
	closeAt(closed, periods[3])
	early := w.box(t, "Eski Halkbank", "halkbank", "", "TRY")
	w.movement(t, early, "2020-01")
	closeAt(early, "2020-02")
	empty := w.box(t, "Yanlış hesap", "teb", "", "TRY")
	closeAt(empty, periods[4])

	st, err := w.svc.Status(ctx, w.scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Items) != 1 || st.Items[0].CashBoxUUID != closed.Uuid {
		t.Fatalf("items: %+v", st.Items)
	}
	m := months(st.Items[0])
	want := []string{StatusNotExpected, StatusReceived, StatusMissing, StatusMissing, StatusNotExpected, StatusNotExpected}
	for i, p := range periods {
		if m[p].Status != want[i] {
			t.Fatalf("%s: %s, want %s (%+v)", p, m[p].Status, want[i], m)
		}
	}
	if st.Missing != 2 {
		t.Fatalf("missing = %d", st.Missing)
	}
	gaps, err := w.svc.Gaps(ctx, w.scope.OrganizationID, true, 0, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if gaps.Total != 2 || len(gaps.Items) != 2 || gaps.Items[0].Period != periods[3] || gaps.Items[1].Period != periods[2] {
		t.Fatalf("gaps: %+v", gaps.Items)
	}
	// Kapanıştan sonraki ay istenemez.
	_, err = w.svc.Request(ctx, w.scope, 0, RequestInput{Items: []RequestItem{{CashBoxUUID: closed.Uuid, Period: periods[4]}}})
	if !errors.Is(err, ErrNotMissing) {
		t.Fatalf("after closing: %v", err)
	}
}
