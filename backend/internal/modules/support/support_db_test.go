package support

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func data(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatal(err)
	}
}

func asOffice(r *http.Request, orgID int64, user int64) *http.Request {
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: orgID, UUID: uuid.New()})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: user, UserID: uuid.New()})
	return r.WithContext(ctx)
}

// Ofis kendi widget kimliğini kaydeder; mükellef portalı kendi ofisininkini
// alır, başka ofisin ayarı karışmaz; ofisi olmayan mükellefte office null.
func TestOfficeWidgetReachesItsTaxpayers(t *testing.T) {
	d := testdb.Open(t)
	h := NewHandler(db.New(d.Pool), "")
	orgA, orgB, user := d.Organization(t), d.Organization(t), d.User(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, orgA, tp)

	put := func(org int64, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.PutSettings(rec, asOffice(httptest.NewRequest("PUT", "/v1/tenant/support-settings", strings.NewReader(body)), org, user))
		return rec
	}
	if rec := put(orgA, `{"clickfix_widget_id":" ofis-a "}`); rec.Code != http.StatusOK {
		t.Fatalf("put A: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(orgB, `{"clickfix_widget_id":"ofis-b"}`); rec.Code != http.StatusOK {
		t.Fatalf("put B: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(orgA, `{"clickfix_widget_id":"<script>"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid id: %d", rec.Code)
	}
	if rec := put(orgA, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing field: %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	h.GetSettings(rec, asOffice(httptest.NewRequest("GET", "/v1/tenant/support-settings", nil), orgA, user))
	var set Settings
	data(t, rec, &set)
	if set.ClickFixWidgetID != "ofis-a" {
		t.Fatalf("office A setting = %q", set.ClickFixWidgetID)
	}

	portal := func() TaxpayerSupport {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/taxpayer/support", nil)
		r = r.WithContext(taxpayerctx.WithScope(r.Context(), taxpayerctx.Scope{ID: tp, Role: "owner"}))
		h.TaxpayerSupport(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("portal: %d %s", rec.Code, rec.Body.String())
		}
		var out TaxpayerSupport
		data(t, rec, &out)
		return out
	}
	got := portal()
	if got.Office == nil || got.Office.ClickFixWidgetID != "ofis-a" || got.Office.Name == "" {
		t.Fatalf("portal support = %+v", got.Office)
	}

	// Kimliği boşaltan ofis: portal platform varsayılanına düşer ("" gelir).
	if rec := put(orgA, `{"clickfix_widget_id":""}`); rec.Code != http.StatusOK {
		t.Fatalf("clear: %d", rec.Code)
	}
	if got := portal(); got.Office == nil || got.Office.ClickFixWidgetID != "" {
		t.Fatalf("cleared = %+v", got.Office)
	}

	d.EndEngagement(t, eng)
	if got := portal(); got.Office != nil {
		t.Fatalf("no office, got %+v", got.Office)
	}
}

func TestClickFixIdentitySignsMemberAndOffice(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	r := httptest.NewRequest("GET", "/v1/tenant/support/clickfix-identity", nil)
	r = r.WithContext(authctx.WithPrincipal(orgctx.WithScope(r.Context(), orgctx.Scope{ID: 1, UUID: org}), authctx.Principal{UserID: user}))

	rec := httptest.NewRecorder()
	NewHandler(nil, "").ClickFixIdentity(rec, r)
	var off Identity
	data(t, rec, &off)
	if off.Available || off.UserHash != "" {
		t.Fatalf("without secret = %+v", off)
	}

	rec = httptest.NewRecorder()
	NewHandler(nil, "s3cret").ClickFixIdentity(rec, r)
	var on Identity
	data(t, rec, &on)
	if !on.Available || on.UserID != user.String() || on.CompanyID != org.String() ||
		on.UserHash != UserHash("s3cret", user.String(), org.String()) {
		t.Fatalf("with secret = %+v", on)
	}
}
