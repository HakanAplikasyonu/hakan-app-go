package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWhatsAppBankStatement (#780): "Banka ekstresi olarak kaydet" sends a
// CSV statement through the extension's capture endpoint into the panel's
// statement import (same parser, same account matching): the picked bank
// account gets the movements, the same file again imports nothing, a file
// whose columns are unknown answers STATEMENT_LAYOUT_UNKNOWN and a photo is
// refused.
func TestWhatsAppBankStatement(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	base := "/v1/tenant/engagements/" + eng

	rec := e.do(t, http.MethodPost, base+"/cash-boxes",
		sweepCase{body: `{"bank_account_number":"77001234","name":"WhatsApp ekstre TL"}`}, false)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("cash box: %d %s", rec.Code, rec.Body)
	}
	var box struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &box)
	if box.Data.UUID == "" {
		t.Fatalf("cash box: %s", rec.Body)
	}
	csv, err := os.ReadFile(filepath.Join(e.backend, "..", "e2e", "fixtures", "whatsapp-ekstre.csv"))
	if err != nil {
		t.Fatal(err)
	}
	save := func(fileName, body string, fields map[string]string) (int, capture) {
		t.Helper()
		rec := e.do(t, http.MethodPost, base+"/whatsapp/captures",
			sweepCase{file: "file", fileName: fileName, fileBody: body, fields: fields}, false)
		var out struct {
			Data  capture `json:"data"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		out.Data.code = out.Error.Code
		return rec.Code, out.Data
	}
	fields := map[string]string{"target": "bank_statement", "chat_key": "905551112233@c.us", "cash_box_uuid": box.Data.UUID}

	code, got := save("ekstre-kasim.csv", string(csv), fields)
	if code != http.StatusCreated || got.Target != "bank_statement" || got.BankStatement == nil ||
		got.BankStatement.Inserted != 3 || got.BankStatement.Lines != 3 || got.BankStatement.Format != "csv" || got.Replayed {
		t.Fatalf("first save: %d %+v %+v", code, got, got.BankStatement)
	}
	if got.Taxpayer.Title == "" || got.BankStatement.ImportID == "" {
		t.Fatalf("taxpayer / import id missing: %+v", got)
	}
	// The movements are on the picked account, like a panel import.
	rec = e.do(t, http.MethodGet, base+"/bank-transactions?limit=50", sweepCase{}, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "WhatsApp ekstre TL") {
		t.Fatalf("transactions: %d %s", rec.Code, rec.Body)
	}

	// The same file again: nothing new.
	code, again := save("ekstre-kasim.csv", string(csv), fields)
	if code != http.StatusCreated || again.BankStatement == nil || again.BankStatement.Inserted != 0 || !again.Replayed {
		t.Fatalf("second save: %d %+v %+v", code, again, again.BankStatement)
	}

	// Unknown columns: imported from the panel instead.
	if code, bad := save("baska.csv", "Gün;Metin;Para\n01.11.2026;X;5,00\n", fields); code != http.StatusUnprocessableEntity || bad.code != "STATEMENT_LAYOUT_UNKNOWN" {
		t.Fatalf("unknown layout: %d %q", code, bad.code)
	}
	// A photo is not a statement.
	if code, _ := save("ekstre.png", pngOf(0xe1), fields); code != http.StatusBadRequest {
		t.Fatalf("photo: %d", code)
	}

	// The import can be undone from the panel like any other.
	rec = e.do(t, http.MethodDelete, base+"/bank-statements/"+got.BankStatement.ImportID, sweepCase{}, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted":3`) {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body)
	}
}

type capture struct {
	Target   string `json:"target"`
	Replayed bool   `json:"replayed"`
	Taxpayer struct {
		Title string `json:"title"`
	} `json:"taxpayer"`
	BankStatement *struct {
		ImportID string `json:"import_id"`
		Format   string `json:"format"`
		Lines    int    `json:"lines"`
		Inserted int    `json:"inserted"`
	} `json:"bank_statement"`
	code string
}
