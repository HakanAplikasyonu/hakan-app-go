// Package parasut is the Paraşüt API v4 source adapter (docs/kaynak 10,
// SourceAdapter of 02 §2). It only reads.
//
// ⚠️ No Paraşüt account was available: the 10 §9 open points (date range
// syntax, page size limit, meta names …) are handled defensively and kept in
// one place here (CLAUDE.md §6).
package parasut

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the Paraşüt API host (10 table).
const DefaultBaseURL = "https://api.parasut.com"

// PageSize: 25 is documented to work (10 §2); the upper limit is unknown ⚠️.
const PageSize = 25

// Tokens is the OAuth pair kept in the vault (10 §1.3: the refresh token
// rotates on every refresh).
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// App is the integrator application registered with Paraşüt (10 §1.1).
type App struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Configured reports whether the application keys are present.
func (a App) Configured() bool {
	return a.ClientID != "" && a.ClientSecret != "" && a.RedirectURL != ""
}

func (a App) base() string {
	if a.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(a.BaseURL, "/")
}

// AuthorizeURL is step 2 of 10 §1.2; state ties the callback to a connection.
func (a App) AuthorizeURL(state string) string {
	v := url.Values{"client_id": {a.ClientID}, "redirect_uri": {a.RedirectURL}, "response_type": {"code"}, "state": {state}}
	return a.base() + "/oauth/authorize?" + v.Encode()
}

// ErrAuth: Paraşüt refused the token or the grant (invalid_grant, 401).
var ErrAuth = errors.New("parasut: authorization failed")

// Exchange trades an authorization code for tokens (10 §1.2 step 4).
func (a App) Exchange(ctx context.Context, hc *http.Client, code string) (Tokens, error) {
	return a.token(ctx, hc, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {a.RedirectURL}})
}

// Refresh renews the pair (10 §1.3).
func (a App) Refresh(ctx context.Context, hc *http.Client, refresh string) (Tokens, error) {
	return a.token(ctx, hc, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

func (a App) token(ctx context.Context, hc *http.Client, form url.Values) (Tokens, error) {
	form.Set("client_id", a.ClientID)
	form.Set("client_secret", a.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base()+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := hc.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized {
		return Tokens{}, fmt.Errorf("%w: %s", ErrAuth, strings.TrimSpace(string(body)))
	}
	if res.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("parasut: token HTTP %d", res.StatusCode)
	}
	var t struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &t); err != nil || t.AccessToken == "" {
		return Tokens{}, fmt.Errorf("parasut: unreadable token answer")
	}
	if t.ExpiresIn <= 0 {
		t.ExpiresIn = 7200
	}
	return Tokens{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}, nil
}

// Client reads one company. Requests are spaced at least Interval apart
// (10 §2: 60/min documented; one per second per connection is safe).
type Client struct {
	App       App
	HTTP      *http.Client
	CompanyID string
	Interval  time.Duration
	// Backoff is the first wait after a 429; it grows linearly (default 10s).
	Backoff time.Duration
	// OnTokens is called after a refresh so the caller can store the new
	// pair atomically (the old refresh token stops working).
	OnTokens func(Tokens) error

	mu     sync.Mutex
	tokens Tokens
	last   time.Time
}

// NewClient builds a client with the stored tokens.
func NewClient(app App, hc *http.Client, companyID string, t Tokens) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Client{App: app, HTTP: hc, CompanyID: companyID, Interval: time.Second, tokens: t}
}

func (c *Client) access(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.tokens.AccessToken != "" && time.Until(c.tokens.ExpiresAt) > time.Minute {
		return c.tokens.AccessToken, nil
	}
	if c.tokens.RefreshToken == "" {
		return "", ErrAuth
	}
	t, err := c.App.Refresh(ctx, c.HTTP, c.tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if c.OnTokens != nil {
		if err := c.OnTokens(t); err != nil {
			return "", err
		}
	}
	c.tokens = t
	return t.AccessToken, nil
}

// backoff is the first wait after a 429 (Backoff, default 10s).
func (c *Client) backoff() time.Duration {
	if c.Backoff > 0 {
		return c.Backoff
	}
	return 10 * time.Second
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	d := c.Interval - time.Since(c.last)
	c.mu.Unlock()
	if d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
	return nil
}

// get fetches path (relative to /v4 or /v4/{company}) with query.
func (c *Client) get(ctx context.Context, path string, q url.Values) (document, error) {
	u := c.App.base() + "/v4" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		if err := c.wait(ctx); err != nil {
			return document{}, err
		}
		tok, err := c.access(ctx, attempt > 0)
		if err != nil {
			return document{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return document{}, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/vnd.api+json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			return document{}, err
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		_ = res.Body.Close()
		if err != nil {
			return document{}, err
		}
		switch {
		case res.StatusCode == http.StatusUnauthorized && attempt == 0:
			continue // access token revoked early: refresh once
		case res.StatusCode == http.StatusTooManyRequests && attempt < 3:
			select {
			case <-ctx.Done():
				return document{}, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * c.backoff()):
			}
			continue
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			return document{}, ErrAuth
		case res.StatusCode >= 300:
			return document{}, fmt.Errorf("parasut: GET %s: HTTP %d", path, res.StatusCode)
		}
		var doc document
		if err := json.Unmarshal(body, &doc); err != nil {
			return document{}, fmt.Errorf("parasut: GET %s: %w", path, err)
		}
		return doc, nil
	}
}

// Company is a firm the user can read (10 §1.2 step 6).
type Company struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TaxNumber string `json:"tax_number,omitempty"`
}

// Companies lists the user's firms (GET /v4/me, include=companies ⚠️).
func (c *Client) Companies(ctx context.Context) ([]Company, error) {
	doc, err := c.get(ctx, "/me", url.Values{"include": {"companies"}})
	if err != nil {
		return nil, err
	}
	var out []Company
	for _, r := range doc.Included {
		if r.Type != "companies" {
			continue
		}
		name := r.str("name")
		if ln := r.str("legal_name"); ln != "" {
			name = ln
		}
		out = append(out, Company{ID: r.ID, Name: name, TaxNumber: r.str("tax_number")})
	}
	return out, nil
}

// pages walks a sorted list (sort=-issue_date) until stop says so or data
// runs out. The documented range filter syntax is unknown (10 §2.1 ⚠️), so
// the window is cut on the client: the safe method of 10 §2.1.
func (c *Client) pages(ctx context.Context, path string, q url.Values, each func(resource, index) (stop bool, err error)) error {
	for page := 1; ; page++ {
		q.Set("page[number]", strconv.Itoa(page))
		q.Set("page[size]", strconv.Itoa(PageSize))
		doc, err := c.get(ctx, "/"+c.CompanyID+path, q)
		if err != nil {
			return err
		}
		rs, err := doc.list()
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			return nil
		}
		ix := newIndex(doc.Included)
		for _, r := range rs {
			stop, err := each(r, ix)
			if err != nil || stop {
				return err
			}
		}
		if len(rs) < PageSize {
			return nil
		}
	}
}
