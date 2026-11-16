// Package tracing is the OpenTelemetry hook (issue #179). It is off unless an
// OTLP endpoint is configured (OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT); the exporter reads the other standard
// OTEL_EXPORTER_OTLP_* variables (headers, protocol, timeout) itself.
//
// Spans carry the route pattern, method and status; no URL, query string,
// body or header. The W3C traceparent header is accepted and propagated.
package tracing

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/buildinfo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
)

// Init installs the global tracer provider. component ("api", "worker") is
// added to the service name. The returned shutdown flushes pending spans; it
// is a no-op when tracing is off.
func Init(ctx context.Context, cfg config.TracingConfig, component string) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if cfg.Endpoint == "" {
		return noop, nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, err
	}
	name := cfg.ServiceName
	if component != "" {
		name += "-" + component
	}
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(name),
		semconv.ServiceVersion(buildinfo.Version),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}

const tracerName = "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/tracing"

// HTTP starts a server span per request. Only the method, the route pattern
// and the status are recorded: no URL, query string, header or body, so no
// personal data reaches the trace backend. With no provider installed the
// global no-op tracer makes this nearly free.
func HTTP(next http.Handler) http.Handler {
	tracer := otel.Tracer(tracerName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/health") {
			next.ServeHTTP(w, r)
			return
		}
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", r.Method)))
		defer span.End()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r.WithContext(ctx))
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	})
}

// Route names the current span after the matched route pattern
// ("GET /v1/tenant/invoices/{uuid}"). The metrics middleware calls it once
// the mux has routed the request.
func Route(ctx context.Context, pattern string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() || pattern == "" {
		return
	}
	span.SetName(pattern)
	span.SetAttributes(attribute.String("http.route", pattern))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush / Hijack (SSE, WebSocket).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
