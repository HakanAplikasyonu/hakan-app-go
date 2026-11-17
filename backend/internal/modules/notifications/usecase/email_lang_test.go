package usecase

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
)

// englishQuerier serves the English template (the fallback when the
// recipient's language has none).
type englishQuerier struct{ sealQuerier }

func (q *englishQuerier) GetTemplateByCodeChannelLang(context.Context, db.GetTemplateByCodeChannelLangParams) (db.NotificationTemplate, error) {
	return db.NotificationTemplate{Subject: testLoginSubject, Body: testLoginBody, Language: "en"}, nil
}

// The email row records the template's language so the layout's own words
// (button fallback, footer) match the text.
func TestEmailRowRecordsTemplateLanguage(t *testing.T) {
	t.Parallel()
	q := &englishQuerier{}
	svc := newSealService(t, q, &recordingSender{})
	if _, err := svc.Enqueue(context.Background(), securityEmail()); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(q.row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload[providers.PayloadLang] != "en" {
		t.Fatalf("payload = %v", payload)
	}
}
