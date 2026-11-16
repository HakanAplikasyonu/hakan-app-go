package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Authorization matrix (issue #323). Every operation in openapi.yaml is
// called through the real server, in the production request scope, as six
// callers. The expected answer of each is written down: authzClass (below)
// gives every operation its class, authzExpect gives a class's answer per
// caller. A new operation fails the test until it is classified.
//
// Callers (matrixCallers):
//
//	anon      no token                                   → 401 everywhere but public routes
//	ownerA    owner of office A (demo-smmm)              → office A: allowed
//	readerA   member of office A, read-only custom role  → reads allowed, writes 403
//	ownerB    owner of office B, asks for office A       → 403 (not a member of A)
//	taxX      taxpayer X (Atlas, office A's client)      → taxpayer routes for X: allowed
//	taxY      taxpayer Y (office B's client), asks for X → 403 (not a member of X)
//
// IDOR (TestAuthzIDOR): office B with its own X-Organization and taxpayer Y
// with its own X-Taxpayer ask for office A's / taxpayer X's records by id:
// the answer is 404 and carries none of their content.
//
// Policy (docs/yetki-matrisi.md): a scope the caller does not belong to
// (X-Organization / X-Taxpayer, a platform route) is 403; inside the
// caller's own scope a record of another scope does not exist: 404.

type matrixCaller struct {
	name    string
	token   string
	headers map[string]string
}

// specOps lists every operation (all methods) in spec order.
func specOps(t *testing.T) []specOp {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(backendRoot(), "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pathRe := regexp.MustCompile(`^  (/[^:\s]+):\s*$`)
	opRe := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	var out []specOp
	var current string
	for _, line := range strings.Split(string(raw), "\n") {
		if m := pathRe.FindStringSubmatch(line); m != nil {
			current = strings.Trim(m[1], `"'`)
			continue
		}
		if m := opRe.FindStringSubmatch(line); m != nil && current != "" {
			out = append(out, specOp{strings.ToUpper(m[1]), current})
		}
	}
	return out
}

// authzEnv is the sweep environment plus office B, taxpayer Y and the
// read-only member of office A.
type authzEnv struct {
	*sweepEnv
	orgB             int64
	engB, tpY        int64
	tpYUUID, engBUID string
	readerTok        string
	ownerBTok        string
	tpYTok           string
}

const (
	authzOrgB     = "b-ofis-authz"
	authzPassword = "Authz-Test-1!"
)

func newAuthzEnv(t *testing.T) *authzEnv {
	t.Helper()
	e := &authzEnv{sweepEnv: newSweepEnv(t)}
	ctx := context.Background()
	hash, err := password.Hash(authzPassword)
	if err != nil {
		t.Fatal(err)
	}
	var readerID, ownerBID, tpYUser int64
	err = rls.System(ctx, e.d.Pool, "test: authz fixture", func(ctx context.Context, tx pgx.Tx) error {
		user := func(email string) (int64, error) {
			var id int64
			err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status, email_verified_at)
				VALUES ($1, $2, 'Test', 'Kullanıcı', 'active', NOW()) RETURNING id`, email, hash).Scan(&id)
			return id, err
		}
		if readerID, err = user("okur@authz.example.test"); err != nil {
			return err
		}
		if ownerBID, err = user("sahip@b.authz.example.test"); err != nil {
			return err
		}
		if tpYUser, err = user("mukellef@y.authz.example.test"); err != nil {
			return err
		}
		// Office B: its owner (system owner role), taxpayer Y and its engagement.
		if err := tx.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'B Mali Müşavirlik') RETURNING id`, authzOrgB).Scan(&e.orgB); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
			SELECT $1, $2, id FROM roles WHERE slug = 'owner' AND is_system AND organization_id IS NULL`, e.orgB, ownerBID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title, tax_office) VALUES ('legal', '9000000025', 'Yıldız Tekstil A.Ş.', 'Zincirlikuyu')
			RETURNING id, uuid::text`).Scan(&e.tpY, &e.tpYUUID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id, uuid::text`,
			e.orgB, e.tpY).Scan(&e.engB, &e.engBUID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, e.tpY, tpYUser); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("authz fixture: %v", err)
	}

	// The read-only role of office A: every assignable *.read / *.read_all permission.
	perms := e.readPermissions(t)
	body, _ := json.Marshal(map[string]any{"key": "salt_okur", "name": "Salt okur", "permissions": perms})
	if rec := e.do(t, http.MethodPost, "/v1/tenant/roles", sweepCase{body: string(body)}, false); rec.Code != http.StatusCreated {
		t.Fatalf("create read-only role: %d %s", rec.Code, rec.Body)
	}
	if err := rls.System(ctx, e.d.Pool, "test: authz reader", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id)
			SELECT $1, $2, id FROM roles WHERE organization_id = $1 AND slug = 'salt_okur'`, e.orgID, readerID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.readerTok = e.loginAs(t, "okur@authz.example.test", authzPassword)
	e.ownerBTok = e.loginAs(t, "sahip@b.authz.example.test", authzPassword)
	e.tpYTok = e.loginAs(t, "mukellef@y.authz.example.test", authzPassword)
	return e
}

func (e *authzEnv) loginAs(t *testing.T, email, pass string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"`+pass+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out struct {
		Data authmodel.Tokens `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Data.AccessToken == "" {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body)
	}
	return out.Data.AccessToken
}

// readPermissions are the tenant permissions ending in ".read" the office
// may give a custom role.
func (e *authzEnv) readPermissions(t *testing.T) []string {
	t.Helper()
	rec := e.do(t, http.MethodGet, "/v1/tenant/role-permissions", sweepCase{}, false)
	var out struct {
		Data struct {
			Items []struct {
				Slug string `json:"slug"`
			} `json:"items"`
		} `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("role permissions: %d %s", rec.Code, rec.Body)
	}
	var perms []string
	for _, p := range out.Data.Items {
		if strings.HasSuffix(p.Slug, ".read") || strings.HasSuffix(p.Slug, ".read_all") {
			perms = append(perms, p.Slug)
		}
	}
	if len(perms) == 0 {
		t.Fatalf("no read permissions in %s", rec.Body)
	}
	return perms
}

// callers: who asks, with which scope headers. Office callers name office
// A; taxpayer callers name taxpayer X.
func (e *authzEnv) callers() []matrixCaller {
	a := map[string]string{"X-Organization": "demo-smmm", "X-Taxpayer": e.tpUUID}
	return []matrixCaller{
		{"anon", "", a},
		{"ownerA", e.ownerTok, a},
		{"readerA", e.readerTok, a},
		{"ownerB", e.ownerBTok, a},
		{"taxX", e.tpTok, a},
		{"taxY", e.tpYTok, a},
	}
}

// request sends method path as caller with a body worth sending (the
// sweep's case for writes, {} otherwise).
func (e *authzEnv) request(t *testing.T, c matrixCaller, method, path string, sc sweepCase) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader = strings.NewReader(`{}`)
	if sc.body != "" {
		body = strings.NewReader(sc.body)
	}
	if method == http.MethodGet || method == http.MethodDelete {
		body = nil
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// fill puts office A's / taxpayer X's ids in the path; a parameter the demo
// data has no row for gets a random uuid.
func (e *authzEnv) fill(t *testing.T, path string) string {
	t.Helper()
	return paramRe.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := authzParams[name]; ok {
			return v
		}
		if name == "uuid" {
			for prefix := range uuidQueries {
				if strings.HasPrefix(path, prefix) {
					return e.param(t, name, path)
				}
			}
			return uuid.NewString()
		}
		if _, known := paramQueries[name]; known || name == "engagementUuid" || name == "period" || name == "rowNo" || name == "kind" || name == "importId" {
			return e.param(t, name, path)
		}
		return uuid.NewString()
	})
}

// authzParams fill path parameters the sweep does not know with a fixed value.
var authzParams = map[string]string{}

// TestAuthzMatrixDiscover prints the observed answers (AUTHZ_DISCOVER=1):
// a tool for writing authzClass, not a check.
func TestAuthzMatrixDiscover(t *testing.T) {
	if os.Getenv("AUTHZ_DISCOVER") == "" {
		t.Skip("AUTHZ_DISCOVER=1 prints the observed matrix")
	}
	e := newAuthzEnv(t)
	ops := matrixOrder(specOps(t))
	var lines []string
	for _, op := range ops {
		path := e.fill(t, op.path)
		var cells []string
		for _, c := range e.callers() {
			if op.method != http.MethodGet && (c.name == "ownerA" || c.name == "taxX") {
				cells = append(cells, c.name+"=skip")
				continue
			}
			rec := e.request(t, c, op.method, path, sweepCaseFor(op))
			cells = append(cells, fmt.Sprintf("%s=%d", c.name, rec.Code))
		}
		lines = append(lines, fmt.Sprintf("%-7s %-95s %s", op.method, op.path, strings.Join(cells, " ")))
	}
	sort.Strings(lines)
	_ = os.WriteFile(filepath.Join(os.TempDir(), "authz-matrix.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	t.Logf("wrote %s (%d ops)", filepath.Join(os.TempDir(), "authz-matrix.txt"), len(lines))
}

// matrixOrder runs reads first, then writes, and the ones that end a
// membership or an engagement (the sweep's `last` cases) at the very end.
func matrixOrder(ops []specOp) []specOp {
	var reads, writes, last []specOp
	for _, op := range ops {
		switch {
		case op.method == http.MethodGet:
			reads = append(reads, op)
		case sweepCaseFor(op).last:
			last = append(last, op)
		default:
			writes = append(writes, op)
		}
	}
	return append(append(reads, writes...), last...)
}

// sweepCaseFor is the write sweep's body for op, if it has one.
func sweepCaseFor(op specOp) sweepCase {
	key := op.method + " " + op.path
	if c, ok := tenantWrites[key]; ok {
		return c
	}
	return taxpayerWrites[key]
}

// idorLeaks are strings of office A / taxpayer X that must not appear in an
// answer to office B or taxpayer Y.
var idorLeaks = []string{"9000000017", "Atlas Gıda", "demo-smmm"}

// TestAuthzIDORDiscover prints office B's and taxpayer Y's answers for
// office A's / taxpayer X's ids (AUTHZ_DISCOVER=1).
func TestAuthzIDORDiscover(t *testing.T) {
	if os.Getenv("AUTHZ_DISCOVER") == "" {
		t.Skip("AUTHZ_DISCOVER=1 prints the observed IDOR answers")
	}
	e := newAuthzEnv(t)
	var lines []string
	for _, op := range matrixOrder(specOps(t)) {
		if !strings.Contains(op.path, "{") || sweepCaseFor(op).last {
			continue
		}
		var c matrixCaller
		var path string
		switch {
		case strings.HasPrefix(op.path, "/v1/tenant/"):
			c = matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
			// B's own engagement, A's records.
			path = strings.ReplaceAll(op.path, "{engagementUuid}", e.engBUID)
			path = e.fill(t, path)
		case strings.HasPrefix(op.path, "/v1/taxpayer/"):
			c = matrixCaller{"taxY", e.tpYTok, map[string]string{"X-Taxpayer": e.tpYUUID}}
			path = e.fill(t, op.path)
		default:
			continue
		}
		rec := e.request(t, c, op.method, path, sweepCaseFor(op))
		leak := ""
		for _, s := range idorLeaks {
			if strings.Contains(rec.Body.String(), s) {
				leak += " LEAK:" + s
			}
		}
		lines = append(lines, fmt.Sprintf("%-7s %-95s %s=%d%s %s", op.method, op.path, c.name, rec.Code, leak, firstLine(rec.Body.String())))
	}
	// Office A's engagement under office B's own header.
	for _, op := range specOps(t) {
		if !strings.HasPrefix(op.path, "/v1/tenant/engagements/{engagementUuid}") || op.method != http.MethodGet {
			continue
		}
		c := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
		rec := e.request(t, c, op.method, e.fill(t, op.path), sweepCase{})
		lines = append(lines, fmt.Sprintf("ENG-A   %-95s ownerB=%d", op.path, rec.Code))
	}
	_ = os.WriteFile(filepath.Join(os.TempDir(), "authz-idor.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

type authzWant int

const (
	wantAllow authzWant = iota // reached the handler: not 401, not 403
	want401
	want403
	wantSkip // an allowed write: the write sweep runs it, the matrix does not
)

// authzExpect is the answer of class c to caller (method: the write rule).
func authzExpect(c authzClass, caller, method string) authzWant {
	write := method != http.MethodGet
	if c == azPublicOff {
		if caller == "anon" {
			return want403
		}
		return wantSkip
	}
	if caller == "anon" {
		if c == azPublic {
			return wantAllow
		}
		return want401
	}
	switch c {
	case azPublic, azSelf:
		if write {
			return wantSkip // the caller's own account: sign-out, password, MFA…
		}
		return wantAllow
	case azPlatform:
		return want403
	case azTaxpayerRead, azTaxpayerWrite:
		if caller != "taxX" {
			return want403
		}
		if c == azTaxpayerWrite {
			return wantSkip
		}
		return wantAllow
	}
	// Office routes: only office A's members.
	switch caller {
	case "ownerA":
		if write {
			return wantSkip
		}
		return wantAllow
	case "readerA":
		switch c {
		case azTenantRead, azTenantReadAction, azTenantMember:
			return wantAllow
		default:
			return want403
		}
	default: // ownerB (asks for office A), taxX, taxY
		return want403
	}
}

func (w authzWant) ok(status int) bool {
	switch w {
	case wantAllow:
		return status != http.StatusUnauthorized && status != http.StatusForbidden
	case want401:
		return status == http.StatusUnauthorized
	case want403:
		return status == http.StatusForbidden
	}
	return true
}

func (w authzWant) String() string {
	return [...]string{"allowed (not 401/403)", "401", "403", "skip"}[w]
}

// TestAuthzMatrix: every operation × every caller answers as authzTable and
// authzExpect say.
func TestAuthzMatrix(t *testing.T) {
	ops := specOps(t)
	listed := map[string]bool{}
	for _, op := range ops {
		key := op.method + " " + op.path
		listed[key] = true
		if _, ok := authzTable[key]; !ok {
			t.Errorf("%s has no row in authzTable (authz_table_test.go): classify it", key)
		}
	}
	for key := range authzTable {
		if !listed[key] {
			t.Errorf("authzTable row %s is not an operation in openapi.yaml any more: remove it", key)
		}
	}
	if t.Failed() {
		return
	}

	e := newAuthzEnv(t)
	for _, op := range matrixOrder(ops) {
		class := authzTable[op.method+" "+op.path]
		path := e.fill(t, op.path)
		for _, c := range e.callers() {
			want := authzExpect(class, c.name, op.method)
			if want == wantSkip {
				continue
			}
			rec := e.request(t, c, op.method, path, sweepCaseFor(op))
			if !want.ok(rec.Code) {
				t.Errorf("%s %s as %s = %d, want %s: %s", op.method, op.path, c.name, rec.Code, want, firstLine(rec.Body.String()))
			}
		}
	}
}

// identifying reports whether a path names a record (beyond the engagement
// and values that are not ids: a period, a rule code, a settings kind).
func identifying(path string) bool {
	for _, m := range paramRe.FindAllStringSubmatch(path, -1) {
		switch m[1] {
		case "engagementUuid", "period", "code", "kind", "importId", "provider":
		default:
			return true
		}
	}
	return false
}

// TestAuthzIDOR: office B, in its own scope, asks for office A's records by
// id; taxpayer Y, in its own scope, for taxpayer X's. Nothing is found and
// nothing of A / X comes back. Office A's engagement named under office B
// is refused like a foreign scope (403, the same answer as for an unknown
// or unassigned engagement: its existence is not revealed).
func TestAuthzIDOR(t *testing.T) {
	e := newAuthzEnv(t)
	ownB := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
	ownY := matrixCaller{"taxY", e.tpYTok, map[string]string{"X-Taxpayer": e.tpYUUID}}
	for _, op := range matrixOrder(specOps(t)) {
		if sweepCaseFor(op).last {
			continue
		}
		var c matrixCaller
		var path string
		switch {
		case strings.HasPrefix(op.path, "/v1/tenant/") && identifying(op.path):
			c, path = ownB, e.fill(t, strings.ReplaceAll(op.path, "{engagementUuid}", e.engBUID))
		case strings.HasPrefix(op.path, "/v1/taxpayer/") && identifying(op.path):
			c, path = ownY, e.fill(t, op.path)
		default:
			continue
		}
		rec := e.request(t, c, op.method, path, sweepCaseFor(op))
		if rec.Code < 400 {
			t.Errorf("IDOR: %s %s as %s in its own scope = %d: %s", op.method, op.path, c.name, rec.Code, firstLine(rec.Body.String()))
		}
		if op.method == http.MethodGet && rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Errorf("IDOR: %s %s as %s = %d, want 404 (a record of another scope does not exist)", op.method, op.path, c.name, rec.Code)
		}
		assertNoLeak(t, op.method+" "+op.path+" as "+c.name, rec.Body.String())
	}
	for _, op := range specOps(t) {
		if op.method != http.MethodGet || !strings.HasPrefix(op.path, "/v1/tenant/engagements/{engagementUuid}") {
			continue
		}
		rec := e.request(t, ownB, op.method, e.fill(t, op.path), sweepCase{})
		if rec.Code != http.StatusForbidden {
			t.Errorf("office A's engagement under office B: %s %s = %d, want 403", op.method, op.path, rec.Code)
		}
		assertNoLeak(t, op.path+" (A's engagement under B)", rec.Body.String())
	}
}

// TestAuthzScopedLists: office B's and taxpayer Y's own lists (every GET
// without a record id) carry nothing of office A / taxpayer X.
func TestAuthzScopedLists(t *testing.T) {
	e := newAuthzEnv(t)
	ownB := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
	ownY := matrixCaller{"taxY", e.tpYTok, map[string]string{"X-Taxpayer": e.tpYUUID}}
	n := 0
	for _, op := range specOps(t) {
		if op.method != http.MethodGet || identifying(op.path) {
			continue
		}
		var c matrixCaller
		switch {
		case strings.HasPrefix(op.path, "/v1/tenant/"):
			c = ownB
		case strings.HasPrefix(op.path, "/v1/taxpayer/"):
			c = ownY
		default:
			continue
		}
		path := strings.ReplaceAll(op.path, "{engagementUuid}", e.engBUID)
		path = e.fill(t, path)
		rec := e.request(t, c, op.method, path, sweepCase{})
		n++
		assertNoLeak(t, op.path+" as "+c.name, rec.Body.String())
	}
	if n < 50 {
		t.Fatalf("only %d list routes checked", n)
	}
}

func assertNoLeak(t *testing.T, what, body string) {
	t.Helper()
	for _, s := range idorLeaks {
		if strings.Contains(body, s) {
			t.Errorf("leak: %s answered with %q: %s", what, s, firstLine(body))
		}
	}
}

// TestAuthzExportScope: a Luca export of office A's engagement carries only
// its own journal; office B's export only B's; neither can download the
// other's files.
func TestAuthzExportScope(t *testing.T) {
	e := newAuthzEnv(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	ownB := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
	type side struct {
		c      matrixCaller
		eng    string
		marker string
		files  []string
		batch  string
	}
	a := &side{c: ownA, eng: e.param(t, "engagementUuid", ""), marker: "ATLAS-GIZLI-KAYIT"}
	b := &side{c: ownB, eng: e.engBUID, marker: "YILDIZ-GIZLI-KAYIT"}
	for _, s := range []*side{a, b} {
		base := "/v1/tenant/engagements/" + s.eng
		call := func(method, path, body string, want int) []byte {
			t.Helper()
			rec := e.request(t, s.c, method, base+path, sweepCase{body: body})
			if rec.Code != want {
				t.Fatalf("%s %s as %s = %d, want %d: %s", method, path, s.c.name, rec.Code, want, rec.Body)
			}
			return rec.Body.Bytes()
		}
		call(http.MethodPut, "/journal-export-settings", `{"block_on_unknown_account":false}`, http.StatusOK)
		var d struct {
			Data struct {
				UUID string `json:"uuid"`
			} `json:"data"`
		}
		_ = json.Unmarshal(call(http.MethodPost, "/journal-drafts", `{"entry_date":"2026-10-15T00:00:00Z","description":"`+s.marker+`","lines":[{"account":"100","debit":"100.00","credit":"0","description":"`+s.marker+`"},{"account":"600","debit":"0","credit":"100.00","description":"`+s.marker+`"}]}`, http.StatusCreated), &d)
		call(http.MethodPost, "/journal-drafts/"+d.Data.UUID+"/approve", `{}`, http.StatusOK)
		var batch struct {
			Data struct {
				UUID  string `json:"uuid"`
				Files []struct {
					Name string `json:"name"`
				} `json:"files"`
			} `json:"data"`
		}
		_ = json.Unmarshal(call(http.MethodPost, "/journal-exports", `{"driver":"luca","period":"2026-10"}`, http.StatusCreated), &batch)
		if len(batch.Data.Files) == 0 {
			t.Fatalf("%s: export without files", s.c.name)
		}
		s.batch = batch.Data.UUID
		for _, f := range batch.Data.Files {
			s.files = append(s.files, f.Name)
		}
	}
	download := func(as matrixCaller, owner *side, eng, file string) *httptest.ResponseRecorder {
		return e.request(t, as, http.MethodGet, "/v1/tenant/engagements/"+eng+"/journal-exports/"+owner.batch+"/files/"+file, sweepCase{})
	}
	for _, pair := range []struct{ own, other *side }{{a, b}, {b, a}} {
		for _, f := range pair.own.files {
			rec := download(pair.own.c, pair.own, pair.own.eng, f)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s downloads its own %s = %d", pair.own.c.name, f, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), pair.own.marker) {
				t.Logf("%s: own marker not in %s (driver may not export descriptions)", pair.own.c.name, f)
			}
			if strings.Contains(rec.Body.String(), pair.other.marker) {
				t.Errorf("export leak: %s's %s carries %s", pair.own.c.name, f, pair.other.marker)
			}
			// The other office: its own engagement + this batch, and this engagement.
			if rec := download(pair.other.c, pair.own, pair.other.eng, f); rec.Code != http.StatusNotFound {
				t.Errorf("%s downloads %s's export via its own engagement = %d, want 404", pair.other.c.name, pair.own.c.name, rec.Code)
			}
			if rec := download(pair.other.c, pair.own, pair.own.eng, f); rec.Code != http.StatusForbidden {
				t.Errorf("%s downloads %s's export via that engagement = %d, want 403", pair.other.c.name, pair.own.c.name, rec.Code)
			}
		}
	}
}

// TestAuthzSelfRecords: one user's own records (notifications, sessions,
// push devices, passkeys) are not found, read or changed by another user.
func TestAuthzSelfRecords(t *testing.T) {
	e := newAuthzEnv(t)
	a := matrixCaller{"ownerA", e.ownerTok, nil}
	b := matrixCaller{"ownerB", e.ownerBTok, nil}
	ctx := context.Background()
	var userA int64
	var notif, passkey string
	if err := rls.System(ctx, e.d.Pool, "test: authz self records", func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = 'sahip@demo.hakan.local'`).Scan(&userA); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO notifications (user_id, channel, status, title, body, action_url)
			VALUES ($1, 'inapp', 'delivered', 'ATLAS-GIZLI bildirim', 'Atlas Gıda', '/t/demo-smmm/review') RETURNING uuid::text`, userA).Scan(&notif); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO webauthn_credentials (user_id, credential_id, public_key, name)
			VALUES ($1, 'authz-cred-a', 'authz-key-a', 'A anahtarı') RETURNING uuid::text`, userA).Scan(&passkey)
	}); err != nil {
		t.Fatal(err)
	}
	status := func(c matrixCaller, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return e.request(t, c, method, path, sweepCase{body: body})
	}
	expect := func(what string, rec *httptest.ResponseRecorder, want int) {
		t.Helper()
		if rec.Code != want {
			t.Errorf("%s = %d, want %d: %s", what, rec.Code, want, firstLine(rec.Body.String()))
		}
		assertNoLeak(t, what, rec.Body.String())
	}

	// Notifications: the owner sees it; another user finds nothing.
	if rec := status(a, http.MethodGet, "/v1/notifications/"+notif, ""); rec.Code != http.StatusOK {
		t.Fatalf("owner reads own notification = %d: %s", rec.Code, rec.Body)
	}
	expect("B reads A's notification", status(b, http.MethodGet, "/v1/notifications/"+notif, ""), http.StatusNotFound)
	expect("B marks A's notification read", status(b, http.MethodPost, "/v1/notifications/"+notif+"/read", ""), http.StatusNotFound)
	expect("B follows A's notification action", status(b, http.MethodGet, "/v1/notifications/"+notif+"/action?exp=9999999999&sig=00", ""), http.StatusNotFound)

	// Sessions: B cannot end A's session.
	var sessions struct {
		Data struct {
			Items []struct {
				UUID    string `json:"uuid"`
				Current bool   `json:"current"`
			} `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(status(a, http.MethodGet, "/v1/auth/sessions", "").Body.Bytes(), &sessions)
	var sessionA string
	for _, s := range sessions.Data.Items {
		if s.Current {
			sessionA = s.UUID
		}
	}
	if sessionA == "" {
		t.Fatal("A has no current session in the list")
	}
	expect("B revokes A's session", status(b, http.MethodDelete, "/v1/auth/sessions/"+sessionA, ""), http.StatusNotFound)
	if rec := status(a, http.MethodGet, "/v1/auth/me", ""); rec.Code != http.StatusOK {
		t.Errorf("A after B's revoke attempt = %d, want 200", rec.Code)
	}

	// Push devices: B's delete of A's token removes nothing of A's.
	const token = "ExponentPushToken[authz-a-device]"
	if rec := status(a, http.MethodPost, "/v1/push-devices", `{"token":"`+token+`","platform":"ios","locale":"tr"}`); rec.Code >= 300 {
		t.Fatalf("A registers a push device = %d: %s", rec.Code, rec.Body)
	}
	_ = status(b, http.MethodDelete, "/v1/push-devices/"+url.PathEscape(token), "")
	if n := e.one(t, `SELECT count(*) FROM push_devices WHERE token = 'ExponentPushToken[authz-a-device]' AND user_id = (SELECT id FROM users WHERE email = 'sahip@demo.hakan.local') AND $1 > 0 AND $2 > 0 AND $3 > 0`, "push"); n != "1" {
		t.Errorf("A's push device after B's delete: %s rows, want 1", n)
	}

	// Passkeys: B can neither rename nor delete A's.
	expect("B renames A's passkey", status(b, http.MethodPatch, "/v1/auth/passkeys/"+passkey, `{"name":"B"}`), http.StatusNotFound)
	_ = status(b, http.MethodDelete, "/v1/auth/passkeys/"+passkey, "")
	if n := e.one(t, `SELECT count(*) FROM webauthn_credentials WHERE uuid = '`+passkey+`'::uuid AND name = 'A anahtarı' AND $1 > 0 AND $2 > 0 AND $3 > 0`, "passkey"); n != "1" {
		t.Errorf("A's passkey after B's rename / delete: %s rows unchanged, want 1", n)
	}
}
