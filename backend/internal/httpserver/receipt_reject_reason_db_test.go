package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Fiş ret gerekçesi (#780): mükellef fişi uygulamadan gönderir; ofis
// reddederken gerekçe seçer. "diger" açıklamasız 400 alır, bilinmeyen kod
// 400 alır; gerekçe denetim kaydına düşer ve mükellef Gönderdiklerim'de görür.
func TestReceiptRejectReason(t *testing.T) {
	e := newAuthzEnv(t)
	rec := e.do(t, http.MethodPost, "/v1/taxpayer/receipts", sweepCase{file: "image", fileName: "fis.png", fileBody: pngOf(0xe4)}, true)
	// #780: the receipt is accepted at once (202 "processing") and read in
	// the background.
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	e.waitReceiptReads(t)
	var up struct {
		Data struct {
			UUID   string `json:"uuid"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil || up.Data.UUID == "" {
		t.Fatalf("upload body: %s", rec.Body)
	}
	path := "/v1/tenant/engagements/" + e.param(t, "engagementUuid", "") + "/receipts/" + up.Data.UUID + "/reject"

	for _, body := range []string{`{"reason_code":"diger"}`, `{"reason_code":"diger","reason_text":"   "}`, `{"reason_code":"bilinmeyen"}`,
		`{"reason_code":"okunaksiz","reason_text":"` + strings.Repeat("a", 501) + `"}`, `{"gerekce":"x"}`} {
		if rec := e.do(t, http.MethodPost, path, sweepCase{body: body}, false); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if n := e.one(t, `SELECT count(*)::text FROM receipt_captures WHERE uuid = '`+up.Data.UUID+`' AND status = 'rejected' AND $1 > 0 AND $2 > 0 AND $3 > 0`, "not yet"); n != "0" {
		t.Fatalf("an invalid reason rejected the receipt")
	}

	rec = e.do(t, http.MethodPost, path, sweepCase{body: `{"reason_code":"kisisel_harcama","reason_text":"Market alışverişi"}`}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body)
	}
	if n := e.one(t, `SELECT count(*)::text FROM activity_events WHERE organization_id = $1 AND action = 'receipt.rejected'
		AND resource = 'receipt' AND resource_uuid = '`+up.Data.UUID+`'
		AND payload->>'reason_code' = 'kisisel_harcama' AND payload->>'reason_text' = 'Market alışverişi'
		AND payload->>'engagement_uuid' = (SELECT uuid::text FROM engagements WHERE id = $2) AND $3 > 0`, "audit"); n != "1" {
		t.Fatalf("audit rows = %s", n)
	}

	rec = e.do(t, http.MethodGet, "/v1/taxpayer/submissions", sweepCase{}, true)
	var list struct {
		Data struct {
			Items []struct {
				UUID       string `json:"uuid"`
				Status     string `json:"status"`
				Reason     string `json:"reason"`
				ReasonCode string `json:"reason_code"`
			} `json:"items"`
		} `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("submissions: %d %s", rec.Code, rec.Body)
	}
	found := false
	for _, it := range list.Data.Items {
		if it.UUID == up.Data.UUID {
			found = true
			if it.Status != "rejected" || it.ReasonCode != "kisisel_harcama" || it.Reason != "İşletme gideri değil: Market alışverişi" {
				t.Fatalf("submission = %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("receipt not in submissions: %s", rec.Body)
	}
}
