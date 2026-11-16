package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

var errBase = errors.New("base")

func TestRedactMasksIdentifiersInLogs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(Redact(slog.NewJSONHandler(&buf, nil))).With("tenant", "ofis 1234567890")
	wrapped := errors.Join(errBase, errors.New("iban TR330006100519786457841326 reddedildi"))
	log.Info("fatura VKN:1234567890 için eşleşme yok",
		"tckn", "12345678901",
		"note", "alıcı TCKN 12345678901, e-posta a@b.co",
		"error", wrapped,
		"invoice_count", 1234567890, // a number, not text
		"request_id", "7f1c2d3e-0000-4000-8000-a12345678901",
		slog.Group("party", "iban", "TR33 0006 1005 1978 6457 8413 26", "name", "Ayşe"),
	)
	out := buf.String()
	for _, leak := range []string{"1234567890 ", "VKN:1234567890", "12345678901,", "TR330006100519786457841326", "a@b.co", "ofis 1234567890"} {
		if strings.Contains(out, leak) {
			t.Fatalf("%q leaked: %s", leak, out)
		}
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["msg"] != "fatura VKN:[vkn] için eşleşme yok" || rec["tckn"] != "[redacted]" || rec["tenant"] != "ofis [vkn]" {
		t.Fatalf("record = %v", rec)
	}
	if rec["invoice_count"] != float64(1234567890) {
		t.Fatalf("numbers must pass: %v", rec["invoice_count"])
	}
	if rec["request_id"] != "7f1c2d3e-0000-4000-8000-a12345678901" {
		t.Fatalf("request id changed: %v", rec["request_id"])
	}
	if p := rec["party"].(map[string]any); p["iban"] != "[redacted]" || p["name"] != "Ayşe" {
		t.Fatalf("group = %v", p)
	}
	if !strings.Contains(rec["error"].(string), "[iban]") {
		t.Fatalf("error = %v", rec["error"])
	}
}

func TestRedactKeepsErrorChain(t *testing.T) {
	a := redactAttr(slog.Any("error", errors.Join(errBase, errors.New("x"))))
	err, ok := a.Value.Any().(error)
	if !ok || !errors.Is(err, errBase) {
		t.Fatalf("error chain lost: %#v", a.Value.Any())
	}
}

func TestNewLoggerRedacts(t *testing.T) {
	if _, ok := New("info", "json").Handler().(*redactHandler); !ok {
		t.Fatal("New must wrap the handler with Redact")
	}
}

func TestRedactStructuredValues(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(Redact(slog.NewJSONHandler(&buf, nil)))
	log.Info("x", "ids", []int64{1, 2}, "payload", map[string]any{"note": "VKN 1234567890", "n": 3}, "bare", []int64{1234567890})
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if ids, ok := rec["ids"].([]any); !ok || len(ids) != 2 {
		t.Fatalf("ids = %v", rec["ids"])
	}
	if p := rec["payload"].(map[string]any); p["note"] != "VKN [vkn]" || p["n"] != float64(3) {
		t.Fatalf("payload = %v", p)
	}
	if rec["bare"] != "[[vkn]]" {
		t.Fatalf("bare = %v", rec["bare"])
	}
}

func TestRedactAddsRequestAndTenantIDs(t *testing.T) {
	prev := RequestIDFromContext
	RequestIDFromContext = func(context.Context) string { return "req-1" }
	t.Cleanup(func() { RequestIDFromContext = prev })
	var buf bytes.Buffer
	log := slog.New(Redact(slog.NewJSONHandler(&buf, nil)))
	ctx := orgctx.WithScope(context.Background(), orgctx.Scope{ID: 42})
	log.InfoContext(ctx, "x")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["request_id"] != "req-1" || rec["organization_id"] != float64(42) {
		t.Fatalf("record = %v", rec)
	}
}
