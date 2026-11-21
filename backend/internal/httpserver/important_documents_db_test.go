package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// Önemli belgeler (#780 B): yüklerken ya da sonradan "önemli" işareti,
// important=true süzgeci, "Mükellef de görsün" kapatma, geçersiz tür 400 ve
// ofis ayarı (varsayılan açık).
func TestImportantDocumentsFlow(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng + "/archive-documents"

	type doc struct {
		UUID              string  `json:"uuid"`
		Title             string  `json:"title"`
		ImportantKind     *string `json:"important_kind"`
		VisibleToTaxpayer bool    `json:"visible_to_taxpayer"`
		ExpiryStatus      string  `json:"expiry_status"`
	}
	type page struct {
		Items  []doc `json:"items"`
		Counts struct {
			Important         int64 `json:"important"`
			ImportantExpiring int64 `json:"important_expiring"`
		} `json:"counts"`
	}
	decode := func(t *testing.T, body []byte, v any) {
		t.Helper()
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(env.Data, v); err != nil {
			t.Fatalf("%v: %s", err, env.Data)
		}
	}
	day := func(d int) string { return time.Now().AddDate(0, 0, d).Format(time.DateOnly) }

	rec := e.do(t, http.MethodPost, base, sweepCase{file: "file", fileName: "levha.pdf", fileBody: "%PDF-1.4\n% vergi levhasi onemli\n",
		fields: map[string]string{"category": "tax", "title": "Vergi levhası", "important_kind": "tax_certificate",
			"valid_until": day(10), "visible_to_taxpayer": "true"}}, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var levha doc
	decode(t, rec.Body.Bytes(), &levha)
	if levha.ImportantKind == nil || *levha.ImportantKind != "tax_certificate" || levha.ExpiryStatus != "expiring" {
		t.Fatalf("levha = %+v", levha)
	}

	rec = e.do(t, http.MethodPost, base, sweepCase{file: "file", fileName: "imza.pdf", fileBody: "%PDF-1.4\n% imza sirkuleri sonradan\n",
		fields: map[string]string{"category": "trade_registry", "title": "İmza sirküleri"}}, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload 2: %d %s", rec.Code, rec.Body)
	}
	var imza doc
	decode(t, rec.Body.Bytes(), &imza)
	if imza.ImportantKind != nil {
		t.Fatalf("imza important = %v", *imza.ImportantKind)
	}

	// Mevcut belge önemli olarak işaretlenir.
	rec = e.do(t, http.MethodPatch, base+"/"+imza.UUID, sweepCase{body: `{"important_kind":"signature_circular","visible_to_taxpayer":true}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	decode(t, rec.Body.Bytes(), &imza)
	if imza.ImportantKind == nil || *imza.ImportantKind != "signature_circular" || !imza.VisibleToTaxpayer {
		t.Fatalf("imza = %+v", imza)
	}

	rec = e.do(t, http.MethodGet, base+"?important=true", sweepCase{}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var p page
	decode(t, rec.Body.Bytes(), &p)
	if len(p.Items) < 2 || p.Counts.Important < 2 || p.Counts.ImportantExpiring < 1 {
		t.Fatalf("important page = %+v", p)
	}
	if p.Items[0].UUID != levha.UUID { // bitişi yakın olan önce
		t.Fatalf("first = %s, want levha", p.Items[0].Title)
	}

	// "Mükellef görebilir" kapatılır; işaret kaldırılır.
	rec = e.do(t, http.MethodPatch, base+"/"+levha.UUID, sweepCase{body: `{"visible_to_taxpayer":false}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("hide: %d %s", rec.Code, rec.Body)
	}
	decode(t, rec.Body.Bytes(), &levha)
	if levha.VisibleToTaxpayer || levha.ImportantKind == nil {
		t.Fatalf("hidden levha = %+v", levha)
	}
	rec = e.do(t, http.MethodPatch, base+"/"+imza.UUID, sweepCase{body: `{"important_kind":""}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("unmark: %d %s", rec.Code, rec.Body)
	}
	decode(t, rec.Body.Bytes(), &imza)
	if imza.ImportantKind != nil {
		t.Fatalf("unmarked imza = %+v", imza)
	}
	if rec := e.do(t, http.MethodPatch, base+"/"+imza.UUID, sweepCase{body: `{"important_kind":"pasaport"}`}, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad kind: %d %s", rec.Code, rec.Body)
	}

	// Ofis ayarı: varsayılan açık, kapatılıp geri açılır.
	type settings struct {
		Enabled bool `json:"enabled"`
	}
	var st settings
	rec = e.do(t, http.MethodGet, "/v1/tenant/important-document-settings", sweepCase{}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	decode(t, rec.Body.Bytes(), &st)
	if !st.Enabled {
		t.Fatal("default setting should be on")
	}
	for _, v := range []string{`{"enabled":false}`, `{"enabled":true}`} {
		if rec := e.do(t, http.MethodPut, "/v1/tenant/important-document-settings", sweepCase{body: v}, false); rec.Code != http.StatusOK {
			t.Fatalf("put %s: %d %s", v, rec.Code, rec.Body)
		}
	}
	if rec := e.do(t, http.MethodPut, "/v1/tenant/important-document-settings", sweepCase{body: `{}`}, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("put empty: %d %s", rec.Code, rec.Body)
	}
}
