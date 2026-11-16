package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// seaweedFull answers PutObject like SeaweedFS with no writable volume left.
func seaweedFull(status int, code, message string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>` + code + `</Code><Message>` + message + `</Message><Resource>/app/x</Resource><RequestId>1</RequestId></Error>`))
	}))
}

func upload(t *testing.T, srv *httptest.Server) error {
	t.Helper()
	d, err := NewS3(context.Background(), config.ObjectStoreConfig{
		Endpoint: srv.URL, Bucket: "app", AccessKey: "k", SecretKey: "s", UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d.Upload(context.Background(), File{Body: bytes.NewReader([]byte("<Invoice/>")), Size: 10, ContentType: "application/xml"}, "taxpayers/1/ubl/a.xml")
}

// Nesne deposu dolunca yazma hatası 503 STORAGE_UNAVAILABLE olur (#247).
func TestUploadFullStoreIsUnavailable(t *testing.T) {
	before := testutil.ToFloat64(metricsFailures(ReasonNoSpace))
	srv := seaweedFull(http.StatusInternalServerError, "InternalError",
		`No writable volumes and no free volumes left for {"collection":"app","replication":"000"}`)
	defer srv.Close()
	err := upload(t, srv)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	var ue *UnavailableError
	if !errors.As(err, &ue) || ue.Reason != ReasonNoSpace {
		t.Fatalf("reason = %+v", ue)
	}
	if got := testutil.ToFloat64(metricsFailures(ReasonNoSpace)); got != before+1 {
		t.Fatalf("no_space failures = %v, want %v", got, before+1)
	}

	// The HTTP layer answers 503 with the code, not a generic 500.
	rec := httptest.NewRecorder()
	response.InternalErr(rec, httptest.NewRequest(http.MethodPost, "/x", nil), err, "upload failed")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"STORAGE_UNAVAILABLE"`) || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "volumes") {
		t.Fatalf("internal detail leaked: %s", rec.Body.String())
	}
}

func TestUploadServerErrorIsUnavailable(t *testing.T) {
	srv := seaweedFull(http.StatusServiceUnavailable, "ServiceUnavailable", "please slow down")
	defer srv.Close()
	var ue *UnavailableError
	if err := upload(t, srv); !errors.As(err, &ue) || ue.Reason != ReasonServer {
		t.Fatalf("err = %v", err)
	}
}

// İstemci hataları (yetki, geçersiz istek) depo kesintisi sayılmaz.
func TestUploadClientErrorIsNotUnavailable(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{{http.StatusForbidden, "AccessDenied"}, {http.StatusBadRequest, "InvalidArgument"}} {
		srv := seaweedFull(tc.status, tc.code, "nope")
		err := upload(t, srv)
		srv.Close()
		if err == nil || errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s: err = %v", tc.code, err)
		}
		rec := httptest.NewRecorder()
		response.InternalErr(rec, httptest.NewRequest(http.MethodPost, "/x", nil), err, "upload failed")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d", tc.code, rec.Code)
		}
	}
	if classifyWrite(context.Canceled) != "" {
		t.Fatal("a cancelled request is not an outage")
	}
}

func metricsFailures(reason string) prometheus.Counter {
	return metrics.Default.StorageWriteFailures(reason)
}
