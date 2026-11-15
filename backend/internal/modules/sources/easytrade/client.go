// Package easytrade is the Easytrade Muhasebe API source adapter (docs/kaynak
// 09, SourceAdapter of 02 §2). It only reads.
//
// The common model uses Easytrade's names, so mapping is thin. ⚠️ The 09 §9
// questions (list schemas, date filters, status values, tax_breakdowns) are
// open: lists only give ids, details carry the content, windows are cut on
// the client (09 §2), unknown values are kept raw.
package easytrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

var (
	// ErrAuth: 401 after a refresh, or 403 without a license code (09 §1.3).
	ErrAuth = errors.New("easytrade: authorization failed")
	// ErrLicense: 403 LICENSE_EXPIRED; the connection is disabled, not retried.
	ErrLicense = errors.New("easytrade: taxpayer's Easytrade license expired")
	// ErrNoBranch: 400 "Aktif şube bulunamadı".
	ErrNoBranch = errors.New("easytrade: no active branch")
)

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

	token Token
	last  time.Time
}

// NewClient builds a client.
func NewClient(base string, hc *http.Client, companyID string, t Token) *Client {
	if base == "" {
		base = DefaultBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), HTTP: hc, CompanyID: companyID, token: t, Interval: 200 * time.Millisecond}
}

// Login exchanges the user's email and password for a token (09 §1.1). The
// password is not kept.
func Login(ctx context.Context, base string, hc *http.Client, email, password string) (Token, error) {
	c := NewClient(base, hc, "", Token{})
	var out struct {
		Data struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "device_name": "mali-musavir-portali"})
	if err := c.do(ctx, http.MethodPost, "/auth/login", nil, body, &out, false); err != nil {
		return Token{}, err
	}
	if out.Data.Token == "" {
		return Token{}, ErrAuth
	}
	return token(out.Data.Token, out.Data.ExpiresAt), nil
}

func token(v, exp string) Token {
	t := Token{Value: v, ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	if e, err := time.Parse(time.RFC3339, exp); err == nil {
		t.ExpiresAt = e
	}
	return t
}

func (c *Client) refresh(ctx context.Context) error {
	var out struct {
		Data struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/auth/refresh", nil, nil, &out, false); err != nil {
		return err
	}
	if out.Data.Token == "" {
		return ErrAuth
	}
	t := token(out.Data.Token, out.Data.ExpiresAt)
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
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	_ = res.Body.Close()
	if err != nil {
		return err
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
		return ErrAuth
	case res.StatusCode == http.StatusBadRequest && bytes.Contains(raw, []byte("Aktif şube")):
		return ErrNoBranch
	case res.StatusCode >= 300:
		return fmt.Errorf("easytrade: %s %s: HTTP %d", method, path, res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
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

// listIDs walks a list sorted by dateField desc and returns the ids issued
// in [from, to], stopping at the first older record (09 §2). List items are
// read for id and date only (their schema is undocumented, 09 §9 soru 5).
func (c *Client) listIDs(ctx context.Context, path, dateField string, from, to time.Time) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(PerPage)}, "sort": {dateField}, "direction": {"desc"}}
		var out struct {
			Data []map[string]json.RawMessage `json:"data"`
			Meta struct {
				CurrentPage int `json:"current_page"`
				LastPage    int `json:"last_page"`
			} `json:"meta"`
		}
		if err := c.get(ctx, path, q, &out); err != nil {
			return nil, err
		}
		for _, item := range out.Data {
			d, ok := dateOf(item[dateField])
			if !ok {
				continue
			}
			if d.Before(from) {
				return ids, nil
			}
			if d.After(to) {
				continue
			}
			ids = append(ids, idOf(item["id"]))
		}
		if len(out.Data) == 0 || out.Meta.LastPage == 0 || out.Meta.CurrentPage >= out.Meta.LastPage {
			return ids, nil
		}
	}
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
