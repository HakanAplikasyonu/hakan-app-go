package officedash

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

func TestNormalizeContactPhone(t *testing.T) {
	ok := map[string]string{
		"":                   "",
		"0532 123 45 67":     "0532 123 45 67",
		"05321234567":        "0532 123 45 67",
		"5321234567":         "0532 123 45 67",
		"+90 532 123 45 67":  "0532 123 45 67",
		"+90 (532) 123-4567": "0532 123 45 67",
		"905321234567":       "0532 123 45 67",
		"0212 555 01 02":     "0212 555 01 02",
		"0850 111 22 33":     "0850 111 22 33",
		"+49 151 1234 5678":  "+4915112345678",
	}
	for in, want := range ok {
		got, valid := NormalizeContactPhone(in)
		if !valid || got != want {
			t.Errorf("NormalizeContactPhone(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"12345", "0632 123 45 67", "0532 123 45 6x", "0532-123-45-678", "+0 123 456 789", "tel: 0532 123 45 67", "0532 123 45 67 +"} {
		if got, valid := NormalizeContactPhone(in); valid {
			t.Errorf("NormalizeContactPhone(%q) = %q, want invalid", in, got)
		}
	}
}

func TestNormalizeContactEmail(t *testing.T) {
	ok := map[string]string{
		"":                        "",
		" Muhasebe@Ornek.com.tr ": "muhasebe@ornek.com.tr",
		"a.b+c@d.co":              "a.b+c@d.co",
	}
	for in, want := range ok {
		got, valid := NormalizeContactEmail(in)
		if !valid || got != want {
			t.Errorf("NormalizeContactEmail(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"muhasebe", "a@b", "a@b.", "@ornek.com", "Ali <ali@ornek.com>", "a b@ornek.com", "a@.com", strings.Repeat("a", 250) + "@b.co"} {
		if got, valid := NormalizeContactEmail(in); valid {
			t.Errorf("NormalizeContactEmail(%q) = %q, want invalid", in, got)
		}
	}
}

type fakeContactQ struct {
	ClientQueries
	phone, email                         string
	personName, personPhone, personEmail string
	tp                                   uuid.UUID
	updates                              int
}

func (f *fakeContactQ) GetEngagementTaxpayerContact(context.Context, db.GetEngagementTaxpayerContactParams) (db.GetEngagementTaxpayerContactRow, error) {
	return db.GetEngagementTaxpayerContactRow{Uuid: f.tp, Kind: "legal", TaxID: "9000000017", Title: "Örnek Ticaret A.Ş.",
		ContactPhone: f.phone, ContactEmail: f.email,
		ContactPersonName: f.personName, ContactPersonPhone: f.personPhone, ContactPersonEmail: f.personEmail}, nil
}

func (f *fakeContactQ) UpdateEngagementTaxpayerContact(_ context.Context, p db.UpdateEngagementTaxpayerContactParams) (db.UpdateEngagementTaxpayerContactRow, error) {
	f.updates++
	f.phone, f.email = p.ContactPhone.String, p.ContactEmail.String
	f.personName, f.personPhone, f.personEmail = p.ContactPersonName.String, p.ContactPersonPhone.String, p.ContactPersonEmail.String
	return db.UpdateEngagementTaxpayerContactRow{Uuid: f.tp, ContactPhone: f.phone, ContactEmail: f.email,
		ContactPersonName: f.personName, ContactPersonPhone: f.personPhone, ContactPersonEmail: f.personEmail}, nil
}

func (f *fakeContactQ) ListEngagementTaxpayerMembers(context.Context, db.ListEngagementTaxpayerMembersParams) ([]db.ListEngagementTaxpayerMembersRow, error) {
	return nil, nil
}

type auditCall struct {
	actor   *int64
	action  string
	payload map[string]any
}

type fakeAudit struct{ calls []auditCall }

func (f *fakeAudit) Record(_ context.Context, actor *int64, action, _ string, _ *uuid.UUID, payload map[string]any, _ *http.Request) {
	f.calls = append(f.calls, auditCall{actor, action, payload})
}

func putContacts(h *ClientsHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/v1/tenant/engagements/x/contacts", strings.NewReader(body))
	ctx := engagementctx.WithScope(req.Context(), engagementctx.Scope{ID: 7, UUID: uuid.New(), OrganizationID: 3})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: 42})
	rec := httptest.NewRecorder()
	h.UpdateContacts(rec, req.WithContext(ctx))
	return rec
}

func TestUpdateContactsValidation(t *testing.T) {
	q := &fakeContactQ{tp: uuid.New()}
	a := &fakeAudit{}
	h := &ClientsHandler{Q: q, Audit: a}
	rec := putContacts(h, `{"phone":"0632 1","email":"yanlis"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "VALIDATION_ERROR" || len(env.Error.Details) != 2 ||
		env.Error.Details[0].Code != CodePhoneInvalid || env.Error.Details[1].Code != CodeEmailInvalid {
		t.Fatalf("error = %+v", env.Error)
	}
	if q.updates != 0 || len(a.calls) != 0 {
		t.Fatalf("invalid input wrote: updates %d, audit %d", q.updates, len(a.calls))
	}
	if rec := putContacts(h, `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body = %d", rec.Code)
	}
}

func TestUpdateContactsSavesAndAudits(t *testing.T) {
	q := &fakeContactQ{tp: uuid.New(), phone: "0212 555 01 02"}
	a := &fakeAudit{}
	h := &ClientsHandler{Q: q, Audit: a}
	rec := putContacts(h, `{"phone":"+90 532 123 4567","email":"Muhasebe@Ornek.com.tr"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	if q.phone != "0532 123 45 67" || q.email != "muhasebe@ornek.com.tr" {
		t.Fatalf("saved = %q %q", q.phone, q.email)
	}
	var env struct {
		Data Contact `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Data.Phone == nil || *env.Data.Phone != "0532 123 45 67" {
		t.Fatalf("response = %s", rec.Body)
	}
	if len(a.calls) != 1 || a.calls[0].action != ActionContactUpdated || a.calls[0].actor == nil || *a.calls[0].actor != 42 {
		t.Fatalf("audit = %+v", a.calls)
	}
	changes, _ := a.calls[0].payload["changes"].(map[string]any)
	phone, _ := changes["phone"].(map[string]string)
	if phone["from"] != "0212 555 01 02" || phone["to"] != "0532 123 45 67" || changes["email"] == nil {
		t.Fatalf("changes = %+v", changes)
	}

	// Aynı değerler: kayıt yazılır ama denetim kaydı tekrar düşmez.
	if rec := putContacts(h, `{"phone":"0532 123 45 67","email":"muhasebe@ornek.com.tr"}`); rec.Code != http.StatusOK {
		t.Fatalf("same = %d", rec.Code)
	}
	if len(a.calls) != 1 {
		t.Fatalf("unchanged contact audited: %+v", a.calls)
	}

	// "" temizler; gönderilmeyen (ya da null) alan olduğu gibi kalır.
	if rec := putContacts(h, `{"phone":"","email":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear = %d", rec.Code)
	}
	if q.phone != "" || q.email != "muhasebe@ornek.com.tr" || len(a.calls) != 2 {
		t.Fatalf("clear: %q %q audit %d", q.phone, q.email, len(a.calls))
	}
}

func TestNormalizeContactPersonName(t *testing.T) {
	ok := map[string]string{
		"":                       "",
		"   ":                    "",
		"  Ayşe   Örnekoğlu ":    "Ayşe Örnekoğlu",
		strings.Repeat("ş", 120): strings.Repeat("ş", 120),
	}
	for in, want := range ok {
		got, valid := NormalizeContactPersonName(in)
		if !valid || got != want {
			t.Errorf("NormalizeContactPersonName(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{strings.Repeat("a", 121), "Ali\x00Veli"} {
		if got, valid := NormalizeContactPersonName(in); valid {
			t.Errorf("NormalizeContactPersonName(%q) = %q, want invalid", in, got)
		}
	}
}

func TestUpdateContactsPersonValidation(t *testing.T) {
	q := &fakeContactQ{tp: uuid.New()}
	a := &fakeAudit{}
	h := &ClientsHandler{Q: q, Audit: a}
	rec := putContacts(h, `{"person_name":"`+strings.Repeat("a", 121)+`","person_phone":"12","person_email":"yanlis"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	var env struct {
		Error struct {
			Details []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || len(env.Error.Details) != 3 {
		t.Fatalf("error = %s", rec.Body)
	}
	want := [][2]string{{"person_name", CodeNameInvalid}, {"person_phone", CodePhoneInvalid}, {"person_email", CodeEmailInvalid}}
	for i, d := range env.Error.Details {
		if d.Field != want[i][0] || d.Code != want[i][1] {
			t.Fatalf("detail %d = %+v, want %v", i, d, want[i])
		}
	}
	if q.updates != 0 || len(a.calls) != 0 {
		t.Fatalf("invalid input wrote: updates %d, audit %d", q.updates, len(a.calls))
	}
}

func TestUpdateContactsPersonSavesClearsAndAudits(t *testing.T) {
	q := &fakeContactQ{tp: uuid.New(), phone: "0212 555 01 02"}
	a := &fakeAudit{}
	h := &ClientsHandler{Q: q, Audit: a}
	rec := putContacts(h, `{"person_name":"  Deniz  Örnek ","person_phone":"05301112233","person_email":"Deniz@Ornek.com.tr"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	if q.personName != "Deniz Örnek" || q.personPhone != "0530 111 22 33" || q.personEmail != "deniz@ornek.com.tr" || q.phone != "0212 555 01 02" {
		t.Fatalf("saved = %+v", q)
	}
	var env struct {
		Data Contact `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Data.Person == nil || env.Data.Person.Name == nil ||
		*env.Data.Person.Name != "Deniz Örnek" || env.Data.Person.Phone == nil || *env.Data.Person.Phone != "0530 111 22 33" {
		t.Fatalf("response = %s", rec.Body)
	}
	if len(a.calls) != 1 {
		t.Fatalf("audit = %+v", a.calls)
	}
	changes, _ := a.calls[0].payload["changes"].(map[string]any)
	name, _ := changes["person_name"].(map[string]string)
	if name["from"] != "" || name["to"] != "Deniz Örnek" || changes["person_phone"] == nil || changes["person_email"] == nil || changes["phone"] != nil {
		t.Fatalf("changes = %+v", changes)
	}

	// Yalnız telefon silinir; ad ve e-posta (gönderilmedi / null) kalır.
	if rec := putContacts(h, `{"person_phone":"","person_email":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear = %d", rec.Code)
	}
	if q.personPhone != "" || q.personName != "Deniz Örnek" || q.personEmail != "deniz@ornek.com.tr" || len(a.calls) != 2 {
		t.Fatalf("clear: %+v audit %d", q, len(a.calls))
	}
	changes, _ = a.calls[1].payload["changes"].(map[string]any)
	if p, _ := changes["person_phone"].(map[string]string); p["from"] != "0530 111 22 33" || p["to"] != "" || len(changes) != 1 {
		t.Fatalf("clear changes = %+v", changes)
	}

	// Hepsi silinince yanıtta yetkili kişi null olur.
	rec = putContacts(h, `{"person_name":"","person_email":""}`)
	var cleared struct {
		Data Contact `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &cleared) != nil || cleared.Data.Person != nil {
		t.Fatalf("all cleared = %d %s", rec.Code, rec.Body)
	}
}
