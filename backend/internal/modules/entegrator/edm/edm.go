// Package edm is a read-only client for the EDM Bilişim özel entegratör
// SOAP service EFaturaEDM (e-Fatura gelen/giden kutusu). It registers
// itself as "edm" in go-ubltr's provider registry, so the entegrator
// module opens it like the go-ubltr integrators.
//
// Flow (WSDL of the test system, checked 2026-11-22): Login (USER_NAME,
// PASSWORD) returns a SESSION_ID that every later call sends in
// REQUEST_HEADER; GetInvoice lists (HEADER_ONLY=Y) or downloads
// (HEADER_ONLY=N, CONTENT base64) invoices by DIRECTION IN/OUT; Logout
// closes the session. Only these three calls are used: sending, answering,
// marking and cancelling are not implemented (read rule, 30 §5).
package edm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "edm"

// Service endpoints (the WSDL's soap:address on each system).
const (
	TestEndpoint = "https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc"
	LiveEndpoint = "https://portal2.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc"
)

// loginSessionID is sent before a session exists.
const loginSessionID = "-1"

// maxLimit bounds one header-only listing page.
const maxLimit = 1000

// Config configures a Provider.
type Config struct {
	Username, Password string
	// Endpoint overrides the service URL (tests, a contract-specific host).
	Endpoint string
	// Test selects TestEndpoint when Endpoint is empty; otherwise
	// LiveEndpoint is used.
	Test bool
	// ApplicationName is sent as REQUEST_HEADER/APPLICATION_NAME.
	ApplicationName string
	HTTP            provider.Doer
}

// Provider is the EDM client. One session is shared by all calls and
// renewed once when EDM reports it as expired.
type Provider struct {
	cfg Config
	ws  *soap.Client

	mu      sync.Mutex
	session string
	gen     uint64
}

var (
	_ provider.Provider     = (*Provider)(nil)
	_ provider.InboxLister  = (*Provider)(nil)
	_ provider.OutboxLister = (*Provider)(nil)
	_ provider.Fetcher      = (*Provider)(nil)
)

// New returns a Provider.
func New(cfg Config) (*Provider, error) {
	cfg.Username = strings.TrimSpace(cfg.Username)
	if cfg.Username == "" || cfg.Password == "" {
		return nil, &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "open", Message: "username and password are required"}
	}
	ep := strings.TrimSpace(cfg.Endpoint)
	if ep == "" {
		ep = LiveEndpoint
		if cfg.Test {
			ep = TestEndpoint
		}
	}
	if cfg.ApplicationName == "" {
		cfg.ApplicationName = "hakan-app"
	}
	doer := cfg.HTTP
	if doer == nil {
		doer = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Provider{cfg: cfg, ws: &soap.Client{URL: ep, HTTP: doer, Provider: Name}}, nil
}

// Open is the registry factory.
func Open(c provider.Config) (provider.Provider, error) {
	return New(Config{
		Username: c.Username, Password: c.Password, Endpoint: c.Endpoint, Test: c.Test,
		ApplicationName: c.Options["application_name"], HTTP: c.HTTP,
	})
}

func init() { provider.Register(Name, Open) }

// Name implements provider.Provider.
func (p *Provider) Name() string { return Name }

// Capabilities implements provider.Provider. e-Arşiv documents that EDM
// returns in the outbox listing (HEADER/EARCHIVE=true) are indexed as
// e-Arşiv; there is no separate e-Arşiv listing in this module.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {provider.OpListInbox, provider.OpListOutbox, provider.OpFetch},
		},
		Formats:         []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:        "soap",
		MinPollInterval: 15 * time.Minute,
		TestEndpoint:    true,
	}
}

func txnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (p *Provider) header(session string) requestHeader {
	return requestHeader{
		SessionID: session, ClientTxnID: txnID(),
		ActionDate:      time.Now().Format("2006-01-02T15:04:05"),
		ApplicationName: p.cfg.ApplicationName, Compressed: "N",
	}
}

// Login opens a session (and keeps it for later calls). The connection
// test calls it to check the user without listing.
func (p *Provider) Login(ctx context.Context) error {
	_, _, err := p.sessionID(ctx)
	return err
}

func (p *Provider) sessionID(ctx context.Context) (string, uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != "" {
		return p.session, p.gen, nil
	}
	var resp loginResponse
	err := p.ws.Call(ctx, actionLogin, loginRequest{
		NS: nsTempuri, Header: p.header(loginSessionID),
		UserName: p.cfg.Username, Password: p.cfg.Password,
	}, &resp)
	if err != nil {
		return "", 0, classify("login", err)
	}
	id := strings.TrimSpace(resp.SessionID)
	if id == "" {
		return "", 0, &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "login", Message: "empty SESSION_ID"}
	}
	p.session = id
	p.gen++
	return p.session, p.gen, nil
}

func (p *Provider) invalidate(gen uint64) {
	p.mu.Lock()
	if p.gen == gen {
		p.session = ""
	}
	p.mu.Unlock()
}

// Logout closes the current session, if any. Errors are returned but a
// failed logout leaves nothing to clean up: EDM expires sessions itself.
func (p *Provider) Logout(ctx context.Context) error {
	p.mu.Lock()
	id := p.session
	p.session = ""
	p.mu.Unlock()
	if id == "" {
		return nil
	}
	if err := p.ws.Call(ctx, actionLogout, logoutRequest{NS: nsTempuri, Header: p.header(id)}, nil); err != nil {
		return classify("logout", err)
	}
	return nil
}

// getInvoice runs GetInvoice in the current session; a session error
// triggers one new login and a retry.
func (p *Provider) getInvoice(ctx context.Context, op string, key searchKey, headerOnly bool, contentType string) (*getInvoiceResponse, error) {
	ho := "N"
	if headerOnly {
		ho = "Y"
	}
	for attempt := 0; ; attempt++ {
		id, gen, err := p.sessionID(ctx)
		if err != nil {
			return nil, err
		}
		var resp getInvoiceResponse
		err = p.ws.Call(ctx, actionGetInvoice, getInvoiceRequest{
			NS: nsTempuri, Header: p.header(id), SearchKey: key, HeaderOnly: ho, ContentType: contentType,
		}, &resp)
		if err == nil {
			return &resp, nil
		}
		perr := classify(op, err)
		if attempt == 0 && isSessionError(perr) {
			p.invalidate(gen)
			continue
		}
		return nil, perr
	}
}

// ListInbox implements provider.InboxLister (DIRECTION=IN).
func (p *Provider) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return p.list(ctx, q, provider.Inbound, provider.OpListInbox)
}

// ListOutbox implements provider.OutboxLister (DIRECTION=OUT).
func (p *Provider) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return p.list(ctx, q, provider.Outbound, provider.OpListOutbox)
}

// istanbul is the zone EDM's portal works in; creation times are sent with
// their offset so the window does not depend on the server's zone.
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*60*60)
}()

func (p *Provider) list(ctx context.Context, q provider.ListQuery, dir provider.Direction, op provider.Operation) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, op, q.Kind)
	}
	limit := q.Limit
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	offset := 0
	if q.Cursor != "" {
		n, err := strconv.Atoi(q.Cursor)
		if err != nil || n < 0 {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: string(op), Message: "bad cursor"}
		}
		offset = n
	}
	key := searchKey{Limit: limit, ReadIncluded: "true", Direction: wireDirection(dir), Offset: &offset}
	if !q.From.IsZero() {
		key.CRStartDate = q.From.In(istanbul).Format(time.RFC3339)
	}
	if !q.To.IsZero() {
		key.CREndDate = q.To.In(istanbul).Format(time.RFC3339)
	}
	resp, err := p.getInvoice(ctx, string(op), key, true, "XML")
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.Page{}, nil
		}
		return nil, err
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(resp.Invoices))}
	for _, inv := range resp.Invoices {
		page.Items = append(page.Items, documentInfo(inv, dir))
	}
	if len(resp.Invoices) >= limit {
		page.Next = strconv.Itoa(offset + len(resp.Invoices))
	}
	return page, nil
}

func wireDirection(d provider.Direction) string {
	if d == provider.Inbound {
		return "IN"
	}
	return "OUT"
}

func documentInfo(inv invoice, dir provider.Direction) provider.DocumentInfo {
	h := inv.Header
	gib, _ := strconv.Atoi(strings.TrimSpace(h.GIBStatusCode))
	d := provider.DocumentInfo{
		Kind:         provider.EInvoice,
		Direction:    dir,
		UUID:         strings.TrimSpace(inv.UUID),
		ID:           strings.TrimSpace(inv.ID),
		Profile:      strings.TrimSpace(h.ProfileID),
		Type:         strings.TrimSpace(h.InvoiceType),
		IssueDate:    strings.TrimSpace(h.IssueDate),
		SenderVKN:    strings.TrimSpace(h.Sender),
		SenderName:   strings.TrimSpace(h.Supplier),
		ReceiverVKN:  strings.TrimSpace(h.Receiver),
		ReceiverName: strings.TrimSpace(h.Customer),
		Currency:     strings.TrimSpace(h.Payable.Currency),
		Payable:      strings.TrimSpace(h.Payable.Value),
		Status:       documentStatus(h.Status, h.ResponseCode, gib),
		GIBCode:      gib,
		ProviderCode: normalizeWords(h.Status),
		CreatedAt:    parseTime(h.CDate),
	}
	if strings.EqualFold(strings.TrimSpace(h.EArchive), "true") {
		d.Kind = provider.EArchive
	}
	return d
}

// statusWords maps EDM's "STATUS - SUBSTATUS" text (spaces removed). The
// vocabulary is the i2i e-invoice one EDM's service is built on (same
// schema namespace, schemas.i2i.com/ei/common).
var statusWords = map[string]provider.Status{
	"LOAD-SUCCEED":                      provider.StatusQueued,
	"LOAD-PROCESSING":                   provider.StatusQueued,
	"LOAD-FAILED":                       provider.StatusFailed,
	"PACKAGE-PROCESSING":                provider.StatusProcessing,
	"PACKAGE-SUCCEED":                   provider.StatusProcessing,
	"PACKAGE-FAILED":                    provider.StatusProcessing,
	"SIGN-PROCESSING":                   provider.StatusProcessing,
	"SIGN-SUCCEED":                      provider.StatusProcessing,
	"SIGN-FAILED":                       provider.StatusProcessing,
	"SEND-PROCESSING":                   provider.StatusProcessing,
	"SEND-WAIT_GIB_RESPONSE":            provider.StatusProcessing,
	"SEND-WAIT_SYSTEM_RESPONSE":         provider.StatusSent,
	"SEND-WAIT_APPLICATION_RESPONSE":    provider.StatusDelivered,
	"SEND-SUCCEED":                      provider.StatusDelivered,
	"SEND-FAILED":                       provider.StatusFailed,
	"ACCEPTED-SUCCEED":                  provider.StatusAccepted,
	"REJECTED-SUCCEED":                  provider.StatusRejected,
	"RECEIVE-SUCCEED":                   provider.StatusReceived,
	"RECEIVE-WAIT_APPLICATION_RESPONSE": provider.StatusReceived,
	"ACCEPT-SUCCEED":                    provider.StatusAccepted,
	"ACCEPT-FAILED":                     provider.StatusFailed,
	"REJECT-SUCCEED":                    provider.StatusRejected,
	"REJECT-FAILED":                     provider.StatusFailed,
}

func normalizeWords(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// documentStatus combines STATUS, RESPONSE_CODE and GIB_STATUS_CODE; a
// failing GİB code wins over an in-progress vendor status.
func documentStatus(status, response string, gib int) provider.Status {
	s, ok := statusWords[normalizeWords(status)]
	if !ok {
		s = provider.StatusUnknown
	}
	switch strings.ToUpper(strings.TrimSpace(response)) {
	case "KABUL", "ACCEPT", "ACCEPTED":
		s = provider.StatusAccepted
	case "RED", "REJECT", "REJECTED":
		s = provider.StatusRejected
	}
	g := provider.GIBStatus(gib)
	switch {
	case s == provider.StatusUnknown:
		s = g
	case g == provider.StatusFailed && !s.Terminal():
		s = g
	}
	return s
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.ParseInLocation(layout, s, istanbul); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Fetch implements provider.Fetcher with GetInvoice (HEADER_ONLY=N) by
// UUID; CONTENT is base64, unzipped when EDM sends it zipped.
func (p *Provider) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	if strings.TrimSpace(q.UUID) == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "fetch", Message: "UUID is required"}
	}
	if q.Kind != "" && q.Kind != provider.EInvoice && q.Kind != provider.EArchive {
		return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
	}
	if q.Format == "" {
		q.Format = provider.FormatUBL
	}
	ct := "XML"
	switch q.Format {
	case provider.FormatUBL:
	case provider.FormatPDF:
		ct = "PDF"
	case provider.FormatHTML:
		ct = "HTML"
	default:
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "fetch", Message: "unsupported format " + string(q.Format)}
	}
	dir := q.Direction
	if dir == "" {
		dir = provider.Inbound
	}
	resp, err := p.getInvoice(ctx, "fetch", searchKey{UUID: q.UUID, ReadIncluded: "true", Direction: wireDirection(dir)}, false, ct)
	if err != nil {
		return nil, err
	}
	if len(resp.Invoices) == 0 || strings.TrimSpace(resp.Invoices[0].Content) == "" {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "fetch", Message: "document " + q.UUID + " not found"}
	}
	data, err := decodeContent(resp.Invoices[0].Content)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "fetch", Message: "decode CONTENT", Err: err}
	}
	return &provider.FetchResult{Format: q.Format, ContentType: contentType(q.Format), Data: data}, nil
}

func decodeContent(s string) ([]byte, error) {
	b, err := pack.FromBase64(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	for i := 0; i < 2 && pack.IsZip(b); i++ {
		if _, b, err = pack.UnzipOne(b); err != nil {
			return nil, err
		}
	}
	if len(b) == 0 {
		return nil, errors.New("empty content")
	}
	return b, nil
}

func contentType(f provider.Format) string {
	switch f {
	case provider.FormatPDF:
		return "application/pdf"
	case provider.FormatHTML:
		return "text/html; charset=utf-8"
	}
	return "application/xml"
}

// classify turns a soap/transport error into a *provider.Error with a
// kind the office can act on: auth (wrong user or password), permission
// (the user may not use the web service), timeout, unavailable, or the
// fault's own code otherwise.
func classify(op string, err error) *provider.Error {
	var perr *provider.Error
	if !errors.As(err, &perr) {
		perr = provider.TransportError(Name, op, err)
	}
	out := *perr
	out.Provider, out.Op = Name, op
	var ne net.Error
	if out.Kind == provider.KindTransport && errors.As(err, &ne) && ne.Timeout() {
		out.Kind = provider.KindTimeout
	}
	var f *soap.Fault
	if !errors.As(err, &f) {
		return &out
	}
	msg := strings.TrimSpace(f.Reason)
	if re := parseFault(f.Detail); re != nil {
		out.Code = re.Code
		if m := re.message(); m != "" {
			msg = m
		}
	}
	out.Message = msg
	out.Kind = faultKind(op, msg)
	out.Retryable = false
	return &out
}

func lower(s string) string { return strings.ToLowerSpecial(unicode.TurkishCase, s) }

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// faultKind reads a RequestFault text. EDM publishes no error code table;
// the texts are matched instead.
func faultKind(op, msg string) provider.ErrorKind {
	m := lower(msg)
	credential := containsAny(m, "şifre", "sifre", "parola", "password", "kullanıcı adı", "kullanici adi", "username")
	switch {
	case !credential && containsAny(m, "yetki", "izin", "permission", "not authorized", "unauthorized"):
		return provider.KindPermission
	case op == "login" || credential:
		return provider.KindAuth
	case containsAny(m, "session", "oturum"):
		return provider.KindAuth
	case containsAny(m, "bulunamad", "not found", "kayıt yok", "kayit yok"):
		return provider.KindNotFound
	case containsAny(m, "zaman aşımı", "timeout"):
		return provider.KindTimeout
	case containsAny(m, "servis", "service unavailable", "bakım"):
		return provider.KindUnavailable
	}
	return provider.KindUnknown
}

func isSessionError(e *provider.Error) bool {
	if e == nil || e.Op == "login" {
		return false
	}
	return containsAny(lower(e.Message), "session", "oturum")
}
