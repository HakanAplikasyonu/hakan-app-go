// Package entegrator is the e-Fatura / e-Arşiv integrator channel (issue #40,
// docs 30 §5–§8, 37 §5): a read-only client over go-ubltr's provider
// registry, the entegrator_documents index and the common sync flow
// (windows, window splitting, UBL queue).
//
// Read rule (30 §5): the portal never changes the taxpayer's state at the
// integrator. Client therefore exposes only listing, fetching, status and
// user lookups; sending, answering, cancelling and "mark taken" are not
// reachable through it at compile time (client_test.go checks this).
package entegrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
)

// SourcePrefix prefixes source_connections.source_system and the invoices'
// source_system (`entegrator_<name>`, 30 §6).
const SourcePrefix = "entegrator_"

// ProviderName returns the go-ubltr registry name for a source system
// ("entegrator_izibiz" → "izibiz").
func ProviderName(sourceSystem string) (string, bool) {
	name, ok := strings.CutPrefix(sourceSystem, SourcePrefix)
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// ListQuery is a read-only listing. Unlike provider.ListQuery it has no
// OnlyNew field: "only not yet taken" listings depend on the taxpayer's
// own acknowledgement state and are never used (30 §5).
type ListQuery struct {
	Kind     provider.DocumentKind
	From, To time.Time // creation / receive time window
	Cursor   string
	Limit    int
}

func (q ListQuery) provider() provider.ListQuery {
	return provider.ListQuery{Kind: q.Kind, From: q.From, To: q.To, Cursor: q.Cursor, Limit: q.Limit, OnlyNew: false}
}

// Client is the read-only view of one integrator account. It keeps only the
// read interfaces of the underlying provider; the provider value itself is
// not stored, so no type assertion can reach its write methods.
type Client struct {
	name   string
	caps   provider.Capabilities
	inbox  provider.InboxLister
	outbox provider.OutboxLister
	fetch  provider.Fetcher
	status provider.StatusChecker
	users  provider.UserChecker
	closer sessionCloser
	retry  RetryPolicy
}

// sessionCloser is implemented by session-based integrators (EDM, İzibiz):
// Logout ends the web-service session. It changes no document state.
type sessionCloser interface {
	Logout(ctx context.Context) error
}

// RetryPolicy retries a read call that failed for a passing reason: the
// integrator was unavailable (5xx), the connection broke or a call timed out
// while the caller's context is still alive (#196). Rate limits (429) are not
// retried here: the next run waits for the integrator's interval instead.
// Every Client call is a read, so a retry cannot repeat a side effect.
type RetryPolicy struct {
	// Attempts is the total number of tries (default 3; 1 turns retries off).
	Attempts int
	// Base is the first backoff (default 500ms); it doubles each try.
	Base time.Duration
}

// DefaultRetry is the policy Wrap gives a Client.
var DefaultRetry = RetryPolicy{Attempts: 3, Base: 500 * time.Millisecond}

// WithRetry replaces the retry policy.
func (c *Client) WithRetry(p RetryPolicy) *Client {
	c.retry = p
	return c
}

// retryable: only passing failures; never a rate limit, auth or bad request.
func retryable(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	switch provider.KindOf(err) {
	case provider.KindUnavailable, provider.KindTransport, provider.KindTimeout:
		return true
	}
	return false
}

func call[T any](ctx context.Context, p RetryPolicy, fn func(context.Context) (T, error)) (T, error) {
	attempts, base := p.Attempts, p.Base
	if attempts <= 0 {
		attempts = DefaultRetry.Attempts
	}
	if base <= 0 {
		base = DefaultRetry.Base
	}
	var (
		out T
		err error
	)
	for i := range attempts {
		out, err = fn(ctx)
		if !retryable(ctx, err) || i == attempts-1 {
			return out, err
		}
		t := time.NewTimer(base << i)
		select {
		case <-ctx.Done():
			t.Stop()
			return out, err
		case <-t.C:
		}
	}
	return out, err
}

// Open opens a registered integrator (provider.Open) and wraps it.
func Open(name string, cfg provider.Config) (*Client, error) {
	p, err := provider.Open(name, cfg)
	if err != nil {
		return nil, err
	}
	return Wrap(p), nil
}

// Wrap keeps the read interfaces of p.
func Wrap(p provider.Provider) *Client {
	c := &Client{name: p.Name(), caps: p.Capabilities(), retry: DefaultRetry}
	c.inbox, _ = p.(provider.InboxLister)
	c.outbox, _ = p.(provider.OutboxLister)
	c.fetch, _ = p.(provider.Fetcher)
	c.status, _ = p.(provider.StatusChecker)
	c.users, _ = p.(provider.UserChecker)
	c.closer, _ = p.(sessionCloser)
	return c
}

// Close ends the integrator session when the integrator has one (Logout);
// otherwise it does nothing. A failed logout is not an error of the run:
// the integrator expires the session itself.
func (c *Client) Close(ctx context.Context) {
	if c.closer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	_ = c.closer.Logout(ctx)
}

// Name is the integrator name (provider registry name).
func (c *Client) Name() string { return c.name }

// Capabilities as declared by the integrator.
func (c *Client) Capabilities() provider.Capabilities { return c.caps }

// Supports reports whether kind/op is declared and implemented. Write
// operations are never supported through Client.
func (c *Client) Supports(kind provider.DocumentKind, op provider.Operation) bool {
	if !c.caps.Has(kind, op) {
		return false
	}
	switch op {
	case provider.OpListInbox:
		return c.inbox != nil
	case provider.OpListOutbox:
		return c.outbox != nil
	case provider.OpFetch:
		return c.fetch != nil
	case provider.OpStatus:
		return c.status != nil
	case provider.OpCheckUser:
		return c.users != nil
	}
	return false
}

// List lists one page of the inbox (provider.Inbound) or outbox.
func (c *Client) List(ctx context.Context, dir provider.Direction, q ListQuery) (_ *provider.Page, err error) {
	switch dir {
	case provider.Inbound:
		if c.inbox == nil {
			return nil, provider.Unsupported(c.name, provider.OpListInbox, q.Kind)
		}
		defer observe(c.name, "list_inbox", time.Now(), &err)
		return call(ctx, c.retry, func(ctx context.Context) (*provider.Page, error) { return c.inbox.ListInbox(ctx, q.provider()) })
	case provider.Outbound:
		if c.outbox == nil {
			return nil, provider.Unsupported(c.name, provider.OpListOutbox, q.Kind)
		}
		defer observe(c.name, "list_outbox", time.Now(), &err)
		return call(ctx, c.retry, func(ctx context.Context) (*provider.Page, error) { return c.outbox.ListOutbox(ctx, q.provider()) })
	}
	return nil, fmt.Errorf("entegrator: unknown direction %q", dir)
}

// Fetch downloads a document.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (_ *provider.FetchResult, err error) {
	if c.fetch == nil {
		return nil, provider.Unsupported(c.name, provider.OpFetch, q.Kind)
	}
	defer observe(c.name, "fetch", time.Now(), &err)
	return call(ctx, c.retry, func(ctx context.Context) (*provider.FetchResult, error) { return c.fetch.Fetch(ctx, q) })
}

// Status queries one document's status.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (_ *provider.StatusResult, err error) {
	if c.status == nil {
		return nil, provider.Unsupported(c.name, provider.OpStatus, q.Kind)
	}
	defer observe(c.name, "status", time.Now(), &err)
	return call(ctx, c.retry, func(ctx context.Context) (*provider.StatusResult, error) { return c.status.Status(ctx, q) })
}

// CheckUser looks up a VKN/TCKN in the e-Fatura user list.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (_ *provider.UserInfo, err error) {
	if c.users == nil {
		return nil, provider.Unsupported(c.name, provider.OpCheckUser, "")
	}
	defer observe(c.name, "check_user", time.Now(), &err)
	return call(ctx, c.retry, func(ctx context.Context) (*provider.UserInfo, error) { return c.users.CheckUser(ctx, vknTckn) })
}

// observe records one integrator call in the metrics (#179): the outcome is
// "ok" or the provider's error kind, so the error rate per integrator can be
// alerted on.
func observe(name, op string, start time.Time, err *error) {
	metrics.Default.ObserveIntegrator(name, op, outcome(*err), time.Since(start))
}

func outcome(err error) string {
	if err == nil {
		return "ok"
	}
	var pe *provider.Error
	switch {
	case errors.As(err, &pe) && pe.Kind != "":
		return string(pe.Kind)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
