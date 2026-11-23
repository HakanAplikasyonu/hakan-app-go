package lucakoza

import (
	"net"
	"net/http"
	"syscall"
	"time"
)

// cgnat is the shared address space (100.64.0.0/10).
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// blocked reports addresses a user-entered Koza address must not reach:
// the server itself, private and link-local networks (cloud metadata).
func blocked(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip)
}

// GuardedHTTP is the client for user-entered Koza addresses: every
// connection is checked after name resolution (so a name cannot point to an
// internal address), no proxy from the environment, no redirects.
func GuardedHTTP() *http.Client {
	d := &net.Dialer{
		Timeout: 15 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrAddress
			}
			ip := net.ParseIP(host)
			if ip == nil || blocked(ip) {
				return ErrAddress
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 90 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
