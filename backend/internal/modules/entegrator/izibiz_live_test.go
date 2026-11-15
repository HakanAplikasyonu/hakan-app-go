//go:build izibizlive

// Live checks against the İzibiz test system (36 §11, 30 §9). Run with
//
//	IZIBIZ_TEST_USERNAME=... IZIBIZ_TEST_PASSWORD=... TEST_DATABASE_URL=... \
//	  go test -tags izibizlive -run Izibiz ./internal/modules/entegrator/
//
// Credentials come only from the environment.
package entegrator

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func izibizConfig(t *testing.T) provider.Config {
	t.Helper()
	u, p := os.Getenv("IZIBIZ_TEST_USERNAME"), os.Getenv("IZIBIZ_TEST_PASSWORD")
	if u == "" || p == "" {
		t.Skip("IZIBIZ_TEST_USERNAME / IZIBIZ_TEST_PASSWORD not set")
	}
	return provider.Config{Username: u, Password: p, Test: true}
}

// unread lists the inbox documents İzibiz still reports as not read
// (READ_INCLUDED=false). Test-only: production code cannot ask this.
func unread(t *testing.T, cfg provider.Config, from, to time.Time) []string {
	t.Helper()
	p, err := provider.Open("izibiz", cfg)
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

func TestIzibizLiveSync(t *testing.T) {
	cfg := izibizConfig(t)
	ctx := context.Background()
	c, err := Open("izibiz", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// 30 §9/1: bağlantı ve tek gelen fatura listeleme.
	probe, err := c.List(ctx, provider.Inbound, ListQuery{Kind: provider.EInvoice, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.Items) == 0 || probe.Items[0].ReceiverVKN == "" {
		t.Fatalf("no incoming invoice to anchor the test: %+v", probe)
	}
	own := probe.Items[0].ReceiverVKN
	anchor := probe.Items[0].CreatedAt
	if anchor.IsZero() {
		t.Fatal("listed document has no CDATE")
	}
	// A short window around the first document keeps the UBL downloads few.
	from := anchor.Add(-30 * time.Minute)
	to := anchor.Add(90 * time.Minute)
	before := unread(t, cfg, from.Add(-24*time.Hour), to.Add(24*time.Hour))

	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", own)
	d.Engagement(t, org, tp)
	q := db.New(d.Pool)
	conn, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{TaxpayerID: tp, SourceSystem: "entegrator_izibiz", SetupState: "active", Meta: []byte(`{}`)})
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

	// 30 §9/9 ve 36 §11/3: okuma sonrası "okunmamış" kümesi ve durumlar aynı.
	after := unread(t, cfg, from.Add(-24*time.Hour), to.Add(24*time.Hour))
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
