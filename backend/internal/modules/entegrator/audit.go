package entegrator

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// auditHTTP is the HTTP client of integrator calls. Connect and TLS
// handshake are bounded: an endpoint that only answers allowlisted IPs
// (İşNet) shows up as a dial timeout (32 §2).
var auditHTTP = func() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 15 * time.Second
	return &http.Client{Transport: tr, Timeout: 2 * time.Minute}
}()

// auditDoer writes one log line per integrator call (32 §2: connection,
// VKN, method): connection, masked VKN, method, host, path, SOAP action,
// HTTP status, duration and error. Bodies and headers carrying credentials
// are never logged (02 §4, 37 §2); the VKN/TCKN is masked because a TCKN
// is personal data.
type auditDoer struct {
	next   *http.Client
	log    *slog.Logger
	conn   uuid.UUID
	system string
	vkn    string
}

// maskTaxID keeps the first two and last two digits.
func maskTaxID(s string) string {
	if len(s) < 6 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

func (a auditDoer) Do(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := a.next.Do(r) //nolint:gosec // URL: provider endpoint or the vault-stored Credentials.Endpoint override (operator-set, no API writes it today; validate it if one is added, docs/guvenlik-taramalari.md)
	attrs := []any{
		"connection", a.conn, "source_system", a.system, "vkn", maskTaxID(a.vkn), "method", r.Method,
		"host", r.URL.Host, "path", r.URL.Path,
		"soap_action", strings.Trim(r.Header.Get("SOAPAction"), `"`),
		"duration_ms", time.Since(start).Milliseconds(),
	}
	if resp != nil {
		attrs = append(attrs, "status", resp.StatusCode)
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}
	a.log.Info("entegrator_call", attrs...)
	return resp, err
}
