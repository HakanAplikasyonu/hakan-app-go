package statement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// A fresh taxpayer has no account: the file is refused until the office adds
// the IBAN through the cash box endpoint, then the same file imports (#144).
func TestCashBoxEndpointsUnblockImport(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	other := d.Taxpayer(t, "legal", "1234567890")
	q := db.New(d.Pool)
	bank := banking.NewService(d.Pool, nil)
	h := NewHandler(NewImporter(d.Pool, bank), bank, q)
	scope := engagementctx.Scope{TaxpayerID: tp}

	call := func(fn http.HandlerFunc, method, body string, s engagementctx.Scope, pathID string) (int, map[string]any) {
		r := httptest.NewRequest(method, "/x", strings.NewReader(body))
		if pathID != "" {
			r.SetPathValue("cashBoxUuid", pathID)
		}
		r = r.WithContext(engagementctx.WithScope(r.Context(), s))
		w := httptest.NewRecorder()
		fn(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	text := ":20:X\n:25:" + ibanZiraat + "\n:60F:C261001TRY0,00\n:61:2610011001C5,00NTRFNONREF\n:86:X\n:62F:C261001TRY5,00\n"
	req := Request{TaxpayerID: tp, FileName: "x.sta", Data: []byte(text)}
	var ue *UnknownAccountError
	if _, err := h.im.Import(ctx, req); !errors.As(err, &ue) {
		t.Fatalf("before: %v", err)
	}

	code, out := call(h.createCashBox, http.MethodPost, `{"iban":"`+ibanZiraat+`","name":"Ziraat TL"}`, scope, "")
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	if data["bank_id"] != "ziraat" || data["web_service"] != false {
		t.Fatalf("created: %v", data)
	}
	id := data["uuid"].(string)

	if code, out := call(h.createCashBox, http.MethodPost, `{"iban":"`+ibanZiraat+`"}`, scope, ""); code != http.StatusConflict {
		t.Fatalf("duplicate: %d %v", code, out)
	}
	if code, _ := call(h.createCashBox, http.MethodPost, `{"iban":"TR00"}`, scope, ""); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad iban: %d", code)
	}
	if code, _ := call(h.createCashBox, http.MethodPost, `{"iban":"`+ibanZiraat+`","extra":1}`, scope, ""); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", code)
	}

	rep, err := h.im.Import(ctx, req)
	if err != nil || len(rep.Accounts) != 1 || rep.Accounts[0].Inserted != 1 {
		t.Fatalf("after: %+v %v", rep, err)
	}

	if code, out := call(h.updateCashBox, http.MethodPatch, `{"name":"Ana hesap"}`, scope, id); code != http.StatusOK || out["data"].(map[string]any)["name"] != "Ana hesap" {
		t.Fatalf("rename: %d %v", code, out)
	}
	// #780: the accountant closes the account; it stays listed as passive.
	if code, out := call(h.updateCashBox, http.MethodPatch, `{"active":false,"bank_branch_code":"0456"}`, scope, id); code != http.StatusOK ||
		out["data"].(map[string]any)["active"] != false || out["data"].(map[string]any)["bank_branch_code"] != "0456" {
		t.Fatalf("deactivate: %d %v", code, out)
	}
	if code, _ := call(h.updateCashBox, http.MethodPatch, `{"bank_id":"garanti"}`, scope, id); code != http.StatusUnprocessableEntity {
		t.Fatalf("bank of another iban: %d", code)
	}
	if code, out := call(h.updateCashBox, http.MethodPatch, `{"active":true}`, scope, id); code != http.StatusOK || out["data"].(map[string]any)["active"] != true {
		t.Fatalf("reopen: %d %v", code, out)
	}

	// Another taxpayer neither sees nor renames it.
	otherScope := engagementctx.Scope{TaxpayerID: other}
	if code, out := call(h.listCashBoxes, http.MethodGet, "", otherScope, ""); code != http.StatusOK || len(out["data"].(map[string]any)["items"].([]any)) != 0 {
		t.Fatalf("other list: %d %v", code, out)
	}
	if code, _ := call(h.updateCashBox, http.MethodPatch, `{"name":"x"}`, otherScope, id); code != http.StatusNotFound {
		t.Fatalf("other rename: %d", code)
	}
	if code, _ := call(h.updateCashBox, http.MethodPatch, `{"name":"x"}`, scope, uuid.NewString()); code != http.StatusNotFound {
		t.Fatalf("missing rename: %d", code)
	}
	if code, out := call(h.listCashBoxes, http.MethodGet, "", scope, ""); code != http.StatusOK || len(out["data"].(map[string]any)["items"].([]any)) != 1 {
		t.Fatalf("list: %d %v", code, out)
	}
}
