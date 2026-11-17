// Package agent, masaüstü yerel ajanının çalışma zamanıdır (issue #51):
// HAKAN API istemcisi, kalıcı durum, klasör izleme ve heartbeat.
//
// Backend'de yalnızca mevcut uçlar kullanılır: oturum (/v1/auth/*),
// engagement listesi, UBL yükleme (/ubl-uploads) ve ajan kaydı
// (/v1/tenant/agents). Yanıt zarfı backend'in kendi `response.Envelope`
// tipiyle çözülür.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// APIError, API'nin hata zarfından gelir.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// ErrSignedOut: geçerli oturum yok (yenileme belirteci yok ya da reddedildi).
var ErrSignedOut = errors.New("agent: oturum yok")

// Tokens, /v1/auth/login ve /v1/auth/refresh yanıtı.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Client, HAKAN API istemcisidir. Erişim belirteci yalnızca bellekte durur;
// yenileme belirteci değişince OnRotate çağrılır (kabuk anahtar zincirine
// yazar).
type Client struct {
	HTTP     *http.Client
	OnRotate func(refreshToken string)

	mu      sync.Mutex
	base    string
	org     string
	access  string
	refresh string
	expiry  time.Time
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{HTTP: httpClient}
}

// NormalizeBase, kullanıcı girdisini API kök adresine çevirir. Uzak sunucuda
// https zorunlu; düz http yalnızca yerel geliştirmede.
func NormalizeBase(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	raw = strings.TrimSuffix(raw, "/v1")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("API adresi geçerli bir http(s) adresi olmalı")
	}
	h := u.Hostname()
	if u.Scheme == "http" && h != "localhost" && h != "127.0.0.1" && h != "::1" {
		return "", errors.New("uzak sunucu için https gerekli")
	}
	return u.String(), nil
}

func (c *Client) SetBase(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = base
}

func (c *Client) Base() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

// SetOrganization, tenant uçlarına gönderilen X-Organization değeridir.
func (c *Client) SetOrganization(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.org = slug
}

// SetRefreshToken, kabuğun anahtar zincirinden verdiği belirteçle oturumu
// sürdürür; erişim belirteci ilk istekte yenilenir.
func (c *Client) SetRefreshToken(t string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh, c.access, c.expiry = t, "", time.Time{}
}

func (c *Client) SignedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refresh != ""
}

// SignOut belirteçleri bellekten siler.
func (c *Client) SignOut() {
	c.mu.Lock()
	c.refresh, c.access, c.expiry = "", "", time.Time{}
	c.mu.Unlock()
}

func (c *Client) setTokens(t Tokens) {
	c.mu.Lock()
	c.access, c.refresh = t.AccessToken, t.RefreshToken
	// Süre dolmadan bir dakika önce yenile.
	c.expiry = time.Now().Add(time.Duration(t.ExpiresIn)*time.Second - time.Minute)
	cb := c.OnRotate
	c.mu.Unlock()
	if cb != nil {
		cb(t.RefreshToken)
	}
}

// Login, e-posta ve parolayla oturum açar. İki adımlı doğrulama açıksa
// totp boş geçilince API MFA_REQUIRED döner.
func (c *Client) Login(ctx context.Context, email, password, totp string) error {
	body := map[string]string{"email": email, "password": password}
	if totp != "" {
		body["totp_code"] = totp
	}
	var t Tokens
	if err := c.do(ctx, http.MethodPost, "/v1/auth/login", body, &t, false); err != nil {
		return err
	}
	c.setTokens(t)
	return nil
}

// refreshTokens yenileme belirteciyle yeni çift alır. Reddedilirse oturum
// kapanır (ErrSignedOut).
func (c *Client) refreshTokens(ctx context.Context) error {
	c.mu.Lock()
	rt := c.refresh
	c.mu.Unlock()
	if rt == "" {
		return ErrSignedOut
	}
	var t Tokens
	err := c.do(ctx, http.MethodPost, "/v1/auth/refresh", map[string]string{"refresh_token": rt}, &t, false)
	var ae *APIError
	if errors.As(err, &ae) && (ae.Status == http.StatusUnauthorized || ae.Status == http.StatusBadRequest) {
		c.SignOut()
		return ErrSignedOut
	}
	if err != nil {
		return err
	}
	c.setTokens(t)
	return nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok, exp := c.access, c.expiry
	c.mu.Unlock()
	if tok != "" && time.Now().Before(exp) {
		return tok, nil
	}
	if err := c.refreshTokens(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.access, nil
}

// Organization, /v1/auth/me'deki ofis üyeliği.
type Organization struct {
	UUID        string   `json:"uuid"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
}

// Organizations kullanıcının ofislerini döner.
func (c *Client) Organizations(ctx context.Context) ([]Organization, error) {
	var me struct {
		Organizations []Organization `json:"organizations"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/auth/me", nil, &me, true); err != nil {
		return nil, err
	}
	return me.Organizations, nil
}

// Engagement, /v1/tenant/engagements öğesi.
type Engagement struct {
	UUID         string `json:"uuid"`
	TaxpayerUUID string `json:"taxpayer_uuid"`
	// TaxpayerTitle mükellefin unvanı / adı (#206).
	TaxpayerTitle string `json:"taxpayer_title"`
	Status        string `json:"status"`
}

// Engagements etkin hizmet ilişkilerini döner (en çok 500).
func (c *Client) Engagements(ctx context.Context) ([]Engagement, error) {
	var out []Engagement
	for offset := 0; offset < 500; offset += 100 {
		var page struct {
			Items []Engagement `json:"items"`
			Total int          `json:"total"`
		}
		path := fmt.Sprintf("/v1/tenant/engagements?status=active&limit=100&offset=%d", offset)
		if err := c.do(ctx, http.MethodGet, path, nil, &page, true); err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if len(out) >= page.Total || len(page.Items) == 0 {
			break
		}
	}
	return out, nil
}

// UploadJob, UBL yükleme işi.
type UploadJob struct {
	UUID     string `json:"uuid"`
	FileName string `json:"file_name"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// UploadUBL dosyayı mevcut UBL yükleme ucuna gönderir.
func (c *Client) UploadUBL(ctx context.Context, engagementUUID, fileName string, data []byte) (UploadJob, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return UploadJob{}, err
	}
	if _, err := fw.Write(data); err != nil {
		return UploadJob{}, err
	}
	if err := mw.Close(); err != nil {
		return UploadJob{}, err
	}
	var job UploadJob
	path := "/v1/tenant/engagements/" + url.PathEscape(engagementUUID) + "/ubl-uploads"
	err = c.send(ctx, http.MethodPost, path, mw.FormDataContentType(), buf.Bytes(), &job, true)
	return job, err
}

func transferPath(engagementUUID string) string {
	return "/v1/tenant/engagements/" + url.PathEscape(engagementUUID) + "/journal-transfers"
}

// ListTransfers mükellefin bekleyen (queued) ya da üstlenilmiş (claimed)
// aktarım görevleri (#391).
func (c *Client) ListTransfers(ctx context.Context, engagementUUID, status string) ([]RemoteTransfer, error) {
	var page struct {
		Items []RemoteTransfer `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, transferPath(engagementUUID)+"?limit=50&status="+url.QueryEscape(status), nil, &page, true)
	return page.Items, err
}

// ClaimTransfer görevi bu ajana alır.
func (c *Client) ClaimTransfer(ctx context.Context, engagementUUID, transferUUID, agentUUID string) (RemoteTransfer, error) {
	var t RemoteTransfer
	err := c.do(ctx, http.MethodPost, transferPath(engagementUUID)+"/"+url.PathEscape(transferUUID)+"/claim",
		map[string]string{"agent_uuid": agentUUID}, &t, true)
	return t, err
}

// ReportDelivery ajanın sonucunu bildirir.
func (c *Client) ReportDelivery(ctx context.Context, engagementUUID, transferUUID string, rep DeliveryReport) error {
	return c.do(ctx, http.MethodPost, transferPath(engagementUUID)+"/"+url.PathEscape(transferUUID)+"/delivery", rep, nil, true)
}

// MaxExportFileBytes indirilen çıktı dosyası için üst sınır.
const MaxExportFileBytes = 64 << 20

// DownloadExportFile bir çıktı dosyasını mevcut indirme ucundan alır.
func (c *Client) DownloadExportFile(ctx context.Context, engagementUUID, batchUUID, name string) ([]byte, error) {
	path := "/v1/tenant/engagements/" + url.PathEscape(engagementUUID) + "/journal-exports/" +
		url.PathEscape(batchUUID) + "/files/" + url.PathEscape(name)
	for attempt := 0; ; attempt++ {
		data, err := c.raw(ctx, path)
		var ae *APIError
		if attempt == 0 && errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
			c.mu.Lock()
			c.access = ""
			c.mu.Unlock()
			continue
		}
		return data, err
	}
}

// raw yetkili bir GET'in gövdesini zarfsız döner (dosya indirme).
func (c *Client) raw(ctx context.Context, path string) ([]byte, error) {
	c.mu.Lock()
	base, org := c.base, c.org
	c.mu.Unlock()
	if base == "" {
		return nil, errors.New("API adresi ayarlanmadı")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", "hakan-desktop-agent")
	if org != "" {
		req.Header.Set("X-Organization", org)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sunucuya ulaşılamadı: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e := &APIError{Status: resp.StatusCode, Code: "HTTP_ERROR", Message: http.StatusText(resp.StatusCode)}
		var env response.Envelope
		if raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); err == nil && json.Unmarshal(raw, &env) == nil && env.Error != nil {
			e.Code, e.Message = env.Error.Code, env.Error.Message
		}
		return nil, e
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxExportFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxExportFileBytes {
		return nil, errors.New("çıktı dosyası çok büyük")
	}
	return data, nil
}

// AgentInfo, /v1/tenant/agents yanıtı.
type AgentInfo struct {
	UUID             string `json:"uuid"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	HeartbeatSeconds int    `json:"heartbeat_seconds"`
}

// RegisterAgent bu makineyi ofise kaydeder (aynı machine_id → aynı ajan).
func (c *Client) RegisterAgent(ctx context.Context, machineID, name, platform, version string) (AgentInfo, error) {
	var a AgentInfo
	err := c.do(ctx, http.MethodPost, "/v1/tenant/agents", map[string]string{
		"machine_id": machineID, "name": name, "platform": platform, "version": version,
	}, &a, true)
	return a, err
}

// Heartbeat ajanın canlı olduğunu bildirir.
func (c *Client) Heartbeat(ctx context.Context, agentUUID, version string, stats any) (AgentInfo, error) {
	var a AgentInfo
	err := c.do(ctx, http.MethodPost, "/v1/tenant/agents/"+url.PathEscape(agentUUID)+"/heartbeat",
		map[string]any{"version": version, "stats": stats}, &a, true)
	return a, err
}

func (c *Client) do(ctx context.Context, method, path string, in, out any, auth bool) error {
	var body []byte
	ct := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, ct = b, "application/json"
	}
	return c.send(ctx, method, path, ct, body, out, auth)
}

// send isteği gönderir; 401'de belirteci bir kez yenileyip tekrar dener.
func (c *Client) send(ctx context.Context, method, path, contentType string, body []byte, out any, auth bool) error {
	for attempt := 0; ; attempt++ {
		err := c.once(ctx, method, path, contentType, body, out, auth)
		var ae *APIError
		if auth && attempt == 0 && errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
			c.mu.Lock()
			c.access = ""
			c.mu.Unlock()
			continue
		}
		return err
	}
}

func (c *Client) once(ctx context.Context, method, path, contentType string, body []byte, out any, auth bool) error {
	c.mu.Lock()
	base, org := c.base, c.org
	c.mu.Unlock()
	if base == "" {
		return errors.New("API adresi ayarlanmadı")
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hakan-desktop-agent")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if auth {
		tok, err := c.accessToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if org != "" && strings.HasPrefix(path, "/v1/tenant/") {
			req.Header.Set("X-Organization", org)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("sunucuya ulaşılamadı: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	env := response.Envelope{Data: out}
	if err := json.Unmarshal(raw, &env); err != nil {
		if resp.StatusCode >= 300 {
			return &APIError{Status: resp.StatusCode, Code: "HTTP_ERROR", Message: http.StatusText(resp.StatusCode)}
		}
		return fmt.Errorf("yanıt JSON değil: %w", err)
	}
	if resp.StatusCode >= 300 || !env.Success {
		e := &APIError{Status: resp.StatusCode, Code: "HTTP_ERROR", Message: http.StatusText(resp.StatusCode)}
		if env.Error != nil {
			e.Code, e.Message = env.Error.Code, env.Error.Message
		}
		return e
	}
	return nil
}
