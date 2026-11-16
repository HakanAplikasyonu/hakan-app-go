package metrics

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetricsUseRoutePattern(t *testing.T) {
	r := NewRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tenant/invoices/{uuid}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /v1/things", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	h := r.HTTP(mux)
	for _, req := range []*http.Request{
		httptest.NewRequest("GET", "/v1/tenant/invoices/1b4e28ba-2fa1-11d2-883f-0016d3cca427", nil),
		httptest.NewRequest("GET", "/v1/tenant/invoices/ayse@example.com", nil),
		httptest.NewRequest("POST", "/v1/things", nil),
		httptest.NewRequest("GET", "/nope/12345678901", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := testutil.ToFloat64(r.httpRequests.WithLabelValues("GET", "/v1/tenant/invoices/{uuid}", "404")); got != 2 {
		t.Fatalf("pattern counter = %v", got)
	}
	if got := testutil.ToFloat64(r.httpRequests.WithLabelValues("POST", "/v1/things", "200")); got != 1 {
		t.Fatalf("post counter = %v", got)
	}
	if got := testutil.ToFloat64(r.httpRequests.WithLabelValues("GET", "unmatched", "404")); got != 1 {
		t.Fatalf("unmatched counter = %v", got)
	}
	// Raw paths (ids, e-mail, TCKN) never become labels.
	body := scrape(t, r.Handler(""), "")
	for _, leak := range []string{"ayse@example.com", "12345678901", "1b4e28ba"} {
		if strings.Contains(body, leak) {
			t.Fatalf("%q in metrics output", leak)
		}
	}
	if !strings.Contains(body, `http_request_duration_seconds_count{method="GET",route="/v1/tenant/invoices/{uuid}"} 2`) {
		t.Fatalf("histogram missing:\n%s", body)
	}
	if !strings.Contains(body, "go_goroutines") || !strings.Contains(body, "process_") {
		t.Fatal("runtime collectors missing")
	}
}

func TestMetricsHandlerToken(t *testing.T) {
	h := NewRegistry().Handler("s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if body := scrape(t, h, "s3cret"); !strings.Contains(body, "go_goroutines") {
		t.Fatal("token rejected")
	}
}

func TestIntegratorMetrics(t *testing.T) {
	r := NewRegistry()
	r.ObserveIntegrator("izibiz", "list_inbox", "ok", time.Second)
	r.ObserveIntegrator("izibiz", "list_inbox", "auth", time.Second)
	r.ObserveIntegrator("izibiz", "list_inbox", "auth", time.Second)
	if got := testutil.ToFloat64(r.integrator.WithLabelValues("izibiz", "list_inbox", "auth")); got != 2 {
		t.Fatalf("auth errors = %v", got)
	}
}

type fakeInspector map[string]*asynq.QueueInfo

func (f fakeInspector) GetQueueInfo(q string) (*asynq.QueueInfo, error) {
	if i, ok := f[q]; ok {
		return i, nil
	}
	return nil, errors.New("queue not found")
}

func TestQueueCollector(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(NewQueueCollector(fakeInspector{
		"imports": {Pending: 7, Active: 2, Retry: 1, Latency: 3 * time.Second},
	}, []string{"imports", "exports"}))
	body := scrape(t, r.Handler(""), "")
	for _, want := range []string{
		`queue_tasks{queue="imports",state="pending"} 7`,
		`queue_tasks{queue="imports",state="active"} 2`,
		`queue_latency_seconds{queue="imports"} 3`,
		`queue_inspect_up{queue="imports"} 1`,
		`queue_inspect_up{queue="exports"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func scrape(t *testing.T, h http.Handler, token string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Result().Body)
	return string(b)
}
