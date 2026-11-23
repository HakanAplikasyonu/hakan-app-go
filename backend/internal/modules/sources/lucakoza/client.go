// Package lucakoza is the Luca Koza source adapter (Oğuz'un geri bildirimi).
// Koza is the taxpayer's own accounting program; it answers plain HTTP with
// a cookie session. The adapter ONLY READS: it signs in, lists and selects
// the company/branch and lists invoices. No call adds, deletes or changes a
// record in Koza (readonly_test.go fails the build if one appears).
//
// The password is never stored: it is used for one request's session and
// dropped. Only the address, organisation code, user name and the selected
// company/branch are kept on the connection.
//
// ⚠️ Request filter names (belge tarihi aralığı) and the invoice line list's
// name are not documented to us; the client sends the likely names, the
// mapper tries several and also drops rows outside the asked range itself.
package lucakoza

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Koza endpoints. ONLY sign-in, company/branch list and selection, and
// Listele* / Getir* reads are allowed here (readonly_test.go).
const (
	epLogin         = "Giris.do"
	epCompanies     = "YdlUserResponsibilityOrgSs.do"
	epSelectCompany = "GuncelleYtkSirketSubeDegistir.do"
	epInvoices      = "ListeleFtrSsFaturaBaslik.do?detayliListe=true"
)

// endpoints is every Koza call the client makes.
var endpoints = []string{epLogin, epCompanies, epSelectCompany, epInvoices}

var (
	// ErrAddress: the Koza address is not a usable http(s) URL, or points to
	// a network the server must not reach.
	ErrAddress = errors.New("lucakoza: address is not allowed")
	// ErrUnreachable: Koza did not answer.
	ErrUnreachable = errors.New("lucakoza: koza did not answer")
	// ErrAnswer: Koza answered something the client cannot read.
	ErrAnswer = errors.New("lucakoza: unreadable answer")
)

// KozaError is Koza's own refusal (code 99) with its message.
type KozaError struct {
	Step    string // login, companies, select, invoices
	Message string
}

func (e *KozaError) Error() string { return "lucakoza: " + e.Step + " refused: " + e.Message }

// Company is one company/branch the user may work in.
type Company struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Client is one Koza session (its own cookie jar).
type Client struct {
	base string
	http *http.Client
}

// NormalizeBase checks a Koza address and returns it without a trailing
// slash: http or https, a host, no user info, query or fragment.
func NormalizeBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", ErrAddress
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), nil
}

// NewClient opens a session on base (see NormalizeBase). hc's transport is
// reused; the session gets its own cookie jar.
func NewClient(base string, hc *http.Client) (*Client, error) {
	b, err := NormalizeBase(base)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = GuardedHTTP()
	}
	jar, _ := cookiejar.New(nil)
	c := *hc
	c.Jar = jar
	return &Client{base: b, http: &c}, nil
}

const maxAnswer = 64 << 20

func (c *Client) post(ctx context.Context, step, ep string, body any, header map[string]string) (any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+ep, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrAddress
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, ErrAddress) {
			return nil, ErrAddress
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, step)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, step)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s http %d", ErrAnswer, step, res.StatusCode)
	}
	v, err := decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrAnswer, step)
	}
	// Any call may answer {"code": 99, "message": "..."} (session lost,
	// wrong input): that is Koza's refusal, shown with its own message.
	if o := asObj(v); o != nil {
		if code := o.str("code"); code != "" && code != "0" {
			return nil, &KozaError{Step: step, Message: kozaMessage(o)}
		}
	}
	return v, nil
}

var spaces = regexp.MustCompile(`\s+`)

// kozaMessage is Koza's message, cleaned for display (one line, at most 300
// characters).
func kozaMessage(o obj) string {
	m := o.str("message", "mesaj", "msg", "errorMessage")
	m = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, m)
	m = strings.TrimSpace(spaces.ReplaceAllString(m, " "))
	if r := []rune(m); len(r) > 300 {
		m = string(r[:300])
	}
	if m == "" {
		m = "Koza bir açıklama vermedi"
	}
	return m
}

// Login opens the session. code 0 is success; 99 (or any other) is Koza's
// refusal, returned as *KozaError with Koza's message.
func (c *Client) Login(ctx context.Context, orgCode, user, password string) error {
	v, err := c.post(ctx, "login", epLogin, map[string]string{
		"orgCode": orgCode, "userName": user, "userPassword": password,
	}, nil)
	if err != nil {
		return err
	}
	if asObj(v).str("code") != "0" {
		return &KozaError{Step: "login", Message: kozaMessage(asObj(v))}
	}
	return nil
}

// Companies lists the companies/branches the user is responsible for.
func (c *Client) Companies(ctx context.Context) ([]Company, error) {
	v, err := c.post(ctx, "companies", epCompanies, map[string]any{}, nil)
	if err != nil {
		return nil, err
	}
	var out []Company
	for _, r := range rows(v, "list", "data", "items", "result") {
		id := r.str("id", "orgSirketSubeId")
		if id == "" {
			continue
		}
		name := r.str("ack", "aciklama", "tanim", "name")
		if name == "" {
			name = id
		}
		out = append(out, Company{ID: id, Name: name})
	}
	return out, nil
}

// idValue sends a numeric id as a JSON number, anything else as a string.
func idValue(id string) any {
	if _, err := strconv.ParseInt(id, 10, 64); err == nil {
		return json.Number(id)
	}
	return id
}

// SelectCompany switches the session to one company/branch.
func (c *Client) SelectCompany(ctx context.Context, id string) error {
	_, err := c.post(ctx, "select", epSelectCompany, map[string]any{"orgSirketSubeId": idValue(id)}, nil)
	return err
}

// Invoices lists the invoice headers (with lines) of one movement type whose
// document date is in [from, to], unpaged.
func (c *Client) Invoices(ctx context.Context, movement int, from, to time.Time) ([]obj, error) {
	const layout = "02/01/2006"
	v, err := c.post(ctx, "invoices", epInvoices, map[string]any{
		"parUstHareketTuru": movement,
		"belgeTarihiBas":    from.Format(layout),
		"belgeTarihiBit":    to.Format(layout),
	}, map[string]string{"No-Paging": "true"})
	if err != nil {
		return nil, err
	}
	return rows(v, "list", "data", "items", "result"), nil
}
