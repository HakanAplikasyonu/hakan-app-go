package officedash

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Client list page sizes (issue #238).
const (
	ClientsDefaultLimit = 30
	ClientsMaxLimit     = 100
)

// ClientQueries is what the client routes read (db.Queries on database.QueriesFor).
type ClientQueries interface {
	ListOfficeClients(ctx context.Context, arg db.ListOfficeClientsParams) ([]db.ListOfficeClientsRow, error)
	GetEngagementTaxpayerContact(ctx context.Context, arg db.GetEngagementTaxpayerContactParams) (db.GetEngagementTaxpayerContactRow, error)
	ListEngagementTaxpayerMembers(ctx context.Context, arg db.ListEngagementTaxpayerMembersParams) ([]db.ListEngagementTaxpayerMembersRow, error)
	GetEngagementReceiptImage(ctx context.Context, arg db.GetEngagementReceiptImageParams) (db.GetEngagementReceiptImageRow, error)
	GetEngagementReceiptPartImage(ctx context.Context, arg db.GetEngagementReceiptPartImageParams) (db.GetEngagementReceiptPartImageRow, error)
}

// Downloader reads stored objects (storage.Driver).
type Downloader interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// ClientsHandler serves the müşavir mobil client routes.
type ClientsHandler struct {
	Q     ClientQueries
	Store Downloader
}

// Client is one row of the office client list.
type Client struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	Kind           string    `json:"kind"`
	TaxID          string    `json:"tax_id"`
	Title          string    `json:"title"`
	TaxOffice      string    `json:"tax_office"`
	OpenWarnings   int64     `json:"open_warnings"`
	OpenBlockers   int64     `json:"open_blockers"`
	OpenTasks      int64     `json:"open_tasks"`
	AssignedToMe   bool      `json:"assigned_to_me"`
}

// ClientPage is one keyset page (issue #182 pattern).
type ClientPage struct {
	Items      []Client `json:"items"`
	Limit      int32    `json:"limit"`
	NextCursor *string  `json:"next_cursor"`
}

// encodeClientCursor / decodeClientCursor: (title, engagement id) keyset.
func encodeClientCursor(title string, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("c1|" + strconv.FormatInt(id, 10) + "|" + title))
}

var errBadCursor = errors.New("officedash: invalid cursor")

func decodeClientCursor(s string) (string, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return "", 0, errBadCursor
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 || parts[0] != "c1" {
		return "", 0, errBadCursor
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return "", 0, errBadCursor
	}
	return parts[2], id, nil
}

// likePattern escapes LIKE wildcards in a search term.
func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// List: GET /v1/tenant/clients?q=&filter=all|warnings|tasks|mine&cursor=&limit=.
func (h *ClientsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := orgctx.Must(ctx)
	q := r.URL.Query()
	var user int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		user = p.UserInternal
	}
	limit := int32(ClientsDefaultLimit)
	if v := q.Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > ClientsMaxLimit {
			response.BadRequest(w, r, response.CodeValidationError, fmt.Sprintf("limit must be 1..%d", ClientsMaxLimit))
			return
		}
		limit = int32(n)
	}
	p := db.ListOfficeClientsParams{
		OrganizationID: s.ID, UserID: user, AllEngagements: s.Has(rbac.PermTenantEngagementsReadAll), Lim: limit,
	}
	switch q.Get("filter") {
	case "", "all":
	case "warnings":
		p.OnlyWarnings = true
	case "tasks":
		p.OnlyTasks = true
	case "mine":
		p.OnlyMine = true
	default:
		response.BadRequest(w, r, response.CodeValidationError, "filter must be all, warnings, tasks or mine")
		return
	}
	if term := strings.TrimSpace(q.Get("q")); term != "" {
		if len([]rune(term)) > 100 {
			response.BadRequest(w, r, response.CodeValidationError, "q is too long")
			return
		}
		p.Search = pgtype.Text{String: likePattern(term), Valid: true}
	}
	if c := q.Get("cursor"); c != "" {
		title, id, err := decodeClientCursor(c)
		if err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_CURSOR", "cursor is not valid")
			return
		}
		p.CursorTitle = pgtype.Text{String: title, Valid: true}
		p.CursorID = pgtype.Int8{Int64: id, Valid: true}
	}
	rows, err := h.Q.ListOfficeClients(ctx, p)
	if err != nil {
		response.InternalErr(w, r, err, "Could not list taxpayers")
		return
	}
	out := ClientPage{Items: make([]Client, 0, len(rows)), Limit: limit}
	for _, row := range rows {
		out.Items = append(out.Items, Client{
			EngagementUUID: row.EngagementUuid, TaxpayerUUID: row.TaxpayerUuid, Kind: row.Kind, TaxID: row.TaxID,
			Title: row.Title, TaxOffice: row.TaxOffice, OpenWarnings: row.OpenWarnings, OpenBlockers: row.OpenBlockers,
			OpenTasks: row.OpenTasks, AssignedToMe: row.AssignedToMe,
		})
	}
	if n := len(rows); n > 0 && n == int(limit) {
		next := encodeClientCursor(rows[n-1].Title, rows[n-1].EngagementID)
		out.NextCursor = &next
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Contact is the taxpayer's reachable people.
type Contact struct {
	TaxpayerUUID uuid.UUID       `json:"taxpayer_uuid"`
	Kind         string          `json:"kind"`
	TaxID        string          `json:"tax_id"`
	Title        string          `json:"title"`
	TaxOffice    string          `json:"tax_office"`
	Phone        *string         `json:"phone"`
	Email        *string         `json:"email"`
	Members      []ContactMember `json:"members"`
}

// ContactMember is an active mükellef account (taxpayer_members).
type ContactMember struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
	Role    string    `json:"role"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Contacts: GET /v1/tenant/engagements/{engagementUuid}/contacts.
func (h *ClientsHandler) Contacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := engagementctx.Must(ctx)
	t, err := h.Q.GetEngagementTaxpayerContact(ctx, db.GetEngagementTaxpayerContactParams{OrganizationID: e.OrganizationID, EngagementID: e.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Engagement not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not load contacts")
		return
	}
	members, err := h.Q.ListEngagementTaxpayerMembers(ctx, db.ListEngagementTaxpayerMembersParams{OrganizationID: e.OrganizationID, EngagementID: e.ID})
	if err != nil {
		response.InternalErr(w, r, err, "Could not load contacts")
		return
	}
	out := Contact{
		TaxpayerUUID: t.Uuid, Kind: t.Kind, TaxID: t.TaxID, Title: t.Title, TaxOffice: t.TaxOffice,
		Phone: strPtr(t.ContactPhone), Email: strPtr(t.ContactEmail), Members: make([]ContactMember, 0, len(members)),
	}
	for _, m := range members {
		out.Members = append(out.Members, ContactMember{UUID: m.Uuid, Name: m.Name, Surname: m.Surname, Email: m.Email, Role: m.Role})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReceiptImage: GET /v1/tenant/engagements/{engagementUuid}/receipts/{receiptUuid}/image.
// Only images are served (the preview); anything else is 404.
func (h *ClientsHandler) ReceiptImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := engagementctx.Must(ctx)
	id, err := uuid.Parse(r.PathValue("receiptUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "receiptUuid is invalid")
		return
	}
	row, err := h.Q.GetEngagementReceiptImage(ctx, db.GetEngagementReceiptImageParams{OrganizationID: e.OrganizationID, EngagementID: e.ID, ReceiptUuid: id})
	// ?part=N: the N-th photo of a document captured in parts (a long Z
	// report, Oğuz'un demo geri bildirimi); part 1 is the capture's image.
	if raw := r.URL.Query().Get("part"); raw != "" && raw != "1" {
		// Parts 2..5; matched as text so no parsed integer is narrowed (gosec G109).
		var n int16
		for i := int16(2); i <= 5; i++ {
			if raw == strconv.Itoa(int(i)) {
				n = i
			}
		}
		if n == 0 {
			response.BadRequest(w, r, response.CodeValidationError, "part must be 1 to 5")
			return
		}
		var p db.GetEngagementReceiptPartImageRow
		p, err = h.Q.GetEngagementReceiptPartImage(ctx, db.GetEngagementReceiptPartImageParams{
			OrganizationID: e.OrganizationID, EngagementID: e.ID, ReceiptUuid: id, PartNo: n,
		})
		row = db.GetEngagementReceiptImageRow(p)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Receipt not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the image")
		return
	}
	ct := strings.ToLower(row.ContentType.String)
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" && ct != "image/heic" {
		response.NotFound(w, r, "Receipt has no image")
		return
	}
	if h.Store == nil {
		response.InternalErr(w, r, errors.New("officedash: storage is not configured"), "Could not load the image")
		return
	}
	rc, size, err := h.Store.Download(ctx, row.StorageKey)
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the image")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(w, rc)
}
