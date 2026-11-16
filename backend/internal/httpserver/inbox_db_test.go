package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestScopedInboxes (issue #333): the office and mükellef panel inboxes list
// only the caller's own in-app notifications of that office / company,
// filter unread and period, mark one or all read, and never touch another
// user's or another scope's notification. The mükellef's notification is
// the office's row (organization_id = office): readable and markable in
// the mükellef's scope since migration 000092.
func TestScopedInboxes(t *testing.T) {
	e := newSweepEnv(t)
	ctx := context.Background()
	var owner, tpUser int64
	err := rls.System(ctx, e.d.Pool, "test: inbox fixture", func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = 'sahip@demo.hakan.local'`).Scan(&owner); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = 'mukellef@demo.hakan.local'`).Scan(&tpUser); err != nil {
			return err
		}
		ins := `INSERT INTO notifications (organization_id, user_id, channel, status, priority, title, body, payload, created_at)
			VALUES ($1, $2, $3, 'delivered', 'normal', $4, '', $5::jsonb, $6::timestamptz)`
		for _, r := range []struct {
			org     any
			user    int64
			channel string
			title   string
			payload string
			at      string
		}{
			{e.orgID, owner, "inapp", "ofis-ekim", `{}`, "2026-10-10T09:00:00+03:00"},
			{e.orgID, owner, "inapp", "ofis-kasim", `{}`, "2026-11-05T09:00:00+03:00"},
			{e.orgID, owner, "email", "ofis-eposta", `{}`, "2026-11-05T09:00:00+03:00"},
			{nil, owner, "inapp", "kullanici-ofissiz", `{}`, "2026-11-05T09:00:00+03:00"},
			{e.orgID, tpUser, "inapp", "mukellef-istek", `{"taxpayer_uuid":"` + e.tpUUID + `"}`, "2026-11-06T09:00:00+03:00"},
			{e.orgID, tpUser, "inapp", "baska-sirket", `{"taxpayer_uuid":"00000000-0000-0000-0000-000000000001"}`, "2026-11-06T09:00:00+03:00"},
		} {
			if _, err := tx.Exec(ctx, ins, r.org, r.user, r.channel, r.title, r.payload, r.at); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	type page struct {
		Items []struct {
			ID    string `json:"id"`
			UUID  string `json:"uuid"`
			Title string `json:"title"`
		} `json:"items"`
		Total  int64 `json:"total"`
		Unread int64 `json:"unread"`
	}
	get := func(path string, taxpayer bool) (int, page) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if taxpayer {
			req.Header.Set("Authorization", "Bearer "+e.tpTok)
			req.Header.Set("X-Taxpayer", e.tpUUID)
		} else {
			req.Header.Set("Authorization", "Bearer "+e.ownerTok)
			req.Header.Set("X-Organization", "demo-smmm")
		}
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, req)
		var env struct {
			Data page `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &env)
		return w.Code, env.Data
	}
	titles := func(p page) map[string]bool {
		out := map[string]bool{}
		for _, it := range p.Items {
			out[it.Title] = true
		}
		return out
	}

	// Office inbox: the owner's in-app rows of this office only.
	code, p := get("/v1/tenant/notifications", false)
	if code != http.StatusOK || p.Total != 2 || p.Unread != 2 || !titles(p)["ofis-ekim"] || !titles(p)["ofis-kasim"] {
		t.Fatalf("office inbox = %d %+v", code, p)
	}
	if _, p = get("/v1/tenant/notifications?period=2026-10", false); p.Total != 1 || !titles(p)["ofis-ekim"] {
		t.Fatalf("office inbox period = %+v", p)
	}
	if code, _ = get("/v1/tenant/notifications?period=2026-1", false); code != http.StatusBadRequest {
		t.Fatalf("bad period = %d", code)
	}
	// Mükellef inbox: the company's rows of the user, though they are office rows.
	code, p = get("/v1/taxpayer/notifications", true)
	if code != http.StatusOK || p.Total != 1 || !titles(p)["mukellef-istek"] {
		t.Fatalf("taxpayer inbox = %d %+v", code, p)
	}
	tpNotif := p.Items[0].UUID
	if tpNotif == "" {
		tpNotif = p.Items[0].ID
	}

	post := func(path string, taxpayer bool) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if taxpayer {
			req.Header.Set("Authorization", "Bearer "+e.tpTok)
			req.Header.Set("X-Taxpayer", e.tpUUID)
		} else {
			req.Header.Set("Authorization", "Bearer "+e.ownerTok)
			req.Header.Set("X-Organization", "demo-smmm")
		}
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, req)
		return w.Code
	}
	// The office owner cannot mark the mükellef's notification.
	if c := post("/v1/tenant/notifications/"+tpNotif+"/read", false); c != http.StatusNotFound {
		t.Fatalf("office marking the mükellef's notification = %d, want 404", c)
	}
	if c := post("/v1/taxpayer/notifications/"+tpNotif+"/read", true); c != http.StatusOK {
		t.Fatalf("mükellef marking its notification = %d", c)
	}
	if _, p = get("/v1/taxpayer/notifications?unread=true", true); p.Total != 0 {
		t.Fatalf("taxpayer unread after read = %+v", p)
	}
	if c := post("/v1/tenant/notifications/read-all", false); c != http.StatusOK {
		t.Fatalf("office read-all = %d", c)
	}
	if _, p = get("/v1/tenant/notifications", false); p.Unread != 0 || p.Total != 2 {
		t.Fatalf("office after read-all = %+v", p)
	}
	// The office-less row of the owner stays unread: another scope.
	var unreadElsewhere int
	if err := rls.System(ctx, e.d.Pool, "test: read back", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE title IN ('kullanici-ofissiz', 'baska-sirket') AND read_at IS NULL`).Scan(&unreadElsewhere)
	}); err != nil || unreadElsewhere != 2 {
		t.Fatalf("other scopes' notifications unread = %d %v, want 2", unreadElsewhere, err)
	}
}
