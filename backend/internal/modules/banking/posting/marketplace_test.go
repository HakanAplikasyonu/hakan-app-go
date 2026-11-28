package posting

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// A made-up plan: two channels have a cari, Amazon has none.
var marketplacePlan = []engine.PlanAccount{
	{Code: "120", Name: "ALICILAR"},
	{Code: "120.01", Name: "YURT İÇİ ALICILAR"},
	{Code: "120.01.001", Name: "ABC GIDA SAN. TİC. A.Ş.", IsLeaf: true},
	{Code: "120.01.002", Name: "DSM GRUP DANIŞMANLIK İLETİŞİM VE SATIŞ TİC. A.Ş.", IsLeaf: true},
	{Code: "120.01.003", Name: "Hepsiburada", IsLeaf: true},
	{Code: "320.01.001", Name: "TRENDYOL", IsLeaf: true}, // not under 120
}

func TestMarketplaceSuggestion(t *testing.T) {
	for _, c := range []struct {
		desc, code, source, reason string
		missing                    bool
	}{
		{"EFT GELEN TRENDYOL HAKEDIS", "120.01.002", SourceCounterpartyAccount,
			"Açıklamada 'TRENDYOL' geçiyor → Trendyol carisi önerildi: 120.01.002 DSM GRUP DANIŞMANLIK İLETİŞİM VE SATIŞ TİC. A.Ş.", false},
		{"d-market elektronik odeme", "120.01.003", SourceCounterpartyAccount,
			"Açıklamada 'D-MARKET' geçiyor → Hepsiburada carisi önerildi: 120.01.003 Hepsiburada", false},
		{"TEMLİK DSM GRUP ALACAK", "120.01.002", SourceCounterpartyAccount,
			"Açıklamada 'TEMLİK' ve 'DSM GRUP' geçiyor (temlik) → Trendyol carisi önerildi: 120.01.002 DSM GRUP DANIŞMANLIK İLETİŞİM VE SATIŞ TİC. A.Ş.", false},
		{"AMAZON TURKEY ODEME", "120", SourceKindDefault,
			"Açıklamada 'AMAZON' geçiyor → Amazon; pazaryeri carisi tanımlı değil, 120 altında Amazon alt hesabı açılmalı", true},
	} {
		hit, ok := banking.DetectMarketplace(c.desc)
		if !ok {
			t.Fatalf("%q: no channel", c.desc)
		}
		s := MarketplaceSuggestion(hit, "120", marketplacePlan)
		if s.AccountCode != c.code || s.Source != c.source || s.Reason != c.reason || s.Marketplace != hit.Marketplace.Key {
			t.Errorf("%q: got %s %s %q %s", c.desc, s.AccountCode, s.Source, s.Reason, s.Marketplace)
		}
		if c.missing != s.Confidence.Equal(marketplaceMissing) {
			t.Errorf("%q: confidence %s", c.desc, s.Confidence)
		}
		n := marketplaceNote(s.Source, s.Reason, c.desc, s.AccountCode)
		if n == nil || n.Channel != hit.Marketplace.Key || n.Keyword != hit.Keyword || n.AccountMissing != c.missing || n.Temlik != hit.Temlik {
			t.Errorf("%q: note %+v", c.desc, n)
		}
	}
}

// Other suggestions carry no marketplace note, even when the description
// names a channel (the office's alias won).
func TestMarketplaceNoteOnlyForMarketplaceReasons(t *testing.T) {
	for _, c := range []struct{ source, reason, desc string }{
		{SourceAlias, "Takma ad (ad): TRENDYOL → 120.01.009", "TRENDYOL HAKEDIS"},
		{SourceManual, "Müşavir hesabı elle seçti: 120.01.009", "TRENDYOL HAKEDIS"},
		{SourceKindDefault, "İşlem türü (müşteri tahsilatı): 120 Alıcılar", "TRENDYOL HAKEDIS"},
		{SourceCounterpartyAccount, "Açıklamada 'X' geçiyor", "ABC GIDA ODEME"},
	} {
		if n := marketplaceNote(c.source, c.reason, c.desc, "120.01.009"); n != nil {
			t.Errorf("%s %q: note %+v", c.source, c.reason, n)
		}
	}
}
