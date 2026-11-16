// Package metrics exposes Prometheus metrics (issue #179): HTTP latency and
// status, background queue depth and integrator call outcomes, plus the Go
// runtime and process collectors.
//
// Labels carry no personal data and stay low-cardinality: HTTP requests are
// labelled by their route pattern ("GET /v1/tenant/invoices/{uuid}"), never
// by the raw path.
package metrics

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/tracing"
)

// Registry holds the process's metrics. Default is used by package-level
// helpers (HTTP middleware, integrator calls).
var Default = NewRegistry()

// Registry is a Prometheus registry with the application's collectors.
type Registry struct {
	reg          *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	integrator   *prometheus.CounterVec
	integratorD  *prometheus.HistogramVec
}

// NewRegistry builds a registry with the runtime and app collectors.
func NewRegistry() *Registry {
	r := &Registry{reg: prometheus.NewRegistry()}
	r.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, route pattern and status code.",
	}, []string{"method", "route", "status"})
	r.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route pattern.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route"})
	r.integrator = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "entegrator_requests_total",
		Help: "Integrator (e-Fatura) calls by provider, operation and outcome (ok or the error kind).",
	}, []string{"provider", "operation", "outcome"})
	r.integratorD = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "entegrator_request_duration_seconds",
		Help:    "Integrator call latency by provider and operation.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"provider", "operation"})
	r.reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		r.httpRequests, r.httpDuration, r.integrator, r.integratorD,
	)
	return r
}

// MustRegister adds collectors (e.g. the queue depth collector).
func (r *Registry) MustRegister(cs ...prometheus.Collector) { r.reg.MustRegister(cs...) }

// Gatherer is the registry for tests and the handler.
func (r *Registry) Gatherer() prometheus.Gatherer { return r.reg }

// Handler serves the metrics in the Prometheus text format. A non-empty
// token requires "Authorization: Bearer <token>".
func (r *Registry) Handler(token string) http.Handler {
	h := promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
	if token == "" {
		return h
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// Serve listens on addr and serves /metrics until ctx ends. An empty addr
// disables the listener.
func (r *Registry) Serve(ctx context.Context, addr, token string, log *slog.Logger) error {
	if addr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", r.Handler(token))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && log != nil {
			log.Error("metrics_server_failed", "error", err)
		}
	}()
	if log != nil {
		log.Info("metrics_listening", "addr", ln.Addr().String(), "auth", token != "")
	}
	return nil
}

// statusWriter records the response status.
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

// HTTP measures requests. It must wrap the ServeMux directly: the mux sets
// r.Pattern on the request it is given, which this middleware then reads.
func (r *Registry) HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, req)
		tracing.Route(req.Context(), req.Pattern)
		route := req.Pattern
		if route == "" {
			route = "unmatched"
		} else if i := strings.IndexByte(route, ' '); i >= 0 {
			route = route[i+1:] // method is its own label
		}
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		r.httpRequests.WithLabelValues(req.Method, route, strconv.Itoa(status)).Inc()
		r.httpDuration.WithLabelValues(req.Method, route).Observe(time.Since(start).Seconds())
	})
}

// ObserveIntegrator records one integrator call. outcome is "ok" or the
// error kind (auth, rate-limit, unavailable …).
func (r *Registry) ObserveIntegrator(provider, operation, outcome string, d time.Duration) {
	r.integrator.WithLabelValues(provider, operation, outcome).Inc()
	r.integratorD.WithLabelValues(provider, operation).Observe(d.Seconds())
}
