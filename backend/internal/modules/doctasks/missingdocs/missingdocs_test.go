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
	items := Build("2026-10", []Statement{{Name: "Ziraat"}}, true, false, true, []string{"Ekim 2026 Z raporları"})
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

// Eksik ekstre takibinin açtığı istek ("Akbank TRY hesabı – Ekim 2026
// ekstresi") ay başı belge toplamada da "istendi" sayılır; aynı ekstre iki
// başlıkla iki kez istenmez.
func TestBuildRecognisesAccountTitle(t *testing.T) {
	acct := AccountTitle("akbank", "TRY", "", "Akbank", "2026-10", false)
	if acct != "Akbank TRY hesabı – Ekim 2026 ekstresi" {
		t.Fatalf("account title %q", acct)
	}
	items := Build("2026-10", []Statement{{Name: "Akbank", AccountTitle: acct}, {Name: "Ziraat"}}, false, false, false,
		[]string{"akbank try hesabı – ekim 2026 ekstresi"})
	if len(items) != 2 || !items[0].Requested || !items[0].AccountRequested || items[0].Title != "Ekim 2026 Akbank banka ekstresi" {
		t.Fatalf("akbank: %+v", items[0])
	}
	if items[1].Requested || items[1].AccountRequested {
		t.Fatalf("ziraat: %+v", items[1])
	}
	// Eski başlıkla açık istek AccountRequested değildir.
	items = Build("2026-10", []Statement{{Name: "Akbank", AccountTitle: acct}}, false, false, false,
		[]string{"Ekim 2026 Akbank banka ekstresi"})
	if !items[0].Requested || items[0].AccountRequested {
		t.Fatalf("old title: %+v", items[0])
	}
	// Aynı banka ve para biriminde iki hesap: IBAN'ın son dört hanesi.
	if got := AccountTitle("akbank", "TRY", "TR000000000000000000001234", "Akbank 2", "2026-10", true); got != "Akbank TRY hesabı (···1234) – Ekim 2026 ekstresi" {
		t.Fatalf("same kind %q", got)
	}
}
