package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/ledongthuc/pdf"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Exports and files (issue #453). An Excel file is a zip and a PDF keeps
// its text in compressed streams: a plain substring search of the body
// cannot find office A's data in them. bodyText decodes both, and
// assertNoLeak searches the decoded text.

// bodyText is the searchable text of a response body: the XML parts of an
// xlsx / docx (zip), the text of a PDF, otherwise the body itself.
func bodyText(body []byte) string {
	switch {
	case bytes.HasPrefix(body, []byte("PK\x03\x04")):
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return string(body)
		}
		var b strings.Builder
		for _, f := range zr.File {
			if !strings.HasSuffix(f.Name, ".xml") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			raw, _ := io.ReadAll(io.LimitReader(rc, 64<<20))
			_ = rc.Close()
			b.Write(raw)
			b.WriteByte('\n')
		}
		return b.String()
	case bytes.HasPrefix(body, []byte("%PDF")):
		return pdfPlainText(body)
	}
	return string(body)
}

func pdfPlainText(body []byte) (out string) {
	defer func() {
		if recover() != nil { // a malformed PDF: search the raw bytes
			out = string(body)
		}
	}()
	rd, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return string(body)
	}
	var b strings.Builder
	for i := 1; i <= rd.NumPage(); i++ {
		p := rd.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// fileRouteRe: the GET routes that answer with a file (Excel, PDF, a stored
// document) instead of JSON.
var fileRouteRe = regexp.MustCompile(`/(export|pdf|file)$`)

// fileQuery: file routes that need a query to answer.
var fileQuery = map[string]string{
	"/v1/tenant/engagements/{engagementUuid}/report/pdf": "?year=2026",
}

// officeAMarkers are strings only taxpayer X's / office A's data holds:
// idorLeaks plus X's employee names, expense vendors, Z report (ÖKC)
// serials and bank transaction descriptions from the demo data.
func (e *authzEnv) officeAMarkers(t *testing.T) []string {
	t.Helper()
	out := append([]string(nil), idorLeaks...)
	err := rls.System(context.Background(), e.d.Pool, "test: authz markers", func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT v FROM (
			  SELECT full_name AS v FROM employees WHERE taxpayer_id = $1
			  UNION ALL SELECT vendor_name FROM expenses WHERE taxpayer_id = $1
			  UNION ALL SELECT okc_serial_number FROM retail_sales WHERE taxpayer_id = $1
			  UNION ALL SELECT description FROM bank_transactions WHERE taxpayer_id = $1
			) s WHERE v IS NOT NULL AND length(btrim(v)) >= 8`, e.tpID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return err
			}
			out = append(out, strings.TrimSpace(v))
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < len(idorLeaks)+5 {
		t.Fatalf("demo data gives only %d markers: %v", len(out), out)
	}
	return out
}

// TestBodyTextDecodes: the leak search sees inside an Excel file.
func TestBodyTextDecodes(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xl/sharedStrings.xml")
	_, _ = w.Write([]byte(`<sst><si><t>Atlas Gıda</t></si></sst>`))
	_ = zw.Close()
	if bytes.Contains(buf.Bytes(), []byte("Atlas Gıda")) {
		t.Fatal("fixture is not compressed")
	}
	if !strings.Contains(bodyText(buf.Bytes()), "Atlas Gıda") {
		t.Fatal("bodyText does not decode an xlsx")
	}
}

// TestAuthzFileExports: every file route of an engagement (Excel exports,
// PDFs, stored documents). Office A's own export carries its data; office
// B's export of its own engagement carries none of it, and office B naming
// A's record ids under its own engagement gets 404 without content. The
// files are decoded before the search.
func TestAuthzFileExports(t *testing.T) {
	e := newAuthzEnv(t)
	markers := e.officeAMarkers(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	ownB := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
	engA := e.param(t, "engagementUuid", "")

	contains := func(text string) string {
		for _, m := range markers {
			if strings.Contains(text, m) {
				return m
			}
		}
		return ""
	}
	checked, carried := 0, 0
	for _, op := range specOps(t) {
		if op.method != http.MethodGet || !strings.HasPrefix(op.path, "/v1/tenant/engagements/{engagementUuid}/") || !fileRouteRe.MatchString(op.path) {
			continue
		}
		checked++
		pathA := e.fill(t, strings.ReplaceAll(op.path, "{engagementUuid}", engA)) + fileQuery[op.path]
		recA := e.request(t, ownA, op.method, pathA, sweepCase{})
		if recA.Code == http.StatusOK {
			if contains(bodyText(recA.Body.Bytes())) != "" {
				carried++
			}
		}
		// Office B, its own engagement, office A's record ids (if any).
		pathB := e.fill(t, strings.ReplaceAll(op.path, "{engagementUuid}", e.engBUID)) + fileQuery[op.path]
		recB := e.request(t, ownB, op.method, pathB, sweepCase{})
		if identifying(op.path) && recA.Code == http.StatusOK && recB.Code != http.StatusNotFound {
			t.Errorf("%s with office A's ids as office B = %d, want 404", op.path, recB.Code)
		}
		if recB.Code < 400 && recB.Code >= 300 {
			t.Errorf("%s as office B = %d (a redirect may point at office A's file)", op.path, recB.Code)
		}
		if m := contains(bodyText(recB.Body.Bytes())); m != "" {
			t.Errorf("export leak: %s as office B carries office A's %q", op.path, m)
		}
	}
	if checked < 15 {
		t.Fatalf("only %d file routes found", checked)
	}
	// The check must be able to fail: office A's own files show its data.
	if carried < 4 {
		t.Fatalf("only %d of office A's own files carry its data: the leak search would see nothing", carried)
	}
	t.Logf("%d file routes, %d of office A's files carry its markers", checked, carried)
}

// TestCopilotPlansPerUser (issue #453): a co-pilot plan is its author's.
// Another member of the same office, with write permission, neither sees
// it in the list nor reads, approves a step of or cancels it: 404. The
// author still can.
func TestCopilotPlansPerUser(t *testing.T) {
	e := newAuthzEnv(t)
	plan := e.one(t, `SELECT p.uuid::text FROM copilot_plans p JOIN users u ON u.id = p.created_by
		WHERE p.organization_id = $1 AND u.email = 'sahip@demo.hakan.local' AND p.status IN ('proposed', 'running') AND $2 > 0 AND $3 > 0
		ORDER BY p.id DESC LIMIT 1`, "plan")
	if n := e.one(t, `SELECT count(*)::text FROM copilot_plans WHERE uuid::text = '`+plan+`' AND $1 > 0 AND $2 > 0 AND $3 > 0`, "plan"); n != "1" {
		t.Fatalf("the demo data has no open co-pilot plan of the owner")
	}
	hdr := map[string]string{"X-Organization": "demo-smmm"}
	owner := matrixCaller{"ownerA", e.ownerTok, hdr}
	admin := matrixCaller{"adminA", e.login(t, "musavir@demo.hakan.local"), hdr}
	base := "/v1/tenant/copilot/plans/" + plan

	if rec := e.request(t, owner, http.MethodGet, base, sweepCase{}); rec.Code != http.StatusOK {
		t.Fatalf("owner reads own plan = %d: %s", rec.Code, firstLine(rec.Body.String()))
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, base, ""},
		{http.MethodPost, base + "/steps/0", `{"status":"done"}`},
		{http.MethodPost, base + "/cancel", `{}`},
	} {
		rec := e.request(t, admin, c.method, c.path, sweepCase{body: c.body})
		if rec.Code != http.StatusNotFound {
			t.Errorf("another member: %s %s = %d, want 404: %s", c.method, c.path, rec.Code, firstLine(rec.Body.String()))
		}
	}
	rec := e.request(t, admin, http.MethodGet, "/v1/tenant/copilot/plans", sweepCase{})
	if rec.Code != http.StatusOK {
		t.Fatalf("another member lists plans = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), plan) {
		t.Errorf("another member's plan list carries the owner's plan")
	}
	// Untouched: still open for its author.
	rec = e.request(t, owner, http.MethodGet, base, sweepCase{})
	if strings.Contains(rec.Body.String(), `"cancelled"`) || strings.Contains(rec.Body.String(), `"done"`) {
		t.Errorf("owner's plan after the other member's attempts: %s", firstLine(rec.Body.String()))
	}
}
