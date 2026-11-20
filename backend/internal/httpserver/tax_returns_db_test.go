package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Beyanname önizlemesi (#390): demo ofisin verisinden KDV1, MUHSGK ve Ba/Bs;
// taslak düzeltmesi, kontrol listesi onayı, hazırlama (donmuş görüntü),
// devreden KDV'nin sonraki döneme geçmesi, verildi kaydı. Erişim kuralları
// yetki matrisinde (authz_table_test).
func TestTaxReturnFlow(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng + "/tax-returns"

	type preview struct {
		Kind         string   `json:"kind"`
		Period       string   `json:"period"`
		Status       string   `json:"status"`
		Frozen       bool     `json:"frozen"`
		OpenWarnings []string `json:"open_warnings"`
		Calendar     struct {
			DueDate string `json:"due_date"`
		} `json:"calendar"`
		Checklist []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"checklist"`
		Record *struct {
			Status        string  `json:"status"`
			AccrualNumber *string `json:"accrual_number"`
		} `json:"record"`
		KDV1 *struct {
			Sales []struct {
				Rate string `json:"rate"`
			} `json:"sales"`
			Totals struct {
				CalculatedVAT string `json:"calculated_vat"`
				CarriedIn     string `json:"carried_in"`
				Payable       string `json:"payable"`
				CarryOut      string `json:"carry_out"`
			} `json:"totals"`
			Carry struct {
				Source string  `json:"source"`
				Period *string `json:"period"`
			} `json:"carry"`
		} `json:"kdv1"`
		MUHSGK *struct {
			Payroll *struct {
				EmployeeCount int `json:"employee_count"`
			} `json:"payroll"`
			Employees []struct {
				NationalIDMasked string `json:"national_id_masked"`
			} `json:"employees"`
			Totals struct {
				TaxPayable string `json:"tax_payable"`
			} `json:"totals"`
		} `json:"muhsgk"`
		BABS *struct {
			Threshold string `json:"threshold"`
			BA        struct {
				Rows []json.RawMessage `json:"rows"`
			} `json:"ba"`
		} `json:"babs"`
	}
	call := func(t *testing.T, method, path, body string, want int) preview {
		t.Helper()
		rec := e.do(t, method, path, sweepCase{body: body}, false)
		if rec.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		var env struct {
			Data preview `json:"data"`
		}
		if want == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
		}
		return env.Data
	}
	raw := func(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return e.do(t, method, path, sweepCase{body: body}, false)
	}

	// Demo: Eylül KDV1 verildi, Ekim taslak; Ekim devreden KDV'yi Eylül'den alır.
	p := call(t, http.MethodGet, base+"/kdv1/2026-10", "", http.StatusOK)
	if p.Status != "draft" || p.Frozen || p.Record == nil || p.KDV1 == nil || p.Calendar.DueDate != "2026-11-30" ||
		p.KDV1.Carry.Source != "previous_return" {
		t.Fatalf("kdv1 preview %+v", p)
	}
	sep := call(t, http.MethodGet, base+"/kdv1/2026-09", "", http.StatusOK)
	if sep.Status != "filed" || !sep.Frozen || sep.KDV1.Totals.CarryOut != p.KDV1.Totals.CarriedIn {
		t.Fatalf("september %+v, october carried in %s", sep, p.KDV1.Totals.CarriedIn)
	}
	// Kayıt yokken önizleme yazmaz.
	if aug := call(t, http.MethodGet, base+"/kdv1/2026-08", "", http.StatusOK); aug.Record != nil || aug.Status != "draft" {
		t.Fatalf("august %+v", aug)
	}
	if len(p.KDV1.Sales) == 0 {
		t.Errorf("demo October has sales: %+v", p.KDV1)
	}

	// Düzeltme: devreden KDV elle.
	p = call(t, http.MethodPut, base+"/kdv1/2026-10", `{"adjustments":{"carried_in":"100.00"},"note":"Ekim"}`, http.StatusOK)
	if p.Record == nil || p.KDV1.Carry.Source != "manual" || p.KDV1.Totals.CarriedIn != "100.00" {
		t.Fatalf("draft %+v", p)
	}
	if rec := raw(t, http.MethodPut, base+"/kdv1/2026-10", `{"adjustments":{"carried_in":"-5"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative adjustment: %d %s", rec.Code, rec.Body)
	}

	// Onaylanmamış uyarı varsa hazırlanamaz.
	if len(p.OpenWarnings) > 0 {
		rec := raw(t, http.MethodPost, base+"/kdv1/2026-10/prepare", "")
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "TAX_RETURN_UNACKNOWLEDGED") {
			t.Fatalf("prepare with open warnings: %d %s", rec.Code, rec.Body)
		}
	}
	acks, _ := json.Marshal(map[string]any{"acknowledged": p.OpenWarnings})
	p = call(t, http.MethodPost, base+"/kdv1/2026-10/prepare", string(acks), http.StatusOK)
	if p.Status != "prepared" || !p.Frozen || len(p.OpenWarnings) != 0 {
		t.Fatalf("prepared %+v", p)
	}
	octCarry := p.KDV1.Totals.CarryOut
	if rec := raw(t, http.MethodPut, base+"/kdv1/2026-10", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("edit prepared: %d %s", rec.Code, rec.Body)
	}

	// Sonraki dönem devreden KDV'yi hazırlanan Ekim'den alır.
	nov := call(t, http.MethodGet, base+"/kdv1/2026-11", "", http.StatusOK)
	if nov.KDV1.Carry.Source != "previous_return" || nov.KDV1.Carry.Period == nil || *nov.KDV1.Carry.Period != "2026-10" ||
		nov.KDV1.Totals.CarriedIn != octCarry {
		t.Fatalf("november carry %+v (october carry out %s)", nov.KDV1, octCarry)
	}

	// Geri aç, yeniden hazırla, verildi.
	p = call(t, http.MethodPost, base+"/kdv1/2026-10/reopen", "", http.StatusOK)
	if p.Status != "draft" || p.Frozen {
		t.Fatalf("reopened %+v", p)
	}
	if rec := raw(t, http.MethodPost, base+"/kdv1/2026-10/file", `{"filed_on":"2026-11-10"}`); rec.Code != http.StatusConflict {
		t.Fatalf("file draft: %d %s", rec.Code, rec.Body)
	}
	call(t, http.MethodPost, base+"/kdv1/2026-10/prepare", "", http.StatusOK)
	if rec := raw(t, http.MethodPost, base+"/kdv1/2026-10/file", `{"filed_on":"2026-09-01"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("filed before the period ended: %d %s", rec.Code, rec.Body)
	}
	p = call(t, http.MethodPost, base+"/kdv1/2026-10/file", `{"filed_on":"2026-11-10","accrual_number":"KDV-2026-10"}`, http.StatusOK)
	if p.Status != "filed" || p.Record.AccrualNumber == nil || *p.Record.AccrualNumber != "KDV-2026-10" {
		t.Fatalf("filed %+v", p)
	}
	if rec := raw(t, http.MethodPost, base+"/kdv1/2026-10/reopen", ""); rec.Code != http.StatusConflict {
		t.Fatalf("reopen filed: %d %s", rec.Code, rec.Body)
	}

	// MUHSGK: demo Ekim bordrosundan; TCKN maskeli.
	m := call(t, http.MethodGet, base+"/muhsgk/2026-10", "", http.StatusOK)
	if m.MUHSGK == nil || m.MUHSGK.Payroll == nil || m.MUHSGK.Payroll.EmployeeCount == 0 || m.Calendar.DueDate != "2026-11-26" {
		t.Fatalf("muhsgk %+v", m)
	}
	for _, emp := range m.MUHSGK.Employees {
		if emp.NationalIDMasked != "" && !strings.Contains(emp.NationalIDMasked, "*") {
			t.Errorf("national id not masked: %s", emp.NationalIDMasked)
		}
	}
	if rec := raw(t, http.MethodPut, base+"/muhsgk/2026-10", `{"adjustments":{"carried_in":"1.00"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("muhsgk adjustments: %d %s", rec.Code, rec.Body)
	}

	// Ba/Bs.
	b := call(t, http.MethodGet, base+"/babs/2026-10", "", http.StatusOK)
	if b.BABS == nil || b.BABS.Threshold != "5000.00" {
		t.Fatalf("babs %+v", b)
	}

	// Excel çıktısı: verilmiş KDV1 donmuş görüntüsüyle.
	rec := raw(t, http.MethodGet, base+"/kdv1/2026-10/export", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "kdv1-2026-10.xlsx") ||
		!bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}
	if rec := raw(t, http.MethodGet, base+"/babs/2026-10/export", ""); rec.Code != http.StatusOK {
		t.Fatalf("babs export: %d %s", rec.Code, rec.Body)
	}

	// Liste ve hatalı istekler.
	rec = raw(t, http.MethodGet, base+"?year=2026", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"filed"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := raw(t, http.MethodGet, base+"/kdv2/2026-10", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown kind: %d", rec.Code)
	}
	if rec := raw(t, http.MethodGet, base+"/kdv1/2026-13", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad period: %d", rec.Code)
	}
}

// #575: mükelleften beklenen ve gelip işlenmeyen belgeler beyan kontrol
// listesinde onay isteyen madde olur; belge eşleşince madde kalkar.
func TestTaxReturnDocumentChecklist(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	url := "/v1/tenant/engagements/" + eng + "/tax-returns/muhsgk/2026-10"
	codes := func() map[string]string {
		t.Helper()
		rec := e.do(t, http.MethodGet, url, sweepCase{}, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("preview: %d %s", rec.Code, rec.Body)
		}
		var env struct {
			Data struct {
				Checklist []struct {
					Code     string `json:"code"`
					Severity string `json:"severity"`
					Message  string `json:"message"`
				} `json:"checklist"`
				OpenWarnings []string `json:"open_warnings"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, c := range env.Data.Checklist {
			out[c.Code] = c.Severity + " " + c.Message
		}
		for _, w := range env.Data.OpenWarnings {
			out["open:"+w] = ""
		}
		return out
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := rls.System(context.Background(), e.d.Pool, "test: document tasks", func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM document_tasks WHERE engagement_id = $1`, e.engID)
	if c := codes(); c["OPEN_DOCUMENT_TASKS"] != "" || c["SUBMITTED_DOCUMENTS"] != "" {
		t.Fatalf("no tasks yet: %v", c)
	}
	exec(`INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, code, kind, period, status, title, note, due_at,
			next_reminder_at, created_by, submitted_at)
		SELECT $1, $2, $3, 'DOC_REQUEST', 'document_request', '2026-10', s, 'Kira sözleşmesi', '', now(), now(),
		       (SELECT user_id FROM organization_members WHERE organization_id = $1 ORDER BY id LIMIT 1),
		       CASE WHEN s = 'submitted' THEN now() END
		FROM unnest(ARRAY['open', 'open', 'submitted']) s`, e.orgID, e.engID, e.tpID)
	c := codes()
	if !strings.HasPrefix(c["OPEN_DOCUMENT_TASKS"], "warning 2 belge") || !strings.Contains(c["OPEN_DOCUMENT_TASKS"], "Kira sözleşmesi") ||
		!strings.HasPrefix(c["SUBMITTED_DOCUMENTS"], "warning") {
		t.Fatalf("checklist %v", c)
	}
	if _, ok := c["open:OPEN_DOCUMENT_TASKS"]; !ok {
		t.Fatalf("not waiting for acknowledgement: %v", c)
	}
	exec(`UPDATE document_tasks SET status = 'matched', matched_at = now(), submitted_at = COALESCE(submitted_at, now())
		WHERE engagement_id = $1`, e.engID)
	if c := codes(); c["OPEN_DOCUMENT_TASKS"] != "" || c["SUBMITTED_DOCUMENTS"] != "" {
		t.Fatalf("after matching: %v", c)
	}
}
