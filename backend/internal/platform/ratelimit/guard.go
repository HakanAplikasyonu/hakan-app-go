package ratelimit

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// IsUpload reports whether r is a file upload: a multipart write, or a write
// to an ".../upload(s)" or ".../import(s)" path (presigned upload sessions
// and JSON-described imports included).
func IsUpload(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && strings.HasPrefix(mt, "multipart/") {
		return true
	}
	for _, seg := range strings.Split(strings.Trim(r.URL.Path, "/"), "/") {
		switch seg {
		case "upload", "uploads", "ubl-uploads", "bank-statements", "import", "imports", "excel-imports", "logo":
			return true
		}
	}
	return false
}

// UploadGuard limits uploads per session token and per client IP before
// any handler reads the body. Requests without a bearer token are counted
// against the IP only (authentication rejects them later anyway).
func (l *Limiter) UploadGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil || l.store == nil || !IsUpload(r) {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientip.From(r)
		checks := []struct{ action, subject string }{
			{ActionUploadIP, ip},
		}
		if tok := bearer(r.Header.Get("Authorization")); tok != "" {
			// The token itself is never stored: only its hash is the key.
			checks = append([]struct{ action, subject string }{{ActionUploadUser, HashSubject(tok)}}, checks...)
		}
		for _, c := range checks {
			ok, retry := l.allowRule(r.Context(), c.action, c.subject)
			if ok {
				continue
			}
			l.Report(r, Event{Kind: "rate_limited", Action: c.action, IP: ip, Retry: retry}, c.action+"|"+c.subject)
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			}
			response.TooManyRequests(w, r, "Too many uploads. Try again later.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(h string) string {
	h = strings.TrimSpace(h)
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
