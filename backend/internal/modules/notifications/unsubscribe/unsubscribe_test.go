package unsubscribe

import (
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLinks(t *testing.T) {
	l := New("app-key", "https://app.example.com/")
	user := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	raw := l.URL(user)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "app.example.com" || u.Path != "/api/v1/notifications/unsubscribe" {
		t.Fatalf("url = %s", raw)
	}
	if u.Query().Get("u") != user.String() || !l.Verify(user, u.Query().Get("sig")) {
		t.Fatal("signature does not verify")
	}
	other := uuid.MustParse("11111111-2222-3333-4444-555555555556")
	if l.Verify(other, u.Query().Get("sig")) {
		t.Fatal("signature verifies for another user")
	}
	if New("other-key", "https://app.example.com").Verify(user, u.Query().Get("sig")) {
		t.Fatal("signature verifies under another key")
	}
	for _, bad := range []string{"", "zz", strings.Repeat("0", 64)} {
		if l.Verify(user, bad) {
			t.Fatalf("bad signature %q accepted", bad)
		}
	}
	if l.PreferencesURL() != "https://app.example.com/profile" {
		t.Fatal(l.PreferencesURL())
	}
	if (Links{}).URL(user) != "" || New("", "https://a.example").Enabled() {
		t.Fatal("disabled links must produce nothing")
	}
}

// The key is derived: the raw app key is not the HMAC key.
func TestKeyIsDerived(t *testing.T) {
	l := New("app-key", "https://a.example")
	if string(l.key) == "app-key" || len(l.key) != 32 {
		t.Fatal("key not derived")
	}
}
