package entegrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// recorder is the spy that records the order of listings and UBL fetches.
type recorder struct {
	*spy
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(e string) { r.mu.Lock(); r.events = append(r.events, e); r.mu.Unlock() }

func (r *recorder) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	r.add("list:" + q.From.In(istanbul).Format("2006-01"))
	return r.spy.ListInbox(ctx, q)
}

func (r *recorder) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	r.add("list:" + q.From.In(istanbul).Format("2006-01"))
	return r.spy.ListOutbox(ctx, q)
}

func (r *recorder) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	r.add("fetch")
	return r.spy.Fetch(ctx, q)
}

// İlk bağlantı (#780): ilk yükleme bu aydan başlar; son iki ayın faturaları
// (liste + UBL) geçmiş aylar listelenmeden önce gelir ve bağlantı görünümü
// ilerlemeyi gösterir.
func TestBackfillRecentMonthsFirst(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	rec := &recorder{spy: e.spy}
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) { return Wrap(rec), nil })
	recent := e.deliver(t, 1, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2026-11-10T09:00:00+03:00"))
	old := e.deliver(t, 2, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2025-03-10T09:00:00+03:00"))

	conns := NewConnections(e.pool, nil, nil).WithClock(func() time.Time { return e.now })
	if v, err := conns.Get(ctx, e.tp, e.conn.Uuid); err != nil || v.Progress == nil || v.Progress.Phase != "queued" || v.Progress.HistoryMonthsTotal != 21 {
		t.Fatalf("before any run: %+v %v", v.Progress, err)
	}

	start := time.Now()
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill); err != nil {
		t.Fatal(err)
	}
	t.Logf("backfill with the fake took %v (%d integrator calls)", time.Since(start), len(rec.events))

	if len(rec.events) == 0 || rec.events[0] != "list:2026-11" {
		t.Fatalf("first call = %v, want the current month", rec.events)
	}
	firstFetch, firstOld := -1, -1
	for i, ev := range rec.events {
		if ev == "fetch" && firstFetch < 0 {
			firstFetch = i
		}
		if ev != "fetch" && ev < "list:2026-10" && firstOld < 0 {
			firstOld = i
		}
	}
	if firstFetch < 0 || firstOld < 0 || firstFetch > firstOld {
		t.Fatalf("the recent UBL (call %d) must come before the older months (call %d): %v", firstFetch, firstOld, rec.events)
	}
	for _, ettn := range []string{recent, old} {
		if d := e.doc(t, ettn, "incoming"); !d.InvoiceID.Valid {
			t.Fatalf("%s has no invoice", ettn)
		}
	}

	v, err := conns.Get(ctx, e.tp, e.conn.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	p := v.Progress
	if p == nil || p.Phase != "done" || !p.RecentReady || p.ThisMonthDocuments != 1 || p.ThisMonthInvoices != 1 ||
		p.HistoryMonthsTotal != 21 || p.HistoryMonthsDone != 21 {
		t.Fatalf("progress = %+v", p)
	}
}
