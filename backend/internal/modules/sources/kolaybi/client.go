// Package kolaybi is the KolayBi API v1 source adapter (docs/kaynak 11,
// SourceAdapter of 02 §2). It only reads.
//
// ⚠️ Response field names are not fully documented (11 §2): the mapper tries
// the names of the write schema and their likely variants, keeps the raw
// record, and the first sandbox answer fixes them in one place.
package kolaybi

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

// DefaultBaseURL is production (11 table); sandbox is ofis-sandbox-api.
const DefaultBaseURL = "https://ofis-api.kolaybi.com"

var (
	// ErrAuth: the api_key was refused.
	ErrAuth = errors.New("kolaybi: authorization failed")
)

// App is the integrator's channel (11 §1.1).
type App struct {
	BaseURL string
	Channel string
}

func (a App) base() string {
	if a.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(a.BaseURL, "/")
}

// Client reads one company with the taxpayer's api_key.
type Client struct {
	App       App
	HTTP      *http.Client
	APIKey    string
	CompanyID string
	Interval  time.Duration

	token string
	last  time.Time
}

// NewClient builds a client.
func NewClient(app App, hc *http.Client, apiKey, companyID string) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Client{App: app, HTTP: hc, APIKey: apiKey, CompanyID: companyID, Interval: time.Second}
}

func (c *Client) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"api_key": c.APIKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.App.base()+"/kolaybi/v1/access_token", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Channel", c.App.Channel)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return ErrAuth
	}
	// ⚠️ the token's place in the answer: data.access_token or access_token.
	var out struct {
		Data        map[string]any `json:"data"`
		AccessToken string         `json:"access_token"`
	}
	_ = json.Unmarshal(raw, &out)
	tok := out.AccessToken
	if s, ok := out.Data["access_token"].(string); ok && s != "" {
		tok = s
	}
	if tok == "" {
		if s, ok := out.Data["token"].(string); ok {
			tok = s
		}
	}
	if tok == "" {
		return ErrAuth
	}
	c.token = tok
	return nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if c.token == "" {
			if err := c.login(ctx); err != nil {
				return nil, err
			}
		}
		if d := c.Interval - time.Since(c.last); d > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		c.last = time.Now()
		u := c.App.base() + "/kolaybi/v1" + path
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Channel", c.App.Channel)
		req.Header.Set("Accept", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		_ = res.Body.Close()
		if err != nil {
			return nil, err
		}
		switch {
		case res.StatusCode == http.StatusUnauthorized && attempt == 0:
			c.token = "" // 11 §1.2: new token with the api_key, retry once
			continue
		case res.StatusCode == http.StatusTooManyRequests && attempt < 3:
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
			continue
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			return nil, ErrAuth
		case res.StatusCode >= 300:
			return nil, fmt.Errorf("kolaybi: GET %s: HTTP %d", path, res.StatusCode)
		}
		return raw, nil
	}
}

// list reads a list answer: an array, or {data: [...], meta: {...}} ⚠️.
func list(raw []byte) ([]obj, int, int) {
	var arr []obj
	if json.Unmarshal(raw, &arr) == nil {
		return arr, 0, 0
	}
	var env struct {
		Data any `json:"data"`
		Meta struct {
			CurrentPage int `json:"current_page"`
			LastPage    int `json:"last_page"`
		} `json:"meta"`
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	}
	_ = json.Unmarshal(raw, &env)
	var out []obj
	switch d := env.Data.(type) {
	case []any:
		out = objs(d)
	case map[string]any: // Laravel paginator nested in data
		if xs, ok := d["data"].([]any); ok {
			out = objs(xs)
		}
		if cp, ok := d["current_page"].(float64); ok {
			env.CurrentPage = int(cp)
		}
		if lp, ok := d["last_page"].(float64); ok {
			env.LastPage = int(lp)
		}
	}
	cur, last := env.Meta.CurrentPage, env.Meta.LastPage
	if cur == 0 {
		cur, last = env.CurrentPage, env.LastPage
	}
	return out, cur, last
}

func objs(xs []any) []obj {
	out := make([]obj, 0, len(xs))
	for _, x := range xs {
		if m, ok := x.(map[string]any); ok {
			out = append(out, obj(m))
		}
	}
	return out
}

// Company is a firm the api_key can read (11 §1.3).
type Company struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Companies lists the firms.
func (c *Client) Companies(ctx context.Context) ([]Company, error) {
	raw, err := c.get(ctx, "/companies", nil)
	if err != nil {
		return nil, err
	}
	rows, _, _ := list(raw)
	out := make([]Company, 0, len(rows))
	for _, r := range rows {
		out = append(out, Company{ID: r.str("id"), Name: r.pick("title", "name", "company_name")})
	}
	return out, nil
}

// pages reads every page of a list (pagination ⚠️: page until an empty or
// last page; a plain array answer is one page).
func (c *Client) pages(ctx context.Context, path string, q url.Values, each func(obj) error) error {
	seen := map[string]bool{}
	for page := 1; page <= 1000; page++ {
		q.Set("page", strconv.Itoa(page))
		raw, err := c.get(ctx, path, q)
		if err != nil {
			return err
		}
		rows, cur, last := list(raw)
		fresh := 0
		for _, r := range rows {
			id := r.pick("id", "document_id")
			if seen[id] {
				continue
			}
			seen[id] = true
			fresh++
			if err := each(r); err != nil {
				return err
			}
		}
		// An API that ignores "page" repeats page 1: no fresh rows → stop.
		if len(rows) == 0 || fresh == 0 || (last > 0 && cur >= last) || (last == 0 && cur == 0 && page == 1 && !paged(raw)) {
			return nil
		}
	}
	return nil
}

// paged: the answer looks paginated (has meta / links / last_page).
func paged(raw []byte) bool {
	return bytes.Contains(raw, []byte(`"last_page"`)) || bytes.Contains(raw, []byte(`"next_page_url"`)) || bytes.Contains(raw, []byte(`"links"`))
}
