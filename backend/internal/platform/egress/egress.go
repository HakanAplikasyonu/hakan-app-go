// Package egress makes integrator and bank calls leave from a fixed public
// IP (issue #45, D4, 20-banka-genel.md §4).
//
// Integrators such as İşNet only accept calls from allowlisted IPs. The app
// therefore runs those calls in a dedicated worker pool (queue
// "fixed-egress", WORKER_POOL=fixed-egress) deployed where the outbound IP is
// fixed: either a host with a static IP, or any host whose calls go through a
// forward proxy on a static IP (EGRESS_PROXY_URL). Which one is used is a
// deployment decision; the code supports both.
//
// A periodic task (TaskCheck, every CheckInterval, on the fixed-egress queue)
// asks a public "what is my IP" service through the same transport and
// compares the answer with EGRESS_EXPECTED_IPS. A mismatch or a failed check
// fails the task: the queue's error handler logs it and reports it to Sentry
// (the alarm), and the last result is kept in Redis for the platform status
// endpoint (GET /v1/platform/egress).
package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults.
const (
	DefaultCheckURL      = "https://api.ipify.org"
	DefaultCheckInterval = 15 * time.Minute
	DefaultTimeout       = 2 * time.Minute
)

// Config is the fixed-egress setup (EGRESS_* env).
type Config struct {
	// ProxyURL routes fixed-egress calls through a forward proxy
	// (http://, https:// or socks5://). Empty: direct from this host.
	ProxyURL string
	// ExpectedIPs are the allowlisted public IPs. Empty disables the
	// comparison (the check still records the observed IP).
	ExpectedIPs []string
	// CheckURL returns the caller's public IP as plain text.
	CheckURL string
	// CheckInterval is the period of the IP check.
	CheckInterval time.Duration
	// ProxyHosts limits ProxyURL to these hosts; any other host is called
	// directly (issue #256: only the allowlisting integrator and bank hosts
	// leave from the fixed IP). ".example.com" also covers subdomains, the
	// same rule as squid's dstdomain in deploy/egress-proxy. Empty: every
	// call goes through ProxyURL. The CheckURL host is always proxied so
	// the IP check sees the address the integrators see.
	ProxyHosts []string
}

// Normalize fills defaults.
func (c Config) Normalize() Config {
	if strings.TrimSpace(c.CheckURL) == "" {
		c.CheckURL = DefaultCheckURL
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = DefaultCheckInterval
	}
	out := c.ExpectedIPs[:0:0]
	for _, ip := range c.ExpectedIPs {
		if ip = strings.TrimSpace(ip); ip != "" {
			out = append(out, ip)
		}
	}
	c.ExpectedIPs = out
	hosts := make([]string, 0, len(c.ProxyHosts)+1)
	for _, h := range c.ProxyHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) > 0 {
		if u, err := url.Parse(c.CheckURL); err == nil && u.Hostname() != "" && !matchHost(hosts, u.Hostname()) {
			hosts = append(hosts, strings.ToLower(u.Hostname()))
		}
	}
	c.ProxyHosts = hosts
	return c
}

// matchHost reports whether host is in hosts: an exact name, or under a
// ".domain" entry (which also matches the domain itself).
func matchHost(hosts []string, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range hosts {
		if strings.HasPrefix(h, ".") {
			if host == h[1:] || strings.HasSuffix(host, h) {
				return true
			}
			continue
		}
		if host == h {
			return true
		}
	}
	return false
}

// Proxied reports whether a call to host goes through ProxyURL.
func (c Config) Proxied(host string) bool {
	if strings.TrimSpace(c.ProxyURL) == "" {
		return false
	}
	n := c.Normalize()
	return len(n.ProxyHosts) == 0 || matchHost(n.ProxyHosts, host)
}

// Validate checks the configuration without contacting anything.
func (c Config) Validate() error {
	if p := strings.TrimSpace(c.ProxyURL); p != "" {
		u, err := url.Parse(p)
		if err != nil || u.Host == "" {
			return fmt.Errorf("egress: EGRESS_PROXY_URL is not a URL")
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return fmt.Errorf("egress: EGRESS_PROXY_URL scheme must be http, https or socks5")
		}
	}
	for _, ip := range c.ExpectedIPs {
		if net.ParseIP(strings.TrimSpace(ip)) == nil {
			return fmt.Errorf("egress: EGRESS_EXPECTED_IPS has an invalid IP %q", ip)
		}
	}
	for _, h := range c.ProxyHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if strings.Trim(h, ".") == "" || strings.ContainsAny(h, "/:@ *") {
			return fmt.Errorf("egress: EGRESS_PROXY_HOSTS has an invalid host %q", h)
		}
	}
	return nil
}

// HTTPClient returns the client for fixed-egress calls: through ProxyURL when
// set, otherwise direct. It satisfies go-ubltr's provider.Doer.
func (c Config) HTTPClient(timeout time.Duration) (*http.Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Bounded connect and TLS handshake, as for direct integrator calls: an
	// endpoint that only answers allowlisted IPs shows up as a dial timeout.
	tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 15 * time.Second
	// Never pick up HTTP(S)_PROXY from the environment: the route must be
	// exactly the configured one.
	tr.Proxy = nil
	if p := strings.TrimSpace(c.ProxyURL); p != "" {
		u, _ := url.Parse(p)
		n := c.Normalize()
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			if len(n.ProxyHosts) == 0 || matchHost(n.ProxyHosts, req.URL.Hostname()) {
				return u, nil
			}
			return nil, nil // not an allowlisting host: direct
		}
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}

// Status is the result of one IP check.
type Status struct {
	ObservedIP  string    `json:"observed_ip"`
	ExpectedIPs []string  `json:"expected_ips"`
	OK          bool      `json:"ok"`
	Error       string    `json:"error,omitempty"`
	ViaProxy    bool      `json:"via_proxy"`
	CheckedAt   time.Time `json:"checked_at"`
}

// Errors of a check.
var (
	ErrIPMismatch = errors.New("egress: public IP is not one of EGRESS_EXPECTED_IPS")
	ErrCheck      = errors.New("egress: public IP check failed")
)

// Doer sends HTTP requests (*http.Client).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Check asks CheckURL for the public IP through hc and compares it with the
// expected IPs. The returned error is ErrIPMismatch or wraps ErrCheck.
func Check(ctx context.Context, hc Doer, cfg Config, now time.Time) (Status, error) {
	cfg = cfg.Normalize()
	st := Status{ExpectedIPs: cfg.ExpectedIPs, ViaProxy: strings.TrimSpace(cfg.ProxyURL) != "", CheckedAt: now.UTC()}
	if st.ExpectedIPs == nil {
		st.ExpectedIPs = []string{}
	}
	ip, err := publicIP(ctx, hc, cfg.CheckURL)
	if err != nil {
		st.Error = err.Error()
		return st, fmt.Errorf("%w: %v", ErrCheck, err)
	}
	st.ObservedIP = ip
	if len(cfg.ExpectedIPs) == 0 {
		st.OK = true
		return st, nil
	}
	for _, want := range cfg.ExpectedIPs {
		if sameIP(want, ip) {
			st.OK = true
			return st, nil
		}
	}
	st.Error = fmt.Sprintf("observed %s, expected one of %s", ip, strings.Join(cfg.ExpectedIPs, ", "))
	return st, ErrIPMismatch
}

func publicIP(ctx context.Context, hc Doer, checkURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d from %s", resp.StatusCode, checkURL)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("%s did not answer with an IP", checkURL)
	}
	return ip, nil
}

func sameIP(a, b string) bool {
	x, y := net.ParseIP(strings.TrimSpace(a)), net.ParseIP(strings.TrimSpace(b))
	return x != nil && y != nil && x.Equal(y)
}

// IsIPNotAllowlisted reports whether an integrator error says the calling IP
// is not on its allowlist (go-ubltr reports it as kind/code
// "ip-not-allowlisted"; some integrators only say it in the message). Such an
// error means the call left from the wrong IP: the fixed-egress setup, not
// the credentials, needs attention.
func IsIPNotAllowlisted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"ip-not-allowlisted", "ip not allowlisted", "ip_not_allowlisted", "ip not whitelisted", "ip adresi yetkili değil", "ip adresinize izin"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
