package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWhatsAppExtensionFlow (issue #388): the office user pairs the
// extension with a one-time code, the extension's own session links a chat
// to a taxpayer, saves a clicked PDF as a document (stored, downloadable,
// replayed on a second click), a photo as an expense (receipt capture) and
// a PDF into an open document task; a PDF as an expense is refused.
func TestWhatsAppExtensionFlow(t *testing.T) {
	e := newSweepEnv(t)
	type env struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	call := func(tok, method, path, contentType string, body io.Reader) (int, env) {
		t.Helper()
		if body == nil {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("X-Organization", "demo-smmm")
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		var out env
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	upload := func(tok, path string, fields map[string]string, name, data string) (int, env) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte(data))
		_ = mw.Close()
		return call(tok, http.MethodPost, path, mw.FormDataContentType(), &buf)
	}

	// Pairing: the panel creates a code, the extension redeems it once.
	code, out := call(e.ownerTok, http.MethodPost, "/v1/tenant/whatsapp/pairings", "", nil)
	if code != http.StatusCreated {
		t.Fatalf("pairing: %d %s", code, out.Data)
	}
	var pairing struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(out.Data, &pairing)
	if len(pairing.Code) != 9 || pairing.Code[4] != '-' {
		t.Fatalf("code = %q", pairing.Code)
	}
	pairBody := `{"code":"` + strings.ToLower(pairing.Code) + `","device_name":"Chrome · Ofis"}`
	code, out = call("", http.MethodPost, "/v1/auth/whatsapp/pair", "application/json", strings.NewReader(pairBody))
	if code != http.StatusOK {
		t.Fatalf("pair: %d %s", code, out.Data)
	}
	var paired struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Organization struct {
			Slug string `json:"slug"`
		} `json:"organization"`
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	_ = json.Unmarshal(out.Data, &paired)
	if paired.AccessToken == "" || paired.RefreshToken == "" || paired.Organization.Slug != "demo-smmm" || paired.User.Email != "sahip@demo.hakan.local" {
		t.Fatalf("paired = %+v", paired)
	}
	if code, out = call("", http.MethodPost, "/v1/auth/whatsapp/pair", "application/json", strings.NewReader(pairBody)); code != http.StatusNotFound || out.Error.Code != "PAIRING_CODE_INVALID" {
		t.Fatalf("second redeem: %d %s", code, out.Error.Code)
	}
	tok := paired.AccessToken
	total := func(path string) int64 {
		t.Helper()
		code, out := call(e.ownerTok, http.MethodGet, path, "", nil)
		var p struct {
			Total int64 `json:"total"`
		}
		_ = json.Unmarshal(out.Data, &p)
		if code != http.StatusOK {
			t.Fatalf("%s: %d", path, code)
		}
		return p.Total
	}
	// The demo seed already holds the demo chat and two saves.
	seededCaptures, seededLinks := total("/v1/tenant/whatsapp/captures"), total("/v1/tenant/whatsapp/chat-links")

	// Chat → taxpayer: unknown first, remembered after the link.
	engUUID := e.one(t, `SELECT uuid::text FROM engagements WHERE id = $2 AND $1 > 0 AND $3 > 0`, "engagementUuid")
	lookup := func() (key string, linked string) {
		t.Helper()
		code, out := call(tok, http.MethodGet, "/v1/tenant/whatsapp/chat-lookup?chat_key=905551112233%40c.us", "", nil)
		if code != http.StatusOK {
			t.Fatalf("lookup: %d %s", code, out.Data)
		}
		var l struct {
			ChatKey string `json:"chat_key"`
			Link    *struct {
				Taxpayer struct {
					EngagementUUID string `json:"engagement_uuid"`
				} `json:"taxpayer"`
			} `json:"link"`
		}
		_ = json.Unmarshal(out.Data, &l)
		if l.Link != nil {
			linked = l.Link.Taxpayer.EngagementUUID
		}
		return l.ChatKey, linked
	}
	if key, linked := lookup(); key != "905551112233" || linked != "" {
		t.Fatalf("lookup before link: %q %q", key, linked)
	}
	eng := "/v1/tenant/engagements/" + engUUID + "/whatsapp"
	if code, out = call(tok, http.MethodPut, eng+"/chat-link", "application/json", strings.NewReader(`{"chat_key":"+90 555 111 22 33","label":"Atlas"}`)); code != http.StatusOK {
		t.Fatalf("link: %d %s", code, out.Data)
	}
	if _, linked := lookup(); linked != engUUID {
		t.Fatalf("linked to %q, want %s", linked, engUUID)
	}
	if code, _ = call(tok, http.MethodGet, "/v1/tenant/whatsapp/chat-lookup?chat_key=merhaba", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad chat key: %d", code)
	}

	// Document without a task: stored, downloadable, replayed.
	pdf := "%PDF-1.4\n% whatsapp test\n"
	fields := map[string]string{"target": "document", "chat_key": "905551112233@c.us"}
	code, out = upload(tok, eng+"/captures", fields, "kira.pdf", pdf)
	if code != http.StatusCreated {
		t.Fatalf("document: %d %s", code, out.Data)
	}
	var doc struct {
		UUID     string `json:"uuid"`
		HasFile  bool   `json:"has_file"`
		Replayed bool   `json:"replayed"`
	}
	_ = json.Unmarshal(out.Data, &doc)
	if !doc.HasFile || doc.Replayed {
		t.Fatalf("document = %+v", doc)
	}
	code, out = upload(tok, eng+"/captures", fields, "kira.pdf", pdf)
	var again struct {
		UUID     string `json:"uuid"`
		Replayed bool   `json:"replayed"`
	}
	_ = json.Unmarshal(out.Data, &again)
	if code != http.StatusOK || !again.Replayed || again.UUID != doc.UUID {
		t.Fatalf("second save: %d %+v", code, again)
	}
	req := httptest.NewRequest(http.MethodGet, eng+"/captures/"+doc.UUID+"/file", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Organization", "demo-smmm")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != pdf || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("file: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Disposition"))
	}

	// Gider / gelir are read from a photo.
	if code, out = upload(tok, eng+"/captures", map[string]string{"target": "expense"}, "fatura.pdf", pdf); code != http.StatusUnsupportedMediaType || out.Error.Code != "IMAGE_REQUIRED" {
		t.Fatalf("expense pdf: %d %s", code, out.Error.Code)
	}
	code, out = upload(tok, eng+"/captures", map[string]string{"target": "expense"}, "fis.png", pngOf(0xe0))
	var exp struct {
		ReceiptUUID *string `json:"receipt_uuid"`
	}
	_ = json.Unmarshal(out.Data, &exp)
	if code != http.StatusCreated || exp.ReceiptUUID == nil {
		t.Fatalf("expense: %d %s", code, out.Data)
	}

	// A document that answers an open document request.
	code, out = call(e.ownerTok, http.MethodPost, "/v1/tenant/engagements/"+engUUID+"/document-tasks", "application/json",
		strings.NewReader(`{"title":"Kira sözleşmesi","due_days":7}`))
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("task: %d %s", code, out.Data)
	}
	var task struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(out.Data, &task)
	code, out = upload(tok, eng+"/captures", map[string]string{"target": "document", "task_uuid": task.UUID}, "sozlesme.pdf", "%PDF-1.4\n% task\n")
	var answered struct {
		TaskUUID *string `json:"task_uuid"`
		HasFile  bool    `json:"has_file"`
	}
	_ = json.Unmarshal(out.Data, &answered)
	if code != http.StatusCreated || answered.TaskUUID == nil || *answered.TaskUUID != task.UUID || !answered.HasFile {
		t.Fatalf("task document: %d %s", code, out.Data)
	}
	code, out = call(e.ownerTok, http.MethodGet, "/v1/tenant/engagements/"+engUUID+"/document-tasks/"+task.UUID, "", nil)
	var got struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(out.Data, &got)
	if code != http.StatusOK || got.Status != "submitted" {
		t.Fatalf("task after upload: %d %s", code, got.Status)
	}

	// The panel's list.
	if n := total("/v1/tenant/whatsapp/captures"); n != seededCaptures+3 {
		t.Fatalf("captures: %d, want %d", n, seededCaptures+3)
	}
	if n := total("/v1/tenant/whatsapp/captures?target=document"); n < 2 {
		t.Fatalf("document captures: %d", n)
	}
	code, out = call(e.ownerTok, http.MethodGet, "/v1/tenant/whatsapp/chat-links?limit=1", "", nil)
	var links struct {
		Items []struct {
			UUID    string `json:"uuid"`
			ChatKey string `json:"chat_key"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(out.Data, &links)
	if code != http.StatusOK || links.Total != seededLinks+1 || len(links.Items) != 1 || links.Items[0].ChatKey != "905551112233" {
		t.Fatalf("links: %d %s", code, out.Data)
	}
	if code, _ = call(e.ownerTok, http.MethodDelete, "/v1/tenant/whatsapp/chat-links/"+links.Items[0].UUID, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete link: %d", code)
	}
	if _, linked := lookup(); linked != "" {
		t.Fatalf("still linked: %q", linked)
	}
}
