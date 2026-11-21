package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Banka hareketleri listesi (#329): ekstre içe aktarılınca hareketler,
// özet ve süzgeçler. Erişim kuralları yetki matrisinde (authz_table_test).
func TestBankTransactionsList(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng

	rec := e.do(t, http.MethodPost, base+"/cash-boxes",
		sweepCase{body: `{"iban":"TR76 0001 0005 1978 6457 8413 26","name":"Ziraat TL"}`}, false)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("cash box: %d %s", rec.Code, rec.Body)
	}
	sta, err := os.ReadFile(filepath.Join(e.backend, "..", "e2e", "fixtures", "atlas-ziraat-2026-10.sta"))
	if err != nil {
		t.Fatal(err)
	}
	rec = e.do(t, http.MethodPost, base+"/bank-statements",
		sweepCase{file: "file", fileName: "atlas-ziraat-2026-10.sta", fileBody: string(sta)}, false)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}

	type page struct {
		Data struct {
			Items []struct {
				Direction   string `json:"direction"`
				Amount      string `json:"amount"`
				MatchStatus string `json:"match_status"`
				Period      string `json:"period"`
				CashBoxName string `json:"cash_box_name"`
			} `json:"items"`
			Summary struct {
				Total     int64  `json:"total"`
				InTotal   string `json:"in_total"`
				OutTotal  string `json:"out_total"`
				Unmatched int64  `json:"unmatched"`
			} `json:"summary"`
		} `json:"data"`
	}
	get := func(query string) page {
		t.Helper()
		rec := e.do(t, http.MethodGet, base+"/bank-transactions"+query, sweepCase{}, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", query, rec.Code, rec.Body)
		}
		var p page
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	all := get("?period=2026-10")
	if len(all.Data.Items) == 0 || all.Data.Summary.Total != int64(len(all.Data.Items)) {
		t.Fatalf("items %d, summary total %d", len(all.Data.Items), all.Data.Summary.Total)
	}
	var in, out int
	for _, it := range all.Data.Items {
		// "Ziraat TL vadesiz": the demo seed's own bank month (#383);
		// "Garanti TL vadesiz": the ledger difference account (#780 C).
		if it.Period != "2026-10" || (it.CashBoxName != "Ziraat TL" && it.CashBoxName != "Ziraat TL vadesiz" && it.CashBoxName != "Garanti TL vadesiz") {
			t.Fatalf("row: %+v", it)
		}
		switch it.Direction {
		case "in":
			in++
		case "out":
			out++
		default:
			t.Fatalf("direction %q", it.Direction)
		}
	}
	if in > 0 && all.Data.Summary.InTotal == "0.00" || out > 0 && all.Data.Summary.OutTotal == "0.00" {
		t.Fatalf("totals in=%s out=%s for %d in / %d out", all.Data.Summary.InTotal, all.Data.Summary.OutTotal, in, out)
	}

	// Eşleşme süzgeci listeyi daraltır, özeti değiştirmez.
	um := get("?period=2026-10&match_status=unmatched")
	if int64(len(um.Data.Items)) != all.Data.Summary.Unmatched || um.Data.Summary.Total != all.Data.Summary.Total {
		t.Fatalf("unmatched %d items, summary %+v", len(um.Data.Items), um.Data.Summary)
	}
	if other := get("?period=2026-09"); len(other.Data.Items) != 0 {
		t.Fatalf("2026-09: %d items", len(other.Data.Items))
	}
	if lim := get("?period=2026-10&limit=1"); len(lim.Data.Items) != 1 {
		t.Fatalf("limit=1: %d items", len(lim.Data.Items))
	}

	for _, bad := range []string{"?period=2026-13", "?match_status=x", "?limit=0", "?limit=201", "?cash_box_uuid=x", "?offset=-1"} {
		if rec := e.do(t, http.MethodGet, base+"/bank-transactions"+bad, sweepCase{}, false); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
	// Mükellef tarafı bu uca erişemez (ofis ucu).
	if rec := e.do(t, http.MethodGet, base+"/bank-transactions", sweepCase{}, true); rec.Code < 400 {
		t.Errorf("taxpayer token: %d", rec.Code)
	}
}
