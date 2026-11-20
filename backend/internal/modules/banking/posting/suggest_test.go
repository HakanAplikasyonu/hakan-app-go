package posting

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

func TestSameAccount(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"102", "102", true}, {"102", "102.01", true}, {"102.01", " 102 ", true}, {"102.01", "102.01.001", true},
		{"102.01", "102.02", false}, {"102", "1020", false}, {"120.01", "102", false}, {"", "102", false},
	} {
		if got := SameAccount(c.a, c.b); got != c.want {
			t.Errorf("SameAccount(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// A virman or an FX trade has no default counter account: a bare "102"
// would book the bank against itself (102 borç / 102 alacak).
func TestNoBankDefaultForTransfers(t *testing.T) {
	for _, d := range []banking.Detail{banking.DetailOwnTransfer, banking.DetailFXTrade} {
		if k, ok := kindDefaults[d]; ok {
			t.Errorf("%s defaults to %s", d, k.code)
		}
	}
	for d, k := range kindDefaults {
		if SameAccount(k.code, DefaultBankAccount) {
			t.Errorf("%s defaults to the bank account %s", d, k.code)
		}
	}
	if SameAccount(cardPayment.code, DefaultBankAccount) {
		t.Errorf("card payment defaults to %s", cardPayment.code)
	}
}
