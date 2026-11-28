package officedash

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Mükellefin iletişim bilgisini ofis düzenler (#780, Oğuz: "ne mailini
// görebiliyorum ne de telefon numarasını"). Telefon ve e-posta taxpayers
// satırında (000081), yetkili kişinin adı, telefonu ve e-postası da (000246);
// yazma tenant.engagements.write ister, her değişiklik
// etkinlik kaydına (activity_events) düşer.

// Field-level error codes of the contact form (Turkish, for the screens).
const (
	CodePhoneInvalid = "telefon_gecersiz"
	CodeEmailInvalid = "eposta_gecersiz"
	CodeNameInvalid  = "ad_gecersiz"
)

// ActionContactUpdated is the activity log action of a contact change.
const ActionContactUpdated = "taxpayer.contact_updated"

// Auditor records an audit event (activity.Recorder).
type Auditor interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, r *http.Request)
}

// NormalizeContactPhone checks and formats a phone number. Turkish numbers
// (+90, 0 or 10 digits; mobile, landline or 0850) become "0532 123 45 67";
// a foreign number written with "+" keeps its digits ("+4915112345678").
// "" stays "" (no phone).
func NormalizeContactPhone(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", true
	}
	plus := strings.HasPrefix(s, "+")
	var b strings.Builder
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' || (r == '+' && i == 0):
		default:
			return "", false
		}
	}
	d := b.String()
	if plus && !strings.HasPrefix(d, "90") {
		if len(d) < 7 || len(d) > 15 || d[0] == '0' {
			return "", false
		}
		return "+" + d, true
	}
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "90"):
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	// Alan kodu 2xx-4xx (sabit hat), 5xx (cep) ya da 850 (hizmet hattı).
	if len(d) != 10 || (d[0] < '2' || d[0] > '5') && !strings.HasPrefix(d, "850") {
		return "", false
	}
	return "0" + d[0:3] + " " + d[3:6] + " " + d[6:8] + " " + d[8:10], true
}

// NormalizeContactEmail checks an e-mail address (a bare address, no
// display name, a dotted domain, at most 254 characters) and lowercases it.
// "" stays "" (no e-mail).
func NormalizeContactEmail(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", true
	}
	if len(s) > 254 || strings.ContainsAny(s, " <>\"") {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return "", false
	}
	at := strings.LastIndex(s, "@")
	domain := s[at+1:]
	if at < 1 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return strings.ToLower(s), true
}

// ContactInput is the body of PUT .../contacts. A missing (or null) field
// keeps its value; "" clears it. The person_* fields are the taxpayer's
// authorized person (yetkili kişi, 000246): the accounting contact, who is
// often not the mükellef account owner.
type ContactInput struct {
	Phone       *string `json:"phone"`
	Email       *string `json:"email"`
	PersonName  *string `json:"person_name"`
	PersonPhone *string `json:"person_phone"`
	PersonEmail *string `json:"person_email"`
}

// ContactValues are the normalized contact fields ("" = not set).
type ContactValues struct {
	Phone, Email, PersonName, PersonPhone, PersonEmail string
}

// ContactPersonNameMax is the longest authorized person name (characters).
const ContactPersonNameMax = 120

// NormalizeContactPersonName trims a name and collapses inner whitespace;
// "" stays "" (no name). A name over ContactPersonNameMax characters or with
// control characters is invalid.
func NormalizeContactPersonName(raw string) (string, bool) {
	s := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(s) > ContactPersonNameMax {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

// ValidateContact normalizes the sent fields of in (the others stay "");
// details name the bad fields.
func ValidateContact(in ContactInput) (out ContactValues, details []response.Detail) {
	phone := func(field string, v *string, dst *string) {
		if v == nil {
			return
		}
		var ok bool
		if *dst, ok = NormalizeContactPhone(*v); !ok {
			details = append(details, response.Detail{Field: field, Code: CodePhoneInvalid,
				Message: "Telefon numarası geçersiz. Örnek: 0532 123 45 67"})
		}
	}
	email := func(field string, v *string, dst *string) {
		if v == nil {
			return
		}
		var ok bool
		if *dst, ok = NormalizeContactEmail(*v); !ok {
			details = append(details, response.Detail{Field: field, Code: CodeEmailInvalid,
				Message: "E-posta adresi geçersiz. Örnek: muhasebe@firma.com.tr"})
		}
	}
	phone("phone", in.Phone, &out.Phone)
	email("email", in.Email, &out.Email)
	if in.PersonName != nil {
		var ok bool
		if out.PersonName, ok = NormalizeContactPersonName(*in.PersonName); !ok {
			details = append(details, response.Detail{Field: "person_name", Code: CodeNameInvalid,
				Message: "Yetkili kişinin adı en fazla 120 karakter olabilir."})
		}
	}
	phone("person_phone", in.PersonPhone, &out.PersonPhone)
	email("person_email", in.PersonEmail, &out.PersonEmail)
	return out, details
}

func textOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// contactChanges lists the fields that changed, for the audit record.
func contactChanges(before, after ContactValues) map[string]any {
	changes := map[string]any{}
	add := func(key, from, to string, fold bool) {
		if from == to || fold && strings.EqualFold(from, to) {
			return
		}
		changes[key] = map[string]string{"from": from, "to": to}
	}
	add("phone", before.Phone, after.Phone, false)
	add("email", before.Email, after.Email, true)
	add("person_name", before.PersonName, after.PersonName, false)
	add("person_phone", before.PersonPhone, after.PersonPhone, false)
	add("person_email", before.PersonEmail, after.PersonEmail, true)
	return changes
}

// keep returns the old value for a field that was not sent.
func keep(sent *string, v, old string) string {
	if sent == nil {
		return old
	}
	return v
}

// UpdateContacts: PUT /v1/tenant/engagements/{engagementUuid}/contacts.
func (h *ClientsHandler) UpdateContacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := engagementctx.Must(ctx)
	var in ContactInput
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "Geçersiz istek gövdesi")
		return
	}
	v, details := ValidateContact(in)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	row, err := h.Q.GetEngagementTaxpayerContact(ctx, db.GetEngagementTaxpayerContactParams{OrganizationID: e.OrganizationID, EngagementID: e.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not load contacts")
		return
	}
	before := ContactValues{
		Phone: row.ContactPhone, Email: row.ContactEmail,
		PersonName: row.ContactPersonName, PersonPhone: row.ContactPersonPhone, PersonEmail: row.ContactPersonEmail,
	}
	v = ContactValues{
		Phone:       keep(in.Phone, v.Phone, before.Phone),
		Email:       keep(in.Email, v.Email, before.Email),
		PersonName:  keep(in.PersonName, v.PersonName, before.PersonName),
		PersonPhone: keep(in.PersonPhone, v.PersonPhone, before.PersonPhone),
		PersonEmail: keep(in.PersonEmail, v.PersonEmail, before.PersonEmail),
	}
	saved, err := h.Q.UpdateEngagementTaxpayerContact(ctx, db.UpdateEngagementTaxpayerContactParams{
		ContactPhone: textOrNull(v.Phone), ContactEmail: textOrNull(v.Email),
		ContactPersonName: textOrNull(v.PersonName), ContactPersonPhone: textOrNull(v.PersonPhone),
		ContactPersonEmail: textOrNull(v.PersonEmail),
		OrganizationID:     e.OrganizationID, EngagementID: e.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not save contacts")
		return
	}
	after := ContactValues{
		Phone: saved.ContactPhone, Email: saved.ContactEmail,
		PersonName: saved.ContactPersonName, PersonPhone: saved.ContactPersonPhone, PersonEmail: saved.ContactPersonEmail,
	}
	if changes := contactChanges(before, after); len(changes) > 0 && h.Audit != nil {
		var actor *int64
		if p, ok := authctx.PrincipalFrom(ctx); ok {
			uid := p.UserInternal
			actor = &uid
		}
		tp := saved.Uuid
		h.Audit.Record(ctx, actor, ActionContactUpdated, "taxpayer", &tp, map[string]any{
			"engagement_uuid": e.UUID.String(), "changes": changes,
		}, r)
	}
	h.Contacts(w, r)
}
