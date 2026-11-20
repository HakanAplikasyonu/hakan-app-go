package journals

import (
	"context"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// mainAccountNames are the Tek Düzen Hesap Planı main accounts most entries
// use. A line whose code is in no loaded plan shows its main account's name,
// so the screen always says which account is debited and which credited.
var mainAccountNames = map[string]string{
	"100": "Kasa", "101": "Alınan çekler", "102": "Bankalar", "103": "Verilen çekler ve ödeme emirleri",
	"108": "Diğer hazır değerler", "120": "Alıcılar", "121": "Alacak senetleri", "126": "Verilen depozito ve teminatlar",
	"131": "Ortaklardan alacaklar", "136": "Diğer çeşitli alacaklar", "150": "İlk madde ve malzeme",
	"153": "Ticari mallar", "159": "Verilen sipariş avansları", "180": "Gelecek aylara ait giderler",
	"190": "Devreden KDV", "191": "İndirilecek KDV", "193": "Peşin ödenen vergiler ve fonlar",
	"195": "İş avansları", "196": "Personel avansları", "252": "Binalar", "253": "Tesis, makine ve cihazlar",
	"254": "Taşıtlar", "255": "Demirbaşlar", "257": "Birikmiş amortismanlar", "260": "Haklar",
	"300": "Banka kredileri", "309": "Diğer mali borçlar", "320": "Satıcılar", "321": "Borç senetleri",
	"326": "Alınan depozito ve teminatlar", "331": "Ortaklara borçlar", "335": "Personele borçlar",
	"336": "Diğer çeşitli borçlar", "340": "Alınan sipariş avansları", "360": "Ödenecek vergi ve fonlar",
	"361": "Ödenecek sosyal güvenlik kesintileri", "368": "Vadesi geçmiş ertelenmiş veya taksitlendirilmiş vergi ve diğer yükümlülükler",
	"370": "Dönem kârı vergi ve diğer yasal yükümlülük karşılıkları", "371": "Dönem kârının peşin ödenen vergi ve diğer yükümlülükleri",
	"381": "Gider tahakkukları", "391": "Hesaplanan KDV", "400": "Banka kredileri (uzun vadeli)",
	"500": "Sermaye", "501": "Ödenmemiş sermaye", "570": "Geçmiş yıllar kârları", "580": "Geçmiş yıllar zararları",
	"590": "Dönem net kârı", "591": "Dönem net zararı", "600": "Yurt içi satışlar", "601": "Yurt dışı satışlar",
	"602": "Diğer gelirler", "610": "Satıştan iadeler", "611": "Satış iskontoları", "620": "Satılan mamuller maliyeti",
	"621": "Satılan ticari mallar maliyeti", "622": "Satılan hizmet maliyeti", "642": "Faiz gelirleri",
	"646": "Kambiyo kârları", "649": "Diğer olağan gelir ve kârlar", "656": "Kambiyo zararları",
	"659": "Diğer olağan gider ve zararlar", "689": "Diğer olağandışı gider ve zararlar",
	"710": "Direkt ilk madde ve malzeme giderleri", "720": "Direkt işçilik giderleri", "730": "Genel üretim giderleri",
	"740": "Hizmet üretim maliyeti", "750": "Araştırma ve geliştirme giderleri",
	"760": "Pazarlama, satış ve dağıtım giderleri", "770": "Genel yönetim giderleri", "780": "Finansman giderleri",
}

// mainAccountName is the TDHP name of code's main account ("" if unknown).
func mainAccountName(code string) string {
	code = strings.TrimSpace(code)
	if len(code) < 3 {
		return ""
	}
	return mainAccountNames[code[:3]]
}

// accountNames names the codes: the engagement's loaded account plan or the
// cari hafızası first, then the TDHP main account.
func (s *Service) accountNames(ctx context.Context, scope engagementctx.Scope, codes []string) (map[string]string, error) {
	out := make(map[string]string, len(codes))
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := s.q.ListJournalAccountNames(ctx, db.ListJournalAccountNamesParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Codes: codes,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Code] = r.Name
	}
	for _, c := range codes {
		if out[c] == "" {
			if n := mainAccountName(c); n != "" {
				out[c] = n
			}
		}
	}
	return out, nil
}
