package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
)

// TestSessionRevocationEndsAccessTokens (#323): signing out, revoking a
// session, "sign out other sessions" and a password change end the access
// tokens of those sessions at once, not when they expire. A refresh keeps
// its session: the access token issued before it keeps working.
func TestSessionRevocationEndsAccessTokens(t *testing.T) {
	e := newSweepEnv(t)
	const email, pass = "mukellef@demo.hakan.local", "Demo1234!"

	login := func() authmodel.Tokens {
		t.Helper()
		return e.authCall(t, http.MethodPost, "/v1/auth/login", "", `{"email":"`+email+`","password":"`+pass+`"}`, http.StatusOK)
	}
	me := func(access string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}

	one, two := login(), login()
	if me(one.AccessToken) != http.StatusOK || me(two.AccessToken) != http.StatusOK {
		t.Fatal("fresh sessions refused")
	}

	// Rotation keeps the session: the old access token still works.
	rotated := e.authCall(t, http.MethodPost, "/v1/auth/refresh", "", `{"refresh_token":"`+one.RefreshToken+`"}`, http.StatusOK)
	if me(one.AccessToken) != http.StatusOK || me(rotated.AccessToken) != http.StatusOK {
		t.Fatal("refresh ended the session")
	}

	// Sign-out of session one: its access tokens (before and after the
	// rotation) stop; session two goes on.
	e.authCall(t, http.MethodPost, "/v1/auth/logout", rotated.AccessToken, `{"refresh_token":"`+rotated.RefreshToken+`"}`, http.StatusOK)
	if got := me(one.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("access token of a signed-out session (before rotation) = %d, want 401", got)
	}
	if got := me(rotated.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("access token of a signed-out session = %d, want 401", got)
	}
	if got := me(two.AccessToken); got != http.StatusOK {
		t.Fatalf("another session after sign-out = %d, want 200", got)
	}

	// The session list: revoking session two by its uuid ends it.
	three := login()
	var list struct {
		Data struct {
			Items []struct {
				UUID    string `json:"uuid"`
				Current bool   `json:"current"`
			} `json:"items"`
		} `json:"data"`
	}
	raw := e.rawCall(t, http.MethodGet, "/v1/auth/sessions", three.AccessToken, "", http.StatusOK)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	var current, other string
	for _, s := range list.Data.Items {
		if s.Current {
			current = s.UUID
		} else if other == "" {
			other = s.UUID
		}
	}
	if current == "" || other == "" {
		t.Fatalf("session list without the current and another session: %s", raw)
	}
	e.rawCall(t, http.MethodDelete, "/v1/auth/sessions/"+other, three.AccessToken, "", http.StatusOK)

	// "Sign out other sessions": every session but the caller's ends.
	four := login()
	e.rawCall(t, http.MethodPost, "/v1/auth/sessions/revoke-others", three.AccessToken, "", http.StatusOK)
	if got := me(two.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("access token after revoking its session = %d, want 401", got)
	}
	if got := me(four.AccessToken); got != http.StatusUnauthorized {
		t.Errorf("access token after 'sign out other sessions' = %d, want 401", got)
	}
	if got := me(three.AccessToken); got != http.StatusOK {
		t.Fatalf("the caller's own session after 'sign out other sessions' = %d, want 200", got)
	}

	// A password change ends every session, the caller's too.
	five := login()
	e.rawCall(t, http.MethodPost, "/v1/auth/password/change", three.AccessToken,
		`{"current_password":"`+pass+`","new_password":"Yeni-Parola-1234!"}`, http.StatusOK)
	for name, tok := range map[string]string{"caller": three.AccessToken, "other": five.AccessToken} {
		if got := me(tok); got != http.StatusUnauthorized {
			t.Errorf("%s session after a password change = %d, want 401", name, got)
		}
	}
	// Its refresh token does not bring the session back.
	e.rawCall(t, http.MethodPost, "/v1/auth/refresh", "", `{"refresh_token":"`+five.RefreshToken+`"}`, http.StatusUnauthorized)
}

func (e *sweepEnv) rawCall(t *testing.T, method, path, access, body string, want int) []byte {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	return rec.Body.Bytes()
}

func (e *sweepEnv) authCall(t *testing.T, method, path, access, body string, want int) authmodel.Tokens {
	t.Helper()
	var out struct {
		Data authmodel.Tokens `json:"data"`
	}
	raw := e.rawCall(t, method, path, access, body, want)
	if want == http.StatusOK {
		_ = json.Unmarshal(raw, &out)
	}
	return out.Data
}
