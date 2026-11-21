package communication

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves Ayarlar > SMS (#780 A).
type Handler struct{ svc *Service }

// NewHandler builds the handler; svc nil answers 404 on every route.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func actor(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if msg := UserMessage(err); msg != "" {
		response.BadRequest(w, r, response.CodeValidationError, msg)
		return
	}
	if errors.Is(err, ErrNoVault) {
		response.Error(w, r, http.StatusServiceUnavailable, response.CodeInternalError, "Şifre kasası sunucuda ayarlı değil.")
		return
	}
	response.InternalErr(w, r, err, "SMS isteği işlenemedi.")
}

// UserMessage is the Turkish screen text of a refused send or setting; ""
// for errors that are not the user's to fix.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrInvalidSettings):
		if i := strings.LastIndex(err.Error(), ": "); i >= 0 {
			return err.Error()[i+2:]
		}
		return "Ayarlar geçersiz."
	case errors.Is(err, ErrInvalidPhone):
		return "Numara cep telefonu değil ya da hatalı."
	case errors.Is(err, ErrEmptyText):
		return "Mesaj metni boş olamaz."
	case errors.Is(err, ErrCampaignText):
		w := ""
		if i := strings.LastIndex(err.Error(), ": "); i >= 0 {
			w = " (" + err.Error()[i+2:] + ")"
		}
		return "Bilgilendirme kanalından kampanya içerikli mesaj gönderilemez" + w + "."
	case errors.Is(err, ErrNotConfigured):
		return "Bu ofiste SMS gönderimi açık değil. Ayarlar > SMS bölümünden NetGSM hesabını bağlayın."
	case errors.Is(err, ErrNoContact):
		return "Mükellefin kayıtlı iletişim bilgisi yok."
	}
	return ""
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// GetSettings: GET /v1/tenant/sms-settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PutSettings: PUT /v1/tenant/sms-settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in SettingsInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Save(r.Context(), orgctx.Must(r.Context()).ID, actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// TestConnection: POST /v1/tenant/sms-settings/test (sender name list).
func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Test(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type testMessageBody struct {
	Phone string `json:"phone"`
	Text  string `json:"text"`
}

// TestMessage: POST /v1/tenant/sms-messages/test, one SMS to a number the
// office types (in test mode only recorded).
func (h *Handler) TestMessage(w http.ResponseWriter, r *http.Request) {
	var in testMessageBody
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		in.Text = "Bu bir deneme mesajıdır."
	}
	m, err := h.svc.SendSMS(r.Context(), SMSInput{OrganizationID: orgctx.Must(r.Context()).ID, Phone: in.Phone, Text: in.Text,
		SourceRule: "test", SentBy: actor(r)})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, RecordOf(m))
}

// ListMessages: GET /v1/tenant/sms-messages?limit=50.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.Recent(r.Context(), orgctx.Must(r.Context()).ID, int32(limit))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
