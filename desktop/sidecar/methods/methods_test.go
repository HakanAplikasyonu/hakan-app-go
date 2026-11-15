package methods

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ubltr "github.com/furkanmeclis/go-ubltr"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

func call(t *testing.T, h rpc.Handler, params any) (any, *rpc.Error) {
	t.Helper()
	raw, _ := json.Marshal(params)
	res, err := h(context.Background(), raw)
	if err != nil {
		var pe *rpc.Error
		if !errors.As(err, &pe) {
			t.Fatalf("kodsuz hata: %v", err)
		}
		return nil, pe
	}
	return res, nil
}

// Backend'in demo UBL setiyle: sidecar ve backend aynı kütüphaneyle okur.
func TestInspectUBL(t *testing.T) {
	res, perr := call(t, inspectUBL, map[string]string{"path": "../../../backend/internal/demo/ubl/atlas-01-temel-satis.xml"})
	if perr != nil {
		t.Fatal(perr)
	}
	sum := res.(ubltr.Summary)
	if sum.UUID == "" || sum.ID == "" || sum.Lines == 0 || sum.Payable == "" {
		t.Errorf("özet eksik: %+v", sum)
	}

	if _, perr := call(t, inspectUBL, map[string]string{}); perr == nil || perr.Code != rpc.CodeInvalidParams {
		t.Errorf("boş path: %v", perr)
	}
	if _, perr := call(t, inspectUBL, map[string]string{"path": "methods.go"}); perr == nil || perr.Code != "NOT_UBL" {
		t.Errorf("UBL olmayan dosya: %v", perr)
	}
	if _, perr := call(t, inspectUBL, map[string]string{"path": "/yok/boyle/dosya.xml"}); perr == nil || perr.Code != rpc.CodeInvalidParams {
		t.Errorf("olmayan dosya: %v", perr)
	}
}

func TestAPIHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /v1/app/config", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"name":"HAKAN","mode":"platform"},"meta":{"request_id":"r1"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	h := func(ctx context.Context, p json.RawMessage) (any, error) { return apiHealth(ctx, srv.Client(), p) }

	res, perr := call(t, h, map[string]string{"base_url": srv.URL + "/v1/"})
	if perr != nil {
		t.Fatal(perr)
	}
	hr := res.(healthResult)
	if !hr.Healthy || hr.AppName != "HAKAN" || hr.RequestID != "r1" || hr.BaseURL != srv.URL {
		t.Errorf("sonuç: %+v", hr)
	}

	// Backend hata zarfı response.Envelope ile çözülür.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"INTERNAL_ERROR","message":"db"},"meta":{}}`))
	}))
	defer bad.Close()
	h2 := func(ctx context.Context, p json.RawMessage) (any, error) { return apiHealth(ctx, bad.Client(), p) }
	if _, perr := call(t, h2, map[string]string{"base_url": bad.URL}); perr == nil || perr.Code != rpc.CodeUnavailable {
		t.Errorf("hata zarfı: %v", perr)
	}

	for _, u := range []string{"", "ftp://x", "http://api.example.com", "nota url"} {
		if _, perr := call(t, h, map[string]string{"base_url": u}); perr == nil || perr.Code != rpc.CodeInvalidParams {
			t.Errorf("%q kabul edilmemeliydi: %v", u, perr)
		}
	}
}

func TestSourceParams(t *testing.T) {
	var buf strings.Builder
	s := rpc.NewServer(&buf)
	RegisterSources(s)
	in := strings.Join([]string{
		`{"id":1,"method":"source.preview","params":{"config":{"kind":"mikro","host":"x","database":"d","user":"u","password":"p"},"from":"2026-10-01","to":"2026-09-01"}}`,
		`{"id":2,"method":"source.test","params":{"config":{"kind":"logo_tiger","host":"x","database":"d","user":"u","password":"p","firm_no":"1","period_no":"01"}}}`,
		`{"id":3,"method":"source.test","params":{"config":{"kind":"netsis"}}}`,
	}, "\n")
	if err := s.Serve(context.Background(), strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, id := range []string{`"id":1`, `"id":2`, `"id":3`} {
		line := ""
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, id+",") {
				line = l
			}
		}
		if !strings.Contains(line, `"code":"INVALID_PARAMS"`) {
			t.Errorf("%s INVALID_PARAMS olmalıydı: %s", id, line)
		}
		if strings.Contains(line, `"p"`) || strings.Contains(line, "password") {
			t.Errorf("hata parolayı sızdırıyor: %s", line)
		}
	}
}
