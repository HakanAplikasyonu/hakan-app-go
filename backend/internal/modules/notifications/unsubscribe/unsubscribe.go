// Package unsubscribe signs the one-click unsubscribe links of notification
// emails (List-Unsubscribe, RFC 8058). A link names the user and carries an
// HMAC, so the endpoint needs no session: a mail provider posts to it from
// its own servers.
package unsubscribe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// Path is the API route (behind the frontend's /api proxy).
const Path = "/v1/notifications/unsubscribe"

// Links signs and verifies unsubscribe links.
type Links struct {
	key         []byte
	frontendURL string
}

// New derives the signing key from the application encryption key (stable:
// it is never rotated, so links in old emails keep working) with a purpose
// label, so the key signs nothing else.
func New(appKey, frontendURL string) Links {
	if appKey == "" {
		return Links{}
	}
	mac := hmac.New(sha256.New, []byte(appKey))
	_, _ = mac.Write([]byte("notifications/unsubscribe/v1"))
	return Links{key: mac.Sum(nil), frontendURL: strings.TrimRight(frontendURL, "/")}
}

// Enabled reports whether links can be made.
func (l Links) Enabled() bool { return len(l.key) > 0 && l.frontendURL != "" }

// Sign returns the hex HMAC for a user.
func (l Links) Sign(user uuid.UUID) string {
	mac := hmac.New(sha256.New, l.key)
	_, _ = mac.Write([]byte(user.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether sig was made for user.
func (l Links) Verify(user uuid.UUID, sig string) bool {
	if len(l.key) == 0 || user == uuid.Nil {
		return false
	}
	got, err := hex.DecodeString(strings.TrimSpace(sig))
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(l.Sign(user))
	return hmac.Equal(got, want)
}

// URL is the absolute one-click link for user: the frontend origin proxies
// /api/v1/* to the API, so the link lives on the domain the mail is about.
func (l Links) URL(user uuid.UUID) string {
	if !l.Enabled() || user == uuid.Nil {
		return ""
	}
	q := url.Values{}
	q.Set("u", user.String())
	q.Set("sig", l.Sign(user))
	return l.frontendURL + "/api" + Path + "?" + q.Encode()
}

// PreferencesURL is the page where a signed-in user changes notification
// settings (the GET landing of the unsubscribe link sends people there).
func (l Links) PreferencesURL() string {
	if l.frontendURL == "" {
		return ""
	}
	return l.frontendURL + "/profile"
}
