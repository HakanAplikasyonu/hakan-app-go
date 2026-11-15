// Package methods, sidecar'ın kabuğa açtığı yöntemlerdir.
//
// İskelet (#50) yalnızca bağlantıyı ve backend paketlerinin paylaşımını
// gösteren yöntemleri içerir. Yerel ajan işleri (Logo/Mikro okuma, klasör
// izleme, yükleme) #51'de buraya eklenir; yalnızca mevcut backend yükleme
// API'lerini kullanır.
package methods

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// MaxUBLBytes, ubl.inspect'in okuyacağı en büyük dosya.
const MaxUBLBytes = 20 << 20

// Info sidecar sürüm bilgisidir.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	PID       int    `json:"pid"`
}

func NewInfo(version, commit string) Info {
	return Info{
		Version:   version,
		Commit:    commit,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		PID:       os.Getpid(),
	}
}

// Register bütün yöntemleri sunucuya bağlar.
func Register(s *rpc.Server, info Info, client *http.Client) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	s.Handle("ping", func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Echo string `json:"echo"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		return map[string]any{"pong": true, "echo": p.Echo, "time": time.Now().UTC().Format(time.RFC3339Nano)}, nil
	})
	s.Handle("version", func(context.Context, json.RawMessage) (any, error) { return info, nil })
	s.Handle("ubl.inspect", inspectUBL)
	s.Handle("api.health", func(ctx context.Context, params json.RawMessage) (any, error) {
		return apiHealth(ctx, client, params)
	})
}

// inspectUBL yerel bir UBL-TR dosyasının özetini go-ubltr ile (backend'in
// yükleme akışıyla aynı kütüphane) çıkarır. Dosya sunucuya gönderilmez.
func inspectUBL(_ context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := rpc.Decode(params, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "path gerekli")
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "dosya açılamadı: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxUBLBytes+1))
	if err != nil {
		return nil, rpc.Errorf(rpc.CodeInternal, "dosya okunamadı: %v", err)
	}
	if len(data) > MaxUBLBytes {
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "dosya %d MB sınırını aşıyor", MaxUBLBytes>>20)
	}
	sum, err := ubltr.Inspect(data)
	if err != nil {
		return nil, rpc.Errorf("NOT_UBL", "UBL-TR belgesi değil: %v", err)
	}
	return sum, nil
}

type healthResult struct {
	BaseURL   string `json:"base_url"`
	Healthy   bool   `json:"healthy"`
	LatencyMS int64  `json:"latency_ms"`
	// AppName, /v1/app/config yanıtından; HAKAN API'si olduğunu doğrular.
	AppName   string `json:"app_name,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// apiHealth backend'e ulaşılabildiğini denetler: /healthz ve standart zarfla
// dönen /v1/app/config. Zarf backend'in kendi `response.Envelope` tipiyle
// çözülür; sözleşme iki tarafta aynı koddan gelir.
func apiHealth(ctx context.Context, client *http.Client, params json.RawMessage) (any, error) {
	var p struct {
		BaseURL string `json:"base_url"`
	}
	if err := rpc.Decode(params, &p); err != nil {
		return nil, err
	}
	base, err := normalizeBase(p.BaseURL)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := get(ctx, client, base+"/healthz", nil); err != nil {
		return nil, err
	}
	res := healthResult{BaseURL: base, Healthy: true, LatencyMS: time.Since(start).Milliseconds()}

	var cfg struct {
		Name string `json:"name"`
	}
	env := response.Envelope{Data: &cfg}
	if err := get(ctx, client, base+"/v1/app/config", &env); err != nil {
		return nil, err
	}
	if !env.Success {
		msg := "beklenmeyen yanıt"
		if env.Error != nil {
			msg = env.Error.Code + ": " + env.Error.Message
		}
		return nil, rpc.Errorf(rpc.CodeUnavailable, "API yapılandırması alınamadı: %s", msg)
	}
	res.AppName = cfg.Name
	res.RequestID = env.Meta.RequestID
	return res, nil
}

func normalizeBase(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	raw = strings.TrimSuffix(raw, "/v1")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", rpc.Errorf(rpc.CodeInvalidParams, "base_url geçerli bir http(s) adresi olmalı")
	}
	// Düz http yalnızca yerel geliştirmede.
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return "", rpc.Errorf(rpc.CodeInvalidParams, "uzak sunucu için https gerekli")
	}
	return u.String(), nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func get(ctx context.Context, client *http.Client, u string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return rpc.Errorf(rpc.CodeInvalidParams, "%v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return rpc.Errorf(rpc.CodeUnavailable, "sunucuya ulaşılamadı: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return rpc.Errorf(rpc.CodeUnavailable, "yanıt okunamadı: %v", err)
	}
	if into != nil && len(body) > 0 {
		if err := json.Unmarshal(body, into); err != nil {
			return rpc.Errorf(rpc.CodeUnavailable, "%s: JSON değil (HTTP %d)", u, resp.StatusCode)
		}
	}
	if resp.StatusCode >= 300 {
		var env response.Envelope
		if json.Unmarshal(body, &env) == nil && env.Error != nil {
			return rpc.Errorf(rpc.CodeUnavailable, "HTTP %d %s: %s", resp.StatusCode, env.Error.Code, env.Error.Message)
		}
		return rpc.Errorf(rpc.CodeUnavailable, "HTTP %d", resp.StatusCode)
	}
	return nil
}
