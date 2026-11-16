// Command egress-check shows and tests the fixed outbound route (issue
// #256). It uses the same EGRESS_* settings and HTTP client as the worker
// and the bank adapter, so what it reports is what İşNet and Vakıfbank see.
//
//	egress-check                          # the public IP calls leave from (via the proxy)
//	egress-check -probe https://host/     # one request through the configured route
//	egress-check -probe URL -force-proxy  # send it through the proxy even if host is not listed
//
// Exit codes: 0 ok, 1 error, 2 IP not in EGRESS_EXPECTED_IPS, 3 the proxy
// refused the host (not in its allowlist).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/egress"
)

func main() {
	probe := flag.String("probe", "", "URL to request through the configured route")
	force := flag.Bool("force-proxy", false, "with -probe: use the proxy for any host")
	flag.Parse()

	cfg := egress.Config{
		ProxyURL:    os.Getenv("EGRESS_PROXY_URL"),
		ExpectedIPs: split(os.Getenv("EGRESS_EXPECTED_IPS")),
		CheckURL:    os.Getenv("EGRESS_IP_CHECK_URL"),
		ProxyHosts:  split(os.Getenv("EGRESS_PROXY_HOSTS")),
	}.Normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if *probe == "" {
		os.Exit(checkIP(ctx, cfg))
	}
	os.Exit(probeURL(ctx, cfg, *probe, *force))
}

func checkIP(ctx context.Context, cfg egress.Config) int {
	hc, err := cfg.HTTPClient(20 * time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "egress-check:", err)
		return 1
	}
	st, err := egress.Check(ctx, hc, cfg, time.Now())
	fmt.Printf("observed_ip=%s via_proxy=%v expected=%s\n", st.ObservedIP, st.ViaProxy, strings.Join(st.ExpectedIPs, ","))
	switch {
	case errors.Is(err, egress.ErrIPMismatch):
		fmt.Fprintln(os.Stderr, "egress-check: the observed IP is not in EGRESS_EXPECTED_IPS")
		return 2
	case err != nil:
		fmt.Fprintln(os.Stderr, "egress-check:", err)
		return 1
	}
	return 0
}

func probeURL(ctx context.Context, cfg egress.Config, raw string, force bool) int {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		fmt.Fprintln(os.Stderr, "egress-check: -probe needs an absolute URL")
		return 1
	}
	if force {
		cfg.ProxyHosts = nil // every host through the proxy
	}
	proxied := cfg.Proxied(u.Hostname())
	hc, err := cfg.HTTPClient(20 * time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "egress-check:", err)
		return 1
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := hc.Do(req)
	if err != nil {
		if proxied && strings.Contains(err.Error(), "Forbidden") {
			fmt.Printf("host=%s proxied=true result=denied_by_proxy\n", u.Hostname())
			return 3
		}
		fmt.Fprintf(os.Stderr, "egress-check: host=%s proxied=%v: %v\n", u.Hostname(), proxied, err)
		return 1
	}
	_ = resp.Body.Close()
	fmt.Printf("host=%s proxied=%v status=%d\n", u.Hostname(), proxied, resp.StatusCode)
	return 0
}

func split(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
