package statement

import (
	"errors"
	"testing"
)

func TestCashBoxInputParams(t *testing.T) {
	p, err := CashBoxInput{IBAN: " tr76 0001 0005 1978 6457 8413 26 "}.Params(7)
	if err != nil {
		t.Fatal(err)
	}
	if p.TaxpayerID != 7 || p.Type != "bank_account" || p.Currency != "TRY" ||
		p.Iban.String != ibanZiraat || p.BankID.String != "ziraat" || p.Name != "Banka TRY ···1326" {
		t.Fatalf("defaults: %+v", p)
	}

	p, err = CashBoxInput{Type: "cash", Currency: "usd", Name: " Merkez kasa "}.Params(1)
	if err != nil || p.Type != "cash" || p.Currency != "USD" || p.Name != "Merkez kasa" || p.Iban.Valid {
		t.Fatalf("cash: %+v %v", p, err)
	}

	p, err = CashBoxInput{BankAccountNumber: "12345678", BankBranchCode: "0123"}.Params(1)
	if err != nil || p.Iban.Valid || p.BankAccountNumber.String != "12345678" || p.BankBranchCode.String != "0123" {
		t.Fatalf("account number: %+v %v", p, err)
	}

	bad := map[string]CashBoxInput{
		"type":     {Type: "wallet", IBAN: ibanZiraat},
		"iban":     {IBAN: "TR760001000519786457841327"}, // check digit off
		"currency": {IBAN: ibanZiraat, Currency: "TL1"},
	}
	for field, in := range bad {
		var ie *InvalidCashBoxError
		if _, err := in.Params(1); !errors.As(err, &ie) || ie.Field != field {
			t.Errorf("%s: %v", field, err)
		}
	}
	var ie *InvalidCashBoxError
	if _, err := (CashBoxInput{}).Params(1); !errors.As(err, &ie) || ie.Field != "iban" {
		t.Errorf("bank account without iban or number: %v", err)
	}
}
