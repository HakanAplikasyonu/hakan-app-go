package httpserver

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Excel / CSV imports of M15 (issue #453) against the real database:
// expenses, Z reports, payroll, BA/BS, bank statements and the generic
// Excel import. Broken and boundary files (not a spreadsheet, empty,
// header only, bad values, too many rows, too large) answer 4xx or report
// the bad rows; none answers 5xx, and a refused file writes nothing.

// xlsxOf builds a one-sheet workbook from rows.
func xlsxOf(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cell, &r); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// upload posts a multipart file (and fields) as caller c.
func (e *authzEnv) upload(t *testing.T, c matrixCaller, path, name string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// counts is the number of rows of the tables the imports write, for
// taxpayer X / office A's engagement.
func (e *authzEnv) importCounts(t *testing.T) string {
	t.Helper()
	return e.one(t, `SELECT concat_ws(',',
		(SELECT count(*) FROM expenses WHERE taxpayer_id = $3),
		(SELECT count(*) FROM retail_sales WHERE taxpayer_id = $3),
		(SELECT count(*) FROM payroll_runs WHERE taxpayer_id = $3),
		(SELECT count(*) FROM payroll_lines WHERE taxpayer_id = $3),
		(SELECT count(*) FROM babs_declarations WHERE organization_id = $1 AND engagement_id = $2),
		(SELECT count(*) FROM bank_transactions WHERE taxpayer_id = $3),
		(SELECT count(*) FROM excel_imports WHERE organization_id = $1 AND engagement_id = $2))`, "counts")
}

// TestM15ImportsHostileFiles: every M15 import with broken and boundary
// files.
func TestM15ImportsHostileFiles(t *testing.T) {
	e := newAuthzEnv(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	base := "/v1/tenant/engagements/" + e.param(t, "engagementUuid", "")

	type target struct {
		name, path string
		fields     map[string]string
	}
	targets := []target{
		{"masraf", base + "/expenses/import", nil},
		{"z-raporu", base + "/retail-sales/import", nil},
		{"bordro", base + "/payroll/imports", map[string]string{"period": "2026-09"}},
		{"ba-bs", base + "/reconciliation/babs/imports", map[string]string{"period": "2026-09"}},
		{"banka", base + "/bank-statements", nil},
		{"excel-import", base + "/excel-imports", map[string]string{"data_type": "expenses"}},
	}

	many := [][]any{{"Tarih", "Satıcı", "VKN / TCKN", "Fiş No", "Toplam", "VKN/TCKN", "Tutar", "Adı Soyadı", "Brüt Ücret", "Net Ücret"}}
	for i := 0; i < 5001; i++ {
		many = append(many, []any{"01.09.2026", fmt.Sprintf("Satıcı %d", i), "1234567890", fmt.Sprint(i), "100,00", "1234567890", "6000", fmt.Sprintf("Kişi %d", i), "1000", "800"})
	}
	files := []struct {
		name, file string
		data       []byte
		// refused: the whole file must be refused (4xx) and write nothing.
		refused bool
	}{
		{"not a spreadsheet", "x.xlsx", []byte("bu bir Excel dosyası değil \x00\x01\x02"), true},
		{"empty", "x.xlsx", nil, true},
		{"zip but not xlsx", "x.xlsx", xlsxZipJunk(t), true},
		{"empty workbook", "x.xlsx", xlsxOf(t, nil), true},
		{"no header", "x.xlsx", xlsxOf(t, [][]any{{"a", "b"}, {"1", "2"}}), true},
		{"header only", "x.xlsx", xlsxOf(t, [][]any{{"Tarih", "Satıcı", "Toplam", "VKN/TCKN", "Tutar", "Adı Soyadı", "Brüt Ücret", "Net Ücret"}}), false},
		{"bad values", "x.xlsx", xlsxOf(t, [][]any{
			{"Tarih", "Satıcı", "VKN / TCKN", "Fiş No", "Toplam", "VKN/TCKN", "Tutar", "Adı Soyadı", "Brüt Ücret", "Net Ücret"},
			{"31.02.2026", "=HYPERLINK(\"http://example.test\")", "12", "1", "-5", "abc", "çok", "", "yok", "-"},
			{"", strings.Repeat("Ü", 10000), "99999999999999", "2", "1e309", "1", "NaN", strings.Repeat("x", 5000), "1.2.3,4", "∞"},
		}), false},
		{"csv with BOM and semicolons", "x.csv", []byte("\xef\xbb\xbfTarih;Satıcı;Toplam;VKN/TCKN;Tutar\n32.13.2026;\"kapanmamış;1;2\n"), false},
		{"too many rows", "x.xlsx", xlsxOf(t, many), false},
	}

	for _, tg := range targets {
		for _, f := range files {
			t.Run(tg.name+"/"+f.name, func(t *testing.T) {
				before := e.importCounts(t)
				path := tg.path
				if tg.name == "bordro" {
					path += "?preview=true"
				}
				rec := e.upload(t, ownA, path, f.file, f.data, tg.fields)
				if rec.Code >= 500 {
					t.Fatalf("= %d, want 4xx or a row report: %s", rec.Code, firstLine(rec.Body.String()))
				}
				// The generic import keeps a refused file as a failed or
				// mapping-review import (201); the payroll preview reports
				// the missing columns (200). Neither writes rows.
				lenient := tg.name == "excel-import" || tg.name == "bordro"
				if f.refused && rec.Code < 400 && !lenient {
					t.Errorf("= %d, want 4xx for a file that is not a usable sheet: %s", rec.Code, firstLine(rec.Body.String()))
				}
				if !json.Valid(rec.Body.Bytes()) {
					t.Errorf("answer is not JSON: %s", firstLine(rec.Body.String()))
				}
				if (rec.Code >= 400 || lenient) && tg.name != "excel-import" {
					if after := e.importCounts(t); after != before {
						t.Errorf("refused (%d) but rows changed: %s → %s", rec.Code, before, after)
					}
				}
			})
		}
	}

	// Too large: more than the 10 MB / 20 MB limit is refused before parsing.
	big := bytes.Repeat([]byte("a"), 21<<20)
	for _, tg := range targets {
		rec := e.upload(t, ownA, tg.path, "big.csv", big, tg.fields)
		if rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: 21 MB file = %d, want 4xx", tg.name, rec.Code)
		}
	}
}

// xlsxZipJunk is a zip that is not a workbook.
func xlsxZipJunk(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := writeZip(&buf, map[string]string{"readme.txt": "boş"}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExpenseImportRoundTrip: the expense export is the import template;
// imported again unchanged it adds nothing, and a bad row is reported with
// its line while the good ones go in. Office B importing the same file
// into its own engagement leaves office A's expenses as they were.
func TestExpenseImportRoundTrip(t *testing.T) {
	e := newAuthzEnv(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	ownB := matrixCaller{"ownerB", e.ownerBTok, map[string]string{"X-Organization": authzOrgB}}
	base := "/v1/tenant/engagements/" + e.param(t, "engagementUuid", "")

	exp := e.request(t, ownA, http.MethodGet, base+"/expenses/export", sweepCase{})
	if exp.Code != http.StatusOK || !bytes.HasPrefix(exp.Body.Bytes(), []byte("PK")) {
		t.Fatalf("export = %d", exp.Code)
	}
	count := func() string {
		return e.one(t, `SELECT count(*)::text FROM expenses WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0`, "expenses")
	}
	before := count()
	type result struct {
		Rows     int `json:"rows"`
		Inserted int `json:"inserted"`
		Skipped  int `json:"skipped"`
		Errors   []struct {
			Line int `json:"line"`
		} `json:"errors"`
	}
	decode := func(rec *httptest.ResponseRecorder) result {
		t.Helper()
		var out struct {
			Data result `json:"data"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
			t.Fatalf("import = %d: %s", rec.Code, firstLine(rec.Body.String()))
		}
		return out.Data
	}
	again := decode(e.upload(t, ownA, base+"/expenses/import", "masraflar.xlsx", exp.Body.Bytes(), nil))
	if again.Rows == 0 || again.Inserted != 0 || again.Skipped != again.Rows {
		t.Errorf("re-import of the export: %+v, want every row skipped", again)
	}
	if count() != before {
		t.Errorf("re-import changed the expenses: %s → %s", before, count())
	}

	mixed := xlsxOf(t, [][]any{
		{"Tarih", "Satıcı", "VKN / TCKN", "Fiş No", "Toplam"},
		{"05.09.2026", "Sınır Kırtasiye", "1234567890", "S-1", "118,00"},
		{"31.02.2026", "Bozuk Tarih", "1234567890", "S-2", "10,00"},
		{"06.09.2026", "Sınır Kırtasiye", "1234567890", "S-3", "0,01"},
	})
	res := decode(e.upload(t, ownA, base+"/expenses/import", "m.xlsx", mixed, nil))
	if res.Inserted != 2 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Errorf("mixed file: %+v, want 2 inserted and row 3 reported", res)
	}
	afterA := count()

	// Office B, its own engagement: A's expenses do not change.
	recB := e.upload(t, ownB, "/v1/tenant/engagements/"+e.engBUID+"/expenses/import", "m.xlsx", mixed, nil)
	if recB.Code >= 500 {
		t.Fatalf("B import = %d", recB.Code)
	}
	if count() != afterA {
		t.Errorf("office B's import changed office A's expenses: %s → %s", afterA, count())
	}
}

func writeZip(w *bytes.Buffer, files map[string]string) error {
	zw := zip.NewWriter(w)
	for name, body := range files {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(body)); err != nil {
			return err
		}
	}
	return zw.Close()
}

// TestRetailSaleImportRoundTrip: the Z report export imported again
// unchanged adds nothing (the same device and Z number on the same day is
// the same report, whatever its source).
func TestRetailSaleImportRoundTrip(t *testing.T) {
	e := newAuthzEnv(t)
	ownA := matrixCaller{"ownerA", e.ownerTok, map[string]string{"X-Organization": "demo-smmm"}}
	base := "/v1/tenant/engagements/" + e.param(t, "engagementUuid", "")
	exp := e.request(t, ownA, http.MethodGet, base+"/retail-sales/export", sweepCase{})
	if exp.Code != http.StatusOK {
		t.Fatalf("export = %d", exp.Code)
	}
	count := func() string {
		return e.one(t, `SELECT count(*)::text FROM retail_sales WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0`, "retail")
	}
	before := count()
	rec := e.upload(t, ownA, base+"/retail-sales/import", "z.xlsx", exp.Body.Bytes(), nil)
	var out struct {
		Data struct {
			Rows     int `json:"rows"`
			Inserted int `json:"inserted"`
		} `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("import = %d: %s", rec.Code, firstLine(rec.Body.String()))
	}
	if out.Data.Rows == 0 || out.Data.Inserted != 0 || count() != before {
		t.Errorf("re-import of the Z export: %+v, %s → %s rows, want nothing added", out.Data, before, count())
	}
}
