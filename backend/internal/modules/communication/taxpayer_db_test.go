package communication

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakePortal struct{ eng db.Engagement }

func (f fakePortal) GetActiveEngagementForTaxpayer(context.Context, int64) (db.Engagement, error) {
	return f.eng, nil
}

// TestTaxpayerMessages (#780 "Mesajlarım"): the company sees only its own
// office's SMS and e-mails, newest first, without dry runs and without the
// office's internal fields; addresses are masked.
func TestTaxpayerMessages(t *testing.T) {
	ctx := context.Background()
	d := testdb.Open(t)
	org := d.Organization(t)
	tpX := d.Taxpayer(t, "legal", "1111111111")
	engX := d.Engagement(t, org, tpX)
	engY := d.Engagement(t, org, d.Taxpayer(t, "legal", "2222222222"))

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	sms := `INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, provider_job_id,
		error_code, source_rule, created_at, sent_at, delivered_at)
		VALUES ($1, $2, $3, $4, 'OFISADI', $5, 'JOB-GIZLI-1', 'KOD-GIZLI', 'kural_gizli', $6::timestamptz, $6::timestamptz, NULLIF($7, '')::timestamptz)`
	exec(sms, org, engX, "5321234567", "X iletildi", "sent", "2026-11-10T10:00:00Z", "2026-11-10T10:00:05Z")
	exec(sms, org, engX, "5321234567", "X başarısız", "failed", "2026-11-11T10:00:00Z", "")
	exec(sms, org, engX, "5321234567", "X deneme modu", "dry_run", "2026-11-12T10:00:00Z", "")
	exec(sms, org, engY, "5329999999", "Y mesajı", "sent", "2026-11-13T10:00:00Z", "")
	exec(`INSERT INTO mail_messages (organization_id, engagement_id, to_email, subject, body, status, source_rule, created_at, sent_at, opened_at)
		VALUES ($1, $2, 'muhasebe@ornek.com.tr', 'Ekim KDV', 'X e-posta', 'sent', 'kural_gizli', '2026-11-14T10:00:00Z', '2026-11-14T10:00:00Z', NOW())`, org, engX)

	h := NewHandler(NewService(d.Pool, nil, Options{})).WithPortal(fakePortal{eng: db.Engagement{ID: engX, OrganizationID: org}})
	get := func(q string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/v1/taxpayer/messages"+q, nil)
		req = req.WithContext(taxpayerctx.WithScope(req.Context(), taxpayerctx.Scope{ID: tpX}))
		rec := httptest.NewRecorder()
		h.TaxpayerMessages(rec, req)
		return rec.Code, rec.Body.String()
	}
	code, body := get("")
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, body)
	}
	var out struct {
		Data struct {
			Items []TaxpayerMessage `json:"items"`
			Total int64             `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Total != 3 || len(out.Data.Items) != 3 {
		t.Fatalf("total %d, items %d, want 3 (no dry run, no other taxpayer): %s", out.Data.Total, len(out.Data.Items), body)
	}
	want := []struct{ channel, body, status, to string }{
		{ChannelEmail, "X e-posta", MessageSent, "m***@ornek.com.tr"},
		{ChannelSMS, "X başarısız", MessageFailed, "******** 67"},
		{ChannelSMS, "X iletildi", MessageDelivered, "******** 67"},
	}
	for i, w := range want {
		g := out.Data.Items[i]
		if g.Channel != w.channel || g.Body != w.body || g.Status != w.status || g.To != w.to {
			t.Errorf("item %d = %+v, want %+v", i, g, w)
		}
	}
	if out.Data.Items[0].Subject != "Ekim KDV" {
		t.Errorf("subject = %q", out.Data.Items[0].Subject)
	}
	for _, leak := range []string{"Y mesajı", "deneme modu", "JOB-GIZLI", "KOD-GIZLI", "kural_gizli", "OFISADI",
		"5321234567", "muhasebe@", "opened_at", "sent_by", "provider_job_id"} {
		if strings.Contains(body, leak) {
			t.Errorf("answer carries %q: %s", leak, body)
		}
	}

	code, body = get("?limit=1&offset=1")
	if err := json.Unmarshal([]byte(body), &out); err != nil || code != http.StatusOK {
		t.Fatalf("page = %d %s", code, body)
	}
	if len(out.Data.Items) != 1 || out.Data.Items[0].Body != "X başarısız" || out.Data.Total != 3 {
		t.Errorf("page 2 = %s", body)
	}
}

func TestMaskAddress(t *testing.T) {
	for in, want := range map[string]string{"+90 532 123 45 67": "********** 67", "12": "**", "": ""} {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"ali@x.com": "a***@x.com", "bozuk": "***", "çiğdem@y.tr": "ç***@y.tr"} {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
