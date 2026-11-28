package taxpayerportal

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

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
