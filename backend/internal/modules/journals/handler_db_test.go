package journals

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

func (e env) do(t *testing.T, fn http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body != "" {
		rd = bytes.NewReader([]byte(body))
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, "/x", rd)
	for k, v := range pv {
		r.SetPathValue(k, v)
	}
	ctx := engagementctx.WithScope(r.Context(), e.scope)
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: e.actor})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: e.scope.OrganizationID, Permissions: []string{rbac.PermTenantEngagementsReadAll}})
	w := httptest.NewRecorder()
	// As in the API (issue #166): one row level security transaction per
	// request.
	rls.Middleware(e.pool, nil, nil)(fn).ServeHTTP(w, r.WithContext(ctx))
	return w
}

func data[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data  T
		Error struct {
			Code    string
			Details []struct{ Code, Field string }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
	return env.Data
}

// #143: taslak oluştur → onayla → Luca çıktısı → dosya indir → geri al →
// yeniden çıktı; hepsi HTTP üzerinden.
func TestJournalHTTPFlow(t *testing.T) {
	e := setup(t)
	h := NewHandler(e.svc, e.store)

	// Unbalanced draft: created, approval refused with JOURNAL_UNBALANCED.
	w := e.do(t, h.createDraft, "POST", `{"entry_date":"2026-10-20T00:00:00Z","description":"dengesiz",
		"lines":[{"account":"770.01","debit":"100"},{"account":"320.01","credit":"90"}]}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create unbalanced %d %s", w.Code, w.Body)
	}
	bad := data[Draft](t, w)
	if w := e.do(t, h.approve, "POST", "", map[string]string{"draftUuid": bad.UUID.String()}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "JOURNAL_UNBALANCED") {
		t.Fatalf("approve unbalanced %d %s", w.Code, w.Body)
	}

	w = e.do(t, h.createDraft, "POST", `{"entry_date":"2026-10-15T00:00:00Z","description":"kira",
		"lines":[{"account":"770.01","debit":"100"},{"account":"320.01","credit":"100"}]}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body)
	}
	d := data[Draft](t, w)
	if d.Period != "2026-10" || d.Lines != 2 {
		t.Fatalf("draft %+v", d)
	}
	if w := e.do(t, h.createDraft, "POST", `{"entry_date":"2026-10-15T00:00:00Z","lines":[{"account":"770","debit":"1","credit":"1"}]}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid line = %d", w.Code)
	}

	w = e.do(t, h.approve, "POST", "", map[string]string{"draftUuid": d.UUID.String()})
	if w.Code != http.StatusOK || data[DraftDetail](t, w).Status != "approved" {
		t.Fatalf("approve %d %s", w.Code, w.Body)
	}
	w = e.do(t, h.getDraft, "GET", "", map[string]string{"draftUuid": d.UUID.String()})
	det := data[DraftDetail](t, w)
	if len(det.Items) != 2 || det.Items[0].Account != "770.01" || !det.Debit.Equal(det.Credit) {
		t.Fatalf("detail %+v", det)
	}
	if p := data[DraftPage](t, e.do(t, h.listDrafts, "GET", "", nil)); p.Total != 2 {
		t.Fatalf("list %+v", p)
	}

	// Drivers include luca and zirve.
	var names []string
	for _, di := range data[struct{ Items []struct{ Name string } }](t, e.do(t, h.drivers, "GET", "", nil)).Items {
		names = append(names, di.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "luca") || !strings.Contains(strings.Join(names, ","), "zirve") {
		t.Fatalf("drivers = %v", names)
	}

	if w := e.do(t, h.export, "POST", `{"driver":"luca"}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("export without selection = %d", w.Code)
	}
	if w := e.do(t, h.export, "POST", `{"driver":"nope","period":"2026-10"}`, nil); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "UNKNOWN_DRIVER") {
		t.Fatalf("unknown driver %d %s", w.Code, w.Body)
	}
	w = e.do(t, h.export, "POST", `{"driver":"luca","drafts":["`+d.UUID.String()+`"]}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("export %d %s", w.Code, w.Body)
	}
	b := data[BatchView](t, w)
	if b.Driver != "luca" || len(b.Files) != 1 || b.Entries != 1 {
		t.Fatalf("batch %+v", b)
	}
	if w := e.do(t, h.export, "POST", `{"driver":"luca","drafts":["`+d.UUID.String()+`"]}`, nil); w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusConflict {
		t.Fatalf("second export of the same draft = %d %s", w.Code, w.Body)
	}

	list := data[struct{ Items []BatchSummary }](t, e.do(t, h.listBatches, "GET", "", nil))
	if len(list.Items) != 1 || list.Items[0].Entries != 1 || len(list.Items[0].Files) != 1 {
		t.Fatalf("batches %+v", list)
	}
	name := list.Items[0].Files[0].Name
	w = e.do(t, h.download, "GET", "", map[string]string{"batchUuid": b.UUID.String(), "fileName": name})
	if w.Code != http.StatusOK || !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) || !strings.Contains(w.Header().Get("Content-Disposition"), name) {
		t.Fatalf("download %d %q", w.Code, w.Header().Get("Content-Disposition"))
	}
	if w := e.do(t, h.download, "GET", "", map[string]string{"batchUuid": b.UUID.String(), "fileName": "../../etc/passwd"}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown file name = %d", w.Code)
	}

	if w := e.do(t, h.reverse, "POST", `{"reason":""}`, map[string]string{"draftUuid": d.UUID.String()}); w.Code != http.StatusBadRequest {
		t.Fatalf("reverse without reason = %d", w.Code)
	}
	w = e.do(t, h.reverse, "POST", `{"reason":"hesap kodu düzeltildi"}`, map[string]string{"draftUuid": d.UUID.String()})
	if w.Code != http.StatusOK || data[DraftDetail](t, w).Status != "approved" {
		t.Fatalf("reverse %d %s", w.Code, w.Body)
	}
	got := data[BatchDetail](t, e.do(t, h.getBatch, "GET", "", map[string]string{"batchUuid": b.UUID.String()}))
	if got.Reversed != 1 || got.Items[0].ReversalReason != "hesap kodu düzeltildi" {
		t.Fatalf("batch after reverse %+v", got)
	}
	if w := e.do(t, h.export, "POST", `{"driver":"zirve","period":"2026-10"}`, nil); w.Code != http.StatusCreated {
		t.Fatalf("re-export to zirve %d %s", w.Code, w.Body)
	}

	// Settings round trip.
	if st := data[Settings](t, e.do(t, h.getSettings, "GET", "", nil)); !st.BlockOnUnknownAccount || st.InvoicesReachLucaDirectly {
		t.Fatalf("default settings %+v", st)
	}
	w = e.do(t, h.putSettings, "PUT", `{"invoices_reach_luca_directly":true,"block_on_unknown_account":false}`, nil)
	if st := data[Settings](t, w); !st.InvoicesReachLucaDirectly || st.BlockOnUnknownAccount {
		t.Fatalf("saved settings %d %+v", w.Code, st)
	}
	_ = context.Background()
}
