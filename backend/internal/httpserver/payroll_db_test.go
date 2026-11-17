package httpserver

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Bordro (#382): Luca CSV içe aktarma, dönem bordrosu, SGK tahakkuku, fiş
// taslağı, banka eşleşmesi, Excel çıktısı ve bordro PDF'inin mükellef
// panelinde görünmesi. Erişim kuralları yetki matrisinde (authz_table_test).
func TestPayrollFlow(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng + "/payroll"

	form := func(t *testing.T, path string, fields map[string]string, fileName, body string, taxpayer bool) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile("file", fileName)
		_, _ = fw.Write([]byte(body))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+e.ownerTok)
		req.Header.Set("X-Organization", "demo-smmm")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}
	decode := func(t *testing.T, rec *httptest.ResponseRecorder, v any) {
		t.Helper()
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(env.Data, v); err != nil {
			t.Fatalf("%v: %s", err, env.Data)
		}
	}
	get := func(t *testing.T, path string, v any) {
		t.Helper()
		rec := e.do(t, http.MethodGet, path, sweepCase{}, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
		if v != nil {
			decode(t, rec, v)
		}
	}

	csv := "LUCA BORDRO DÖKÜMÜ\nDönem: 2026/07\n\n" +
		"Adı Soyadı;TC Kimlik No;Brüt Ücret;SGK İşçi Payı;İşsizlik İşçi Payı;Ödenecek Gelir Vergisi;Damga Vergisi;Net Ücret;SGK İşveren Payı;İşsizlik İşveren Payı;SGK Teşvik\n" +
		"Ayşe Yılmaz;10000000146;40.000,00;5.600,00;400,00;2.100,00;153,60;31.746,40;8.200,00;800,00;2.000,00\n" +
		"Mehmet Demir;12345678950;26.005,50;3.640,77;260,06;0,00;0,00;22.104,67;5.331,13;520,11;1.300,28\n"

	// Önizleme yazmaz.
	rec := form(t, base+"/imports?preview=true", nil, "luca.csv", csv, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	var prev struct {
		Period       string   `json:"period"`
		SourceSystem string   `json:"source_system"`
		Missing      []string `json:"missing"`
		LineCount    int      `json:"line_count"`
		Committed    bool     `json:"committed"`
	}
	decode(t, rec, &prev)
	if prev.Period != "2026-07" || prev.SourceSystem != "luca" || len(prev.Missing) != 0 || prev.LineCount != 2 || prev.Committed {
		t.Fatalf("preview %+v", prev)
	}
	var runs struct {
		Items []struct {
			UUID          string `json:"uuid"`
			Period        string `json:"period"`
			PaymentStatus string `json:"payment_status"`
			Totals        struct {
				Net string `json:"net_total"`
			} `json:"totals"`
			SGK *struct {
				Payable string `json:"payable_amount"`
				Status  string `json:"status"`
			} `json:"sgk"`
			JournalDraftUUID *string `json:"journal_draft_uuid"`
		} `json:"items"`
	}
	// Demo seed has its own runs (2026-09, 2026-10); this test uses July.
	july := func() int {
		for i, r := range runs.Items {
			if r.Period == "2026-07" {
				return i
			}
		}
		return -1
	}
	get(t, base+"/runs", &runs)
	if july() >= 0 {
		t.Fatalf("preview wrote a run: %+v", runs.Items)
	}

	// Dönem dosyada yoksa 422.
	if rec := form(t, base+"/imports", nil, "x.csv", "Ad Soyad;Brüt;Net\nAli Veli;100;80\n", false); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "PAYROLL_PERIOD_REQUIRED") {
		t.Fatalf("no period: %d %s", rec.Code, rec.Body)
	}

	rec = form(t, base+"/imports", nil, "luca.csv", csv, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}
	// Aynı dönem yeniden: satırlar değişir, personel çoğalmaz.
	if rec := form(t, base+"/imports", nil, "luca.csv", csv, false); rec.Code != http.StatusCreated {
		t.Fatalf("re-import: %d %s", rec.Code, rec.Body)
	}
	get(t, base+"/runs?year=2026", &runs)
	j := july()
	if j < 0 || runs.Items[j].Totals.Net != "53851.07" ||
		runs.Items[j].SGK == nil || runs.Items[j].SGK.Payable != "21451.79" || runs.Items[j].PaymentStatus != "unpaid" {
		t.Fatalf("runs %+v", runs.Items)
	}
	run := runs.Items[j].UUID

	var emps struct {
		Items []struct {
			FullName string  `json:"full_name"`
			Masked   *string `json:"national_id_masked"`
		} `json:"items"`
		Active int64 `json:"active"`
	}
	get(t, base+"/employees?q=Ayşe", &emps)
	if len(emps.Items) != 1 || emps.Items[0].Masked == nil || *emps.Items[0].Masked != "100******46" {
		t.Fatalf("employees %+v", emps)
	}
	if strings.Contains(e.do(t, http.MethodGet, base+"/employees", sweepCase{}, false).Body.String(), "10000000146") {
		t.Fatal("clear TCKN in the response")
	}
	if rec := e.do(t, http.MethodPost, base+"/employees", sweepCase{body: `{"full_name":"Kopya","national_id":"10000000146"}`}, false); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate TCKN: %d %s", rec.Code, rec.Body)
	}

	var detail struct {
		Lines []struct {
			EmployeeName string `json:"employee_name"`
			Net          string `json:"net"`
		} `json:"lines"`
	}
	get(t, base+"/runs/"+run, &detail)
	if len(detail.Lines) != 2 || detail.Lines[1].Net != "22104.67" {
		t.Fatalf("detail %+v", detail)
	}

	// Excel çıktısı.
	xrec := e.do(t, http.MethodGet, base+"/runs/"+run+"/export", sweepCase{}, false)
	if xrec.Code != http.StatusOK || !strings.HasPrefix(xrec.Body.String(), "PK") {
		t.Fatalf("export: %d %q", xrec.Code, xrec.Header().Get("Content-Type"))
	}

	// Fiş taslağı: dengeli, ikinci kez 409.
	rec = e.do(t, http.MethodPost, base+"/runs/"+run+"/journal-draft", sweepCase{}, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("journal draft: %d %s", rec.Code, rec.Body)
	}
	var draft struct {
		UUID   string `json:"uuid"`
		Debit  string `json:"debit"`
		Credit string `json:"credit"`
	}
	decode(t, rec, &draft)
	if draft.Debit != draft.Credit || draft.Debit != "77556.46" {
		t.Fatalf("draft %+v", draft)
	}
	if rec := e.do(t, http.MethodPost, base+"/runs/"+run+"/journal-draft", sweepCase{}, false); rec.Code != http.StatusConflict {
		t.Fatalf("second draft: %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/v1/tenant/engagements/"+eng+"/journal-drafts/"+draft.UUID+"/approve", sweepCase{}, false); rec.Code >= 300 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	// Onaylı fişin bordrosu yeniden içe aktarılmaz, silinmez.
	if rec := form(t, base+"/imports", nil, "luca.csv", csv, false); rec.Code != http.StatusConflict {
		t.Fatalf("re-import after approval: %d %s", rec.Code, rec.Body)
	}

	// Banka: toplu maaş ödemesi ve SGK ödemesi.
	rec = e.do(t, http.MethodPost, "/v1/tenant/engagements/"+eng+"/cash-boxes",
		sweepCase{body: `{"iban":"TR76 0001 0005 1978 6457 8413 26","name":"Ziraat TL"}`}, false)
	if rec.Code >= 300 {
		t.Fatalf("cash box: %d %s", rec.Code, rec.Body)
	}
	var box struct {
		UUID string `json:"uuid"`
	}
	decode(t, rec, &box)
	sta := "Tarih;Açıklama;Tutar;Bakiye\n05.08.2026;TEMMUZ MAAŞ ÖDEMESİ;-53.851,07;100.000,00\n28.08.2026;SGK PRİM ÖDEMESİ 2026/07;-21.451,79;78.548,21\n"
	if rec := form(t, "/v1/tenant/engagements/"+eng+"/bank-statements", map[string]string{"cash_box_uuid": box.UUID}, "ekstre.csv", sta, false); rec.Code >= 300 {
		t.Fatalf("statement: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(t, http.MethodPost, base+"/bank-match", sweepCase{body: `{"period":"2026-07"}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("bank match: %d %s", rec.Code, rec.Body)
	}
	var m struct {
		Salary int `json:"salary_matched"`
		SGK    int `json:"sgk_matched"`
	}
	decode(t, rec, &m)
	if m.Salary != 1 || m.SGK != 1 {
		t.Fatalf("match %+v (%s)", m, rec.Body)
	}
	get(t, base+"/runs", &runs)
	if j := july(); runs.Items[j].PaymentStatus != "paid" || runs.Items[j].SGK.Status != "paid" || runs.Items[j].JournalDraftUUID == nil {
		t.Fatalf("after match %+v", runs.Items[j])
	}
	var bank struct {
		Summary struct {
			Matched int64 `json:"matched"`
		} `json:"summary"`
	}
	get(t, "/v1/tenant/engagements/"+eng+"/bank-transactions?period=2026-08", &bank)
	if bank.Summary.Matched != 2 {
		t.Fatalf("bank summary %+v", bank.Summary)
	}

	// Bordro PDF'i: ofis yükler, mükellef görür ve indirir; gizli olan görünmez.
	pdf := "%PDF-1.4\n% ekim bordrosu\n"
	if rec := form(t, base+"/documents", map[string]string{"period": "2026-07"}, "ekim.txt", "not a pdf", false); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-pdf: %d %s", rec.Code, rec.Body)
	}
	rec = form(t, base+"/documents", map[string]string{"period": "2026-07", "title": "Temmuz bordrosu"}, "ekim-bordro.pdf", pdf, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("document: %d %s", rec.Code, rec.Body)
	}
	if rec := form(t, base+"/documents", map[string]string{"period": "2026-07"}, "ekim-bordro.pdf", pdf, false); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate document: %d", rec.Code)
	}
	if rec := form(t, base+"/documents", map[string]string{"period": "2026-07", "visible_to_taxpayer": "false"}, "ic.pdf", pdf+"% ic\n", false); rec.Code != http.StatusCreated {
		t.Fatalf("hidden document: %d %s", rec.Code, rec.Body)
	}
	var docs struct {
		Items []struct {
			UUID  string `json:"uuid"`
			Title string `json:"title"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	get(t, base+"/documents?period=2026-07", &docs)
	if docs.Total != 2 {
		t.Fatalf("office documents %+v", docs)
	}
	rec = e.do(t, http.MethodGet, "/v1/taxpayer/payroll/documents?period=2026-07", sweepCase{}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("taxpayer documents: %d %s", rec.Code, rec.Body)
	}
	decode(t, rec, &docs)
	if docs.Total != 1 || docs.Items[0].Title != "Temmuz bordrosu" {
		t.Fatalf("taxpayer documents %+v", docs)
	}
	frec := e.do(t, http.MethodGet, "/v1/taxpayer/payroll/documents/"+docs.Items[0].UUID+"/file", sweepCase{}, true)
	if frec.Code != http.StatusOK || frec.Body.String() != pdf || !strings.HasPrefix(frec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("taxpayer file: %d %q", frec.Code, frec.Header().Get("Content-Disposition"))
	}
	// Mükellef ofis uçlarına erişemez.
	if rec := e.do(t, http.MethodGet, base+"/runs", sweepCase{}, true); rec.Code < 400 {
		t.Fatalf("taxpayer on office route: %d", rec.Code)
	}
	for _, bad := range []string{"/runs?year=26", "/runs?limit=0", "/employees?status=x", "/documents?period=2026-13", "/sgk-premiums?offset=-1"} {
		if rec := e.do(t, http.MethodGet, base+bad, sweepCase{}, false); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
}
