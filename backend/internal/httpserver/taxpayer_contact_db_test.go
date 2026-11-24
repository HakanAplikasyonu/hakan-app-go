package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Mükellef iletişim bilgisi (#780): yazma yetkisi olan ofis üyesi telefonu ve
// e-postayı düzenler (biçim denetimi, Türkiye numarası düzeltilir, denetim
// kaydı düşer); salt okur 403 alır; mükellef kendi bilgisini /context'te görür.
func TestTaxpayerContactUpdate(t *testing.T) {
	e := newAuthzEnv(t)
	eng := e.param(t, "engagementUuid", "")
	path := "/v1/tenant/engagements/" + eng + "/contacts"

	reader := matrixCaller{"readerA", e.readerTok, map[string]string{"X-Organization": "demo-smmm"}}
	if rec := e.request(t, reader, http.MethodGet, path, sweepCase{}); rec.Code != http.StatusOK {
		t.Fatalf("reader get: %d %s", rec.Code, rec.Body)
	}
	if rec := e.request(t, reader, http.MethodPut, path, sweepCase{body: `{"phone":"0532 000 00 00"}`}); rec.Code != http.StatusForbidden {
		t.Fatalf("reader put: %d %s", rec.Code, rec.Body)
	}

	rec := e.do(t, http.MethodPut, path, sweepCase{body: `{"phone":"12","email":"yanlis"}`}, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body)
	}
	var bad struct {
		Error struct {
			Details []struct {
				Code string `json:"code"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bad); err != nil || len(bad.Error.Details) != 2 || bad.Error.Details[0].Code != "telefon_gecersiz" {
		t.Fatalf("invalid body: %s", rec.Body)
	}

	rec = e.do(t, http.MethodPut, path, sweepCase{body: `{"phone":"+90 (532) 111 2233","email":"Iletisim@Ornek-Firma.com.tr"}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	var ok struct {
		Data struct {
			Phone *string `json:"phone"`
			Email *string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil || ok.Data.Phone == nil || *ok.Data.Phone != "0532 111 22 33" ||
		ok.Data.Email == nil || *ok.Data.Email != "iletisim@ornek-firma.com.tr" {
		t.Fatalf("put body: %s", rec.Body)
	}
	if n := e.one(t, `SELECT count(*)::text FROM activity_events WHERE organization_id = $1 AND action = 'taxpayer.contact_updated'
		AND payload->>'engagement_uuid' = (SELECT uuid::text FROM engagements WHERE id = $2) AND $3 > 0`, "audit"); n != "1" {
		t.Fatalf("audit rows = %s", n)
	}

	rec = e.do(t, http.MethodGet, "/v1/taxpayer/context", sweepCase{}, true)
	var ctx struct {
		Data struct {
			Contact struct {
				Phone *string `json:"phone"`
				Email *string `json:"email"`
			} `json:"contact"`
		} `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &ctx) != nil || ctx.Data.Contact.Phone == nil || *ctx.Data.Contact.Phone != "0532 111 22 33" {
		t.Fatalf("taxpayer context: %d %s", rec.Code, rec.Body)
	}
}
