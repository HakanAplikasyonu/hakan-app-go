package helper

// Ofis ayarı, ofis izni ve aktarıcı bilgisayar (luca-ajan-sozlesme.md 1.3
// §1). Luca ile ortaklık yok: yardımcının ofis adına Luca'ya girip aktarım
// yapması için ofisin AÇIK izni gerekir. İzin (kim, ne zaman) sunucuda
// kayıtlıdır; izin yoksa yardımcı görev almaz.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ConsentTextVersion masaüstündeki izin metninin sürümü (metin değişince
// artar; sunucu hangi metnin onaylandığını saklar).
const ConsentTextVersion = "1"

// Consent ofisin Luca yardımcısı izni.
type Consent struct {
	GivenAt     time.Time `json:"given_at"`
	GivenByName string    `json:"given_by_name"`
	AgentName   string    `json:"agent_name,omitempty"`
	TextVersion string    `json:"text_version,omitempty"`
}

// RunnerAgent görevleri yapan (son yoklayan) aktarıcı bilgisayar.
type RunnerAgent struct {
	AgentUUID  string     `json:"agent_uuid"`
	Name       string     `json:"name"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	Online     bool       `json:"online"`
}

// HelperSettings GET /v1/tenant/desktop-helper-settings.
type HelperSettings struct {
	LucaEnabled bool         `json:"luca_enabled"`
	LucaConsent *Consent     `json:"luca_consent"`
	LucaRunner  *RunnerAgent `json:"luca_runner"`
}

const settingsPath = "/v1/tenant/desktop-helper-settings"

// Settings ofis ayarını okur.
func (c TaskClient) Settings(ctx context.Context) (HelperSettings, error) {
	var s HelperSettings
	err := c.T.DoJSON(ctx, http.MethodGet, settingsPath, nil, &s)
	return s, err
}

// GiveConsent ofis iznini kaydeder (kim: oturumdaki kullanıcı, ne zaman:
// sunucu saati).
func (c TaskClient) GiveConsent(ctx context.Context, agentUUID string) (HelperSettings, error) {
	var s HelperSettings
	err := c.T.DoJSON(ctx, http.MethodPost, settingsPath+"/luca-consent",
		map[string]string{"agent_uuid": agentUUID, "text_version": ConsentTextVersion}, &s)
	return s, err
}

// WithdrawConsent ofis iznini geri alır.
func (c TaskClient) WithdrawConsent(ctx context.Context) (HelperSettings, error) {
	var s HelperSettings
	err := c.T.DoJSON(ctx, http.MethodDelete, settingsPath+"/luca-consent", nil, &s)
	return s, err
}

// ErrExportOpen seçilen mükelleflerin o dönem için zaten açık aktarım
// görevi var (409 EXPORT_OPEN, sözleşme 1.2).
var ErrExportOpen = errors.New("seçilen mükelleflerin bu dönem için zaten açık bir Luca aktarımı var; yeni görev açılmadı")

// ExportRequest POST …/agent-tasks/luca-export gövdesi.
type ExportRequest struct {
	EngagementUUIDs []string `json:"engagement_uuids"`
	Period          string   `json:"period"`
	Kinds           []string `json:"kinds,omitempty"`
}

// ExportRun luca-export yanıtı.
type ExportRun struct {
	RunUUID string `json:"run_uuid"`
	Items   []Task `json:"items"`
}

// StartExport "Luca'ya aktar" iş sırası açar. Hiçbir görev açılamadıysa
// (hepsinin açık görevi var) ErrExportOpen döner.
func (c TaskClient) StartExport(ctx context.Context, req ExportRequest) (ExportRun, error) {
	var run ExportRun
	err := c.T.DoJSON(ctx, http.MethodPost, tasksPath+"/luca-export", req, &run)
	if status, code := apiStatus(err); status == http.StatusConflict && code == "EXPORT_OPEN" {
		return ExportRun{}, ErrExportOpen
	}
	return run, err
}

// openQuery yoklama sorgusu; agent_uuid ile sunucu bu bilgisayarı
// "aktarıcı" olarak görür (son yoklama zamanı → çevrimiçi göstergesi).
func openQuery(agentUUID string) string {
	q := url.Values{}
	q.Set("open", "true")
	q.Set("limit", "100")
	if agentUUID != "" {
		q.Set("agent_uuid", agentUUID)
	}
	return tasksPath + "?" + q.Encode()
}
