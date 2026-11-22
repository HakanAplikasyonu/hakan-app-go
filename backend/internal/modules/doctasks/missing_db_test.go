package doctasks

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks/missingdocs"
)

type fakeSMS struct {
	sent  []string
	rules []string
}

func (f *fakeSMS) SMSStatus(context.Context, int64) (bool, bool, error) { return true, true, nil }

func (f *fakeSMS) SendToEngagement(_ context.Context, _ int64, _ uuid.UUID, text, rule, _ string, _ int64) (SMSOutcome, error) {
	f.sent = append(f.sent, text)
	f.rules = append(f.rules, rule)
	return SMSOutcome{Status: "dry_run"}, nil
}

// TestMissingDocuments (#780): a taxpayer that sent Z reports in July but
// nothing in September has "Eylül 2026 Z raporları" missing; asking for it
// opens one request (and one SMS), asking again opens nothing.
func TestMissingDocuments(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	sms := &fakeSMS{}
	w.svc.WithSMS(sms)
	if _, err := w.d.Pool.Exec(ctx, `
		INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period, total_amount)
		VALUES ($1, 'test', 'z-1', 'manual', '2026-07-10', '2026-07', 100)`, w.taxpayerID); err != nil {
		t.Fatal(err)
	}

	v, err := w.svc.Missing(ctx, w.scope, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if !v.SMSAvailable || !v.SMSTestMode {
		t.Fatalf("sms flags: %+v", v)
	}
	var z *missingdocs.Item
	for i := range v.Items {
		if v.Items[i].Kind == missingdocs.KindZReports {
			z = &v.Items[i]
		}
	}
	if z == nil || z.Title != "Eylül 2026 Z raporları" || z.Requested {
		t.Fatalf("missing items: %+v", v.Items)
	}

	res, err := w.svc.RequestMissing(ctx, w.scope, 0, MissingRequestInput{Period: "2026-09", Items: []MissingItemInput{{Kind: missingdocs.KindZReports}}, SMS: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 1 || res.Created[0].Title != "Eylül 2026 Z raporları" || res.Created[0].Period != "2026-09" {
		t.Fatalf("created: %+v", res)
	}
	if res.SMS == nil || res.SMS.Status != "dry_run" || len(sms.sent) != 1 || sms.rules[0] != missingdocs.SourceRule {
		t.Fatalf("sms: %+v %v", res.SMS, sms.sent)
	}

	again, err := w.svc.RequestMissing(ctx, w.scope, 0, MissingRequestInput{Period: "2026-09", Items: []MissingItemInput{{Kind: missingdocs.KindZReports}}, SMS: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Created) != 0 || len(again.Existing) != 1 || again.SMS.Code != SMSCodeNothingNew || len(sms.sent) != 1 {
		t.Fatalf("repeat: %+v", again)
	}
	v, err = w.svc.Missing(ctx, w.scope, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range v.Items {
		if it.Kind == missingdocs.KindZReports && !it.Requested {
			t.Fatalf("not marked requested: %+v", it)
		}
	}

	if _, err := w.svc.RequestMissing(ctx, w.scope, 0, MissingRequestInput{Period: "2026-09", Items: []MissingItemInput{{Kind: "nope"}}}); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := w.svc.RequestMissing(ctx, w.scope, 0, MissingRequestInput{Period: "2026-09", Items: []MissingItemInput{{Kind: missingdocs.KindBankStatement}}}); err == nil {
		t.Fatal("statement without account accepted")
	}
}

func TestMissingSMSText(t *testing.T) {
	got := MissingSMSText("Yılmaz SMMM", "2026-10", []string{"Z raporları", "masraf fişleri"})
	want := "Yılmaz SMMM: Ekim 2026 için Z raporları, masraf fişleri bekleniyor. Mükellef uygulamasından fotoğraf ya da PDF olarak yükleyebilirsiniz."
	if got != want {
		t.Fatalf("got %q", got)
	}
}
