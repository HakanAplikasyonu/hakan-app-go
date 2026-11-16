package officedash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeStore map[string][]byte

func (f fakeStore) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := f[key]
	if !ok {
		return nil, 0, fmt.Errorf("no object %s", key)
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

type clientsEnv struct {
	d        *testdb.DB
	h        *ClientsHandler
	org      int64
	owner    int64
	member   int64
	engs     []int64
	tps      []int64
	otherOrg int64
}

// Five taxpayers in office A ("Mükellef 90000000001".."05"), the member is
// assigned to the 2nd and 4th; office B has one taxpayer that must never show.
func setupClients(t *testing.T) clientsEnv {
	t.Helper()
	d := testdb.Open(t)
	e := clientsEnv{d: d, org: d.Organization(t), otherOrg: d.Organization(t), owner: d.User(t), member: d.User(t)}
	e.h = &ClientsHandler{Q: database.QueriesFor(d.Pool), Store: fakeStore{"receipt-key": []byte("JPEGDATA")}}
	d.Member(t, e.org, e.owner, "owner")
	d.Member(t, e.org, e.member, "member")
	for i := 1; i <= 5; i++ {
		tp := d.Taxpayer(t, "legal", fmt.Sprintf("90000000%02d", i))
		e.tps = append(e.tps, tp)
		e.engs = append(e.engs, d.Engagement(t, e.org, tp))
	}
	d.Assign(t, e.org, e.engs[1], e.member)
	d.Assign(t, e.org, e.engs[3], e.member)
	tpB := d.Taxpayer(t, "legal", "9100000001")
	d.Engagement(t, e.otherOrg, tpB)
	// An open blocker on taxpayer 3.
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, entity_type, entity_id, code, severity, producer)
		VALUES ($1, $2, $3, 'period', 1, 'TEST_CODE', 'blocker', 'test')`, e.org, e.engs[2], e.tps[2]); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e clientsEnv) list(t *testing.T, user int64, perms []string, query string) (int, ClientPage) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/v1/tenant/clients?"+query, nil)
	ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: e.org, Permissions: perms})
	w := httptest.NewRecorder()
	rls.Middleware(e.d.Pool, database.VisibleTaxpayers(e.d.Pool), nil)(http.HandlerFunc(e.h.List)).ServeHTTP(w, r.WithContext(ctx))
	var env struct{ Data ClientPage }
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, env.Data
}

func (e clientsEnv) engagement(t *testing.T, fn http.HandlerFunc, i int, pv map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var engUUID, tpUUID uuid.UUID
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT e.uuid, t.uuid FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id WHERE e.id = $1`, e.engs[i]).Scan(&engUUID, &tpUUID); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	for k, v := range pv {
		r.SetPathValue(k, v)
	}
	ctx := engagementctx.WithScope(r.Context(), engagementctx.Scope{ID: e.engs[i], UUID: engUUID, OrganizationID: e.org, TaxpayerID: e.tps[i], TaxpayerUUID: tpUUID, Status: "active"})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: e.owner})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: e.org, Permissions: allPerms})
	w := httptest.NewRecorder()
	rls.Middleware(e.d.Pool, nil, nil)(fn).ServeHTTP(w, r.WithContext(ctx))
	return w
}

var allPerms = []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll, rbac.PermTenantReceiptsRead}

func TestClientsKeysetPagesCoverAllInOrder(t *testing.T) {
	e := setupClients(t)
	var titles []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		q := "limit=2"
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		code, p := e.list(t, e.owner, allPerms, q)
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		for _, c := range p.Items {
			titles = append(titles, c.Title)
		}
		if p.NextCursor == nil {
			break
		}
		cursor = *p.NextCursor
	}
	want := []string{"Mükellef 9000000001", "Mükellef 9000000002", "Mükellef 9000000003", "Mükellef 9000000004", "Mükellef 9000000005"}
	if fmt.Sprint(titles) != fmt.Sprint(want) {
		t.Fatalf("titles = %v, want %v (office B must not appear)", titles, want)
	}
}

func TestClientsSearchFilterAndScope(t *testing.T) {
	e := setupClients(t)
	if _, p := e.list(t, e.owner, allPerms, "q=000003"); len(p.Items) != 1 || p.Items[0].OpenBlockers != 1 || p.Items[0].OpenWarnings != 1 {
		t.Fatalf("search by title = %+v", p.Items)
	}
	if _, p := e.list(t, e.owner, allPerms, "q=9000000004"); len(p.Items) != 1 {
		t.Fatalf("search by tax id = %+v", p.Items)
	}
	if _, p := e.list(t, e.owner, allPerms, "q=%25"); len(p.Items) != 0 {
		t.Fatalf("LIKE wildcard must be literal: %+v", p.Items)
	}
	if _, p := e.list(t, e.owner, allPerms, "filter=warnings"); len(p.Items) != 1 || p.Items[0].Title != "Mükellef 9000000003" {
		t.Fatalf("filter=warnings = %+v", p.Items)
	}
	// The member without read_all sees only assigned engagements.
	_, p := e.list(t, e.member, []string{rbac.PermTenantEngagementsRead}, "")
	if len(p.Items) != 2 || !p.Items[0].AssignedToMe || p.Items[0].Title != "Mükellef 9000000002" {
		t.Fatalf("assigned only = %+v", p.Items)
	}
	if _, p := e.list(t, e.owner, allPerms, "filter=mine"); len(p.Items) != 0 {
		t.Fatalf("owner has no assignments, filter=mine = %+v", p.Items)
	}
	if code, _ := e.list(t, e.owner, allPerms, "cursor=bogus"); code != http.StatusBadRequest {
		t.Fatalf("bad cursor = %d", code)
	}
	if code, _ := e.list(t, e.owner, allPerms, "filter=nope"); code != http.StatusBadRequest {
		t.Fatalf("bad filter = %d", code)
	}
}

func TestContactsAndReceiptImage(t *testing.T) {
	e := setupClients(t)
	ctx := context.Background()
	tpUser := e.d.User(t)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, e.tps[0], tpUser); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Pool.Exec(ctx, `UPDATE taxpayers SET contact_phone = '+90 212 555 01 01' WHERE id = $1`, e.tps[0]); err != nil {
		t.Fatal(err)
	}
	w := e.engagement(t, e.h.Contacts, 0, nil)
	var env struct{ Data Contact }
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || w.Code != http.StatusOK {
		t.Fatalf("contacts %d %s", w.Code, w.Body)
	}
	c := env.Data
	if c.Phone == nil || *c.Phone != "+90 212 555 01 01" || c.Email != nil || len(c.Members) != 1 || c.Members[0].Role != "owner" {
		t.Fatalf("contacts = %+v", c)
	}
	// Another taxpayer's engagement has no members and no phone.
	if w := e.engagement(t, e.h.Contacts, 1, nil); w.Code != http.StatusOK || bytes.Contains(w.Body.Bytes(), []byte("555")) {
		t.Fatalf("contacts of engagement 2 leaked: %s", w.Body)
	}

	var att int64
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO attachments (taxpayer_id, kind, storage_key, sha256, size_bytes, content_type)
		VALUES ($1, 'image', 'receipt-key', repeat('a', 64), 8, 'image/jpeg') RETURNING id`, e.tps[0]).Scan(&att); err != nil {
		t.Fatal(err)
	}
	var rid uuid.UUID
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO receipt_captures (organization_id, engagement_id, taxpayer_id, attachment_id, media_type, status)
		VALUES ($1, $2, $3, $4, 'image/jpeg', 'draft') RETURNING uuid`, e.org, e.engs[0], e.tps[0], att).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	w = e.engagement(t, e.h.ReceiptImage, 0, map[string]string{"receiptUuid": rid.String()})
	if w.Code != http.StatusOK || w.Body.String() != "JPEGDATA" || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("image %d %q %s", w.Code, w.Body, w.Header())
	}
	// The receipt belongs to engagement 1: asking through engagement 2 is 404.
	if w := e.engagement(t, e.h.ReceiptImage, 1, map[string]string{"receiptUuid": rid.String()}); w.Code != http.StatusNotFound {
		t.Fatalf("cross-engagement image = %d", w.Code)
	}
}

func TestClientCursorRoundTrip(t *testing.T) {
	c := encodeClientCursor("Atlas | Gıda", 42)
	title, id, err := decodeClientCursor(c)
	if err != nil || title != "Atlas | Gıda" || id != 42 {
		t.Fatalf("round trip = %q %d %v", title, id, err)
	}
	for _, bad := range []string{"", "x", encodeClientCursor("a", 1) + "!"} {
		if _, _, err := decodeClientCursor(bad); err == nil {
			t.Errorf("decode(%q) accepted", bad)
		}
	}
}
