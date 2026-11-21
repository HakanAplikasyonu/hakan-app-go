package communication

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mailtpl"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// E-mail tracking (#780 D): every e-mail the service sends carries an HTML
// part with a 1x1 pixel (opened_at) and attachment / document links go
// through a redirect (downloaded_at). Both are public routes keyed by the
// record's unguessable track_token; the first hit writes the time, later
// hits change nothing.

// Public paths of the tracking routes (behind the frontend's /api proxy).
const (
	trackPathPrefix = "/v1/public/mail/"
	openSuffix      = "/open"
	linkSuffix      = "/link"
)

// Tracking configures the links in sent e-mail.
type Tracking struct {
	// FrontendURL is the web origin; its /api proxy serves the routes and
	// it is always an allowed redirect target.
	FrontendURL string
	// ExtraOrigins are further allowed redirect origins (the storage's
	// public URL for document downloads).
	ExtraOrigins []string
}

// TrackingFromConfig: the frontend origin plus the storage's public URL
// (document downloads) as redirect targets.
func TrackingFromConfig(cfg config.Config) Tracking {
	t := Tracking{FrontendURL: cfg.Auth.FrontendURL}
	for _, u := range []string{cfg.Storage.MinIO.PublicBaseURL, cfg.Storage.S3.PublicBaseURL} {
		if strings.TrimSpace(u) != "" {
			t.ExtraOrigins = append(t.ExtraOrigins, u)
		}
	}
	return t
}

// WithTracking turns on the pixel and tracked links. Without a frontend URL
// e-mail goes out as plain text, untracked.
func (s *Service) WithTracking(t Tracking) *Service {
	t.FrontendURL = strings.TrimRight(strings.TrimSpace(t.FrontendURL), "/")
	s.tracking = t
	return s
}

// PixelURL is the open-tracking image of a message ("" when tracking is off).
func (s *Service) PixelURL(token uuid.UUID) string {
	if s.tracking.FrontendURL == "" {
		return ""
	}
	return s.tracking.FrontendURL + "/api" + trackPathPrefix + token.String() + openSuffix
}

// TrackedLink wraps target (an attachment or document link of the app) so
// a click writes downloaded_at. A target that is not an allowed redirect is
// returned unchanged, untracked.
func (s *Service) TrackedLink(token uuid.UUID, target string) string {
	if s.tracking.FrontendURL == "" || s.RedirectTarget(target) == "" {
		return target
	}
	return s.tracking.FrontendURL + "/api" + trackPathPrefix + token.String() + linkSuffix + "?to=" + url.QueryEscape(target)
}

// origin is scheme://host[:port] in lower case.
func origin(u *url.URL) string {
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// RedirectTarget validates a link target: an absolute http(s) URL on the
// app's own frontend or storage origin, or a path on the frontend. It
// returns the absolute URL to redirect to, or "" (no open redirect).
func (s *Service) RedirectTarget(target string) string {
	target = strings.TrimSpace(target)
	front := s.tracking.FrontendURL
	if target == "" || front == "" || strings.ContainsAny(target, "\\\r\n\t") {
		return ""
	}
	if strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "//") {
		target = front + target
	}
	u, err := url.Parse(target)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	allowed := append([]string{front}, s.tracking.ExtraOrigins...)
	for _, a := range allowed {
		au, err := url.Parse(strings.TrimSpace(a))
		if err != nil || au.Host == "" {
			continue
		}
		if origin(au) == origin(u) {
			return u.String()
		}
	}
	return ""
}

// MarkOpened records the first open of the message with this token; an
// unknown token changes nothing. It reports whether a record changed.
func (s *Service) MarkOpened(ctx context.Context, token uuid.UUID) (bool, error) {
	var n int64
	err := rls.System(ctx, s.pool, "communication: e-mail open pixel", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		n, err = s.q.MarkMailOpenedByToken(ctx, token)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("mark mail opened: %w", err)
	}
	return n > 0, nil
}

// MarkDownloaded records the first click on a tracked link (and the open,
// when the pixel was blocked).
func (s *Service) MarkDownloaded(ctx context.Context, token uuid.UUID) (bool, error) {
	var n int64
	err := rls.System(ctx, s.pool, "communication: e-mail attachment link", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		n, err = s.q.MarkMailDownloadedByToken(ctx, token)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("mark mail downloaded: %w", err)
	}
	return n > 0, nil
}

// mailHTML renders the office e-mail in the shared layout with the pixel.
// "" when tracking is off or rendering fails (the text part still goes).
func (s *Service) mailHTML(subject, body string, token uuid.UUID) string {
	pixel := s.PixelURL(token)
	if pixel == "" {
		return ""
	}
	var blocks []mailtpl.Block
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			blocks = append(blocks, mailtpl.Block{Kind: mailtpl.Paragraph, Text: p})
		}
	}
	out, err := mailtpl.New(s.tracking.FrontendURL).RenderSafe(mailtpl.Email{Subject: subject, Blocks: blocks, PlainText: body})
	if err != nil || out.HTML == "" {
		return ""
	}
	return withPixel(out.HTML, pixel)
}

// withPixel puts the 1x1 image just before </body>.
func withPixel(html, pixel string) string {
	img := `<img src="` + htmlAttr(pixel) + `" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0;opacity:0" />`
	if i := strings.LastIndex(strings.ToLower(html), "</body>"); i >= 0 {
		return html[:i] + img + html[i:]
	}
	return html + img
}

func htmlAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
