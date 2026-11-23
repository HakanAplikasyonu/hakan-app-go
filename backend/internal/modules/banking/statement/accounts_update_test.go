package statement

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

const ibanGaranti = "TR660006200000000001002222"

func TestCashBoxInputBank(t *testing.T) {
	p, err := CashBoxInput{BankID: "garanti", IBAN: ibanGaranti, Currency: "EUR"}.Params(1)
	if err != nil || p.BankID.String != "garanti" || p.Currency != "EUR" {
		t.Fatalf("garanti: %+v %v", p, err)
	}
	// The picked bank without an IBAN: account number only.
	p, err = CashBoxInput{BankID: "akbank", BankAccountNumber: "998877"}.Params(1)
	if err != nil || p.BankID.String != "akbank" {
		t.Fatalf("akbank: %+v %v", p, err)
	}
	var ie *InvalidCashBoxError
	if _, err := (CashBoxInput{BankID: "isbank", IBAN: ibanGaranti}).Params(1); !errors.As(err, &ie) || ie.Field != "iban" {
		t.Errorf("iban of another bank: %v", err)
	}
	if _, err := (CashBoxInput{BankID: "nobank", IBAN: ibanGaranti}).Params(1); !errors.As(err, &ie) || ie.Field != "bank_id" {
		t.Errorf("unknown bank: %v", err)
	}
}

func TestCashBoxPatchApply(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	box := db.CashBox{
		TaxpayerID: 3, Type: "bank_account", Name: "Ana hesap", Currency: "TRY",
		Iban: pgtype.Text{String: ibanZiraat, Valid: true}, BankID: pgtype.Text{String: "ziraat", Valid: true},
	}
	str := func(s string) *string { return &s }
	no := false
	p, err := CashBoxPatch{Name: str(" Yeni ad "), BankBranchCode: str("0456"), Active: &no}.Apply(box, now)
	if err != nil || p.Name != "Yeni ad" || p.BankBranchCode.String != "0456" || !p.DeactivatedAt.Time.Equal(now) ||
		p.Iban.String != ibanZiraat || p.BankID.String != "ziraat" {
		t.Fatalf("rename + close: %+v %v", p, err)
	}
	box.DeactivatedAt = p.DeactivatedAt
	yes := true
	if p, err = (CashBoxPatch{Active: &yes}).Apply(box, now); err != nil || p.DeactivatedAt.Valid {
		t.Fatalf("reopen: %+v %v", p, err)
	}
	// A new IBAN moves the bank with it.
	if p, err = (CashBoxPatch{IBAN: str(ibanGaranti)}).Apply(box, now); err != nil || p.BankID.String != "garanti" {
		t.Fatalf("new iban: %+v %v", p, err)
	}
	bad := map[string]CashBoxPatch{
		"name":     {Name: str(" ")},
		"iban":     {IBAN: str("TR760001000519786457841327")},
		"bank_id":  {BankID: str("nobank")},
		"currency": {Currency: str("TL1")},
	}
	for field, in := range bad {
		var ie *InvalidCashBoxError
		if _, err := in.Apply(box, now); !errors.As(err, &ie) || ie.Field != field {
			t.Errorf("%s: %v", field, err)
		}
	}
	var ie *InvalidCashBoxError
	if _, err := (CashBoxPatch{IBAN: str("")}).Apply(box, now); !errors.As(err, &ie) || ie.Field != "iban" {
		t.Errorf("clearing the only identity: %v", err)
	}
	linked := box
	linked.BankConnectionID = pgtype.Int8{Int64: 9, Valid: true}
	if _, err := (CashBoxPatch{IBAN: str(ibanGaranti)}).Apply(linked, now); !errors.As(err, &ie) || ie.Field != "iban" {
		t.Errorf("web service account iban change: %v", err)
	}
}
