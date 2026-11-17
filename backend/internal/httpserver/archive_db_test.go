package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Belge arşivi ve İSG evrakları (#393): yükleme, arama, kategori ve yaklaşan
// bitiş süzgeci, dosya indirme, başka mükellefin engagement'ından erişilemez.
// Yetki kuralları yetki matrisinde (authz_table_test).
func TestArchiveDocumentsFlow(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng + "/archive-documents"

	type doc struct {
		UUID         string  `json:"uuid"`
		Category     string  `json:"category"`
		ISGKind      *string `json:"isg_kind"`
		Title        string  `json:"title"`
		ValidUntil   *string `json:"valid_until"`
		ExpiryStatus string  `json:"expiry_status"`
		DaysLeft     *int    `json:"days_left"`
		ContentType  string  `json:"content_type"`
	}
	type page struct {
		Items  []doc `json:"items"`
		Total  int64 `json:"total"`
		Counts struct {
			Total, ISG, Expired, Expiring int64
			ISGExpired                    int64 `json:"isg_expired"`
			ISGExpiring                   int64 `json:"isg_expiring"`
		} `json:"counts"`
	}
	decode := func(t *testing.T, body []byte, v any) {
		t.Helper()
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(env.Data, v); err != nil {
			t.Fatalf("%v: %s", err, env.Data)
		}
	}
	upload := func(t *testing.T, name, body string, fields map[string]string, want int) doc {
		t.Helper()
		rec := e.do(t, http.MethodPost, base, sweepCase{file: "file", fileName: name, fileBody: body, fields: fields}, false)
		if rec.Code != want {
			t.Fatalf("upload %s: %d %s", name, rec.Code, rec.Body)
		}
		var d doc
		if want == http.StatusCreated {
			decode(t, rec.Body.Bytes(), &d)
		}
		return d
	}
	list := func(t *testing.T, query string) page {
		t.Helper()
		rec := e.do(t, http.MethodGet, base+query, sweepCase{}, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", query, rec.Code, rec.Body)
		}
		var p page
		decode(t, rec.Body.Bytes(), &p)
		return p
	}

	day := func(d int) string { return time.Now().AddDate(0, 0, d).Format(time.DateOnly) }
	before := list(t, "")

	upload(t, "kira.pdf", "%PDF-1.4\n% kira sozlesmesi test\n", map[string]string{
		"category": "contract", "title": "Ambar kiralama sözleşmesi", "document_date": day(-200),
	}, http.StatusCreated)
	expired := upload(t, "egitim.pdf", "%PDF-1.4\n% isg egitim test\n", map[string]string{
		"category": "isg", "isg_kind": "isg_training", "title": "Temel İSG eğitimi", "valid_until": day(-10),
	}, http.StatusCreated)
	soon := upload(t, "risk.pdf", "%PDF-1.4\n% risk degerlendirme test\n", map[string]string{
		"category": "isg", "isg_kind": "risk_assessment", "title": "Risk değerlendirmesi", "valid_until": day(10),
	}, http.StatusCreated)
	if expired.ExpiryStatus != "expired" || soon.ExpiryStatus != "expiring" || soon.DaysLeft == nil || *soon.DaysLeft < 9 {
		t.Fatalf("expiry = %+v / %+v", expired, soon)
	}

	// Kurallar: İSG türü zorunlu, yalnızca İSG'de; tarih sırası; dosya türü; mükerrer.
	upload(t, "x.pdf", "%PDF-1.4\n% a\n", map[string]string{"category": "isg"}, http.StatusBadRequest)
	upload(t, "x.pdf", "%PDF-1.4\n% b\n", map[string]string{"category": "tax", "isg_kind": "health_report"}, http.StatusBadRequest)
	upload(t, "x.pdf", "%PDF-1.4\n% c\n", map[string]string{"document_date": day(0), "valid_until": day(-1)}, http.StatusBadRequest)
	upload(t, "x.exe", "MZ\x90\x00 not a document", nil, http.StatusUnsupportedMediaType)
	upload(t, "risk-kopya.pdf", "%PDF-1.4\n% risk degerlendirme test\n", map[string]string{"category": "general"}, http.StatusConflict)

	all := list(t, "")
	if all.Counts.Total != before.Counts.Total+3 || all.Counts.ISG != before.Counts.ISG+2 ||
		all.Counts.ISGExpired != before.Counts.ISGExpired+1 || all.Counts.ISGExpiring != before.Counts.ISGExpiring+1 {
		t.Fatalf("counts = %+v, before %+v", all.Counts, before.Counts)
	}
	if p := list(t, "?q=ambar"); p.Total != 1 || p.Items[0].Category != "contract" {
		t.Fatalf("search = %+v", p)
	}
	if p := list(t, "?q=%25"); p.Total != 0 {
		t.Fatalf("a literal %% matched %d documents", p.Total)
	}
	isg := list(t, "?category=isg&expiring=true")
	if len(isg.Items) < 2 || isg.Items[0].UUID != expired.UUID {
		t.Fatalf("expiring İSG, soonest first = %+v", isg.Items)
	}
	for _, d := range isg.Items {
		if d.Category != "isg" || d.ExpiryStatus == "valid" || d.ExpiryStatus == "none" {
			t.Fatalf("expiring filter returned %+v", d)
		}
	}
	if p := list(t, "?isg_kind=risk_assessment"); p.Total < 1 || p.Items[0].ISGKind == nil || *p.Items[0].ISGKind != "risk_assessment" {
		t.Fatalf("isg_kind filter = %+v", p.Items)
	}
	if rec := e.do(t, http.MethodGet, base+"?category=nope", sweepCase{}, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown category: %d", rec.Code)
	}

	rec := e.do(t, http.MethodGet, base+"/"+soon.UUID+"/file", sweepCase{}, false)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.Contains(rec.Body.String(), "risk degerlendirme") || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("file: %d %v %q", rec.Code, rec.Header(), rec.Body)
	}

	// Başka mükellefin engagement'ı bu evrakı göremez.
	other := e.one(t, `SELECT uuid::text FROM engagements WHERE organization_id = $1 AND id <> $2 AND taxpayer_id <> $3 AND ended_at IS NULL ORDER BY id LIMIT 1`, "engagementUuid")
	obase := "/v1/tenant/engagements/" + other + "/archive-documents"
	if rec := e.do(t, http.MethodGet, obase+"/"+soon.UUID+"/file", sweepCase{}, false); rec.Code != http.StatusNotFound {
		t.Fatalf("other engagement file: %d", rec.Code)
	}
	if rec := e.do(t, http.MethodDelete, obase+"/"+soon.UUID, sweepCase{}, false); rec.Code != http.StatusNotFound {
		t.Fatalf("other engagement delete: %d", rec.Code)
	}

	if rec := e.do(t, http.MethodDelete, base+"/"+soon.UUID, sweepCase{}, false); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, http.MethodGet, base+"/"+soon.UUID+"/file", sweepCase{}, false); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted file: %d", rec.Code)
	}
}
