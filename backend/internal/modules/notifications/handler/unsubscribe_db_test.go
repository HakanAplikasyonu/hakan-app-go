package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/unsubscribe"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

// TestOneClickUnsubscribe drives the List-Unsubscribe target as a mail
// provider does (RFC 8058): no session, a form POST to the signed URL.
func TestOneClickUnsubscribe(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	var userID int64
	var userUUID uuid.UUID
	if err := d.Pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ('abone@example.test', 'x', 'A', 'B') RETURNING id, uuid`).Scan(&userID, &userUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO notification_preferences (user_id, email_enabled, inapp_enabled, realtime_enabled, push_enabled) VALUES ($1, true, false, true, true)`, userID); err != nil {
		t.Fatal(err)
	}
	links := unsubscribe.New("test-app-key", "https://app.example.com")
	svc := usecase.New(db.New(d.Pool), nil, nil, slog.Default()).WithUnsubscribe(links)
	h := New(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/notifications/unsubscribe", h.Unsubscribe)
	mux.HandleFunc("GET /v1/notifications/unsubscribe", h.UnsubscribeLanding)

	signed, _ := url.Parse(links.URL(userUUID))
	query := "?" + signed.RawQuery
	post := func(q string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/notifications/unsubscribe"+q, strings.NewReader("List-Unsubscribe=One-Click"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	emailOn := func() bool {
		var on bool
		if err := d.Pool.QueryRow(ctx, `SELECT email_enabled FROM notification_preferences WHERE user_id = $1`, userID).Scan(&on); err != nil {
			t.Fatal(err)
		}
		return on
	}

	// A GET (browser, link scanner) changes nothing and lands on settings.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/notifications/unsubscribe"+query, nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://app.example.com/profile" || !emailOn() {
		t.Fatalf("GET: %d %q email=%v", w.Code, w.Header().Get("Location"), emailOn())
	}

	other := uuid.New()
	for name, q := range map[string]string{
		"no signature":       "?u=" + userUUID.String(),
		"wrong signature":    "?u=" + userUUID.String() + "&sig=" + strings.Repeat("ab", 32),
		"another user's sig": "?u=" + other.String() + "&sig=" + signed.Query().Get("sig"),
		"bad uuid":           "?u=x&sig=" + signed.Query().Get("sig"),
	} {
		if w := post(q); w.Code < 400 || !emailOn() {
			t.Fatalf("%s: %d email=%v", name, w.Code, emailOn())
		}
	}

	if w := post(query); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"email_enabled":false`) {
		t.Fatalf("POST: %d %s", w.Code, w.Body.String())
	}
	if emailOn() {
		t.Fatal("email notifications still on")
	}
	// Only the email channel changed.
	var inapp, realtime, push bool
	if err := d.Pool.QueryRow(ctx, `SELECT inapp_enabled, realtime_enabled, push_enabled FROM notification_preferences WHERE user_id = $1`, userID).Scan(&inapp, &realtime, &push); err != nil {
		t.Fatal(err)
	}
	if inapp || !realtime || !push {
		t.Fatalf("other channels changed: inapp=%v realtime=%v push=%v", inapp, realtime, push)
	}
	// Idempotent: providers may retry.
	if w := post(query); w.Code != http.StatusOK {
		t.Fatalf("repeat POST: %d", w.Code)
	}
	// A valid link for a user that no longer exists.
	gone, _ := url.Parse(links.URL(other))
	if w := post("?" + gone.RawQuery); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
}
