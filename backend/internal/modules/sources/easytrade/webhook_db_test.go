package easytrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// TestWebhook: a signed push pulls the company's connection forward and
// enqueues its sync; unsigned, stale or wrongly signed pushes are refused;
// without a secret the receiver is off.
func TestWebhook(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	srv := testfakes.Serve(t, testfakes.EasytradeSample())
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), srv.URL, srv.Client())
	svc.interval = 0
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	c, err := svc.Connect(ctx, tp, "a@b.c", testfakes.EasytradePassword, testfakes.EasytradeCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunScheduled(ctx, tp, c.UUID); err != nil {
		t.Fatal(err)
	}

	secret := "whsec-test"
	var queued []uuid.UUID
	wh := NewWebhook(svc, secret, func(_ context.Context, _ int64, id uuid.UUID) error { queued = append(queued, id); return nil })
	wh.now = func() time.Time { return now.Add(time.Hour) }
	mux := http.NewServeMux()
	RegisterWebhook(mux, wh)
	send := func(body string, ts int64, sig string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
		r.Header.Set(HeaderWebhookTimestamp, strconv.FormatInt(ts, 10))
		if sig != "" {
			r.Header.Set(HeaderWebhookSignature, sig)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	ts := now.Add(time.Hour).Unix()
	body := `{"event_id":"e1","event":"invoice.created","company_id":"42","resource":"invoice","resource_id":"7"}`

	for name, w := range map[string]*httptest.ResponseRecorder{
		"unsigned":     send(body, ts, ""),
		"wrong secret": send(body, ts, Sign([]byte("other"), ts, []byte(body))),
		"tampered":     send(strings.Replace(body, "42", "43", 1), ts, Sign([]byte(secret), ts, []byte(body))),
		"stale":        send(body, ts-int64(WebhookTolerance/time.Second)-1, Sign([]byte(secret), ts-int64(WebhookTolerance/time.Second)-1, []byte(body))),
	} {
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: HTTP %d %s", name, w.Code, w.Body)
		}
	}
	if len(queued) != 0 {
		t.Fatalf("refused pushes enqueued %v", queued)
	}

	w := send(body, ts, Sign([]byte(secret), ts, []byte(body)))
	if w.Code != http.StatusAccepted || len(queued) != 1 || queued[0] != c.UUID {
		t.Fatalf("signed push: HTTP %d %s, queued %v", w.Code, w.Body, queued)
	}
	var out struct {
		Data struct{ Accepted int } `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.Accepted != 1 {
		t.Fatalf("accepted %d", out.Data.Accepted)
	}
	list, _ := svc.List(ctx, tp)
	if !list[0].NextSyncAt.Equal(now.Add(time.Hour)) || list[0].LastWebhookAt == nil {
		t.Fatalf("after push: next %v webhook %v", list[0].NextSyncAt, list[0].LastWebhookAt)
	}
	// The sweep picks it now too (no queue: polling fallback).
	svc.now = wh.now
	if n, _ := svc.EnqueueDue(ctx, func(context.Context, int64, uuid.UUID) error { return nil }); n != 1 {
		t.Fatalf("due after push: %d", n)
	}

	// Unknown company: accepted, nothing queued.
	other := `{"company_id":"999"}`
	if w := send(other, ts, Sign([]byte(secret), ts, []byte(other))); w.Code != http.StatusAccepted || len(queued) != 1 {
		t.Fatalf("unknown company: HTTP %d, queued %v", w.Code, queued)
	}
	// No company_id.
	if w := send(`{}`, ts, Sign([]byte(secret), ts, []byte(`{}`))); w.Code != http.StatusBadRequest {
		t.Fatalf("no company: HTTP %d", w.Code)
	}

	// Without a secret the receiver is off.
	offMux := http.NewServeMux()
	RegisterWebhook(offMux, NewWebhook(svc, "", nil))
	r := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	offMux.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no secret: HTTP %d", rec.Code)
	}
}
