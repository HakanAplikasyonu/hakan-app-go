package caritrigger

import "testing"

func TestMainCodeAndTaxID(t *testing.T) {
	if MainCode("incoming") != "320" || MainCode("outgoing") != "120" || MainCode("") != "" {
		t.Fatal("main code")
	}
	cases := []struct {
		c    Counterparty
		want string
	}{
		{Counterparty{TaxNumber: "1234567890"}, "1234567890"},
		{Counterparty{TaxNumber: " ", IdentityNumber: "12345678901"}, "12345678901"},
		{Counterparty{TaxNumber: "12AB"}, ""},
		{Counterparty{}, ""},
	}
	for _, c := range cases {
		if got := TaxID(c.c); got != c.want {
			t.Errorf("TaxID(%+v) = %q, want %q", c.c, got, c.want)
		}
	}
}
