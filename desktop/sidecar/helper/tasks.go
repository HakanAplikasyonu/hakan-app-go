package helper

// Görev API'si (luca-ajan-sozlesme.md sürüm 1): masaüstü görevleri yoklar,
// üstlenir, ilerlemeyi ve her dosyanın sonucunu hemen, en sonda görevin
// sonucunu bildirir.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Görev türleri.
const (
	KindCariSync   = "luca_cari_sync"
	KindBulkExport = "luca_bulk_export"
)

// Görev durumları.
const (
	StatusQueued      = "queued"
	StatusClaimed     = "claimed"
	StatusWaitingUser = "waiting_user"
	StatusRunning     = "running"
)

// Görev sonuçları (POST …/result outcome).
const (
	OutcomeDone       = "done"
	OutcomeFailed     = "failed"
	OutcomeNeedsCheck = "needs_check"
)

// Ajanın hata kodları (sözleşme §3).
const (
	CodeLoginTimeout = "LOGIN_TIMEOUT"
	CodeFirmMismatch = "FIRM_MISMATCH"
	CodeFirmNotFound = "FIRM_NOT_FOUND"
	CodeScreen       = "SCREEN_CHANGED"
	CodeDomain       = "DOMAIN_BLOCKED"
	CodeRate         = "RATE_LIMITED"
	CodeBrowser      = "BROWSER_CLOSED"
	CodeUnread       = "UPLOAD_RESULT_UNREAD"
	CodeCancelled    = "USER_CANCELLED"
)

// Taxpayer görevdeki mükellef (firma doğrulaması için).
type Taxpayer struct {
	Title string `json:"title"`
	TaxID string `json:"tax_id"`
}

// TaskAgent görevi üstlenen ajan.
type TaskAgent struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// TaskProgress ilerleme.
type TaskProgress struct {
	Step    int    `json:"step"`
	Steps   int    `json:"steps"`
	Message string `json:"message"`
}

// TaskDocument dosyadaki bir belge.
type TaskDocument struct {
	DocumentKey  string `json:"document_key"`
	DocumentNo   string `json:"document_no"`
	DocumentDate string `json:"document_date"`
}

// TaskFile görevin bir dosyası (AgentTaskFile).
type TaskFile struct {
	UUID        string            `json:"uuid"`
	Sequence    int               `json:"sequence"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
	SHA256      string            `json:"sha256"`
	Entries     int               `json:"entries"`
	Luca        map[string]string `json:"luca,omitempty"`
	Documents   []TaskDocument    `json:"documents,omitempty"`
	Status      string            `json:"status"`
}

// TaskCounterparty görevin bir carisi (AgentTaskCounterparty).
type TaskCounterparty struct {
	TaxID            string `json:"tax_id"`
	Title            string `json:"title"`
	MainCode         string `json:"main_code"`
	Status           string `json:"status"`
	AccountCode      string `json:"account_code,omitempty"`
	HakanAccountCode string `json:"hakan_account_code,omitempty"`
}

// Task sunucudaki görev (AgentTask).
type Task struct {
	UUID           string             `json:"uuid"`
	RunUUID        *string            `json:"run_uuid"`
	Sequence       int                `json:"sequence"`
	Kind           string             `json:"kind"`
	EngagementUUID string             `json:"engagement_uuid"`
	Taxpayer       Taxpayer           `json:"taxpayer"`
	Period         *string            `json:"period"`
	Status         string             `json:"status"`
	Approved       bool               `json:"approved"`
	Agent          *TaskAgent         `json:"agent"`
	Progress       *TaskProgress      `json:"progress"`
	Files          []TaskFile         `json:"files"`
	Counterparties []TaskCounterparty `json:"counterparties"`
	ErrorCode      string             `json:"error_code,omitempty"`
}

func (t Task) run() string {
	if t.RunUUID == nil {
		return ""
	}
	return *t.RunUUID
}

// EntryResult hatalı satır raporunun bir satırı.
type EntryResult struct {
	EntryNo     string `json:"entry_no,omitempty"`
	DocumentNo  string `json:"document_no,omitempty"`
	Row         int    `json:"row,omitempty"`
	Status      string `json:"status"`
	LucaEntryNo string `json:"luca_entry_no,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// FileResult POST …/files/{f}/result gövdesi.
type FileResult struct {
	AgentUUID   string        `json:"agent_uuid"`
	Status      string        `json:"status"` // ok | failed | unknown
	LucaMessage string        `json:"luca_message,omitempty"`
	Entries     []EntryResult `json:"entries"`
}

// CounterpartyResult görev sonucundaki cari.
type CounterpartyResult struct {
	TaxID       string `json:"tax_id"`
	MainCode    string `json:"main_code"`
	Status      string `json:"status"` // found | created | failed
	AccountCode string `json:"account_code,omitempty"`
	Title       string `json:"title,omitempty"`
	Message     string `json:"message,omitempty"`
}

// TaskResult POST …/result gövdesi.
type TaskResult struct {
	AgentUUID      string               `json:"agent_uuid"`
	Outcome        string               `json:"outcome"`
	ErrorCode      string               `json:"error_code,omitempty"`
	ErrorMessage   string               `json:"error_message,omitempty"`
	Counterparties []CounterpartyResult `json:"counterparties,omitempty"`
}

// ProgressReport POST …/progress gövdesi.
type ProgressReport struct {
	AgentUUID string `json:"agent_uuid"`
	Status    string `json:"status"` // waiting_user | running
	Step      int    `json:"step,omitempty"`
	Steps     int    `json:"steps,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Transport HAKAN API'sine istek (agent.Client karşılar).
type Transport interface {
	DoJSON(ctx context.Context, method, path string, in, out any) error
	Download(ctx context.Context, path string) ([]byte, error)
}

// HTTPStatuser API hatasının HTTP durumunu ve kodunu verir.
type HTTPStatuser interface {
	HTTPStatus() int
	ErrorCode() string
}

func apiStatus(err error) (int, string) {
	var s HTTPStatuser
	if errors.As(err, &s) {
		return s.HTTPStatus(), s.ErrorCode()
	}
	return 0, ""
}

// TaskClient görev uçları.
type TaskClient struct{ T Transport }

const tasksPath = "/v1/tenant/agent-tasks"

func taskPath(uuid string) string { return tasksPath + "/" + url.PathEscape(uuid) }

// Open açık görevler (yoklama).
func (c TaskClient) Open(ctx context.Context) ([]Task, error) {
	var page struct {
		Items []Task `json:"items"`
	}
	err := c.T.DoJSON(ctx, http.MethodGet, tasksPath+"?open=true&limit=100", nil, &page)
	return page.Items, err
}

// Claim görevi üstlenir (aynı ajan için idempotent).
func (c TaskClient) Claim(ctx context.Context, taskUUID, agentUUID string) (Task, error) {
	var t Task
	err := c.T.DoJSON(ctx, http.MethodPost, taskPath(taskUUID)+"/claim", map[string]string{"agent_uuid": agentUUID}, &t)
	return t, err
}

// Progress ilerleme bildirir.
func (c TaskClient) Progress(ctx context.Context, taskUUID string, p ProgressReport) error {
	return c.T.DoJSON(ctx, http.MethodPost, taskPath(taskUUID)+"/progress", p, nil)
}

// FileResultPath dosya sonucunun ucu.
func FileResultPath(taskUUID, fileUUID string) string {
	return taskPath(taskUUID) + "/files/" + url.PathEscape(fileUUID) + "/result"
}

// ResultPath görev sonucunun ucu.
func ResultPath(taskUUID string) string { return taskPath(taskUUID) + "/result" }

// Post sonuç gönderir (dosya ya da görev).
func (c TaskClient) Post(ctx context.Context, path string, body any) error {
	return c.T.DoJSON(ctx, http.MethodPost, path, body, nil)
}

// ErrChecksum indirilen dosya görevdeki özetle tutmadı.
var ErrChecksum = errors.New("indirilen dosya görevdeki dosyayla aynı değil (özet tutmadı)")

// DownloadFile görev dosyasını indirir ve SHA-256'yı denetler.
func (c TaskClient) DownloadFile(ctx context.Context, taskUUID string, f TaskFile) ([]byte, error) {
	data, err := c.T.Download(ctx, taskPath(taskUUID)+"/files/"+url.PathEscape(f.UUID)+"/content")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if f.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), f.SHA256) {
		return nil, ErrChecksum
	}
	return data, nil
}
