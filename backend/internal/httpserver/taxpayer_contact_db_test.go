package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
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

// Yetkili kişi (#780, 000246): ad, telefon ve e-posta aynı PUT ile yazılır;
// biçim denetimi alan alan döner, "" yalnız o alanı siler, eski→yeni değerler
// denetim kaydına düşer; salt okur 403 alır ve hiçbir şey yazılmaz.
func TestTaxpayerContactPersonUpdate(t *testing.T) {
	e := newAuthzEnv(t)
	eng := e.param(t, "engagementUuid", "")
	path := "/v1/tenant/engagements/" + eng + "/contacts"

	reader := matrixCaller{"readerA", e.readerTok, map[string]string{"X-Organization": "demo-smmm"}}
	if rec := e.request(t, reader, http.MethodPut, path, sweepCase{body: `{"person_name":"Okur Deneme"}`}); rec.Code != http.StatusForbidden {
		t.Fatalf("reader put: %d %s", rec.Code, rec.Body)
	}
	if n := e.one(t, `SELECT count(*)::text FROM taxpayers WHERE id = $3 AND contact_person_name = 'Okur Deneme' AND $1 > 0 AND $2 > 0`, "reader"); n != "0" {
		t.Fatalf("reader wrote the person: %s", n)
	}

	rec := e.do(t, http.MethodPut, path, sweepCase{body: `{"person_name":"` + strings.Repeat("a", 121) + `","person_phone":"12","person_email":"yanlis"}`}, false)
	var bad struct {
		Error struct {
			Details []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"details"`
		} `json:"error"`
	}
	if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &bad) != nil || len(bad.Error.Details) != 3 ||
		bad.Error.Details[0].Code != "ad_gecersiz" || bad.Error.Details[1].Field != "person_phone" || bad.Error.Details[2].Field != "person_email" {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body)
	}

	type person struct {
		Name  *string `json:"name"`
		Phone *string `json:"phone"`
		Email *string `json:"email"`
	}
	var ok struct {
		Data struct {
			Person *person `json:"person"`
		} `json:"data"`
	}
	rec = e.do(t, http.MethodPut, path, sweepCase{body: `{"person_name":"  Deniz   Örnek ","person_phone":"+90 530 111 2233","person_email":"Deniz@Ornek-Firma.com.tr"}`}, false)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &ok) != nil || ok.Data.Person == nil ||
		ok.Data.Person.Name == nil || *ok.Data.Person.Name != "Deniz Örnek" ||
		ok.Data.Person.Phone == nil || *ok.Data.Person.Phone != "0530 111 22 33" ||
		ok.Data.Person.Email == nil || *ok.Data.Person.Email != "deniz@ornek-firma.com.tr" {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	if n := e.one(t, `SELECT count(*)::text FROM activity_events WHERE organization_id = $1 AND action = 'taxpayer.contact_updated'
		AND payload->'changes'->'person_name'->>'to' = 'Deniz Örnek'
		AND payload->>'engagement_uuid' = (SELECT uuid::text FROM engagements WHERE id = $2) AND $3 > 0`, "audit"); n != "1" {
		t.Fatalf("audit rows = %s", n)
	}

	// "" yalnız telefonu siler; gönderilmeyen ad ve null e-posta kalır.
	rec = e.do(t, http.MethodPut, path, sweepCase{body: `{"person_phone":"","person_email":null}`}, false)
	ok.Data.Person = nil
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &ok) != nil || ok.Data.Person == nil ||
		ok.Data.Person.Phone != nil || ok.Data.Person.Name == nil || ok.Data.Person.Email == nil {
		t.Fatalf("clear phone: %d %s", rec.Code, rec.Body)
	}
	if n := e.one(t, `SELECT count(*)::text FROM activity_events WHERE organization_id = $1 AND action = 'taxpayer.contact_updated'
		AND payload->'changes'->'person_phone'->>'from' = '0530 111 22 33' AND payload->'changes'->'person_phone'->>'to' = ''
		AND NOT payload->'changes' ? 'person_name' AND $2 > 0 AND $3 > 0`, "audit clear"); n != "1" {
		t.Fatalf("clear audit rows = %s", n)
	}
}
