package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
)

// Büyük demonun bellek modeli: bir mükellefin pencere boyunca bütün
// belgeleri (fatura, Z raporu, fiş, banka hareketi, bordro, yevmiye) aynı
// rastgele akıştan, aynı tutarlarla üretilir. Veritabanına yazım (bigdata.go)
// bu modeli olduğu gibi kopyalar; böylece faturalar ↔ KDV ↔ banka ↔ mizan
// aynı rakamları anlatır.
//
// Tutarlar kuruş (int64): toplamlar tam tutar.

type kurus = int64

func kr(tl float64) kurus { return kurus(math.Round(tl * 100)) }

// pct returns x * p / 100, rounded to the kuruş.
func pct(x kurus, p float64) kurus { return kurus(math.Round(float64(x) * p / 100)) }

type invLine struct {
	name        string
	qty         float64
	unit        string
	base, vat   kurus // document currency
	rate        float64
	whCode      string
	whRatio     float64 // 0.4 = 4/10
	withholding kurus
	exemption   string
	lineType    string
}

type invDoc struct {
	srcID, number, profile, typ, dir string
	ettn                             [16]byte
	date                             time.Time
	period                           string
	cp                               int // counterparty index; -1 = consumer
	currency                         string
	rate                             float64 // TRY per unit, 1 for TRY
	lines                            []invLine
	base, vat, withholding           kurus // document currency
	cancelled                        bool
	paidOn                           time.Time // zero = open
	dueOn                            time.Time
	exemption                        string
}

// payable in document currency.
func (d *invDoc) payable() kurus { return d.base + d.vat - d.withholding }

// try converts a document amount to TL (VAT forecast rounding: per amount).
func (d *invDoc) try(x kurus) kurus {
	if d.currency == "TRY" {
		return x
	}
	return kurus(math.Round(float64(x) * d.rate))
}

type zReport struct {
	date        time.Time
	period      string
	number      int
	bases, vats map[float64]kurus
	cash, card  kurus
	receipts    int
	approved    bool
}

func (z *zReport) totals() (base, vat kurus) {
	for r, b := range z.bases {
		base += b
		vat += z.vats[r]
	}
	return
}

type expenseDoc struct {
	date                  time.Time
	period                string
	vendor                string
	receiptNo             string
	description, category string
	receiptType, payment  string
	rate                  float64
	base, vat             kurus
	approved              bool
	plate                 string
}

type employee struct {
	name, tckn, sgkNo, job string
	hired                  time.Time
	left                   time.Time // zero = still employed
	grossFactor            float64   // × minimum wage
}

// payLine is one employee's month (Luca bordro columns).
type payLine struct {
	emp                                          *employee
	days                                         int
	gross, sgkE, unempE, gvEx, gv, dvEx, dv, net kurus
	sgkER, unempER, incentive                    kurus
}

type payrollMonth struct {
	period string
	lines  []payLine
}

func (p *payrollMonth) sum(f func(*payLine) kurus) kurus {
	var s kurus
	for i := range p.lines {
		s += f(&p.lines[i])
	}
	return s
}

// sgkPayable: employee + employer shares − incentive.
func (p *payrollMonth) sgkPayable() kurus {
	return p.sum(func(l *payLine) kurus { return l.sgkE + l.unempE + l.sgkER + l.unempER - l.incentive })
}
func (p *payrollMonth) netTotal() kurus { return p.sum(func(l *payLine) kurus { return l.net }) }
func (p *payrollMonth) taxTotal() kurus { return p.sum(func(l *payLine) kurus { return l.gv + l.dv }) }

type bankTxn struct {
	srcID      string
	date       time.Time
	dir        string // A in, B out
	amount     kurus
	desc       string
	cpName     string
	cpIBAN     string
	cpTaxID    string
	kind       string
	kindDetail string
	account    string // counter account of the journal line
	invoice    int    // index into client.invoices, -1 none
	match      string // matched / unmatched / suggested
	balance    kurus
	period     string
	// fx: kur farkı of a foreign-currency collection (#568), collected TL
	// minus the invoice's TL amount; booked to 646 (gain) or 656 (loss).
	fx kurus
}

type jLine struct {
	account, desc string
	debit, credit kurus
}

type voucher struct {
	period, desc, docType, entryType string
	date                             time.Time
	lines                            []jLine
	status                           string // draft / approved / exported
	source                           string
}

// clientModel is the whole generated history of one big taxpayer.
type clientModel struct {
	c         *bigClient
	r         *rand.Rand
	periods   []string // the client's periods (from its start), oldest first
	invoices  []invDoc
	zs        []zReport
	expenses  []expenseDoc
	employees []*employee
	payroll   []payrollMonth
	bank      []bankTxn
	vouchers  []voucher
	cps       []int // counterparty pool indices used (customers / suppliers)
	customers []int
	suppliers []int
	// kdv per period: payable / carry forward (model estimate, replaced by
	// the filed returns when the tax step ran).
	kdvPayable map[string]kurus
	kdvCarry   map[string]kurus
	opening    kurus
	okcSerial  string
	iban       string
}

var monthNamesTR = []string{"OCAK", "SUBAT", "MART", "NISAN", "MAYIS", "HAZIRAN", "TEMMUZ", "AGUSTOS", "EYLUL", "EKIM", "KASIM", "ARALIK"}

func periodStart(p string) time.Time {
	t, _ := time.Parse("2006-01", p)
	return t
}

func daysIn(p string) int { return periodStart(p).AddDate(0, 1, -1).Day() }

// minimumGross: monthly gross minimum wage per year (TL), used for payroll
// exemptions; later years grow 25%.
func minimumGross(year int) float64 {
	switch {
	case year <= 2024:
		return 20002.50
	case year == 2025:
		return 26005.50
	case year == 2026:
		return 33030.00
	}
	return 33030.00 * math.Pow(1.25, float64(year-2026))
}

// usdTRY is an invented, smooth exchange rate curve (TL per USD).
func usdTRY(d time.Time) float64 {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	months := d.Sub(base).Hours() / 24 / 30.4
	return math.Round((30.0+months*0.42+math.Sin(months/3)*0.35)*10000) / 10000
}

// buildClient generates a client's history up to w.now. Deterministic: the
// stream depends only on the client's (office, index).
func buildClient(w *bigWorld, c *bigClient) *clientModel {
	m := &clientModel{c: c, r: demo.Rand(c.Office, c.Index, 7), kdvPayable: map[string]kurus{}, kdvCarry: map[string]kurus{}}
	for i, p := range w.periods {
		monthsBack := len(w.periods) - 1 - i
		if monthsBack <= c.StartMonth {
			m.periods = append(m.periods, p)
		}
	}
	r := m.r
	m.okcSerial = fmt.Sprintf("JH%08d", 20000000+c.Office*100000+c.Index*37)
	m.iban = demo.IBAN([]string{"00010", "00012", "00015", "00046", "00062", "00064", "00067"}[c.Index%7],
		fmt.Sprintf("%016d", 5000000000+int64(c.Office)*10000000+int64(c.Index)*104729))
	// Counterparties: each client has its own customers and suppliers from
	// the shared pool.
	n := len(w.cps)
	for i := 0; i < 10+r.IntN(8); i++ {
		m.customers = append(m.customers, (c.Office*53+c.Index*17+i*7)%n)
	}
	for i := 0; i < 8+r.IntN(6); i++ {
		m.suppliers = append(m.suppliers, (c.Office*29+c.Index*11+i*13+5)%n)
	}
	m.cps = uniqueInts(append(append([]int{}, m.customers...), m.suppliers...))
	m.buildEmployees(w)
	for i, p := range m.periods {
		m.buildMonth(w, p, i)
	}
	m.settle(w)
	return m
}

func uniqueInts(in []int) []int {
	seen := map[int]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// salesFor returns the net sales target (TL) of the month.
func (m *clientModel) salesFor(w *bigWorld, p string) float64 {
	t := periodStart(p)
	sec := m.c.Sector
	monthsFromStart := float64(len(w.periods) - 1 - indexOf(w.periods, p))
	trend := math.Pow(1+sec.Growth, -monthsFromStart/12) // older months smaller
	v := sec.MonthlySales * m.c.Size * sec.Season[int(t.Month())-1] * trend * (0.9 + m.r.Float64()*0.2)
	if p == w.periods[len(w.periods)-1] {
		v *= float64(m.today(w).Day()-1) / float64(daysIn(p))
	}
	return v
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func (m *clientModel) today(w *bigWorld) time.Time {
	l := w.now.In(istanbul)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// day picks a date in p before today.
func (m *clientModel) day(w *bigWorld, p string) time.Time {
	last := daysIn(p)
	if p == w.periods[len(w.periods)-1] {
		last = m.today(w).Day() - 1
		if last < 1 {
			last = 1
		}
	}
	return periodStart(p).AddDate(0, 0, m.r.IntN(last))
}

var productNames = map[string][]string{
	"perakende": {"Toptan gıda ürünleri", "Temizlik ürünleri", "İçecek kolisi", "Kuru gıda", "Ambalajlı şarküteri"},
	"insaat":    {"Hakediş bedeli", "Kaba inşaat işçiliği", "İnce işler işçiliği", "Hazır beton", "İnşaat demiri", "Seramik ve vitrifiye"},
	"hizmet":    {"Aylık danışmanlık hizmeti", "Yazılım bakım bedeli", "Temizlik hizmet bedeli", "Organizasyon hizmeti", "Nakliye bedeli"},
	"eticaret":  {"Kablosuz kulaklık", "Akıllı saat", "Pamuk nevresim takımı", "Cilt bakım seti", "Kadın triko kazak", "Telefon kılıfı"},
	"restoran":  {"Toplu yemek hizmeti", "Catering hizmeti", "Etkinlik ikramı"},
}

var purchaseNames = map[string][]string{
	"perakende": {"Ticari mal alımı", "Gıda ürünleri", "Ambalaj malzemesi", "Elektrik bedeli", "Kira bedeli"},
	"insaat":    {"İnşaat demiri", "Çimento", "Hazır beton", "Taşeron işçilik", "İş makinesi kirası", "Akaryakıt"},
	"hizmet":    {"Bilgisayar ekipmanı", "Yazılım lisansı", "Ofis kirası", "İnternet hizmeti", "Kırtasiye"},
	"eticaret":  {"Ticari mal alımı", "Kargo hizmeti", "Pazaryeri komisyonu", "Ambalaj malzemesi", "Reklam hizmeti"},
	"restoran":  {"Et ve tavuk", "Sebze meyve", "Kuru gıda", "İçecek", "Doğalgaz bedeli", "Temizlik malzemesi"},
}

func (m *clientModel) newETTN() [16]byte {
	var b [16]byte
	binary.LittleEndian.PutUint64(b[:8], m.r.Uint64())
	binary.LittleEndian.PutUint64(b[8:], m.r.Uint64())
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return b
}

func numberPrefix(name string) string {
	s := strings.ToUpper(slugASCII(name))
	var out []byte
	for i := 0; i < len(s) && len(out) < 3; i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			out = append(out, s[i])
		}
	}
	for len(out) < 3 {
		out = append(out, 'X')
	}
	return string(out)
}

var asciiFold = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "C", "Ğ", "G", "İ", "I", "Ö", "O", "Ş", "S", "Ü", "U")

func slugASCII(s string) string { return asciiFold.Replace(s) }

// split divides total into n positive parts with random weights.
func (m *clientModel) split(total kurus, n int) []kurus {
	if n <= 0 || total <= 0 {
		return nil
	}
	ws := make([]float64, n)
	sum := 0.0
	for i := range ws {
		ws[i] = 0.3 + m.r.Float64()
		sum += ws[i]
	}
	out := make([]kurus, n)
	var acc kurus
	for i := range ws {
		out[i] = kurus(float64(total) * ws[i] / sum)
		if out[i] < 5000 {
			out[i] = 5000
		}
		acc += out[i]
	}
	out[n-1] += total - acc
	if out[n-1] < 5000 {
		out[n-1] = 5000
	}
	return out
}

func (m *clientModel) addInvoice(w *bigWorld, d invDoc, names []string) {
	r := m.r
	d.ettn = m.newETTN()
	nl := 1 + r.IntN(2)
	parts := m.split(d.base, nl)
	d.base, d.vat, d.withholding = 0, 0, 0
	for i, b := range parts {
		l := invLine{name: names[r.IntN(len(names))], unit: "C62", base: b, rate: d.lines[0].rate,
			whCode: d.lines[0].whCode, whRatio: d.lines[0].whRatio, exemption: d.lines[0].exemption, lineType: d.lines[0].lineType}
		l.qty = float64(1 + r.IntN(40))
		if l.lineType == "service" {
			l.qty, l.unit = 1, "C62"
		}
		l.vat = pct(b, l.rate)
		l.withholding = kurus(math.Round(float64(l.vat) * l.whRatio))
		d.base += l.base
		d.vat += l.vat
		d.withholding += l.withholding
		parts[i] = 0
		d.lines = append(d.lines, l)
	}
	d.lines = d.lines[1:]
	m.invoices = append(m.invoices, d)
}

func (m *clientModel) buildMonth(w *bigWorld, p string, mi int) {
	r := m.r
	sec := m.c.Sector
	sales := m.salesFor(w, p)
	year := periodStart(p).Year()
	seq := func(prefix string) string {
		return fmt.Sprintf("%s%d%09d", prefix, year, len(m.invoices)+1+m.c.Index*100000)
	}
	ownPrefix := numberPrefix(m.c.Title)
	tmpl := func(rate float64, lineType string) []invLine { return []invLine{{rate: rate, lineType: lineType}} }
	goods := "goods"
	if sec.Key == "hizmet" || sec.Key == "insaat" {
		goods = "service"
	}

	invoiceSales := sales
	if sec.Retail {
		invoiceSales = sales * 0.15
		m.buildZ(w, p, sales*0.85)
	}
	// Outgoing invoices.
	switch {
	case sec.Ecommerce:
		cons := sales * 0.8
		nCons := 60 + r.IntN(80)
		if p == w.periods[len(w.periods)-1] {
			nCons = nCons * (m.today(w).Day() - 1) / daysIn(p)
		}
		for _, b := range m.split(kr(cons/1.2), nCons) {
			d := invDoc{srcID: fmt.Sprintf("big:%d:%d:inv:%d", m.c.Office, m.c.Index, len(m.invoices)),
				profile: "EARSIVFATURA", typ: "SATIS", dir: "outgoing", date: m.day(w, p), period: p, cp: -1,
				currency: "TRY", rate: 1, base: b, lines: tmpl(20, "goods")}
			d.number = seq("EAR")
			m.addInvoice(w, d, productNames[sec.Key])
		}
		// Export (istisna, döviz) a few times a quarter.
		if r.IntN(3) == 0 || mi%3 == 1 {
			dt := m.day(w, p)
			rate := usdTRY(dt)
			cur := "USD"
			if r.IntN(2) == 0 {
				cur, rate = "EUR", math.Round(rate*1.08*10000)/10000
			}
			usd := sales * 0.2 / rate
			d := invDoc{srcID: fmt.Sprintf("big:%d:%d:inv:%d", m.c.Office, m.c.Index, len(m.invoices)),
				profile: "IHRACAT", typ: "ISTISNA", dir: "outgoing", date: dt, period: p, cp: m.customers[r.IntN(len(m.customers))],
				currency: cur, rate: rate, base: kr(usd), exemption: "301", lines: []invLine{{rate: 0, exemption: "301", lineType: "goods"}}}
			d.number = seq(ownPrefix)
			m.addInvoice(w, d, productNames[sec.Key])
		}
	default:
		n := map[string][2]int{"perakende": {3, 6}, "insaat": {3, 6}, "hizmet": {8, 12}, "restoran": {2, 3}}[sec.Key]
		cnt := n[0] + r.IntN(n[1])
		rate := float64(sec.SalesRate)
		if sec.Retail {
			rate = 20
			if sec.Key == "restoran" {
				rate = 10
			}
		}
		for _, b := range m.split(kr(invoiceSales), cnt) {
			d := invDoc{srcID: fmt.Sprintf("big:%d:%d:inv:%d", m.c.Office, m.c.Index, len(m.invoices)),
				profile: "TICARIFATURA", typ: "SATIS", dir: "outgoing", date: m.day(w, p), period: p,
				cp: m.customers[r.IntN(len(m.customers))], currency: "TRY", rate: 1, base: b, lines: tmpl(rate, goods)}
			if r.IntN(4) == 0 {
				d.profile = "TEMELFATURA"
			}
			if sec.Withholder && r.IntN(3) == 0 {
				d.typ = "TEVKIFAT"
				if sec.Key == "insaat" {
					d.lines[0].whCode, d.lines[0].whRatio = "601", 0.4
				} else {
					d.lines[0].whCode, d.lines[0].whRatio = "602", 0.9
				}
			}
			d.number = seq(ownPrefix)
			m.addInvoice(w, d, productNames[sec.Key])
		}
	}
	// A sales return now and then (incoming IADE from a customer).
	if r.IntN(5) == 0 && len(m.customers) > 0 && !sec.Ecommerce {
		d := invDoc{srcID: fmt.Sprintf("big:%d:%d:inv:%d", m.c.Office, m.c.Index, len(m.invoices)),
			profile: "TEMELFATURA", typ: "IADE", dir: "incoming", date: m.day(w, p), period: p,
			cp: m.customers[r.IntN(len(m.customers))], currency: "TRY", rate: 1, base: kr(invoiceSales * 0.02), lines: tmpl(20, "goods")}
		d.number = seq(numberPrefix(w.cps[d.cp].Name))
		m.addInvoice(w, d, productNames[sec.Key])
	}
	// Incoming purchase invoices: cost of sales and overheads.
	costShare := map[string]float64{"perakende": 0.68, "insaat": 0.62, "hizmet": 0.35, "eticaret": 0.6, "restoran": 0.45}[sec.Key]
	nIn := map[string][2]int{"perakende": {10, 12}, "insaat": {8, 10}, "hizmet": {4, 6}, "eticaret": {8, 8}, "restoran": {12, 12}}[sec.Key]
	cnt := nIn[0] + r.IntN(nIn[1])
	if p == w.periods[len(w.periods)-1] {
		cnt = 1 + cnt*(m.today(w).Day()-1)/daysIn(p)
	}
	for _, b := range m.split(kr(sales*costShare), cnt) {
		rate := 20.0
		if sec.Key == "perakende" && r.IntN(2) == 0 {
			rate = 1
		} else if sec.Key == "restoran" && r.IntN(3) > 0 {
			rate = 1
		}
		d := invDoc{srcID: fmt.Sprintf("big:%d:%d:inv:%d", m.c.Office, m.c.Index, len(m.invoices)),
			profile: "TICARIFATURA", typ: "SATIS", dir: "incoming", date: m.day(w, p), period: p,
			cp: m.suppliers[r.IntN(len(m.suppliers))], currency: "TRY", rate: 1, base: b, lines: tmpl(rate, "goods")}
		if r.IntN(6) == 0 {
			d.profile = "EARSIVFATURA"
		}
		d.number = seq(numberPrefix(w.cps[d.cp].Name))
		m.addInvoice(w, d, purchaseNames[sec.Key])
	}
	// One cancelled sales invoice every few months.
	if r.IntN(6) == 0 {
		for i := len(m.invoices) - 1; i >= 0; i-- {
			if m.invoices[i].period == p && m.invoices[i].dir == "outgoing" {
				m.invoices[i].cancelled = true
				break
			}
		}
	}
	m.buildExpenses(w, p)
	m.buildPayroll(w, p)
}

func (m *clientModel) buildZ(w *bigWorld, p string, gross float64) {
	r := m.r
	start := periodStart(p)
	last := daysIn(p)
	if p == w.periods[len(w.periods)-1] {
		last = m.today(w).Day() - 1
	}
	var days []time.Time
	for d := 0; d < last; d++ {
		t := start.AddDate(0, 0, d)
		if t.Weekday() == time.Sunday && m.c.Sector.Key == "perakende" && r.IntN(3) > 0 {
			continue
		}
		days = append(days, t)
	}
	if len(days) == 0 {
		return
	}
	rates := map[float64]float64{1: 0.55, 10: 0.2, 20: 0.25}
	if m.c.Sector.Key == "restoran" {
		rates = map[float64]float64{10: 0.9, 20: 0.1}
	}
	parts := m.split(kr(gross), len(days))
	for i, t := range days {
		z := zReport{date: t, period: p, number: 1 + len(m.zs), bases: map[float64]kurus{}, vats: map[float64]kurus{},
			receipts: 40 + r.IntN(90), approved: true}
		var total kurus
		rs := []float64{1, 10, 20}
		for _, rate := range rs {
			share, ok := rates[rate]
			if !ok {
				continue
			}
			g := kurus(float64(parts[i]) * share)
			b := kurus(math.Round(float64(g) * 100 / (100 + rate)))
			z.bases[rate], z.vats[rate] = b, g-b
			total += g
		}
		z.card = kurus(float64(total) * (0.55 + r.Float64()*0.2))
		z.cash = total - z.card
		if p == w.periods[len(w.periods)-1] && (m.c.Problem || r.IntN(3) == 0) && t.After(m.today(w).AddDate(0, 0, -6)) {
			z.approved = false
		}
		m.zs = append(m.zs, z)
	}
}

var expenseKinds = []struct {
	desc, category, rtype string
	rate                  float64
	min, max              float64
	vendors               []string
}{
	{"Araç yakıtı", "akaryakit", "fuel_receipt", 20, 900, 4200, []string{"Doruk Petrol", "Yakamoz Akaryakıt", "Kuzey Enerji Petrol"}},
	{"Personel yemeği", "yemek", "okc_receipt", 10, 350, 2600, []string{"Lezzet Durağı Lokantası", "Çınar Pide", "Kardelen Kafe"}},
	{"Kırtasiye malzemesi", "kirtasiye", "okc_receipt", 20, 150, 1400, []string{"Kalem Kırtasiye", "Ofis Dünyası"}},
	{"Kargo gönderimi", "kargo", "okc_receipt", 20, 90, 900, []string{"Hızlı Kargo", "Ufuk Kurye"}},
	{"Otopark ücreti", "otopark", "okc_receipt", 20, 60, 400, []string{"Merkez Otopark"}},
}

func (m *clientModel) buildExpenses(w *bigWorld, p string) {
	r := m.r
	n := 5 + r.IntN(10)
	cur := p == w.periods[len(w.periods)-1]
	if cur {
		n = n * (m.today(w).Day() - 1) / daysIn(p)
	}
	for i := 0; i < n; i++ {
		k := expenseKinds[r.IntN(len(expenseKinds))]
		gross := kr(k.min + r.Float64()*(k.max-k.min))
		base := kurus(math.Round(float64(gross) * 100 / (100 + k.rate)))
		e := expenseDoc{date: m.day(w, p), period: p, vendor: k.vendors[r.IntN(len(k.vendors))], description: k.desc,
			category: k.category, receiptType: k.rtype, rate: k.rate, base: base, vat: gross - base,
			receiptNo: fmt.Sprintf("%04d", 1+r.IntN(9999)), approved: !cur || r.IntN(3) == 0, payment: "cash"}
		if k.category == "akaryakit" {
			e.plate = demo.Cities[m.c.City.Name].Plate + " " + plateLetters[m.c.Index%len(plateLetters):m.c.Index%len(plateLetters)+1] + plateLetters[m.c.Office+1:m.c.Office+2] + fmt.Sprintf(" %03d", 100+m.c.Index*7%900)
			e.payment = "credit_card"
		}
		m.expenses = append(m.expenses, e)
	}
}

const plateLetters = "ABCDEFGHJKLMNPRSTUVYZ"

var jobTitles = []string{"Satış Danışmanı", "Muhasebe Uzmanı", "Depo Sorumlusu", "Şoför", "Aşçı", "Garson", "Kasiyer",
	"Proje Mühendisi", "Usta", "Operasyon Uzmanı", "Yazılım Geliştirici", "İdari İşler Sorumlusu"}

func (m *clientModel) buildEmployees(w *bigWorld) {
	r := demo.Rand(m.c.Office, m.c.Index, 11)
	start := periodStart(w.periods[0])
	for e := 0; e < m.c.Staff+2; e++ {
		emp := &employee{
			name:        demo.FirstNames[r.IntN(len(demo.FirstNames))] + " " + demo.LastNames[r.IntN(len(demo.LastNames))],
			tckn:        demo.EmployeeTCKN(m.c.Office, m.c.Index, e),
			sgkNo:       fmt.Sprintf("%d%02d%04d", 34+m.c.Office, m.c.Index%100, e+1),
			job:         jobTitles[r.IntN(len(jobTitles))],
			hired:       start.AddDate(0, -r.IntN(48), r.IntN(28)),
			grossFactor: 1 + float64(r.IntN(4))*0.2,
		}
		if e == 0 {
			emp.grossFactor = 1.9
		}
		// The last two join / leave inside the window: turnover.
		switch e {
		case m.c.Staff:
			emp.hired = start.AddDate(0, 6+r.IntN(10), r.IntN(20))
		case m.c.Staff + 1:
			emp.left = start.AddDate(0, 3+r.IntN(12), 14+r.IntN(10))
		}
		m.employees = append(m.employees, emp)
	}
}

func (m *clientModel) buildPayroll(w *bigWorld, p string) {
	if p == w.periods[len(w.periods)-1] {
		return // the current month's payroll is not run yet
	}
	start := periodStart(p)
	end := start.AddDate(0, 1, -1)
	minG := minimumGross(start.Year())
	gvEx := kr(minG * 0.85 * 0.15)
	dvEx := kr(minG * 0.00759)
	pm := payrollMonth{period: p}
	for _, e := range m.employees {
		if e.hired.After(end) || (!e.left.IsZero() && e.left.Before(start)) {
			continue
		}
		days := 30
		if e.hired.After(start) {
			days = 30 - e.hired.Day() + 1
		}
		if !e.left.IsZero() && e.left.Before(end) {
			days = e.left.Day()
		}
		if days < 1 {
			days = 1
		}
		l := payLine{emp: e, days: days}
		l.gross = kr(math.Round(minG*e.grossFactor*float64(days)/30*100) / 100)
		l.sgkE = pct(l.gross, 14)
		l.unempE = pct(l.gross, 1)
		gvBase := l.gross - l.sgkE - l.unempE
		ex := gvEx * kurus(days) / 30
		l.gvEx = min(ex, pct(gvBase, 15))
		l.gv = pct(gvBase, 15) - l.gvEx
		dex := dvEx * kurus(days) / 30
		l.dvEx = min(dex, pct(l.gross, 0.759))
		l.dv = pct(l.gross, 0.759) - l.dvEx
		l.net = l.gross - l.sgkE - l.unempE - l.gv - l.dv
		l.sgkER = pct(l.gross, 20.75)
		l.unempER = pct(l.gross, 2)
		l.incentive = pct(l.gross, 5)
		pm.lines = append(pm.lines, l)
	}
	if len(pm.lines) > 0 {
		m.payroll = append(m.payroll, pm)
	}
}

func (m *clientModel) payrollOf(p string) *payrollMonth {
	for i := range m.payroll {
		if m.payroll[i].period == p {
			return &m.payroll[i]
		}
	}
	return nil
}

// vatOf returns the month's output VAT (after withholding), input VAT and
// the parts of the KDV1 (approved Z reports and expenses only, active
// invoices).
func (m *clientModel) vatOf(p string) (output, input kurus) {
	for i := range m.invoices {
		d := &m.invoices[i]
		if d.period != p || d.cancelled {
			continue
		}
		if d.dir == "outgoing" {
			output += d.try(d.vat) - d.try(d.withholding)
		} else {
			input += d.try(d.vat)
		}
	}
	for i := range m.zs {
		if m.zs[i].period == p && m.zs[i].approved {
			_, v := m.zs[i].totals()
			output += v
		}
	}
	for i := range m.expenses {
		if m.expenses[i].period == p && m.expenses[i].approved {
			input += m.expenses[i].vat
		}
	}
	return
}

// estimateKDV fills the model's KDV payable / carry chain.
func (m *clientModel) estimateKDV() {
	var carry kurus
	for _, p := range m.periods {
		out, in := m.vatOf(p)
		avail := in + carry
		if out >= avail {
			m.kdvPayable[p], carry = out-avail, 0
		} else {
			m.kdvPayable[p], carry = 0, avail-out
		}
		m.kdvCarry[p] = carry
	}
}

// settle decides when invoices were paid (the bank lines are built later,
// once the tax amounts are known).
func (m *clientModel) settle(w *bigWorld) {
	r := m.r
	today := m.today(w)
	for i := range m.invoices {
		d := &m.invoices[i]
		d.dueOn = d.date.AddDate(0, 0, 30)
		if d.cancelled || d.cp < 0 {
			continue // consumer sales are collected through the POS
		}
		lag := r.IntN(50)
		if d.dir == "incoming" {
			lag = 5 + r.IntN(55)
		}
		if r.IntN(12) == 0 {
			lag += 40 // a slow payer
		}
		paid := d.date.AddDate(0, 0, lag)
		if paid.Before(today) {
			d.paidOn = paid
		}
	}
	m.estimateKDV()
}

// taxDue is what the bank pays for period p's returns, from the tax step
// when it ran (filed returns), otherwise the model estimate.
type taxDue struct {
	kdv, muh map[string]kurus
	// filed: the periods whose returns were filed (and so paid).
	filed map[string]bool
}

// filedByDeadline: past periods are filed; the latest closed month is filed
// by most clients when its deadline (26th / 28th of the next month) has
// come; problem clients are late with the one before.
func filedByDeadline(w *bigWorld, c *bigClient, p string) bool {
	idx := indexOf(w.periods, p)
	last := len(w.periods) - 1
	switch {
	case idx >= last:
		return false
	case idx == last-1:
		return w.now.In(istanbul).Day() >= 26 && !c.Problem
	case idx == last-2:
		return !c.Problem
	}
	return true
}

// buildBank produces the bank statement: opening balance, collections,
// supplier payments, POS settlements, salaries, SGK and tax payments, fees.
func (m *clientModel) buildBank(w *bigWorld, cps []demo.Counterparty, tax taxDue) {
	r := demo.Rand(m.c.Office, m.c.Index, 13)
	today := m.today(w)
	add := func(t bankTxn) {
		if !t.date.Before(today) || t.amount <= 0 {
			return
		}
		if t.match == "" {
			t.match = "matched"
		}
		t.period = t.date.Format("2006-01")
		if indexOf(m.periods, t.period) < 0 {
			return
		}
		m.bank = append(m.bank, t)
	}
	first := periodStart(m.periods[0])
	m.opening = kr(m.salesFor(w, m.periods[0]) * (1.2 + r.Float64()))
	// rx: rutin dışı işlemler (#568); r'nin akışını değiştirmez.
	rx := demo.Rand(m.c.Office, m.c.Index, 568)
	for i := range m.invoices {
		d := &m.invoices[i]
		if d.paidOn.IsZero() || d.cp < 0 {
			continue
		}
		cp := cps[d.cp]
		amt := d.try(d.payable())
		if d.dir == "outgoing" {
			kind := []string{"EFT", "FAST", "HAVALE"}[r.IntN(3)]
			who := strings.ToUpper(slugASCII(cp.Name))
			if d.currency != "TRY" {
				// Döviz tahsilatı: tahsil günündeki kurla TL'ye çevrilir; fatura
				// kuruyla arasındaki fark kur farkıdır (646 / 656).
				rate := usdTRY(d.paidOn)
				if d.currency == "EUR" {
					rate = math.Round(rate*1.08*10000) / 10000
				}
				got := kurus(math.Round(float64(d.payable()) * rate))
				add(bankTxn{date: d.paidOn, dir: "A", amount: got, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
					desc: fmt.Sprintf("DOVIZ TAHSILAT %d.%02d %s GONDEREN: %s FT %s KUR %.4f", d.payable()/100, d.payable()%100, d.currency, who, d.number, rate),
					kind: "YABANCI_PARA_HAVALE", kindDetail: "customer_collection", account: "120.01", invoice: i, fx: got - amt})
				continue
			}
			switch u := rx.IntN(20); {
			case u == 0 && amt >= 500000:
				// Kısmi ödeme: müşteri faturayı iki parçada öder.
				first := amt * 6 / 10 / 100 * 100
				add(bankTxn{date: d.paidOn.AddDate(0, 0, -12), dir: "A", amount: first, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
					desc: fmt.Sprintf("%s GELEN GONDEREN: %s FT %s KISMI ODEME", kind, who, d.number),
					kind: kind, kindDetail: "customer_collection", account: "120.01", invoice: i})
				add(bankTxn{date: d.paidOn, dir: "A", amount: amt - first, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
					desc: fmt.Sprintf("%s GELEN GONDEREN: %s FT %s KALAN", kind, who, d.number),
					kind: kind, kindDetail: "customer_collection", account: "120.01", invoice: i})
				continue
			case u == 1 && !m.c.Sector.Retail:
				// Sipariş avansı: tahsilat faturadan önce gelir (340).
				add(bankTxn{date: d.date.AddDate(0, 0, -6), dir: "A", amount: amt, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
					desc: fmt.Sprintf("%s GELEN GONDEREN: %s SIPARIS AVANSI", kind, who),
					kind: kind, kindDetail: "customer_collection", account: "340.01", invoice: i})
				continue
			}
			add(bankTxn{date: d.paidOn, dir: "A", amount: amt, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
				desc: fmt.Sprintf("%s GELEN GONDEREN: %s FT %s", kind, who, d.number),
				kind: kind, kindDetail: "customer_collection", account: "120.01", invoice: i})
		} else if d.typ == "IADE" {
			add(bankTxn{date: d.paidOn, dir: "B", amount: amt, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
				desc: fmt.Sprintf("EFT GIDEN ALICI: %s IADE %s", strings.ToUpper(slugASCII(cp.Name)), d.number),
				kind: "EFT", kindDetail: "supplier_payment", account: "120.01", invoice: i})
		} else {
			add(bankTxn{date: d.paidOn, dir: "B", amount: amt, cpName: cp.Name, cpIBAN: cp.IBAN, cpTaxID: cp.TaxID,
				desc: fmt.Sprintf("EFT GIDEN ALICI: %s FATURA %s", strings.ToUpper(slugASCII(cp.Name)), d.number),
				kind: "EFT", kindDetail: "supplier_payment", account: "320.01", invoice: i})
			add(bankTxn{date: d.paidOn, dir: "B", amount: 850 + kurus(r.IntN(400)), desc: "EFT UCRETI",
				kind: "UCRET_KOMISYON_FAIZ", kindDetail: "commission", account: "780.01", invoice: -1})
		}
	}
	// POS settlements: card sales of the Z reports and e-arşiv consumer
	// sales, settled weekly (next Monday) less the commission.
	weekly := map[time.Time]kurus{}
	for i := range m.zs {
		z := &m.zs[i]
		weekly[nextMonday(z.date)] += z.card
	}
	for i := range m.invoices {
		d := &m.invoices[i]
		if d.cp < 0 && !d.cancelled {
			weekly[nextMonday(d.date)] += d.payable()
		}
	}
	var weeks []time.Time
	for t := range weekly {
		weeks = append(weeks, t)
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].Before(weeks[j]) })
	for _, t := range weeks {
		g := weekly[t]
		comm := pct(g, 1.8)
		add(bankTxn{date: t, dir: "A", amount: g - comm, desc: "UYE ISYERI POS SATIS TUTARI " + t.Format("02.01.2006"),
			kind: "UYE_ISYERI_ISLEMLERI", kindDetail: "pos_settlement", account: "108.01", invoice: -1})
		add(bankTxn{date: t, dir: "B", amount: comm, desc: "POS KOMISYONU",
			kind: "UYE_ISYERI_ISLEMLERI", kindDetail: "pos_commission", account: "780.01", invoice: -1})
	}
	// Cash of the Z reports goes to the bank every week.
	cash := map[time.Time]kurus{}
	for i := range m.zs {
		cash[nextMonday(m.zs[i].date).AddDate(0, 0, 1)] += pct(m.zs[i].cash, 88)
	}
	var cdays []time.Time
	for t := range cash {
		cdays = append(cdays, t)
	}
	sort.Slice(cdays, func(i, j int) bool { return cdays[i].Before(cdays[j]) })
	for _, t := range cdays {
		add(bankTxn{date: t, dir: "A", amount: cash[t] / 10000 * 10000, desc: "NAKIT PARA YATIRMA SUBE",
			kind: "PARA_YATIRMA", kindDetail: "cash_deposit", account: "100.01", invoice: -1})
	}
	// Non-retail clients pay their receipts in cash drawn from the bank.
	if !m.c.Sector.Retail {
		for _, p := range m.periods {
			var spent kurus
			for i := range m.expenses {
				if m.expenses[i].period == p && m.expenses[i].approved {
					spent += m.expenses[i].base + m.expenses[i].vat
				}
			}
			if spent > 0 {
				add(bankTxn{date: periodStart(p).AddDate(0, 0, 2), dir: "B", amount: (spent/100000 + 1) * 100000,
					desc: "ATM NAKIT CEKIM", kind: "PARA_CEKME", kindDetail: "cash_withdrawal", account: "100.01", invoice: -1})
			}
		}
	}
	for _, p := range m.periods {
		ps := periodStart(p)
		next := ps.AddDate(0, 1, 0)
		mon := monthNamesTR[ps.Month()-1]
		if pm := m.payrollOf(p); pm != nil {
			add(bankTxn{date: next.AddDate(0, 0, 4), dir: "B", amount: pm.netTotal(), desc: "MAAS ODEMESI " + mon + " PERSONEL",
				kind: "HAVALE", kindDetail: "salary_payment", account: "335.01", invoice: -1})
			if tax.filed[p] {
				add(bankTxn{date: next.AddDate(0, 0, 25), dir: "B", amount: tax.muh[p], desc: "GIB VERGI ODEMESI MUHSGK " + ps.Format("2006/01"),
					kind: "VERGI_ODEMESI", kindDetail: "tax_payment", account: "360.02", invoice: -1})
				add(bankTxn{date: sgkPaidOn(p), dir: "B", amount: pm.sgkPayable(), desc: "SGK PRIM ODEMESI " + ps.Format("2006/01"),
					kind: "SGK_ODEMESI", kindDetail: "sgk_payment", account: "361.01", invoice: -1})
			}
		}
		if tax.filed[p] {
			add(bankTxn{date: next.AddDate(0, 0, 25+r.IntN(3)), dir: "B", amount: tax.kdv[p], desc: "GIB VERGI ODEMESI KDV " + ps.Format("2006/01"),
				kind: "VERGI_ODEMESI", kindDetail: "tax_payment", account: "360.01", invoice: -1})
		}
		add(bankTxn{date: next.AddDate(0, 0, -1), dir: "B", amount: 15000 + kurus(r.IntN(10000)), desc: "HESAP ISLETIM UCRETI",
			kind: "UCRET_KOMISYON_FAIZ", kindDetail: "bank_fee", account: "780.01", invoice: -1})
	}
	// Rutin dışı işlemler (#568): vadeli hesaba virman ve faizi, şirket
	// ortağından borç alınıp geri ödenmesi.
	for pi, p := range m.periods {
		ps := periodStart(p)
		if pi%4 == 1 {
			amt := kr(m.salesFor(w, p)*0.15) / 100000 * 100000
			add(bankTxn{date: ps.AddDate(0, 0, 2), dir: "B", amount: amt, desc: "VADELI HESABA VIRMAN 32 GUN",
				kind: "HAVALE", kindDetail: "own_account_transfer", account: "102.02", invoice: -1})
			add(bankTxn{date: ps.AddDate(0, 0, 34), dir: "A", amount: amt, desc: "VADELI HESAPTAN VIRMAN ANAPARA",
				kind: "HAVALE", kindDetail: "own_account_transfer", account: "102.02", invoice: -1})
			add(bankTxn{date: ps.AddDate(0, 0, 34), dir: "A", amount: pct(amt, 3.4), desc: "VADELI MEVDUAT FAIZ TAHAKKUKU",
				kind: "UCRET_KOMISYON_FAIZ", kindDetail: "interest_income", account: "642.01", invoice: -1})
		}
		if m.c.Kind == "legal" && pi%9 == 4 {
			partner := demo.FirstNames[rx.IntN(len(demo.FirstNames))] + " " + demo.LastNames[rx.IntN(len(demo.LastNames))]
			amt := kr(m.salesFor(w, p)*0.3) / 100000 * 100000
			add(bankTxn{date: ps.AddDate(0, 0, 8), dir: "A", amount: amt, cpName: partner,
				desc: "HAVALE GELEN GONDEREN: " + strings.ToUpper(slugASCII(partner)) + " ORTAK CARI HESAP",
				kind: "HAVALE", kindDetail: "unknown", account: "331.01", invoice: -1})
			add(bankTxn{date: ps.AddDate(0, 2, 10), dir: "B", amount: amt, cpName: partner,
				desc: "HAVALE GIDEN ALICI: " + strings.ToUpper(slugASCII(partner)) + " ORTAGA BORC IADESI",
				kind: "HAVALE", kindDetail: "unknown", account: "331.01", invoice: -1})
		}
	}
	// Attention items: the problem clients have unexplained lines in the
	// last weeks (no invoice, no rule): a collection and a payment.
	if m.c.Problem {
		for k := 0; k < 3; k++ {
			d := today.AddDate(0, 0, -(3 + k*9))
			cp := cps[(m.c.Index*7+k*31)%len(cps)]
			add(bankTxn{date: d, dir: "A", amount: kr(1500 + r.Float64()*42000), cpName: cp.Name, cpIBAN: cp.IBAN,
				desc: "FAST GELEN GONDEREN: " + strings.ToUpper(slugASCII(cp.Name)) + " ODEME", kind: "FAST",
				kindDetail: "customer_collection", account: "120.01", invoice: -1, match: "unmatched"})
			add(bankTxn{date: d.AddDate(0, 0, 1), dir: "B", amount: kr(800 + r.Float64()*18000),
				desc: "EFT GIDEN ALICI: " + strings.ToUpper(slugASCII(cps[(m.c.Index*3+k*17)%len(cps)].Name)), kind: "EFT",
				kindDetail: "supplier_payment", account: "320.01", invoice: -1, match: "unmatched"})
		}
	}
	sort.SliceStable(m.bank, func(i, j int) bool {
		if !m.bank[i].date.Equal(m.bank[j].date) {
			return m.bank[i].date.Before(m.bank[j].date)
		}
		return m.bank[i].dir < m.bank[j].dir // incoming first
	})
	// Keep the account positive: a loan disbursement when it would go
	// below a cushion.
	bal := m.opening
	out := m.bank[:0:0]
	for _, t := range m.bank {
		if t.dir == "B" && bal-t.amount < 0 {
			loan := ((t.amount-bal)/100000000 + 1) * 100000000 // round up to 1M TL
			out = append(out, bankTxn{date: t.date, dir: "A", amount: loan, desc: "TICARI KREDI KULLANDIRIM",
				kind: "KREDI_KULLANIM", kindDetail: "loan_disbursement", account: "300.01", invoice: -1, match: "matched", period: t.period})
			bal += loan
		}
		if t.dir == "A" {
			bal += t.amount
		} else {
			bal -= t.amount
		}
		t.balance = bal
		out = append(out, t)
	}
	for i := range out {
		out[i].srcID = fmt.Sprintf("big:%d:%d:bank:%d", m.c.Office, m.c.Index, i)
		if p := out[i].period; p == w.periods[len(w.periods)-1] && out[i].match == "matched" && out[i].invoice < 0 && r.IntN(3) == 0 {
			out[i].match = "suggested" // this month's lines wait in the posting queue
		}
	}
	_ = first
	m.bank = out
}

func nextMonday(t time.Time) time.Time {
	d := (8 - int(t.Weekday())) % 7
	if d == 0 {
		d = 7
	}
	return t.AddDate(0, 0, d)
}

// expenseAccount by sector: purchases are inventory for traders, cost for
// the rest.
func (m *clientModel) purchaseAccount() string {
	switch m.c.Sector.Key {
	case "perakende", "eticaret":
		return "153.01"
	case "insaat":
		return "740.01"
	case "restoran":
		return "740.01"
	}
	return "770.01"
}

// buildVouchers turns the month's documents into the vouchers the portal
// exported to Luca (yevmiye): one per source per month.
func (m *clientModel) buildVouchers(w *bigWorld, tax taxDue) {
	last := len(w.periods) - 1
	status := func(p string) string {
		switch indexOf(w.periods, p) {
		case last:
			return "draft"
		}
		return "exported"
	}
	first := periodStart(m.periods[0])
	m.vouchers = append(m.vouchers, voucher{period: m.periods[0], date: first, desc: "Açılış fişi", entryType: "acilis",
		docType: "acilis", status: "exported", source: "manual", lines: []jLine{
			{account: "102.01", desc: "Banka açılış bakiyesi", debit: m.opening},
			{account: "100.01", desc: "Kasa açılış bakiyesi", debit: 2500000},
			{account: "500.01", desc: "Sermaye", credit: m.opening + 2500000},
		}})
	var carry kurus
	for _, p := range m.periods {
		pe := periodStart(p).AddDate(0, 1, -1)
		if p == w.periods[last] {
			pe = m.today(w).AddDate(0, 0, -1)
		}
		st := status(p)
		mon := strings.ToLower(monthNamesTR[periodStart(p).Month()-1])
		// Sales and returns.
		var sl, pl []jLine
		var out, in kurus
		for i := range m.invoices {
			d := &m.invoices[i]
			if d.period != p || d.cancelled {
				continue
			}
			base, vat, wh := d.try(d.base), d.try(d.vat), d.try(d.withholding)
			switch {
			case d.dir == "outgoing":
				rev := "600.01"
				if d.typ == "ISTISNA" {
					rev = "601.01"
				}
				recv := "120.01"
				if d.cp < 0 {
					recv = "108.01"
				}
				sl = append(sl, jLine{account: recv, desc: d.number, debit: base + vat - wh},
					jLine{account: rev, desc: d.number, credit: base})
				if vat-wh > 0 {
					sl = append(sl, jLine{account: "391.01", desc: d.number, credit: vat - wh})
				}
				out += vat - wh
			case d.typ == "IADE":
				sl = append(sl, jLine{account: "610.01", desc: d.number + " satıştan iade", debit: base},
					jLine{account: "191.01", desc: d.number, debit: vat},
					jLine{account: "120.01", desc: d.number, credit: base + vat})
				in += vat
			default:
				pl = append(pl, jLine{account: m.purchaseAccount(), desc: d.number, debit: base},
					jLine{account: "191.01", desc: d.number, debit: vat},
					jLine{account: "320.01", desc: d.number, credit: base + vat})
				in += vat
			}
		}
		if len(sl) > 0 {
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Satış faturaları " + mon, docType: "fatura",
				status: st, source: "invoice", lines: sl})
		}
		if len(pl) > 0 {
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Alış faturaları " + mon, docType: "fatura",
				status: st, source: "invoice", lines: pl})
			// Traders: the month's cost of goods sold (most of the purchases).
			if m.purchaseAccount() == "153.01" {
				var bought kurus
				for _, l := range pl {
					if l.account == "153.01" {
						bought += l.debit
					}
				}
				cogs := pct(bought, 93)
				m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Satılan ticari mal maliyeti " + mon, docType: "maliyet",
					status: st, source: "manual", lines: []jLine{
						{account: "621.01", desc: "SMM", debit: cogs}, {account: "153.01", desc: "SMM", credit: cogs}}})
			}
		}
		// Z reports (approved ones; the pending ones are not booked yet).
		var zl []jLine
		var zCash, zCard, zBase, zVat kurus
		for i := range m.zs {
			z := &m.zs[i]
			if z.period != p || !z.approved {
				continue
			}
			b, v := z.totals()
			zCash, zCard, zBase, zVat = zCash+z.cash, zCard+z.card, zBase+b, zVat+v
		}
		if zBase > 0 {
			zl = []jLine{{account: "100.01", desc: "Z raporları nakit", debit: zCash},
				{account: "108.01", desc: "Z raporları kredi kartı", debit: zCard},
				{account: "600.01", desc: "Z raporları matrah", credit: zBase},
				{account: "391.01", desc: "Z raporları KDV", credit: zVat}}
			zst := st
			// Problem retail clients: last month's Z voucher approved but not
			// sent to Luca → reconciliation shows draft_not_exported.
			if m.c.Problem && indexOf(w.periods, p) == last-1 {
				zst = "approved"
			}
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Z raporları " + mon, docType: "z_report",
				status: zst, source: "receipt", lines: zl})
			out += zVat
		}
		// Expenses (approved).
		var el []jLine
		var eBase, eVat kurus
		for i := range m.expenses {
			e := &m.expenses[i]
			if e.period == p && e.approved {
				eBase, eVat = eBase+e.base, eVat+e.vat
			}
		}
		if eBase > 0 {
			el = []jLine{{account: "770.03", desc: "Masraf fişleri", debit: eBase},
				{account: "191.01", desc: "Masraf fişleri KDV", debit: eVat},
				{account: "100.01", desc: "Masraf fişleri", credit: eBase + eVat}}
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Masraf fişleri " + mon, docType: "receipt",
				status: st, source: "receipt", lines: el})
			in += eVat
		}
		// Payroll accrual.
		if pm := m.payrollOf(p); pm != nil {
			gross := pm.sum(func(l *payLine) kurus { return l.gross })
			er := pm.sum(func(l *payLine) kurus { return l.sgkER + l.unempER - l.incentive })
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Bordro tahakkuku " + mon, docType: "bordro",
				status: st, source: "payroll", lines: []jLine{
					{account: "770.02", desc: "Brüt ücret ve işveren payı", debit: gross + er},
					{account: "335.01", desc: "Net ücretler", credit: pm.netTotal()},
					{account: "360.02", desc: "Gelir ve damga vergisi", credit: pm.taxTotal()},
					{account: "361.01", desc: "SGK primleri", credit: pm.sgkPayable()},
				}})
		}
		// Bank.
		var bl []jLine
		for i := range m.bank {
			t := &m.bank[i]
			if t.period != p || t.match == "unmatched" || t.match == "suggested" {
				continue
			}
			if t.dir == "A" {
				bl = append(bl, jLine{account: "102.01", desc: t.desc, debit: t.amount}, jLine{account: t.account, desc: t.desc, credit: t.amount - t.fx})
				switch {
				case t.fx > 0:
					bl = append(bl, jLine{account: "646.01", desc: "Kur farkı " + t.desc, credit: t.fx})
				case t.fx < 0:
					bl = append(bl, jLine{account: "656.01", desc: "Kur farkı " + t.desc, debit: -t.fx})
				}
			} else {
				bl = append(bl, jLine{account: t.account, desc: t.desc, debit: t.amount}, jLine{account: "102.01", desc: t.desc, credit: t.amount})
			}
		}
		if len(bl) > 0 {
			m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "Banka hareketleri " + mon, docType: "banka",
				status: st, source: "bank_transactions", lines: bl})
		}
		// KDV mahsup: closed months only.
		if p != w.periods[last] {
			avail := in + carry
			lines := []jLine{{account: "391.01", desc: "KDV mahsubu", debit: out}, {account: "191.01", desc: "KDV mahsubu", credit: in}}
			if carry > 0 {
				lines = append(lines, jLine{account: "190.01", desc: "Önceki dönem devreden KDV", credit: carry})
			}
			if out >= avail {
				if out-avail > 0 {
					lines = append(lines, jLine{account: "360.01", desc: "Ödenecek KDV", credit: out - avail})
				}
				carry = 0
			} else {
				carry = avail - out
				lines = append(lines, jLine{account: "190.01", desc: "Sonraki döneme devreden KDV", debit: carry})
			}
			// The bank pays the filed amount; any rounding gap between model
			// and filed return goes to 360.01 / 689 so the books balance.
			if f, ok := tax.kdv[p]; ok && out >= avail && f != out-avail {
				diff := f - (out - avail)
				if diff > 0 {
					lines = append(lines, jLine{account: "689.01", desc: "KDV beyan farkı", debit: diff}, jLine{account: "360.01", desc: "KDV beyan farkı", credit: diff})
				} else {
					lines = append(lines, jLine{account: "360.01", desc: "KDV beyan farkı", debit: -diff}, jLine{account: "679.01", desc: "KDV beyan farkı", credit: -diff})
				}
			}
			if out > 0 || in > 0 {
				m.vouchers = append(m.vouchers, voucher{period: p, date: pe, desc: "KDV mahsup fişi " + mon, docType: "kdv",
					status: st, source: "manual", lines: dropZero(lines)})
			}
		}
	}
}

func dropZero(ls []jLine) []jLine {
	out := ls[:0]
	for _, l := range ls {
		if l.debit != 0 || l.credit != 0 {
			out = append(out, l)
		}
	}
	return out
}

var accountNames = map[string]string{
	"100": "KASA", "100.01": "MERKEZ KASA TL", "102": "BANKALAR", "102.01": "BANKA TL VADESİZ", "102.02": "BANKA TL VADELİ",
	"108": "DİĞER HAZIR DEĞERLER", "108.01": "KREDİ KARTI SLİP ALACAKLARI", "120": "ALICILAR", "120.01": "YURTİÇİ ALICILAR",
	"150": "İLK MADDE VE MALZEME", "150.01": "GIDA MALZEMESİ", "153": "TİCARİ MALLAR", "153.01": "TİCARİ MALLAR", "621": "SATILAN TİCARİ MALLAR MALİYETİ", "621.01": "SATILAN TİCARİ MALLAR MALİYETİ",
	"190": "DEVREDEN KDV", "190.01": "DEVREDEN KDV", "191": "İNDİRİLECEK KDV", "191.01": "İNDİRİLECEK KDV",
	"300": "BANKA KREDİLERİ", "300.01": "TİCARİ KREDİLER", "320": "SATICILAR", "320.01": "YURTİÇİ SATICILAR",
	"331": "ORTAKLARA BORÇLAR", "331.01": "ORTAKLARA BORÇLAR", "340": "ALINAN SİPARİŞ AVANSLARI", "340.01": "ALINAN SİPARİŞ AVANSLARI",
	"335": "PERSONELE BORÇLAR", "335.01": "PERSONELE BORÇLAR", "360": "ÖDENECEK VERGİ VE FONLAR",
	"360.01": "ÖDENECEK KDV", "360.02": "ÖDENECEK GELİR VE DAMGA VERGİSİ", "361": "ÖDENECEK SOSYAL GÜVENLİK KESİNTİLERİ",
	"361.01": "ÖDENECEK SGK PRİMLERİ", "391": "HESAPLANAN KDV", "391.01": "HESAPLANAN KDV", "500": "SERMAYE", "500.01": "SERMAYE",
	"600": "YURTİÇİ SATIŞLAR", "600.01": "YURTİÇİ SATIŞLAR", "601": "YURTDIŞI SATIŞLAR", "601.01": "YURTDIŞI SATIŞLAR",
	"610": "SATIŞTAN İADELER", "610.01": "SATIŞTAN İADELER", "642": "FAİZ GELİRLERİ", "642.01": "MEVDUAT FAİZ GELİRLERİ", "646": "KAMBİYO KARLARI", "646.01": "KAMBİYO KARLARI", "656": "KAMBİYO ZARARLARI", "656.01": "KAMBİYO ZARARLARI",
	"679": "DİĞER OLAĞANDIŞI GELİR VE KARLAR", "679.01": "DİĞER OLAĞANDIŞI GELİRLER",
	"689": "DİĞER OLAĞANDIŞI GİDER VE ZARARLAR", "689.01": "DİĞER OLAĞANDIŞI GİDERLER",
	"740": "HİZMET ÜRETİM MALİYETİ", "740.01": "HİZMET ÜRETİM MALİYETİ", "770": "GENEL YÖNETİM GİDERLERİ",
	"770.01": "GENEL GİDERLER", "770.02": "PERSONEL GİDERLERİ", "770.03": "MASRAF FİŞLERİ",
	"780": "FİNANSMAN GİDERLERİ", "780.01": "BANKA MASRAFLARI",
}

// mizanRow is one trial balance line (cumulative up to the period).
type mizanRow struct {
	code, name    string
	debit, credit kurus
	level         int
	leaf          bool
}

// mizan returns the cumulative trial balance at the end of p: every
// exported voucher up to p, plus extra (Luca-only) entries.
func (m *clientModel) mizan(p string, extra []jLine) []mizanRow {
	sum := map[string][2]kurus{}
	add := func(l jLine) {
		parts := strings.Split(l.account, ".")
		for i := range parts {
			c := strings.Join(parts[:i+1], ".")
			s := sum[c]
			s[0] += l.debit
			s[1] += l.credit
			sum[c] = s
		}
	}
	for _, v := range m.vouchers {
		if v.period <= p && v.status == "exported" {
			for _, l := range v.lines {
				add(l)
			}
		}
	}
	for _, l := range extra {
		add(l)
	}
	codes := make([]string, 0, len(sum))
	for c := range sum {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	out := make([]mizanRow, 0, len(codes))
	for i, c := range codes {
		leaf := i+1 >= len(codes) || !strings.HasPrefix(codes[i+1], c+".")
		name := accountNames[c]
		if name == "" {
			name = "HESAP " + c
		}
		out = append(out, mizanRow{code: c, name: name, debit: sum[c][0], credit: sum[c][1], level: strings.Count(c, ".") + 1, leaf: leaf})
	}
	return out
}

// lucaManual: the problem clients have one entry made in Luca by hand last
// month (kur farkı), so the reconciliation shows an open difference on 120.
func (m *clientModel) lucaManual(w *bigWorld, p string) []jLine {
	if !m.c.Problem || indexOf(w.periods, p) < len(w.periods)-2 {
		return nil
	}
	amt := kr(750 + float64(m.c.Index%7)*415)
	return []jLine{{account: "120.01", desc: "Kur farkı (Luca)", debit: amt}, {account: "646.01", desc: "Kur farkı (Luca)", credit: amt}}
}
