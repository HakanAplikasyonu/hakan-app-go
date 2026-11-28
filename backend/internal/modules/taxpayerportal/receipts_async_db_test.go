package taxpayerportal

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// failingReader: the AI could not read the photo.
type failingReader struct{ calls atomic.Int32 }

func (f *failingReader) Read(context.Context, extract.Request) (extract.Result, error) {
	f.calls.Add(1)
	return extract.Result{}, errors.New("model could not read the photo")
}

// panickingReader: the read crashes.
type panickingReader struct{}

func (panickingReader) Read(context.Context, extract.Request) (extract.Result, error) {
	panic("reader exploded")
}

// gateReader blocks every read until release is closed.
type gateReader struct {
	release chan struct{}
	calls   atomic.Int32
}

func (g *gateReader) Read(ctx context.Context, _ extract.Request) (extract.Result, error) {
	g.calls.Add(1)
	select {
	case <-g.release:
	case <-ctx.Done():
		return extract.Result{}, ctx.Err()
	}
	r, err := extract.Parse(fuelAnswer)
	r.Model = "claude-test"
	return r, err
}

func postReceipt(t *testing.T, e env, tag, key string) (int, map[string]any) {
	t.Helper()
	body, ct := receiptForm(t, tag)
	req := e.req(t, http.MethodPost, "/v1/taxpayer/receipts", body, ct)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := e.serve(req)
	var out map[string]any
	if rec.Code < 300 {
		decode(t, rec, &out)
	}
	return rec.Code, out
}

func getReceipt(t *testing.T, e env, id string) map[string]any {
	t.Helper()
	rec := e.do(t, "GET", "/v1/taxpayer/receipts/"+id, e.tpA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get %s = %d %s", id, rec.Code, rec.Body)
	}
	var out map[string]any
	decode(t, rec, &out)
	return out
}

// #780: a background read that fails leaves the capture failed (Okunamayan
// fiş) with the reader's error, as the synchronous capture did; the
// submissions show it unreadable and a new photo of the same image is read
// again.
func TestTaxpayerReceiptBackgroundReadFails(t *testing.T) {
	e := setup(t)
	reader := &failingReader{}
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), reader, nil, nil, nil)
	e.h.WithReceipts(svc)

	code, out := postReceipt(t, e, "okunamayan", "k-1")
	if code != http.StatusAccepted || out["status"] != "processing" {
		t.Fatalf("upload = %d %v", code, out)
	}
	waitReads(t, svc)
	got := getReceipt(t, e, out["uuid"].(string))
	if got["status"] != "failed" || got["error"] != "model could not read the photo" {
		t.Fatalf("failed read = %v", got)
	}
	var list SubmissionList
	decode(t, e.do(t, "GET", "/v1/taxpayer/submissions", e.tpA, nil, ""), &list)
	if len(list.Items) != 1 || list.Items[0].Status != SubmissionUnreadable {
		t.Fatalf("submissions = %+v", list.Items)
	}
	// The same image again (new key): not a duplicate of a failed read.
	code, again := postReceipt(t, e, "okunamayan", "k-2")
	if code != http.StatusAccepted || again["uuid"] == out["uuid"] {
		t.Fatalf("retry = %d %v", code, again)
	}
	waitReads(t, svc)
	if n := reader.calls.Load(); n != 2 {
		t.Fatalf("reads = %d, want 2", n)
	}
}

// #780: a read that panics does not take the server down; the capture is
// failed so the office still sees it.
func TestTaxpayerReceiptBackgroundReadPanics(t *testing.T) {
	e := setup(t)
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), panickingReader{}, nil, nil, nil)
	e.h.WithReceipts(svc)
	code, out := postReceipt(t, e, "panik", "")
	if code != http.StatusAccepted {
		t.Fatalf("upload = %d %v", code, out)
	}
	waitReads(t, svc)
	got := getReceipt(t, e, out["uuid"].(string))
	if got["status"] != "failed" || got["error"] != receiptsusecase.ReadUnfinished {
		t.Fatalf("after panic = %v", got)
	}
}

// #780: the same image sent again while its first copy is still being read
// is a duplicate of it (no second read); once read, the same image is a
// duplicate of the draft, answered at once with 201.
func TestTaxpayerReceiptSameImageWhileReading(t *testing.T) {
	e := setup(t)
	reader := &gateReader{release: make(chan struct{})}
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), reader, nil, nil, nil)
	e.h.WithReceipts(svc)

	code, first := postReceipt(t, e, "ayni-foto", "a-1")
	if code != http.StatusAccepted {
		t.Fatalf("upload = %d %v", code, first)
	}
	code, dup := postReceipt(t, e, "ayni-foto", "a-2")
	if code != http.StatusCreated || dup["status"] != "duplicate" || dup["duplicate_of"] != first["uuid"] {
		t.Fatalf("same image while reading = %d %v", code, dup)
	}
	// A replay of the first upload answers with the processing record.
	code, replay := postReceipt(t, e, "ayni-foto", "a-1")
	if code != http.StatusOK || replay["uuid"] != first["uuid"] || replay["status"] != "processing" {
		t.Fatalf("replay while reading = %d %v", code, replay)
	}
	close(reader.release)
	waitReads(t, svc)
	if got := getReceipt(t, e, first["uuid"].(string)); got["status"] != "draft" {
		t.Fatalf("first after read = %v", got)
	}
	code, later := postReceipt(t, e, "ayni-foto", "a-3")
	if code != http.StatusCreated || later["status"] != "duplicate" {
		t.Fatalf("same image after read = %d %v", code, later)
	}
	if n := reader.calls.Load(); n != 1 {
		t.Fatalf("reads = %d, want 1", n)
	}
}

// #780: the worker's sweep fails captures left "processing" for more than
// StaleReadAfter (the read was lost: restart, crash) with the timeout's
// error, tells the office about the mükellef's receipt (not about a WhatsApp
// capture of its own user), leaves fresh captures alone, and a read that
// still finishes later does not undo the failure.
func TestSweepStaleReceiptReads(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	officeUser := e.d.User(t)
	e.d.Member(t, e.orgA, officeUser, "accountant")
	e.d.Assign(t, e.orgA, e.engA, officeUser)
	notif := &recNotifier{}
	reader := &gateReader{release: make(chan struct{})}
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), reader, nil, nil, nil).WithNotifier(notif, "https://panel.example")
	e.h.WithReceipts(svc)
	released := false
	release := func() {
		if !released {
			released = true
			close(reader.release)
			waitReads(t, svc)
		}
	}
	t.Cleanup(release)

	_, stale := postReceipt(t, e, "takilan", "takilan-1")
	_, viaWhatsApp := postReceipt(t, e, "takilan-wa", "whatsapp:expense:abc")
	_, fresh := postReceipt(t, e, "taze", "taze-1")
	for _, c := range []map[string]any{stale, viaWhatsApp, fresh} {
		if c["status"] != "processing" {
			t.Fatalf("upload = %v", c)
		}
	}
	if _, err := e.d.Pool.Exec(ctx, `UPDATE receipt_captures SET created_at = now() - interval '11 minutes' WHERE uuid = ANY($1::uuid[])`,
		[]string{stale["uuid"].(string), viaWhatsApp["uuid"].(string)}); err != nil {
		t.Fatal(err)
	}

	n, err := svc.SweepStaleReads(ctx, time.Now())
	if err != nil || n != 2 {
		t.Fatalf("sweep = %d %v, want 2", n, err)
	}
	status := func(id any) (string, string) {
		t.Helper()
		var st, msg string
		if err := e.d.Pool.QueryRow(ctx, `SELECT status, COALESCE(error, '') FROM receipt_captures WHERE uuid = $1`, id).Scan(&st, &msg); err != nil {
			t.Fatal(err)
		}
		return st, msg
	}
	for _, c := range []map[string]any{stale, viaWhatsApp} {
		if st, msg := status(c["uuid"]); st != "failed" || msg != receiptsusecase.ReadUnfinished {
			t.Fatalf("stale capture = %s %q", st, msg)
		}
	}
	if st, _ := status(fresh["uuid"]); st != "processing" {
		t.Fatalf("fresh capture = %s, want processing", st)
	}
	if got := notif.kinds(officeUser); len(got) != 1 || got[0] != receiptsusecase.NoticeReceiptSubmitted {
		t.Fatalf("office notices = %v, want one (not the WhatsApp capture)", got)
	}
	if n, err := svc.SweepStaleReads(ctx, time.Now()); err != nil || n != 0 {
		t.Fatalf("second sweep = %d %v", n, err)
	}

	// The lost reads finish after all: the failed captures stay failed and
	// get no draft; the fresh one is read.
	release()
	if st, _ := status(stale["uuid"]); st != "failed" {
		t.Fatalf("stale after late read = %s", st)
	}
	var drafts int
	if err := e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM receipt_captures WHERE status = 'draft' AND taxpayer_id = $1`, e.tpAID).Scan(&drafts); err != nil || drafts != 1 {
		t.Fatalf("drafts = %d %v, want 1 (the fresh capture)", drafts, err)
	}
	if st, _ := status(fresh["uuid"]); st != "draft" {
		t.Fatalf("fresh after read = %s", st)
	}
}
