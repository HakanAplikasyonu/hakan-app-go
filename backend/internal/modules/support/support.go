// Package support serves the ClickFix Web Widget settings (issue #387).
// The portal has no help desk of its own: requests go to ClickFix.
//
//   - Office panel → software provider: the platform widget
//     (NEXT_PUBLIC_CLICKFIX_WIDGET_ID, frontend). With
//     CLICKFIX_IDENTITY_SECRET set, GET /v1/tenant/support/clickfix-identity
//     signs the member (user_hash) so ClickFix marks them verified.
//   - Taxpayer portal → the taxpayer's own office: the office stores its
//     widget id (office_support_settings); GET /v1/taxpayer/support gives it
//     to the portal. Empty = the portal falls back to the platform default.
package support

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// IdentitySecretFromEnv reads CLICKFIX_IDENTITY_SECRET, the identity
// verification secret of the platform (provider) widget; empty = off.
// Server only; never sent to the browser.
func IdentitySecretFromEnv() string {
	return strings.TrimSpace(os.Getenv("CLICKFIX_IDENTITY_SECRET"))
}

var widgetIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{0,128}$`)

// ErrInvalidWidgetID: the id is not a ClickFix public widget id.
var ErrInvalidWidgetID = errors.New("clickfix_widget_id must be up to 128 letters, digits, '-' or '_'")

// NormalizeWidgetID trims and validates a widget id; "" clears it.
func NormalizeWidgetID(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !widgetIDPattern.MatchString(v) {
		return "", ErrInvalidWidgetID
	}
	return v, nil
}

// UserHash is ClickFix's identity signature: hex(HMAC-SHA256(secret,
// user_id + "\n" + company_id)), or of user_id alone without a company.
func UserHash(secret, userID, companyID string) string {
	msg := userID
	if companyID != "" {
		msg = userID + "\n" + companyID
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// Queries is the db subset the handler uses.
type Queries interface {
	GetOfficeSupportSettings(ctx context.Context, organizationID int64) (db.OfficeSupportSetting, error)
	UpsertOfficeSupportSettings(ctx context.Context, arg db.UpsertOfficeSupportSettingsParams) (db.OfficeSupportSetting, error)
	GetActiveEngagementForTaxpayer(ctx context.Context, taxpayerID int64) (db.Engagement, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// Handler serves the support routes.
type Handler struct {
	q      Queries
	secret string
}

// NewHandler builds the handler; secret may be empty (no verified identity).
func NewHandler(q Queries, identitySecret string) *Handler {
	return &Handler{q: q, secret: strings.TrimSpace(identitySecret)}
}

// Settings is the office's support configuration.
type Settings struct {
	ClickFixWidgetID string     `json:"clickfix_widget_id"`
	UpdatedAt        *time.Time `json:"updated_at"`
}

func settingsOf(row db.OfficeSupportSetting) Settings {
	t := row.UpdatedAt.Time
	return Settings{ClickFixWidgetID: row.ClickfixWidgetID, UpdatedAt: &t}
}

func (h *Handler) load(ctx context.Context, orgID int64) (Settings, error) {
	row, err := h.q.GetOfficeSupportSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return settingsOf(row), nil
}

// GetSettings: GET /v1/tenant/support-settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	set, err := h.load(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load support settings")
		return
	}
	response.JSON(w, r, http.StatusOK, set)
}

type settingsBody struct {
	ClickFixWidgetID *string `json:"clickfix_widget_id"`
}

// PutSettings: PUT /v1/tenant/support-settings (tenant.settings.write).
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.ClickFixWidgetID == nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	id, err := NormalizeWidgetID(*in.ClickFixWidgetID)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	var by pgtype.Int8
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal > 0 {
		by = pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	row, err := h.q.UpsertOfficeSupportSettings(r.Context(), db.UpsertOfficeSupportSettingsParams{
		OrganizationID: orgctx.Must(r.Context()).ID, ClickfixWidgetID: id, UpdatedBy: by,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not save support settings")
		return
	}
	response.JSON(w, r, http.StatusOK, settingsOf(row))
}

// Identity is the verified ClickFix visitor of the office member.
type Identity struct {
	Available bool   `json:"available"`
	UserID    string `json:"user_id,omitempty"`
	UserHash  string `json:"user_hash,omitempty"`
	CompanyID string `json:"company_id,omitempty"`
}

// ClickFixIdentity: GET /v1/tenant/support/clickfix-identity. The member's
// uuid and the office uuid signed with CLICKFIX_IDENTITY_SECRET; without the
// secret {available:false} and the widget keeps the visitor unverified.
func (h *Handler) ClickFixIdentity(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if h.secret == "" || !ok {
		response.JSON(w, r, http.StatusOK, Identity{})
		return
	}
	user, company := p.UserID.String(), orgctx.Must(r.Context()).UUID.String()
	response.JSON(w, r, http.StatusOK, Identity{
		Available: true, UserID: user, CompanyID: company,
		UserHash: UserHash(h.secret, user, company),
	})
}

// TaxpayerSupport is the support channel of the taxpayer's current office.
type TaxpayerSupport struct {
	// Office is null when the company has no active office.
	Office *TaxpayerSupportOffice `json:"office"`
}

// TaxpayerSupportOffice: the office name and its widget id ("" = default).
type TaxpayerSupportOffice struct {
	Name             string `json:"name"`
	ClickFixWidgetID string `json:"clickfix_widget_id"`
}

// TaxpayerSupport: GET /v1/taxpayer/support (X-Taxpayer). The office's
// settings row answers to the office policy, so it is read in the office
// scope narrowed to this taxpayer (rls.ForOffice, as portal uploads do).
func (h *Handler) TaxpayerSupport(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	eng, err := h.q.GetActiveEngagementForTaxpayer(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		response.JSON(w, r, http.StatusOK, TaxpayerSupport{})
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not resolve office")
		return
	}
	var out TaxpayerSupportOffice
	err = rls.ForOffice(r.Context(), eng.OrganizationID, s.ID, func(ctx context.Context) error {
		org, err := h.q.GetOrganizationByID(ctx, eng.OrganizationID)
		if err != nil {
			return err
		}
		set, err := h.load(ctx, eng.OrganizationID)
		if err != nil {
			return err
		}
		out = TaxpayerSupportOffice{Name: org.Name, ClickFixWidgetID: set.ClickFixWidgetID}
		return nil
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not load office support settings")
		return
	}
	response.JSON(w, r, http.StatusOK, TaxpayerSupport{Office: &out})
}
