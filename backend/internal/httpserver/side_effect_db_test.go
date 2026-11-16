package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
)

// TestSideEffectFailuresKeepRequest (issue #311): a notification or activity
// row that cannot be written (here: refused by a trigger) is rolled back to
// its savepoint; the request that caused it answers 2xx and its own record
// is committed. Before #311 the failed insert aborted the request's
// transaction and the request answered 500.
func TestSideEffectFailuresKeepRequest(t *testing.T) {
	e := newSweepEnv(t)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, e.d.URL) // the owner: may create triggers
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	if _, err := owner.Exec(ctx, `
		CREATE FUNCTION sweep_refuse() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'side effect refused by the test'; END $$;
		CREATE TRIGGER sweep_refuse_notifications BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION sweep_refuse();
		CREATE TRIGGER sweep_refuse_activity BEFORE INSERT ON activity_events FOR EACH ROW EXECUTE FUNCTION sweep_refuse();`); err != nil {
		t.Fatal(err)
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		// As the owner: FORCE (000086) hides the rows from a non-superuser
		// owner unless app.system is on (#312).
		tx, err := owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		var n int
		if _, err := tx.Exec(ctx, `SELECT set_config('app.system', 'on', true)`); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	notifBefore := testutil.ToFloat64(metrics.Default.SideEffectFailures("notification"))
	activityBefore := testutil.ToFloat64(metrics.Default.SideEffectFailures("activity"))

	// 1. An invitation: the e-mail notification is the side effect.
	rec := e.do(t, http.MethodPost, "/v1/tenant/invitations", sweepCase{body: `{"email":"yan.etki@example.test","role_key":"member"}`}, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invitation with the notification refused = %d %s", rec.Code, rec.Body)
	}
	if n := count(`SELECT count(*) FROM organization_invitations WHERE email = 'yan.etki@example.test'`); n != 1 {
		t.Fatalf("invitation rows = %d, want 1 (the main record must commit)", n)
	}
	if got := testutil.ToFloat64(metrics.Default.SideEffectFailures("notification")); got <= notifBefore {
		t.Errorf("side_effect_failures_total{name=notification} = %v, want > %v", got, notifBefore)
	}

	// 2. A review decision: the activity row is the side effect.
	eng := e.param(t, "engagementUuid", "")
	inv := e.param(t, "invoiceUuid", "")
	rec = e.do(t, http.MethodPost, "/v1/tenant/engagements/"+eng+"/invoices/"+inv+"/account-suggestions", sweepCase{}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("account suggestions = %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Data struct {
			Items []struct {
				UUID   string `json:"uuid"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var pending string
	for _, it := range out.Data.Items {
		if it.Status == "pending" {
			pending = it.UUID
			break
		}
	}
	if pending == "" {
		t.Fatalf("no pending suggestion to accept: %s", firstLine(rec.Body.String()))
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/tenant/engagements/"+eng+"/suggestions/"+pending+"/accept", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.ownerTok)
	req.Header.Set("X-Organization", "demo-smmm")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("accept with the activity row refused = %d %s", w.Code, w.Body)
	}
	if n := count(`SELECT count(*) FROM suggestions WHERE uuid = $1 AND status = 'accepted'`, pending); n != 1 {
		t.Fatalf("suggestion not accepted in the database (rows %d)", n)
	}
	if got := testutil.ToFloat64(metrics.Default.SideEffectFailures("activity")); got <= activityBefore {
		t.Errorf("side_effect_failures_total{name=activity} = %v, want > %v", got, activityBefore)
	}
	if n := count(`SELECT count(*) FROM notifications`) + count(`SELECT count(*) FROM activity_events`); n != 0 {
		t.Fatalf("side-effect rows written despite the trigger: %d", n)
	}
}
