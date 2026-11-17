package statement

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/text/encoding/charmap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// #465: a bank account's movements as a Luca "Banka Ekstreleri" CSV
// (Windows-1254, dd.MM.yyyy, decimal comma, signed amount by default), the
// history, the "exported again" mark and the mapping guide.
func TestLucaBankStatementExport(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	file := "Hesap IBAN;" + ibanZiraat + "\nİşlem Tarihi;Açıklama;Borç;Alacak;Bakiye\n" +
		"01.10.2026;EFT GELEN DEMİR GIDA ŞTİ;;1.250,00;11.250,00\n" +
		"02.10.2026;KIRA ODEMESI;3.000,00;;8.250,00\n" +
		"03.11.2026;POS;45,90;;8.204,10\n"
	if _, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "e.csv", Data: []byte(file)}); err != nil {
		t.Fatal(err)
	}
	var boxUUID uuid.UUID
	if err := f.d.Pool.QueryRow(ctx, `SELECT uuid FROM cash_boxes WHERE id = $1`, f.box).Scan(&boxUUID); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(f.im, f.bank, f.q)
	scope := engagementctx.Scope{ID: f.eng, OrganizationID: f.org, TaxpayerID: f.tp}
	call := func(fn http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/x", strings.NewReader(body))
		r = r.WithContext(engagementctx.WithScope(r.Context(), scope))
		w := httptest.NewRecorder()
		fn(w, r)
		return w
	}
	body := `{"cash_box_uuid":"` + boxUUID.String() + `","date_from":"2026-10-01","date_to":"2026-10-31"}`
	w := call(h.lucaExport, http.MethodPost, body)
	if w.Code != http.StatusOK || w.Header().Get("X-Repeated-Count") != "0" || w.Header().Get("X-Template-Verified") != "false" {
		t.Fatalf("export: %d %s %v", w.Code, w.Body.String(), w.Header())
	}
	data := w.Body.Bytes()
	if bytes.Contains(data, []byte{0xC4}) || bytes.Contains(data, []byte{0xC5}) || !bytes.Contains(data, []byte{0xDE}) { // Ş = DE, no UTF-8
		t.Fatalf("encoding: % X", data)
	}
	text, _ := charmap.Windows1254.NewDecoder().Bytes(data)
	lines := strings.Split(string(text), "\r\n")
	if len(lines) != 3 || lines[0] != "Evrak Tarihi;Evrak No;Açıklama;Tutar;VKN/TCKN;IBAN;Döviz" ||
		!strings.HasPrefix(lines[1], "01.10.2026;;EFT GELEN DEMİR GIDA ŞTİ;1250,00;") || !strings.HasPrefix(lines[2], "02.10.2026;;KIRA ODEMESI;-3000,00;") {
		t.Fatalf("file:\n%s", text)
	}
	// Two-column layout, comma delimiter; every movement is a repeat now.
	w = call(h.lucaExport, http.MethodPost, strings.TrimSuffix(body, "}")+`,"amount_layout":"two_columns","delimiter":","}`)
	if w.Code != http.StatusOK || w.Header().Get("X-Repeated-Count") != "2" {
		t.Fatalf("second export: %d %s", w.Code, w.Body.String())
	}
	text, _ = charmap.Windows1254.NewDecoder().Bytes(w.Body.Bytes())
	if !strings.HasPrefix(string(text), "Evrak Tarihi,Evrak No,Açıklama,Borç,Alacak,") || !strings.Contains(string(text), "KIRA ODEMESI,\"3000,00\",") {
		t.Fatalf("two columns:\n%s", text)
	}
	// History, newest first.
	var hist struct {
		Data struct {
			Items []LucaExportView `json:"items"`
		} `json:"data"`
	}
	if w := call(h.lucaExports, http.MethodGet, ""); w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &hist) != nil ||
		len(hist.Data.Items) != 2 || hist.Data.Items[0].Repeated != 2 || hist.Data.Items[0].AmountLayout != "two_columns" ||
		hist.Data.Items[1].Lines != 2 || hist.Data.Items[1].CashBoxUUID != boxUUID || hist.Data.Items[0].TemplateVerified {
		t.Fatalf("history: %+v", hist)
	}
	// The bank list marks the movements exported twice; November's none.
	var page struct {
		Data struct {
			Items []TransactionView `json:"items"`
		} `json:"data"`
	}
	r := httptest.NewRequest(http.MethodGet, "/x?limit=50", nil)
	r = r.WithContext(engagementctx.WithScope(r.Context(), scope))
	lw := httptest.NewRecorder()
	h.listTransactions(lw, r)
	if lw.Code != http.StatusOK || json.Unmarshal(lw.Body.Bytes(), &page) != nil || len(page.Data.Items) != 3 {
		t.Fatalf("list: %d %s", lw.Code, lw.Body.String())
	}
	for _, it := range page.Data.Items {
		want := int32(2)
		if it.Period == "2026-11" {
			want = 0
		}
		if it.LucaExports != want || (want > 0) != (it.LucaLastExportAt != nil) {
			t.Fatalf("mark %s: %d %v", it.Date, it.LucaExports, it.LucaLastExportAt)
		}
	}
	// The guide lists both layouts with Luca's fiş sütunu and veri türü.
	var guide struct {
		Data LucaGuide `json:"data"`
	}
	if w := call(h.lucaGuide, http.MethodGet, ""); w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &guide) != nil ||
		guide.Data.TemplateVerified || len(guide.Data.Columns["signed"]) != 7 || len(guide.Data.Columns["two_columns"]) != 8 ||
		guide.Data.Columns["signed"][0].DataType != "Tarih" || guide.Data.Encoding != "windows-1254" {
		t.Fatalf("guide: %+v", guide)
	}
	// Errors: empty range, bad dates, unknown account, bad layout.
	for _, c := range []struct {
		body string
		code int
	}{
		{`{"cash_box_uuid":"` + boxUUID.String() + `","date_from":"2025-01-01","date_to":"2025-01-31"}`, http.StatusUnprocessableEntity},
		{`{"cash_box_uuid":"` + boxUUID.String() + `","date_from":"2026-10-31","date_to":"2026-10-01"}`, http.StatusBadRequest},
		{`{"cash_box_uuid":"` + uuid.NewString() + `","date_from":"2026-10-01","date_to":"2026-10-31"}`, http.StatusNotFound},
		{strings.TrimSuffix(body, "}") + `,"amount_layout":"split"}`, http.StatusBadRequest},
	} {
		if w := call(h.lucaExport, http.MethodPost, c.body); w.Code != c.code {
			t.Errorf("%s: %d %s", c.body, w.Code, w.Body.String())
		}
	}
}
