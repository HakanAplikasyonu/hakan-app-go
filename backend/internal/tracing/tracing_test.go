package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
)

func TestInitOffWithoutEndpoint(t *testing.T) {
	shutdown, err := Init(context.Background(), config.TracingConfig{}, "api")
	if err != nil || shutdown(context.Background()) != nil {
		t.Fatalf("err = %v", err)
	}
}

// Span adları rota kalıbıdır; URL, sorgu ve başlık iz sistemine gitmez.
func TestHTTPSpansCarryNoPersonalData(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); otel.SetTextMapPropagator(prevProp) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tenant/taxpayers/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	// The metrics middleware names the span after routing; emulate it here.
	routed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		Route(r.Context(), r.Pattern)
	})
	req := httptest.NewRequest("GET", "/v1/tenant/taxpayers/x?email=ayse@example.com&vkn=1234567890", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	HTTP(routed).ServeHTTP(httptest.NewRecorder(), req)
	HTTP(routed).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d (health must not be traced)", len(spans))
	}
	s := spans[0]
	if s.Name() != "GET /v1/tenant/taxpayers/{uuid}" {
		t.Fatalf("name = %q", s.Name())
	}
	if s.Parent().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("traceparent not propagated: %v", s.Parent().TraceID())
	}
	if s.Status().Code.String() != "Error" {
		t.Fatalf("status = %v", s.Status())
	}
	for _, a := range s.Attributes() {
		v := a.Value.String()
		if strings.Contains(v, "ayse") || strings.Contains(v, "1234567890") || strings.Contains(v, "secret") || strings.Contains(v, "/x") {
			t.Fatalf("attribute %s=%s leaks request data", a.Key, v)
		}
	}
}
