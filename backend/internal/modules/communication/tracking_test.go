package communication

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func trackingService() *Service {
	return (&Service{}).WithTracking(Tracking{FrontendURL: "https://app.example.com/", ExtraOrigins: []string{"https://cdn.example.com/bucket"}})
}

func TestRedirectTarget(t *testing.T) {
	s := trackingService()
	ok := map[string]string{
		"https://app.example.com/t/ofis/belge": "https://app.example.com/t/ofis/belge",
		"/t/ofis/belge?x=1":                    "https://app.example.com/t/ofis/belge?x=1",
		"https://cdn.example.com/bucket/a.pdf": "https://cdn.example.com/bucket/a.pdf",
		"HTTPS://APP.EXAMPLE.COM/x":            "https://APP.EXAMPLE.COM/x",
	}
	for in, want := range ok {
		if got := s.RedirectTarget(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{
		"", "https://evil.example.com/", "//evil.example.com/x", "/\\evil.example.com", "javascript:alert(1)",
		"https://app.example.com@evil.example.com/", "https://app.example.com.evil.example.com/",
		"ftp://app.example.com/x", "http://app.example.com/x", "https://app.example.com:8443/x", "data:text/html,x",
		"https://app.example.com/\r\nSet-Cookie: x",
	} {
		if got := s.RedirectTarget(bad); got != "" {
			t.Errorf("%q accepted as %q", bad, got)
		}
	}
	if (&Service{}).RedirectTarget("https://app.example.com/") != "" {
		t.Error("tracking off must refuse every target")
	}
}

func TestPixelAndTrackedLink(t *testing.T) {
	s := trackingService()
	tok := uuid.MustParse("6f1c2c1e-6a43-4d55-9d1c-0d7b9f3a1b2c")
	if got := s.PixelURL(tok); got != "https://app.example.com/api/v1/public/mail/"+tok.String()+"/open" {
		t.Fatalf("pixel = %s", got)
	}
	link := s.TrackedLink(tok, "https://cdn.example.com/bucket/a b.pdf")
	if !strings.HasPrefix(link, "https://app.example.com/api/v1/public/mail/"+tok.String()+"/link?to=https%3A%2F%2Fcdn.example.com") {
		t.Fatalf("link = %s", link)
	}
	if (&Service{}).PixelURL(tok) != "" {
		t.Fatal("pixel without tracking")
	}
	html := s.mailHTML("Konu", "Merhaba,\n\nKDV tahakkukunuz hazır.", tok)
	if !strings.Contains(html, s.PixelURL(tok)) || !strings.Contains(html, "KDV tahakkukunuz") {
		t.Fatalf("html = %s", html)
	}
	if i, j := strings.Index(html, `width="1" height="1"`), strings.LastIndex(strings.ToLower(html), "</body>"); i < 0 || j < i {
		t.Fatal("pixel must sit before </body>")
	}
	if (&Service{}).mailHTML("Konu", "Metin", tok) != "" {
		t.Fatal("html part without tracking")
	}
}

func TestSortHistoryNewestFirst(t *testing.T) {
	now := time.Now()
	items := []HistoryItem{{Channel: "email", CreatedAt: now.Add(-time.Hour)}, {Channel: "sms", CreatedAt: now}, {Channel: "sms", CreatedAt: now.Add(-2 * time.Hour)}}
	SortHistory(items)
	if !items[0].CreatedAt.Equal(now) || !items[2].CreatedAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("order = %+v", items)
	}
}
