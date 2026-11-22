package entegrator

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/google/uuid"
)

// "Bağlantıyı test et" (#780): one read call to the integrator with the
// entered or stored web-service user, answered at once. It changes neither
// the connection row nor anything at the integrator; the first sync still
// runs on the worker.

// TestTimeout bounds a connection test.
const TestTimeout = 45 * time.Second

// Test failure kinds shown to the office (frontend sources.einvoice.test_*).
const (
	TestAuth        = "auth"        // wrong user name / password
	TestPermission  = "permission"  // user has no web-service right (or IP not allowlisted)
	TestTimeoutKind = "timeout"     // no answer in time
	TestUnavailable = "unavailable" // service down / not reachable
	TestRateLimit   = "rate_limit"  // too many calls
	TestUnknown     = "unknown"
)

// TestResult is the answer of a connection test.
type TestResult struct {
	OK bool `json:"ok"`
	// Kind is set when OK is false (TestAuth, TestPermission, …).
	Kind string `json:"kind,omitempty"`
	// Code is a short technical code for support (provider kind, the
	// integrator's error code, HTTP status); never the raw message.
	Code     string    `json:"code,omitempty"`
	TestedAt time.Time `json:"tested_at"`
}

// WithTest sets the HTTP client of connection tests (the fixed-egress
// client in the API, issue #256) and endpoint overrides by integrator id
// (tests: internal/testfakes). Empty overrides use the integrator's own
// test or live endpoint.
func (c *Connections) WithTest(hc *http.Client, endpoints map[string]string) *Connections {
	c.http, c.endpoints = hc, endpoints
	return c
}

// Test checks a connection's web-service user. values, when not empty, are
// checked instead of the stored credentials and are not saved.
func (c *Connections) Test(ctx context.Context, taxpayerID int64, id uuid.UUID, values map[string]string) (TestResult, error) {
	row, err := c.load(ctx, taxpayerID, id)
	if err != nil {
		return TestResult{}, err
	}
	name, _ := ProviderName(row.SourceSystem)
	info, ok := integrator(name)
	if !ok {
		return TestResult{}, ErrUnknownIntegrator
	}
	var m connMeta
	_ = json.Unmarshal(row.Meta, &m)
	var cr Credentials
	if hasValues(values) && !info.IPFiltered {
		if cr, err = buildCredentials(info, values, m.Environment == "test"); err != nil {
			return TestResult{}, err
		}
	} else {
		if row.CredentialsRef.Valid && c.vault == nil {
			return TestResult{}, ErrNoVault
		}
		cr, err = connCredentials(ctx, c.vault, row)
		if errors.Is(err, ErrNoCredentials) {
			return TestResult{}, ErrNotReady
		}
		if err != nil {
			return TestResult{}, err
		}
	}
	if ep := c.endpoints[name]; ep != "" {
		cr.Endpoint = ep
	}
	own, err := c.q.GetTaxpayerOwnTaxID(ctx, taxpayerID)
	if err != nil {
		return TestResult{}, err
	}
	hc := c.http
	if hc == nil {
		hc = auditHTTP
	}
	res := TestResult{TestedAt: time.Now().UTC()}
	cl, err := Open(name, provider.Config{
		Username: cr.Username, Password: cr.Password, APIKey: cr.APIKey,
		Endpoint: cr.Endpoint, Test: cr.Test, VKN: own, Options: cr.Options, HTTP: hc,
	})
	if err == nil {
		cl.WithRetry(RetryPolicy{Attempts: 1})
		tctx, cancel := context.WithTimeout(ctx, TestTimeout)
		err = probe(tctx, cl, res.TestedAt)
		cancel()
		cl.Close(ctx)
	}
	if err != nil {
		res.Kind, res.Code = ExplainError(err)
		return res, nil
	}
	res.OK = true
	return res, nil
}

func hasValues(values map[string]string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// probe lists the last day of the first listing the integrator supports
// (one page of one document): it logs in and checks the read right.
func probe(ctx context.Context, cl *Client, now time.Time) error {
	q := ListQuery{From: now.Add(-24 * time.Hour), To: now, Limit: 1}
	for _, k := range []provider.DocumentKind{provider.EInvoice, provider.EArchive} {
		for _, d := range []provider.Direction{provider.Inbound, provider.Outbound} {
			op := provider.OpListInbox
			if d == provider.Outbound {
				op = provider.OpListOutbox
			}
			if !cl.Supports(k, op) {
				continue
			}
			q.Kind = k
			_, err := cl.List(ctx, d, q)
			return err
		}
	}
	return nil
}

// ExplainError maps an integrator error to a test failure kind and a short
// technical code ("auth:2001", "unavailable:HTTP 503", "timeout").
func ExplainError(err error) (kind, code string) {
	var pe *provider.Error
	if errors.As(err, &pe) {
		parts := []string{string(pe.Kind)}
		if c := strings.TrimSpace(pe.Code); c != "" && len(c) <= 40 {
			parts = append(parts, c)
		}
		if pe.HTTPStatus != 0 {
			parts = append(parts, "HTTP "+strconv.Itoa(pe.HTTPStatus))
		}
		code = strings.Join(parts, ":")
		switch pe.Kind {
		case provider.KindAuth:
			return TestAuth, code
		case provider.KindPermission:
			return TestPermission, code
		case provider.KindTimeout:
			return TestTimeoutKind, code
		case provider.KindUnavailable, provider.KindTransport:
			if isTimeout(pe.Err) {
				return TestTimeoutKind, code
			}
			return TestUnavailable, code
		case provider.KindRateLimit:
			return TestRateLimit, code
		case provider.KindNotFound:
			if pe.HTTPStatus == http.StatusNotFound {
				return TestUnavailable, code
			}
		}
		return TestUnknown, code
	}
	if isTimeout(err) {
		return TestTimeoutKind, "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return TestUnavailable, "network"
	}
	return TestUnknown, "error"
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
