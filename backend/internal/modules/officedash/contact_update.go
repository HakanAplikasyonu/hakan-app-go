package officedash

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

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
// satırında (000081); yazma tenant.engagements.write ister, her değişiklik
// etkinlik kaydına (activity_events) düşer.

// Field-level error codes of the contact form (Turkish, for the screens).
const (
	CodePhoneInvalid = "telefon_gecersiz"
	CodeEmailInvalid = "eposta_gecersiz"
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
// keeps its value; "" clears it.
type ContactInput struct {
	Phone *string `json:"phone"`
	Email *string `json:"email"`
}

// ValidateContact normalizes the input; details name the bad fields.
func ValidateContact(in ContactInput) (phone, email string, details []response.Detail) {
	var ok bool
	if in.Phone != nil {
		if phone, ok = NormalizeContactPhone(*in.Phone); !ok {
			details = append(details, response.Detail{Field: "phone", Code: CodePhoneInvalid,
				Message: "Telefon numarası geçersiz. Örnek: 0532 123 45 67"})
		}
	}
	if in.Email != nil {
		if email, ok = NormalizeContactEmail(*in.Email); !ok {
			details = append(details, response.Detail{Field: "email", Code: CodeEmailInvalid,
				Message: "E-posta adresi geçersiz. Örnek: muhasebe@firma.com.tr"})
		}
	}
	return phone, email, details
}

func textOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// contactChanges lists the fields that changed, for the audit record.
func contactChanges(beforePhone, beforeEmail, afterPhone, afterEmail string) map[string]any {
	changes := map[string]any{}
	if beforePhone != afterPhone {
		changes["phone"] = map[string]string{"from": beforePhone, "to": afterPhone}
	}
	if !strings.EqualFold(beforeEmail, afterEmail) {
		changes["email"] = map[string]string{"from": beforeEmail, "to": afterEmail}
	}
	return changes
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
	phone, email, details := ValidateContact(in)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	before, err := h.Q.GetEngagementTaxpayerContact(ctx, db.GetEngagementTaxpayerContactParams{OrganizationID: e.OrganizationID, EngagementID: e.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not load contacts")
		return
	}
	if in.Phone == nil {
		phone = before.ContactPhone
	}
	if in.Email == nil {
		email = before.ContactEmail
	}
	after, err := h.Q.UpdateEngagementTaxpayerContact(ctx, db.UpdateEngagementTaxpayerContactParams{
		ContactPhone: textOrNull(phone), ContactEmail: textOrNull(email),
		OrganizationID: e.OrganizationID, EngagementID: e.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not save contacts")
		return
	}
	if changes := contactChanges(before.ContactPhone, before.ContactEmail, after.ContactPhone, after.ContactEmail); len(changes) > 0 && h.Audit != nil {
		var actor *int64
		if p, ok := authctx.PrincipalFrom(ctx); ok {
			uid := p.UserInternal
			actor = &uid
		}
		tp := after.Uuid
		h.Audit.Record(ctx, actor, ActionContactUpdated, "taxpayer", &tp, map[string]any{
			"engagement_uuid": e.UUID.String(), "changes": changes,
		}, r)
	}
	h.Contacts(w, r)
}
