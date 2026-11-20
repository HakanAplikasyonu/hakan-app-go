package demo

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Büyük demo (sunum): birkaç ofis, ofis başına onlarca mükellef ve geriye
// dönük en az 24 aylık veri. Buradaki her kimlik uydurmadır: VKN / TCKN'ler
// sağlama algoritmasını geçer ama sabit önekli test aralığındadır (gerçek bir
// mükellefe ait değildir), e-postalar .test / example.com, telefonlar
// 0555 000 xx xx biçimindedir.
//
// Üretim deterministiktir: aynı tohum ve aynı çapa ay (anchor) her çalışmada
// aynı veriyi verir. Her mükellefin rastgele akışı kendi (ofis, sıra)
// çiftinden türetilir; ölçek değişince var olan mükelleflerin verisi değişmez.

// BigSeed is the fixed random seed of the big demo.
const BigSeed uint64 = 20261118

// Tax id prefixes of the big demo. dropDemo removes a taxpayer only when its
// id has one of them AND it is engaged with demo offices only.
const (
	// BigVKNPrefix + ofis (1 hane) + sıra (6 hane) + sağlama = 10 hane.
	BigVKNPrefix = "93"
	// BigTCKNPrefix + ofis (1 hane) + sıra (4 hane) + 2 sağlama = 11 hane.
	BigTCKNPrefix = "1000"
	// BigCounterpartyPrefix: karşı taraf (müşteri / tedarikçi) VKN'leri.
	BigCounterpartyPrefix = "94"
)

// BigOffice is one office (tenant) of the big demo. The first one is the
// regular demo office (OrgSlug): the big mode adds taxpayers next to the
// Atlas / Selin story, the others are new offices.
type BigOffice struct {
	Slug, Name string
	City       string
	// Members: owner first, then staff. TaxpayerUser signs in to /m as the
	// owner of the office's first big taxpayer.
	Members      []Member
	TaxpayerUser Member
}

// BigOffices lists every office the big mode can build; the scale picks the
// first N.
var BigOffices = []BigOffice{
	{
		Slug: OrgSlug, Name: OrgName, City: "İstanbul",
		Members:      Members,
		TaxpayerUser: Member{Email: "mukellef2@" + EmailDomain, Name: "Hande", Surname: "Kurt", Title: "Mükellef yetkilisi"},
	},
	{
		Slug: "demo-baskent", Name: "Başkent Mali Müşavirlik (demo)", City: "Ankara",
		Members: []Member{
			{Email: "sahip@baskent.demo.test", Name: "Levent", Surname: "Arslan", RoleKey: "owner", Title: "Sahip (SMMM)"},
			{Email: "musavir@baskent.demo.test", Name: "Gülşen", Surname: "Aydın", RoleKey: "admin", Title: "Müşavir"},
			{Email: "personel@baskent.demo.test", Name: "Emre", Surname: "Koç", RoleKey: "member", Title: "Muhasebe personeli"},
		},
		TaxpayerUser: Member{Email: "mukellef@baskent.demo.test", Name: "Serkan", Surname: "Uçar", Title: "Mükellef yetkilisi"},
	},
	{
		Slug: "demo-ege", Name: "Ege Mali Müşavirlik (demo)", City: "İzmir",
		Members: []Member{
			{Email: "sahip@ege.demo.test", Name: "Nazlı", Surname: "Özkan", RoleKey: "owner", Title: "Sahip (SMMM)"},
			{Email: "musavir@ege.demo.test", Name: "Tolga", Surname: "Erdem", RoleKey: "admin", Title: "Müşavir"},
			{Email: "personel@ege.demo.test", Name: "Ceren", Surname: "Yalçın", RoleKey: "member", Title: "Muhasebe personeli"},
		},
		TaxpayerUser: Member{Email: "mukellef@ege.demo.test", Name: "Barış", Surname: "Tunç", Title: "Mükellef yetkilisi"},
	},
}

// BigStaffPerOffice: office members (owner included) of every big-demo
// office. A real office of 30 taxpayers runs with about ten people; the
// listed members come first, the rest are generated.
const BigStaffPerOffice = 10

// bigStaffRoles: titles of the generated members, in order.
var bigStaffRoles = []struct{ role, title, local string }{
	{"admin", "Müşavir (SMMM)", "musavir"},
	{"member", "Muhasebe personeli", "personel"},
	{"member", "Muhasebe personeli", "personel"},
	{"member", "Bordro sorumlusu", "bordro"},
	{"member", "Muhasebe personeli", "personel"},
	{"member", "Stajyer", "stajyer"},
	{"member", "Stajyer", "stajyer"},
}

// Staff returns every member of office o of the big demo: the listed ones
// first (owner first), then generated colleagues up to BigStaffPerOffice.
// Generated e-mails share the owner's domain (personel2@…, stajyer3@…).
func (b BigOffice) Staff(o int) []Member {
	out := append([]Member{}, b.Members...)
	domain := b.Members[0].Email[strings.IndexByte(b.Members[0].Email, '@')+1:]
	used := map[string]bool{}
	for _, m := range out {
		used[m.Email] = true
	}
	r := Rand(o, 900, 0x57aff)
	for i := 0; len(out) < BigStaffPerOffice; i++ {
		def := bigStaffRoles[i%len(bigStaffRoles)]
		email := ""
		for n := 2; ; n++ {
			email = fmt.Sprintf("%s%d@%s", def.local, n, domain)
			if !used[email] {
				break
			}
		}
		used[email] = true
		out = append(out, Member{Email: email, Name: FirstNames[r.IntN(len(FirstNames))],
			Surname: LastNames[r.IntN(len(LastNames))], RoleKey: def.role, Title: def.title})
	}
	return out
}

// BigOfficeSlugs returns the slugs of every big-demo office (reset scope).
func BigOfficeSlugs() []string {
	out := make([]string, len(BigOffices))
	for i, o := range BigOffices {
		out[i] = o.Slug
	}
	return out
}

// Sector drives the amounts, VAT rates and seasonality of a taxpayer.
type Sector struct {
	Key, Name string
	// Words make up company titles ("Yıldız Yapı İnşaat Ltd. Şti.").
	Words []string
	// MonthlySales is the typical net monthly sales (TL) at the start of the
	// 24-month window; Growth is the yearly trend.
	MonthlySales float64
	Growth       float64
	// Season multiplies sales per calendar month (index 0 = Ocak).
	Season [12]float64
	// SalesRate is the main VAT rate of sales; Retail: Z reports instead of
	// most sales invoices; Ecommerce: e-arşiv sales to consumers.
	SalesRate  int
	Retail     bool
	Ecommerce  bool
	Withholder bool // tevkifatlı satış (inşaat, hizmet)
	Exporter   bool // istisna / döviz satış
	// Staff: employees on the payroll (range).
	StaffMin, StaffMax int
}

var flatSeason = [12]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

// Sectors of the big demo.
var Sectors = []Sector{
	{Key: "perakende", Name: "Perakende", Words: []string{"Market", "Gıda", "Şarküteri", "Kuruyemiş", "Züccaciye"},
		MonthlySales: 850000, Growth: 0.18, Season: [12]float64{0.9, 0.85, 0.95, 1, 1.05, 1.05, 1.1, 1.1, 1, 0.95, 1, 1.3},
		SalesRate: 1, Retail: true, StaffMin: 2, StaffMax: 6},
	{Key: "insaat", Name: "İnşaat", Words: []string{"Yapı", "İnşaat", "Taahhüt", "Proje", "Yapı Malzemeleri"},
		MonthlySales: 2600000, Growth: 0.22, Season: [12]float64{0.6, 0.65, 0.85, 1.05, 1.2, 1.25, 1.25, 1.2, 1.1, 1, 0.85, 0.7},
		SalesRate: 20, Withholder: true, StaffMin: 4, StaffMax: 12},
	{Key: "hizmet", Name: "Hizmet", Words: []string{"Danışmanlık", "Bilişim", "Temizlik Hizmetleri", "Organizasyon", "Lojistik"},
		MonthlySales: 620000, Growth: 0.25, Season: flatSeason,
		SalesRate: 20, Withholder: true, StaffMin: 1, StaffMax: 5},
	{Key: "eticaret", Name: "E-ticaret", Words: []string{"Online", "Elektronik", "Moda", "Ev Tekstili", "Kozmetik"},
		MonthlySales: 950000, Growth: 0.35, Season: [12]float64{0.85, 0.8, 0.9, 0.95, 1, 0.95, 0.9, 0.95, 1, 1.05, 1.45, 1.35},
		SalesRate: 20, Ecommerce: true, Exporter: true, StaffMin: 1, StaffMax: 4},
	{Key: "restoran", Name: "Restoran", Words: []string{"Lokanta", "Kebap", "Kafe", "Pide Salonu", "Balık Restoran"},
		MonthlySales: 680000, Growth: 0.2, Season: [12]float64{0.8, 0.8, 0.9, 1, 1.1, 1.2, 1.3, 1.3, 1.1, 1, 0.9, 0.95},
		SalesRate: 10, Retail: true, StaffMin: 3, StaffMax: 8},
}

// City is a Turkish province with districts (tax offices carry the
// district name, as most do).
type City struct {
	Name      string
	Districts []string
	Plate     string
}

// Cities by office: a taxpayer mostly sits in its office's city.
var Cities = map[string]City{
	"İstanbul": {Name: "İstanbul", Plate: "34", Districts: []string{"Kadıköy", "Beşiktaş", "Üsküdar", "Şişli", "Bakırköy", "Ataşehir", "Pendik", "Beylikdüzü"}},
	"Ankara":   {Name: "Ankara", Plate: "06", Districts: []string{"Çankaya", "Keçiören", "Yenimahalle", "Mamak", "Etimesgut", "Sincan", "Altındağ"}},
	"İzmir":    {Name: "İzmir", Plate: "35", Districts: []string{"Konak", "Karşıyaka", "Bornova", "Buca", "Çiğli", "Bayraklı", "Gaziemir"}},
	"Bursa":    {Name: "Bursa", Plate: "16", Districts: []string{"Osmangazi", "Nilüfer", "Yıldırım"}},
	"Antalya":  {Name: "Antalya", Plate: "07", Districts: []string{"Muratpaşa", "Konyaaltı", "Kepez"}},
	"Kocaeli":  {Name: "Kocaeli", Plate: "41", Districts: []string{"İzmit", "Gebze", "Darıca"}},
}

var otherCities = []string{"Bursa", "Antalya", "Kocaeli"}

// FirstNames / LastNames: common Turkish names; combinations are invented.
var (
	FirstNames = []string{"Ahmet", "Mehmet", "Mustafa", "Ali", "Hüseyin", "Hasan", "İbrahim", "Murat", "Ömer", "Yusuf",
		"Fatma", "Ayşe", "Emine", "Hatice", "Zeynep", "Elif", "Meryem", "Şerife", "Sultan", "Hacer",
		"Kemal", "Orhan", "Cem", "Selim", "Volkan", "Kerem", "Onur", "Berk", "Deniz", "Ece",
		"Gizem", "Pınar", "Sibel", "Esra", "Derya", "Burcu", "Ozan", "Uğur", "Tuncay", "Sevgi"}
	LastNames = []string{"Yılmaz", "Kaya", "Demir", "Şahin", "Çelik", "Yıldız", "Yıldırım", "Öztürk", "Aydın", "Özdemir",
		"Arslan", "Doğan", "Kılıç", "Aslan", "Çetin", "Kara", "Koç", "Kurt", "Özkan", "Şimşek",
		"Polat", "Korkmaz", "Erdoğan", "Güneş", "Bulut", "Tekin", "Avcı", "Taş", "Aksoy", "Uysal"}
	// titleWords open company titles ("Anadolu", "Yeşil", …).
	titleWords = []string{"Anadolu", "Yeşil", "Mavi", "Güneş", "Doruk", "Ergin", "Bereket", "Kuzey", "Marmara", "Akdeniz",
		"Ege", "Toros", "Kardelen", "Nehir", "Ufuk", "Zirve", "Bilge", "Asil", "Pera", "Lale",
		"Çınar", "Defne", "Ilgaz", "Kartal", "Sedir", "Umut", "Vadi", "Yakamoz", "Elvan", "Tuna"}
	// counterpartyWords build supplier / customer names.
	counterpartyWords = []string{"Tedarik", "Dağıtım", "Ambalaj", "Kırtasiye", "Enerji", "Lojistik", "Gıda Toptan", "Metal",
		"Kimya", "Tekstil", "Yazılım", "Reklam", "Mobilya", "Elektrik", "Hırdavat", "Otomotiv", "Matbaa", "Su Ürünleri"}
)

// BigTaxpayer is one generated taxpayer.
type BigTaxpayer struct {
	Office, Index int
	Kind          string // legal / individual
	Legal         string // "Ltd. Şti.", "A.Ş." or "" (şahıs)
	TaxID         string
	Title         string
	TaxOffice     string
	City          City
	District      string
	Sector        Sector
	Phone, Email  string
	// StartMonth: months before the anchor month the engagement (and the
	// data) starts; most clients are older than the window.
	StartMonth int
	// Size scales the sector's typical amounts (0.4 – 2.5).
	Size float64
	// Problem marks the clients that need attention this month (late
	// return, unmatched bank lines, open reconciliation difference).
	Problem bool
	Staff   int
}

// Rand returns the deterministic random stream of (office, index, salt).
func Rand(office, index int, salt uint64) *rand.Rand {
	return rand.New(rand.NewPCG(BigSeed^salt, uint64(office)<<32|uint64(index))) //nolint:gosec // deterministik demo verisi, güvenlik amaçlı değil
}

// GenerateTaxpayers returns n taxpayers of office o (0-based), deterministic.
func GenerateTaxpayers(o, n int) []BigTaxpayer {
	off := BigOffices[o]
	out := make([]BigTaxpayer, 0, n)
	for i := 0; i < n; i++ {
		r := Rand(o, i, 1)
		sec := Sectors[i%len(Sectors)]
		cityName := off.City
		if r.IntN(10) == 0 {
			cityName = otherCities[r.IntN(len(otherCities))]
		}
		city := Cities[cityName]
		district := city.Districts[r.IntN(len(city.Districts))]
		tp := BigTaxpayer{Office: o, Index: i, Sector: sec, City: city, District: district, TaxOffice: district,
			Size: 0.4 + r.Float64()*2.1}
		switch k := r.IntN(10); {
		case k < 3:
			tp.Kind, tp.Legal = "individual", ""
			tp.TaxID = BigTCKN(o, i)
			tp.Title = FirstNames[r.IntN(len(FirstNames))] + " " + LastNames[r.IntN(len(LastNames))]
			tp.Size *= 0.6
		case k < 8:
			tp.Kind, tp.Legal = "legal", "Ltd. Şti."
		default:
			tp.Kind, tp.Legal = "legal", "A.Ş."
			tp.Size *= 1.6
		}
		if tp.Kind == "legal" {
			tp.TaxID = BigVKN(o, i)
			tp.Title = titleWords[(i*7+o*3+r.IntN(5))%len(titleWords)] + " " + sec.Words[r.IntN(len(sec.Words))]
			if tp.Legal == "A.Ş." {
				tp.Title += " Sanayi ve Ticaret A.Ş."
			} else {
				tp.Title += " Ticaret Ltd. Şti."
			}
		}
		tp.Phone = fmt.Sprintf("0555 000 %02d %02d", (o*31+i)/100%100, (o*31+i)%100)
		tp.Email = "muhasebe@" + slugify(tp.Title) + ".example.com"
		if r.IntN(6) == 0 {
			tp.StartMonth = 4 + r.IntN(14) // pencere içinde başlayan yeni müşteri
		} else {
			tp.StartMonth = 999
		}
		// Each office has a few clients that need attention this month.
		tp.Problem = i%6 == 2
		tp.Staff = sec.StaffMin + r.IntN(sec.StaffMax-sec.StaffMin+1)
		if tp.Kind == "individual" && tp.Staff > 3 {
			tp.Staff = 3
		}
		out = append(out, tp)
	}
	return out
}

// Counterparty is a generated customer / supplier.
type Counterparty struct {
	TaxID, Name, City, District, IBAN string
}

// GenerateCounterparties returns the shared customer / supplier pool.
func GenerateCounterparties(n int) []Counterparty {
	out := make([]Counterparty, 0, n)
	cities := []string{"İstanbul", "Ankara", "İzmir", "Bursa", "Antalya", "Kocaeli"}
	for i := 0; i < n; i++ {
		r := Rand(99, i, 2)
		c := Cities[cities[r.IntN(len(cities))]]
		legal := " Ltd. Şti."
		if i%4 == 0 {
			legal = " A.Ş."
		}
		out = append(out, Counterparty{
			TaxID:    checkedVKN(fmt.Sprintf("%s%07d", BigCounterpartyPrefix, 100+i)),
			Name:     titleWords[(i*11)%len(titleWords)] + " " + counterpartyWords[(i*5+i/len(titleWords))%len(counterpartyWords)] + legal,
			City:     c.Name,
			District: c.Districts[r.IntN(len(c.Districts))],
			IBAN:     IBAN(fmt.Sprintf("%05d", 10+i%9*10), fmt.Sprintf("%016d", 7000000000+int64(i)*7919)),
		})
	}
	return out
}

// BigVKN returns the VKN of big taxpayer (o, i).
func BigVKN(o, i int) string { return checkedVKN(fmt.Sprintf("%s%d%06d", BigVKNPrefix, o+1, i+1)) }

// BigTCKN returns the TCKN of big individual taxpayer (o, i).
func BigTCKN(o, i int) string { return checkedTCKN(fmt.Sprintf("%s%d%04d", BigTCKNPrefix, o+1, i+1)) }

// EmployeeTCKN returns an invented TCKN for employee e of taxpayer (o, i).
func EmployeeTCKN(o, i, e int) string {
	return checkedTCKN(fmt.Sprintf("2%01d%03d%04d", o+1, i%1000, e+1)[:9])
}

func checkedVKN(nine string) string {
	for d := 0; d <= 9; d++ {
		if s := nine + string(rune('0'+d)); taxid.ValidVKN(s) {
			return s
		}
	}
	panic("demo: no VKN check digit for " + nine)
}

func checkedTCKN(nine string) string {
	for d := 0; d < 100; d++ {
		if s := fmt.Sprintf("%s%02d", nine, d); taxid.ValidTCKN(s) {
			return s
		}
	}
	panic("demo: no TCKN check digits for " + nine)
}

// IBAN builds a TR IBAN with valid check digits from a 5-digit bank code and
// a 16-digit account part (reserve digit 0). The account is invented.
func IBAN(bank, account string) string {
	bban := bank + "0" + account
	// TR = 29 27, check digits 00 → move to the end and mod 97.
	s := bban + "292700"
	rem := 0
	for _, c := range s {
		rem = (rem*10 + int(c-'0')) % 97
	}
	return fmt.Sprintf("TR%02d%s", 98-rem, bban)
}

var slugReplacer = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u", ".", "", " ", "-")

func slugify(s string) string {
	s = strings.ToLower(slugReplacer.Replace(s))
	s = strings.ReplaceAll(s, "--", "-")
	return strings.Trim(s, "-")
}
