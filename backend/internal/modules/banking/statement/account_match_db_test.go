package statement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// mt940For is a one-line MT940 statement of iban in currency.
func mt940For(iban, currency, ref string) []byte {
	return []byte(":20:" + ref + "\n:25:" + iban + "\n:60F:C261001" + currency + "100,00\n" +
		":61:2610011001C5,00NTRFNONREF//" + ref + "\n:86:FAST GELEN\n:62F:C261001" + currency + "105,00\n")
}

// Oğuz'un geri bildirimi: a statement finds its account by IBAN first, then
// by bank (and currency when the bank has more than one account); an
// account not on the list comes back with the "Listeye ekle" pre-fill.
func TestStatementAccountMatching(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	q := db.New(d.Pool)
	bank := banking.NewService(d.Pool, nil)
	im := NewImporter(d.Pool, bank)
	mk := func(p db.InsertCashBoxParams) db.CashBox {
		p.TaxpayerID, p.Type = tp, "bank_account"
		b, err := q.InsertCashBox(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	garanti := mk(db.InsertCashBoxParams{Name: "Garanti TL", Currency: "TRY", Iban: text(ibanGaranti), BankID: text("garanti")})
	akbankTRY := mk(db.InsertCashBoxParams{Name: "Akbank TL", Currency: "TRY", BankID: text("akbank")})
	akbankUSD := mk(db.InsertCashBoxParams{Name: "Akbank Dolar", Currency: "USD", BankID: text("akbank")})

	// IBAN match.
	rep, err := im.Import(ctx, Request{TaxpayerID: tp, FileName: "g.sta", Data: mt940For(ibanGaranti, "TRY", "G1")})
	if err != nil || len(rep.Accounts) != 1 || rep.Accounts[0].CashBoxUUID != garanti.Uuid {
		t.Fatalf("iban match: %+v %v", rep.Accounts, err)
	}

	// Bank-only match: the Akbank IBAN is not on the list, but the taxpayer
	// keeps "Akbank TRY" and "Akbank USD" without an IBAN; the currency picks.
	for cur, want := range map[string]db.CashBox{"TRY": akbankTRY, "USD": akbankUSD} {
		rep, err = im.Import(ctx, Request{TaxpayerID: tp, FileName: "a.sta", Data: mt940For(ibanAkbank, cur, "A"+cur), Preview: true})
		if err != nil || len(rep.Accounts) != 1 || rep.Accounts[0].CashBoxUUID != want.Uuid {
			t.Fatalf("bank match %s: %+v %v", cur, rep.Accounts, err)
		}
	}
	// No account of that bank in EUR: not guessed.
	var ue *UnknownAccountError
	if _, err = im.Import(ctx, Request{TaxpayerID: tp, FileName: "a.sta", Data: mt940For(ibanAkbank, "EUR", "AE"), Preview: true}); !errors.As(err, &ue) {
		t.Fatalf("bank match, other currency: %v", err)
	}

	// Unknown account: the suggestion carries bank, IBAN and currency.
	_, err = im.Import(ctx, Request{TaxpayerID: tp, FileName: "z.sta", Data: mt940For(ibanZiraat, "TRY", "Z1"), Preview: true})
	if !errors.As(err, &ue) || ue.Suggestion == nil ||
		*ue.Suggestion != (AccountSuggestion{BankID: "ziraat", IBAN: ibanZiraat, Currency: "TRY"}) {
		t.Fatalf("unknown account: %v %+v", err, ue)
	}
	// The answer carries it as ADD_ACCOUNT details only when the office has
	// the follow-up on.
	for _, on := range []bool{true, false} {
		w := httptest.NewRecorder()
		writeError(w, httptest.NewRequest("POST", "/x", nil), err, on)
		var out struct {
			Error struct {
				Code    string `json:"code"`
				Details []struct {
					Field   string `json:"field"`
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		got := map[string]string{}
		for _, dt := range out.Error.Details {
			if dt.Code == "ADD_ACCOUNT" {
				got[dt.Field] = dt.Message
			}
		}
		if out.Error.Code != "UNKNOWN_BANK_ACCOUNT" || (on && (got["iban"] != ibanZiraat || got["bank_id"] != "ziraat" || got["currency"] != "TRY")) ||
			(!on && len(got) != 0) {
			t.Fatalf("answer (on=%v): %d %s", on, w.Code, w.Body.String())
		}
	}

	// A closed account is not picked by bank.
	if _, err := q.UpdateCashBox(ctx, db.UpdateCashBoxParams{
		TaxpayerID: tp, Uuid: akbankUSD.Uuid, Name: akbankUSD.Name, BankID: akbankUSD.BankID, Currency: akbankUSD.Currency,
		DeactivatedAt: pgtype.Timestamptz{Valid: true, Time: akbankUSD.CreatedAt.Time},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = im.Import(ctx, Request{TaxpayerID: tp, FileName: "a.sta", Data: mt940For(ibanAkbank, "USD", "AU2"), Preview: true}); !errors.As(err, &ue) {
		t.Fatalf("closed account picked: %v", err)
	}
}
