//go:build uyumsoftlive

// Live checks against the Uyumsoft test system (33 §13, 30 §9). Run with
//
//	UYUMSOFT_TEST_USERNAME=... UYUMSOFT_TEST_PASSWORD=... TEST_DATABASE_URL=... \
//	  go test -tags uyumsoftlive -run Uyumsoft ./internal/modules/entegrator/
//
// Credentials come only from the environment.
package entegrator

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func uyumsoftConfig(t *testing.T) provider.Config {
	t.Helper()
	u, p := os.Getenv("UYUMSOFT_TEST_USERNAME"), os.Getenv("UYUMSOFT_TEST_PASSWORD")
	if u == "" || p == "" {
		t.Skip("UYUMSOFT_TEST_USERNAME / UYUMSOFT_TEST_PASSWORD not set")
	}
	return provider.Config{Username: u, Password: p, Test: true}
}

// unread lists the inbox documents Uyumsoft still reports as not read
// (OnlyNewestInvoices=true). Test-only: production code cannot ask this.
func uyumsoftUnread(t *testing.T, cfg provider.Config, from, to time.Time) []string {
	t.Helper()
	p, err := provider.Open("uyumsoft", cfg)
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.(provider.InboxLister).ListInbox(context.Background(), provider.ListQuery{Kind: provider.EInvoice, From: from, To: to, OnlyNew: true, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range page.Items {
		out = append(out, it.UUID+"/"+string(it.Status))
	}
	sort.Strings(out)
	return out
}

func TestUyumsoftLiveSync(t *testing.T) {
	cfg := uyumsoftConfig(t)
	ctx := context.Background()
	c, err := Open("uyumsoft", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// 30 §9/1: bağlantı ve tek gelen fatura listeleme.
	probe, err := c.List(ctx, provider.Inbound, ListQuery{Kind: provider.EInvoice, From: time.Now().AddDate(0, -1, 0), To: time.Now(), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.Items) == 0 {
		t.Fatal("no incoming invoice to anchor the test")
	}
	// Neither list carries the account's own VKN: read it from the UBL of
	// an incoming invoice (the party that is not the sender).
	first := probe.Items[0]
	ubl, err := c.Fetch(ctx, provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: first.UUID, Format: provider.FormatUBL})
	if err != nil {
		t.Fatal(err)
	}
	own := ""
	for _, m := range regexp.MustCompile(`schemeID="(?:VKN|TCKN)"[^>]*>\s*([0-9]{10,11})`).FindAllSubmatch(ubl.Data, -1) {
		if v := string(m[1]); v != first.SenderVKN {
			own = v
			break
		}
	}
	if own == "" && bytes.Contains(ubl.Data, []byte(">"+first.SenderVKN+"<")) {
		own = first.SenderVKN // the test account invoices itself
	}
	if own == "" {
		t.Fatal("own VKN not found in the UBL")
	}
	t.Logf("account VKN read from UBL (%d digits)", len(own))
	anchor := probe.Items[0].CreatedAt
	if anchor.IsZero() {
		t.Fatal("listed document has no CDATE")
	}
	// A short window around the first document keeps the UBL downloads few.
	from := anchor.Add(-30 * time.Minute)
	to := anchor.Add(time.Hour)
	before := uyumsoftUnread(t, cfg, from.Add(-24*time.Hour), to.Add(24*time.Hour))

	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", own)
	d.Engagement(t, org, tp)
	q := db.New(d.Pool)
	conn, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{TaxpayerID: tp, SourceSystem: "entegrator_uyumsoft", SetupState: "active", Meta: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(d.Pool, nil, storage.NewMemory(), Config{IncrementalLookback: to.Sub(from), RequestGap: 200 * time.Millisecond}, nil).
		WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) { return c, nil }).
		WithClock(func() time.Time { return to })
	rep, err := svc.Run(ctx, tp, conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("report: %s %+v", rep.Status, rep.Counters)
	if rep.Counters.Listed == 0 || rep.Counters.UBLFetched == 0 {
		t.Fatalf("nothing synced: %+v", rep.Counters)
	}

	// 30 §9/2: adet ve ödenecek tutar toplamı, liste başlığıyla UBL'den
	// yazılan fatura birebir.
	rows, err := d.Pool.Query(ctx, `
		SELECT e.ettn, e.header_raw->>'Payable', i.payable_amount::text
		FROM entegrator_documents e JOIN invoices i ON i.taxpayer_id = e.taxpayer_id AND i.id = e.invoice_id
		WHERE e.taxpayer_id = $1`, tp)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var listed, stored decimal.Decimal
	for rows.Next() {
		var ettn, lp, sp string
		if err := rows.Scan(&ettn, &lp, &sp); err != nil {
			t.Fatal(err)
		}
		a, _ := decimal.NewFromString(lp)
		b, _ := decimal.NewFromString(sp)
		if !a.Equal(b) {
			t.Errorf("%s: listed payable %s, UBL %s", ettn, lp, sp)
		}
		listed, stored = listed.Add(a), stored.Add(b)
		n++
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	t.Logf("%d invoices, payable listed %s / stored %s", n, listed, stored)
	if n != rep.Counters.UBLFetched {
		t.Errorf("stored invoices %d, fetched %d", n, rep.Counters.UBLFetched)
	}

	// 30 §9/7: durum dağılımı (reddedilen ticari fatura cancelled).
	srows, err := d.Pool.Query(ctx, `SELECT e.doc_kind, e.direction, e.status_raw, e.status_normalized, array_to_string(e.warnings, ','), count(*)
		FROM entegrator_documents e WHERE e.taxpayer_id = $1 GROUP BY 1, 2, 3, 4, 5 ORDER BY 1, 2, 3`, tp)
	if err != nil {
		t.Fatal(err)
	}
	for srows.Next() {
		var k, dir, raw, norm, w string
		var cnt int
		if err := srows.Scan(&k, &dir, &raw, &norm, &w, &cnt); err != nil {
			t.Fatal(err)
		}
		if raw[:min(8, len(raw))] == "rejected" && norm != StatusCancelled {
			t.Errorf("rejected document is %s", norm)
		}
		t.Logf("  %s %s %s → %s [%s] ×%d", k, dir, raw, norm, w, cnt)
	}

	// 30 §9/9 ve 33 §6: okuma sonrası "okunmamış" kümesi ve durumlar aynı.
	after := uyumsoftUnread(t, cfg, from.Add(-24*time.Hour), to.Add(24*time.Hour))
	if len(before) != len(after) {
		t.Fatalf("unread before %d, after %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("unread set changed: %s → %s", before[i], after[i])
		}
	}
	t.Logf("unread documents unchanged: %d", len(after))
}
