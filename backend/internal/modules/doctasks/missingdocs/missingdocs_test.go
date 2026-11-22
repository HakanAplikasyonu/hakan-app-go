package missingdocs

import "testing"

func TestTitle(t *testing.T) {
	cases := []struct{ got, want string }{
		{Title(KindBankStatement, "2026-10", "Ziraat"), "Ekim 2026 Ziraat banka ekstresi"},
		{Title(KindZReports, "2026-10", ""), "Ekim 2026 Z raporları"},
		{Title(KindExpenses, "2026-01", ""), "Ocak 2026 masraf fişleri"},
		{Title(KindPurchaseInvoices, "2026-12", ""), "Aralık 2026 alış faturaları"},
		{PeriodText("bad"), "bad"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestBuild(t *testing.T) {
	items := Build("2026-10", []string{"Ziraat"}, true, false, true, []string{"Ekim 2026 Z raporları"})
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Kind != KindBankStatement || items[0].Requested {
		t.Errorf("statement = %+v", items[0])
	}
	if items[1].Kind != KindZReports || !items[1].Requested {
		t.Errorf("z = %+v", items[1])
	}
	if items[2].Kind != KindPurchaseInvoices || items[2].Requested {
		t.Errorf("purchase = %+v", items[2])
	}
	if !ValidKind(KindExpenses) || ValidKind("x") {
		t.Error("ValidKind")
	}
}
