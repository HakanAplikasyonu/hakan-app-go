// Package easytrade is the Easytrade Muhasebe API source adapter (docs/kaynak
// 09, SourceAdapter of 02 §2). It only reads.
//
// The common model uses Easytrade's names, so mapping is thin. ⚠️ The 09 §9
// questions (list schemas, status values) are partly open; what Easytrade's
// own mobile client does is the reference: invoice lists filter on
// filters[issue_date][start|end] and filters[branch_id] and sort by
// created_at (issue_date sorting is unstable across pages), the VAT rate is
// the "0015" row of an item's tax_breakdowns, unknown values are kept raw.
//
// Issue #385: scheduled sync (schedule.go), signed webhook (webhook.go),
// financial transactions and Z reports (financial.go). The portal-side
// contract and the list of what Easytrade still has to offer ("Easytrade'den
// istenecekler") are in docs/easytrade-senkron.md.
package easytrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL (CLAUDE.md §7).
const DefaultBaseURL = "https://pro.easytradetr.com/api"

// PerPage is the documented maximum (09 §2).
const PerPage = 100

// maxPages bounds one list walk (100 × 100 records per branch and window).
const maxPages = 100

var (
	// ErrAuth: 401 after a refresh, or 403 without a license code (09 §1.3).
	ErrAuth = errors.New("easytrade: authorization failed")
	// ErrLicense: 403 LICENSE_EXPIRED; the connection is disabled, not retried.
	ErrLicense = errors.New("easytrade: taxpayer's Easytrade license expired")
	// ErrUpstream: any other non-2xx answer (429, 5xx …); the schedule retries.
	ErrUpstream = errors.New("easytrade: request failed")
	// ErrNoBranch: 400 "Aktif şube bulunamadı".
	ErrNoBranch = errors.New("easytrade: no active branch")
	// ErrBadLogin: /auth/login refused the email or password (401, also 400
	// and 403). It is an ErrAuth.
	ErrBadLogin = fmt.Errorf("%w: wrong email or password", ErrAuth)
	// ErrInvalidInput: /auth/login answered 422, Easytrade's validation
	// error (a field is missing or malformed, e.g. a password shorter than
	// 4 characters). It is an ErrAuth.
	ErrInvalidInput = fmt.Errorf("%w: invalid login input", ErrAuth)
	// ErrForbidden: 403 without LICENSE_EXPIRED; the user has no permission
	// in the company. It is an ErrAuth.
	ErrForbidden = fmt.Errorf("%w: no permission in the company", ErrAuth)
	// ErrCompanyMismatch: the entered ID is not the user's Easytrade company.
	ErrCompanyMismatch = errors.New("easytrade: company ID does not match the user's company")
	// ErrTimeout: Easytrade did not answer in time. It is an ErrUpstream.
	ErrTimeout = fmt.Errorf("%w: timeout", ErrUpstream)
	// ErrUnreachable: no answer at all (DNS, refused, TLS). It is an ErrUpstream.
	ErrUnreachable = fmt.Errorf("%w: not reachable", ErrUpstream)
	// ErrBadResponse: Easytrade answered 2xx with a body we cannot read
	// (an HTML page, a changed shape, a login answer without a token). It
	// is an ErrUpstream, never a wrong password.
	ErrBadResponse = fmt.Errorf("%w: unreadable answer", ErrUpstream)
)

// HTTPError is a non-2xx answer not covered by the sentinels above; it
// matches ErrUpstream. Host (host only) and Message (Easytrade's
// "message") are shown to the user.
type HTTPError struct {
	Method, Path string
	Status       int
	Host         string
	Message      string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("easytrade: %s %s: HTTP %d", e.Method, e.Path, e.Status)
}

// BadResponseError is an unreadable 2xx answer; it matches ErrBadResponse.
type BadResponseError struct {
	Path, Host string
	Status     int
}

func (e *BadResponseError) Error() string {
	return fmt.Sprintf("easytrade: %s: unreadable answer (HTTP %d)", e.Path, e.Status)
}

// Unwrap gives ErrBadResponse.
func (e *BadResponseError) Unwrap() error { return ErrBadResponse }

// NetError is a transport failure with the server that was called; it
// matches ErrTimeout or ErrUnreachable.
type NetError struct {
	Host  string
	Kind  error
	Cause string
}

func (e *NetError) Error() string { return fmt.Sprintf("%v (%s)", e.Kind, e.Cause) }

// Unwrap gives the kind (ErrTimeout or ErrUnreachable).
func (e *NetError) Unwrap() error { return e.Kind }

// Is makes errors.Is(err, ErrUpstream) true.
func (e *HTTPError) Is(target error) bool { return target == ErrUpstream }

// LoginError is a refused /auth/login with what Easytrade said: its
// "message", its "error_code" and, on 422, the first entry of its "errors"
// object. It matches ErrInvalidInput (422), ErrLicense (403 with
// error_code LICENSE_EXPIRED), ErrForbidden (any other 403: no permission,
// e.g. for the client channel) or ErrBadLogin (401).
type LoginError struct {
	// Host is the Easytrade server that answered (host only: no path,
	// query or credentials), so a wrong EASYTRADE_API_BASE_URL shows.
	Host      string
	Status    int
	Message   string
	ErrorCode string
	// Field and FieldMessage: the first field of "errors" and its first text.
	Field, FieldMessage string
}

func (e *LoginError) Error() string {
	return fmt.Sprintf("easytrade: login refused: HTTP %d", e.Status)
}

// Unwrap gives the sentinel the status stands for.
func (e *LoginError) Unwrap() error {
	switch {
	case e.Status == http.StatusUnprocessableEntity:
		return ErrInvalidInput
	case e.Status == http.StatusForbidden && e.ErrorCode == "LICENSE_EXPIRED":
		return ErrLicense
	case e.Status == http.StatusForbidden:
		return ErrForbidden
	}
	return ErrBadLogin
}

// fieldLabels names Easytrade's login fields in Turkish.
var fieldLabels = map[string]string{
	"email":       "E-posta",
	"password":    "Şifre",
	"device_name": "Cihaz adı",
	"device_type": "Cihaz türü",
}

// FieldLabel is the Turkish name of an Easytrade field, or the field itself.
func FieldLabel(field string) string {
	if l, ok := fieldLabels[field]; ok {
		return l
	}
	return field
}

// Reason is what Easytrade said: on 422 "<field>: <text>", else its
// message; empty when Easytrade said nothing usable.
func (e *LoginError) Reason() string {
	switch {
	case e.Status == http.StatusUnprocessableEntity && e.FieldMessage != "":
		return FieldLabel(e.Field) + ": " + e.FieldMessage
	case errors.Is(e, ErrLicense):
		return "Easytrade lisansınızın süresi dolmuş."
	}
	return e.Message
}

// UserMessage is the Turkish text shown on screen:
// "Easytrade (pro.easytradetr.com) 401: E-posta veya şifre hatalı."
func (e *LoginError) UserMessage() string {
	reason := e.Reason()
	if reason == "" {
		switch e.Status {
		case http.StatusUnprocessableEntity:
			reason = "Gönderilen giriş bilgileri geçersiz."
		case http.StatusForbidden:
			reason = "Bu Easytrade kullanıcısının bu kanaldan giriş yetkisi yok."
		default:
			reason = "E-posta ya da şifre kabul edilmedi."
		}
	}
	host := e.Host
	if host == "" {
		host = "?"
	}
	return fmt.Sprintf("Easytrade (%s) %d: %s", host, e.Status, reason)
}

// hostOf is base's host (and port), never its path, query or user info.
func hostOf(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return u.Host
}

// loginError reads Easytrade's refusal body:
// {"message": "...", "errors": {"password": ["..."]}}. The first field is
// the first key in the body's order (a Go map would lose it).
func loginError(host string, status int, raw []byte) *LoginError {
	e := &LoginError{Host: host, Status: status}
	var body struct {
		Message   string          `json:"message"`
		ErrorCode string          `json:"error_code"`
		Errors    json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(raw, &body) != nil {
		if bytes.Contains(raw, []byte("LICENSE_EXPIRED")) {
			e.ErrorCode = "LICENSE_EXPIRED"
		}
		return e
	}
	e.Message, e.ErrorCode = strings.TrimSpace(body.Message), strings.TrimSpace(body.ErrorCode)
	e.Field, e.FieldMessage = loginFieldError(body.Errors)
	return e
}

// loginFieldError is the first email or password entry of "errors" (in the
// body's order), else its first entry.
func loginFieldError(raw json.RawMessage) (string, string) {
	fs := fieldErrors(raw)
	for _, f := range fs {
		if f[0] == "email" || f[0] == "password" {
			return f[0], f[1]
		}
	}
	if len(fs) > 0 {
		return fs[0][0], fs[0][1]
	}
	return "", ""
}

// firstFieldError returns the first key of an {"field": ["text", …]} object
// and its first text (a plain string value is accepted too).
func firstFieldError(raw json.RawMessage) (string, string) {
	if fs := fieldErrors(raw); len(fs) > 0 {
		return fs[0][0], fs[0][1]
	}
	return "", ""
}

// fieldErrors lists {field, first text} of an "errors" object in the body's
// order (a Go map would lose it); fields without text are left out.
func fieldErrors(raw json.RawMessage) [][2]string {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out [][2]string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return out
		}
		field, _ := t.(string)
		var v json.RawMessage
		if dec.Decode(&v) != nil {
			return out
		}
		if msg := fieldText(v); msg != "" {
			out = append(out, [2]string{field, msg})
		}
	}
	return out
}

// fieldText is the first text of a field's ["text", …] (or "text") value.
func fieldText(v json.RawMessage) string {
	var list []string
	if json.Unmarshal(v, &list) == nil && len(list) > 0 {
		return strings.TrimSpace(list[0])
	}
	var one string
	if json.Unmarshal(v, &one) == nil {
		return strings.TrimSpace(one)
	}
	return ""
}

// NormalizeEmail trims the email and lowercases it; Easytrade's accounts are
// case-insensitive, a stray space or capital letter from a phone keyboard
// should not fail the login.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailDomain is the part after "@" (logged instead of the address).
func emailDomain(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return ""
}

// netErr classifies a transport error. The caller's own cancellation and
// deadline stay what they are (a run stops on them); a client timeout is
// ErrTimeout, anything else ErrUnreachable. Before, both fell through to a
// 500 on the connect screen.
func netErr(ctx context.Context, host string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return &NetError{Host: host, Kind: ErrTimeout, Cause: err.Error()}
	}
	return &NetError{Host: host, Kind: ErrUnreachable, Cause: err.Error()}
}

// Token is the Sanctum bearer token (30 days, 09 §1.1).
type Token struct {
	Value     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Client reads one company.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	CompanyID string
	BranchID  string
	Interval  time.Duration
	// OnToken stores a refreshed token.
	OnToken func(Token) error
	// ZReports reads Z reports; nil → HTTPZReports (issue #385).
	ZReports ZReportSource

	token Token
	last  time.Time
}

// BaseURL is the API root: an empty, blank or quoted-empty value (a local
// env file passing EASYTRADE_API_BASE_URL="") is DefaultBaseURL.
func BaseURL(base string) string {
	base = strings.Trim(strings.TrimSpace(base), `"'`)
	if strings.TrimSpace(base) == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(base, "/")
}

// NewClient builds a client.
func NewClient(base string, hc *http.Client, companyID string, t Token) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Client{BaseURL: BaseURL(base), HTTP: hc, CompanyID: companyID, token: t, Interval: 200 * time.Millisecond}
}

// authUser is the user block of /auth/login and /auth/refresh: Easytrade
// puts it under data.user or at the top level (its mobile client reads
// both), the company the user works for under user.company. company.id is
// the "ID" the accountant enters.
// Only the fields we use are declared: is_active (a string), roles (an
// object) and anything new are ignored, never a decode error.
type authUser struct {
	Company *struct {
		ID   flexID `json:"id"`
		Name string `json:"name"`
	} `json:"company"`
}

// flexID is an id Easytrade sends as a string ("123", the documented type)
// or a number (123); null or any other JSON is empty, never an error.
type flexID string

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexID) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexID(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) == nil {
		*f = flexID(n.String())
		return nil
	}
	*f = ""
	return nil
}

func (f flexID) String() string { return string(f) }

// authAnswer is the /auth/login and /auth/refresh answer. The documented
// shape: data.token, data.token_type, data.expires_in (seconds) and a
// top-level user, which may be null on refresh (the login's company is
// kept then). data.user and data.expires_at are read too (mobile client).
type authAnswer struct {
	Success *bool `json:"success"`
	Data    struct {
		Token     string      `json:"token"`
		ExpiresAt string      `json:"expires_at"`
		ExpiresIn json.Number `json:"expires_in"`
		User      *authUser   `json:"user"`
	} `json:"data"`
	User *authUser `json:"user"`
}

// token is the answer's token with its expiry.
func (a authAnswer) token() Token {
	t := token(a.Data.Token, a.Data.ExpiresAt)
	if a.Data.ExpiresAt == "" {
		if n, err := a.Data.ExpiresIn.Int64(); err == nil && n > 0 {
			t.ExpiresAt = time.Now().Add(time.Duration(n) * time.Second)
		}
	}
	return t
}

// company returns the company ID and name the answer names, if any.
func (a authAnswer) company() (string, string) {
	for _, u := range []*authUser{a.Data.User, a.User} {
		if u != nil && u.Company != nil && u.Company.ID.String() != "" {
			return u.Company.ID.String(), u.Company.Name
		}
	}
	return "", ""
}

// Session is a fresh sign-in: the token and the company Easytrade says the
// user belongs to (empty when the answer does not say).
type Session struct {
	Token       Token
	CompanyID   string
	CompanyName string
}

// Login exchanges the user's email and password for a token (09 §1.1). The
// password is not kept; the email is trimmed and lowercased.
func Login(ctx context.Context, base string, hc *http.Client, email, password string) (Token, error) {
	s, err := SignIn(ctx, base, hc, email, password)
	return s.Token, err
}

// SignIn logs in and, like Easytrade's own mobile client, refreshes the
// token right away: the login token may not carry the company yet, the
// refreshed one does. A failed refresh keeps the login token.
func SignIn(ctx context.Context, base string, hc *http.Client, email, password string) (Session, error) {
	c := NewClient(base, hc, "", Token{})
	c.Interval = 0
	var out authAnswer
	email = NormalizeEmail(email)
	// The password goes as typed: spaces may be part of it.
	body, _ := json.Marshal(map[string]any{"email": email, "password": password, "device_name": "mali-musavir-portali", "revoke_other_tokens": false})
	if err := c.do(ctx, http.MethodPost, "/auth/login", nil, body, &out, false); err != nil {
		var le *LoginError
		if errors.As(err, &le) {
			// Never the password or the address: the domain tells users apart.
			slog.WarnContext(ctx, "easytrade login refused", "easytrade_host", le.Host, "status", le.Status,
				"easytrade_message", le.Message, "error_code", le.ErrorCode, "field", le.Field, "field_message", le.FieldMessage,
				"email_domain", emailDomain(email))
		}
		return Session{}, err
	}
	// The token alone decides: Easytrade's own documented 200 example
	// carries success:false next to a token.
	if out.Data.Token == "" {
		return Session{}, &BadResponseError{Path: "/auth/login", Host: hostOf(c.BaseURL), Status: http.StatusOK}
	}
	s := Session{Token: out.token()}
	s.CompanyID, s.CompanyName = out.company()
	c.token = s.Token
	var ref authAnswer
	if err := c.do(ctx, http.MethodPost, "/auth/refresh", nil, nil, &ref, false); err == nil && ref.Data.Token != "" {
		s.Token = ref.token()
		if id, name := ref.company(); id != "" {
			s.CompanyID, s.CompanyName = id, name
		}
	}
	return s, nil
}

func token(v, exp string) Token {
	t := Token{Value: v, ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	if e, err := time.Parse(time.RFC3339, exp); err == nil {
		t.ExpiresAt = e
	}
	return t
}

func (c *Client) refresh(ctx context.Context) error {
	var out authAnswer
	if err := c.do(ctx, http.MethodPost, "/auth/refresh", nil, nil, &out, false); err != nil {
		return err
	}
	if out.Data.Token == "" {
		return ErrAuth
	}
	t := out.token()
	if c.OnToken != nil {
		if err := c.OnToken(t); err != nil {
			return err
		}
	}
	c.token = t
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body []byte, out any, retry bool) error {
	if d := c.Interval - time.Since(c.last); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	c.last = time.Now()
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token.Value != "" {
		req.Header.Set("Authorization", "Bearer "+c.token.Value)
	}
	if c.CompanyID != "" {
		req.Header.Set("X-Company-ID", c.CompanyID)
	}
	if c.BranchID != "" {
		req.Header.Set("X-Branch-ID", c.BranchID)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return netErr(ctx, hostOf(c.BaseURL), err)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	_ = res.Body.Close()
	if err != nil {
		return netErr(ctx, hostOf(c.BaseURL), err)
	}
	host := hostOf(c.BaseURL)
	if path == "/auth/login" && res.StatusCode >= 300 {
		// Easytrade answers a wrong email or password with 401, a
		// malformed email or password (a too short password …) with 422
		// and a missing permission (client channel) or an expired license
		// (error_code LICENSE_EXPIRED) with 403. Anything else (400, a 422
		// on another field, 5xx …) is an HTTPError with Easytrade's text.
		le := loginError(host, res.StatusCode, raw)
		switch {
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden,
			res.StatusCode == http.StatusUnprocessableEntity && (le.Field == "email" || le.Field == "password"):
			return le
		}
		return &HTTPError{Method: method, Path: path, Status: res.StatusCode, Host: host, Message: le.Reason()}
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized && retry:
		if err := c.refresh(ctx); err != nil {
			return ErrAuth
		}
		return c.do(ctx, method, path, q, body, out, false)
	case res.StatusCode == http.StatusUnauthorized:
		return ErrAuth
	case res.StatusCode == http.StatusForbidden:
		if bytes.Contains(raw, []byte("LICENSE_EXPIRED")) {
			return ErrLicense
		}
		return ErrForbidden
	case res.StatusCode == http.StatusBadRequest && bytes.Contains(raw, []byte("Aktif şube")):
		return ErrNoBranch
	case res.StatusCode >= 300:
		return &HTTPError{Method: method, Path: path, Status: res.StatusCode, Host: host, Message: loginError(host, res.StatusCode, raw).Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		// An HTML page (proxy, maintenance) or a changed shape instead of
		// the JSON we read.
		return &BadResponseError{Path: path, Host: host, Status: res.StatusCode}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, q, nil, out, true)
}

// Branch of the company (09 §1.2).
type Branch struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Branches lists the company's branches; each list is read per branch.
func (c *Client) Branches(ctx context.Context) ([]Branch, error) {
	var out struct {
		Data []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/company/branches", nil, &out); err != nil {
		return nil, err
	}
	bs := make([]Branch, 0, len(out.Data))
	for _, b := range out.Data {
		bs = append(bs, Branch{ID: b.ID.String(), Name: b.Name})
	}
	return bs, nil
}

// listMode says how a list is asked for a date window.
type listMode int

const (
	// byFilter: filters[<date>][start|end] and filters[branch_id], sorted by
	// created_at, every page read (invoice lists, mobile client). Sorting by
	// the document date is unstable on Easytrade: rows of the same day
	// repeat and go missing across pages.
	byFilter listMode = iota
	// byDateParams: date_from / date_to, sorted by created_at (expenses).
	byDateParams
	// byDateScan: no server-side date filter; sorted by the date desc and
	// stopped at the first older record (financial transactions, Z reports).
	byDateScan
)

// modeOf picks the list mode of path.
func modeOf(path string) listMode {
	switch {
	case strings.HasPrefix(path, "/company/invoices/"):
		return byFilter
	case path == "/company/expenses":
		return byDateParams
	}
	return byDateScan
}

// listIDs returns the ids of path's records whose dateField falls in
// [from, to], de-duplicated. List items are read for id and date only;
// details carry the content.
func (c *Client) listIDs(ctx context.Context, path, dateField string, from, to time.Time) ([]string, error) {
	mode := modeOf(path)
	seen := map[string]bool{}
	var ids []string
	total := -1
	// pass walks the pages in one direction; it reports whether the walk
	// stopped at an older record (date scan only).
	pass := func(direction string) error {
		for page := 1; page <= maxPages; page++ {
			q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(PerPage)}, "direction": {direction}}
			switch mode {
			case byFilter:
				q.Set("sort", "created_at")
				q.Set("filters["+dateField+"][start]", from.Format(time.DateOnly))
				q.Set("filters["+dateField+"][end]", to.Format(time.DateOnly))
				if c.BranchID != "" && path != "/company/invoices/inbox" {
					q.Set("filters[branch_id]", c.BranchID)
				}
			case byDateParams:
				q.Set("sort", "created_at")
				q.Set("date_from", from.Format(time.DateOnly))
				q.Set("date_to", to.Format(time.DateOnly))
			default:
				q.Set("sort", dateField)
			}
			var out struct {
				Data []map[string]json.RawMessage `json:"data"`
				Meta struct {
					CurrentPage int          `json:"current_page"`
					LastPage    int          `json:"last_page"`
					Total       *json.Number `json:"total"`
				} `json:"meta"`
			}
			if err := c.get(ctx, path, q, &out); err != nil {
				return err
			}
			if out.Meta.Total != nil {
				if n, err := out.Meta.Total.Int64(); err == nil {
					total = int(n)
				}
			}
			older := false
			for _, item := range out.Data {
				d, ok := dateOf(item[dateField])
				if !ok {
					continue
				}
				if d.Before(from) {
					older = true
					continue
				}
				if d.After(to) {
					continue
				}
				if id := idOf(item["id"]); id != "" && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			if mode == byDateScan && older {
				return nil
			}
			if len(out.Data) == 0 || len(out.Data) < PerPage && out.Meta.LastPage == 0 ||
				(out.Meta.LastPage > 0 && out.Meta.CurrentPage >= out.Meta.LastPage) {
				return nil
			}
		}
		return nil
	}
	if err := pass("desc"); err != nil {
		return nil, err
	}
	// A list that reports a total we did not reach (rows moved between
	// pages while we read) is read once more the other way.
	if mode != byDateScan && total >= 0 && len(seen) < total {
		if err := pass("asc"); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func idOf(raw json.RawMessage) string {
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func dateOf(raw json.RawMessage) (time.Time, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil || len(s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return t, err == nil
}
