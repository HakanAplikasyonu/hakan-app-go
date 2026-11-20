package taxreturns

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Export writes the return as an .xlsx workbook in the form's order, for
// re-keying or importing into the accounting program (Luca / Zirve "Excel'den
// beyanname aktarımı"): one sheet per form part, field names as on the GİB
// form, amounts as numbers. A prepared or filed return exports its snapshot;
// a draft exports the live preview and says so in the title row.
func (s *Service) Export(ctx context.Context, sc engagementctx.Scope, kind, period string) ([]byte, string, error) {
	p, err := s.Preview(ctx, sc, kind, period)
	if err != nil {
		return nil, "", err
	}
	data, err := Workbook(p)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("%s-%s.xlsx", kind, period), nil
}

// Workbook renders a preview as .xlsx.
func Workbook(p Preview) ([]byte, error) {
	w := newBook()
	defer func() { _ = w.f.Close() }()
	switch {
	case p.KDV1 != nil:
		w.kdv1(p, *p.KDV1)
	case p.MUHSGK != nil:
		w.muhsgk(p, *p.MUHSGK)
	case p.BABS != nil:
		w.babs(p, *p.BABS)
	}
	w.checklist(p)
	w.sections(p)
	if err := w.f.DeleteSheet("Sheet1"); err != nil {
		return nil, err
	}
	w.f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := w.f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Titles of the kinds and statuses in the workbook.
var (
	kindTitles   = map[string]string{KindKDV1: "1 No.lu KDV Beyannamesi", KindMUHSGK: "Muhtasar ve Prim Hizmet Beyannamesi", KindBABS: "Form Ba / Bs"}
	statusTitles = map[string]string{StatusDraft: "Taslak (hazırlanmadı, güncel veriden)", StatusPrepared: "Hazırlandı", StatusFiled: "Verildi"}
	groupTitles  = map[string]string{
		ExemptionFull: "Tam istisna", ExemptionPartial: "Kısmi istisna",
		ExemptionExportRegistered: "İhraç kaydıyla teslim", ExemptionOther: "Diğer istisna",
	}
	withholdingTitles = map[string]string{
		WithholdingWage: "Ücret ödemeleri", WithholdingSelfEmployed: "Serbest meslek ödemeleri",
		WithholdingAgricultural: "Zirai kazanç (müstahsil)", WithholdingOther: "Diğer stopajlı ödemeler",
	}
	carryTitles = map[string]string{
		CarryManual: "elle girildi", CarryPrevious: "önceki dönem beyanından",
		CarryOpening: "açılış devrinden", CarryNone: "bilinmiyor (0 alındı)",
	}
)

type book struct {
	f                     *excelize.File
	head, bold, money, ti int
	sheet                 string
	row                   int
	// sheets records every written cell too, so the same form renders as
	// PDF (summaryPDF).
	sheets []*recSheet
}

func newBook() *book {
	f := excelize.NewFile()
	b := &book{f: f}
	b.head, _ = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#EEF2F7"}, Pattern: 1}})
	b.bold, _ = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, CustomNumFmt: ptr("#,##0.00")})
	b.money, _ = f.NewStyle(&excelize.Style{CustomNumFmt: ptr("#,##0.00")})
	b.ti, _ = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}})
	return b
}

func ptr[T any](v T) *T { return &v }

func (b *book) newSheet(name string, widths ...float64) {
	_, _ = b.f.NewSheet(name)
	b.sheet, b.row = name, 1
	b.sheets = append(b.sheets, &recSheet{name: name, widths: widths, cells: map[int]map[int]recCell{}})
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = b.f.SetColWidth(name, col, col, w)
	}
}

func (b *book) title(p Preview, sub string) {
	b.cell(1, kindTitles[p.Kind]+" — "+p.Period+sub, b.ti)
	b.row++
	b.cell(1, "Durum: "+statusTitles[p.Status]+" · Son gün: "+p.Calendar.DueDate, 0)
	if p.Record != nil && p.Record.AccrualNumber != nil {
		b.cell(3, "Tahakkuk fiş no: "+*p.Record.AccrualNumber, 0)
	}
	b.row += 2
}

func (b *book) cell(col int, v any, style int) {
	ref, _ := excelize.CoordinatesToCellName(col, b.row)
	switch t := v.(type) {
	case Money:
		f, _ := t.Dec().Float64()
		_ = b.f.SetCellValue(b.sheet, ref, f)
		if style == 0 {
			style = b.money
		}
	default:
		_ = b.f.SetCellValue(b.sheet, ref, v)
	}
	if style != 0 {
		_ = b.f.SetCellStyle(b.sheet, ref, ref, style)
	}
	b.record(col, v, style)
}

// line writes one row: the values from column 1.
func (b *book) line(style int, vals ...any) {
	for i, v := range vals {
		s := style
		if _, ok := v.(Money); ok && style == b.head {
			s = b.bold
		}
		b.cell(i+1, v, s)
	}
	b.row++
}

func (b *book) section(name string, headers ...any) {
	b.row++
	b.cell(1, name, b.ti)
	b.row++
	if len(headers) > 0 {
		b.line(b.head, headers...)
	}
}

func rateLabel(r string) string {
	if r == "" {
		return "Bilinmiyor"
	}
	return "%" + r
}

func (b *book) kdv1(p Preview, k KDV1) {
	b.newSheet("KDV1", 44, 16, 16, 16, 16, 16, 16)
	b.title(p, "")
	t := k.Totals

	b.section("Matrah — teslim ve hizmetler", "KDV oranı", "Matrah", "Vergi", "Fatura matrahı", "Z raporu matrahı", "Alıştan iade matrahı")
	for _, s := range k.Sales {
		b.line(0, rateLabel(s.Rate), s.Base, s.VAT, s.InvoiceBase, s.RetailBase, s.PurchaseReturnBase)
	}
	if len(k.Withholdings) > 0 {
		b.section("Kısmi tevkifat uygulanan işlemler", "İşlem türü (kod)", "Matrah", "KDV oranı", "Tevkifat oranı", "Hesaplanan KDV", "Tevkif edilen", "Beyan edilen vergi")
		for _, w := range k.Withholdings {
			b.line(0, w.Code, w.Base, rateLabel(w.Rate), w.Ratio, w.VAT, w.Withheld, w.DeclaredVAT)
		}
	}
	if len(k.SpecialBases) > 0 {
		b.section("Özel matrah şekline tabi işlemler", "Kod", "Matrah", "KDV oranı", "Vergi")
		for _, s := range k.SpecialBases {
			b.line(0, s.Code, s.Base, rateLabel(s.Rate), s.VAT)
		}
	}
	b.section("Matrah toplamı ve hesaplanan KDV")
	b.line(0, "Matrah toplamı", t.BaseTotal)
	b.line(0, "Hesaplanan KDV", t.CalculatedVAT)
	b.line(0, "İlave edilecek KDV", t.AdditionalVAT)
	b.line(b.head, "Toplam katma değer vergisi", t.TotalVAT)

	b.section("İndirimler")
	b.line(0, "Önceki dönemden devreden indirilecek KDV ("+carryTitles[k.Carry.Source]+")", t.CarriedIn)
	b.line(0, "Bu döneme ait indirilecek KDV", t.CurrentInput)
	b.line(0, "Satıştan iade edilen mallar nedeniyle indirilecek KDV", t.SalesReturnsVAT)
	b.line(0, "Sorumlu sıfatıyla beyan edilen KDV (KDV2)", t.ResponsibleVAT)
	b.line(0, "Diğer indirimler", t.OtherDeductions)
	b.line(b.head, "İndirimler toplamı", t.TotalDeductions)
	b.section("Bu döneme ait indirilecek KDV — oran kırılımı", "KDV oranı", "Matrah", "Vergi", "Fatura KDV'si", "Masraf fişi KDV'si", "Fatura adedi", "Fiş adedi")
	for _, x := range k.Purchases {
		b.line(0, rateLabel(x.Rate), x.Base, x.VAT, x.InvoiceVAT, x.ExpenseVAT, x.InvoiceCount, x.ExpenseCount)
	}

	if len(k.Exemptions) > 0 {
		b.section("İstisnalar", "Grup", "İşlem türü (kod)", "Teslim bedeli", "Tecil edilecek KDV", "Fatura adedi")
		for _, e := range k.Exemptions {
			vat := any("")
			if e.Group == ExemptionExportRegistered {
				vat = e.VAT
			}
			b.line(0, groupTitles[e.Group], e.Code, e.Base, vat, e.InvoiceCount)
		}
	}

	b.section("Sonuç hesapları")
	b.line(0, "Tecil edilecek katma değer vergisi", t.DeferredVAT)
	b.line(b.head, "Ödenmesi gereken katma değer vergisi", t.Payable)
	b.line(b.head, "Sonraki döneme devreden katma değer vergisi", t.CarryOut)

	b.section("Ek bilgiler")
	b.line(0, "Teslim ve hizmetlerin karşılığını teşkil eden bedel (aylık)", t.DeliveryTotal)
	b.line(0, "Kredi kartı ile tahsil edilen teslim ve hizmet bedeli (KDV dahil)", t.CardSales)
}

func (b *book) muhsgk(p Preview, m MUHSGK) {
	b.newSheet("MUHSGK", 40, 18, 16, 16, 16)
	b.title(p, "")
	b.section("Vergi bildirimi", "Ödemenin türü", "Gayrisafi tutar / matrah", "Kesinti oranı", "Kesilen vergi", "Belge / kişi sayısı")
	for _, w := range m.Withholdings {
		rate := any("")
		if w.Rate != "" && w.Rate != "0" {
			rate = "%" + w.Rate
		}
		b.line(0, withholdingTitles[w.Kind], w.Base, rate, w.Tax, w.Count)
	}
	b.line(b.head, "Gelir vergisi toplamı", "", "", m.Totals.IncomeTax)
	b.line(b.head, "Damga vergisi (ücret)", "", "", m.Totals.StampTax)
	b.line(b.head, "Ödenecek vergi toplamı", "", "", m.Totals.TaxPayable)

	if pr := m.Payroll; pr != nil {
		b.section("Bordro toplamları")
		b.line(0, "Çalışan sayısı", pr.EmployeeCount)
		b.line(0, "Prim günü toplamı", pr.DaysTotal)
		b.line(0, "Brüt ücret", pr.Gross)
		b.line(0, "Gelir vergisi matrahı", pr.IncomeTaxBase)
		b.line(0, "Gelir vergisi istisnası", pr.IncomeTaxExemption)
		b.line(0, "Ödenecek gelir vergisi", pr.IncomeTax)
		b.line(0, "Damga vergisi istisnası", pr.StampTaxExemption)
		b.line(0, "Ödenecek damga vergisi", pr.StampTax)
		b.line(0, "SGK prime esas kazanç", pr.SGKBase)
		b.line(0, "SGK işçi payı", pr.SGKEmployee)
		b.line(0, "İşsizlik sigortası işçi payı", pr.UnemploymentEmployee)
		b.line(0, "SGK işveren payı", pr.SGKEmployer)
		b.line(0, "İşsizlik sigortası işveren payı", pr.UnemploymentEmployer)
		b.line(0, "SGK teşvik tutarı", pr.SGKIncentive)
	}
	b.section("SGK prim bildirimi")
	pm := m.Premium
	b.line(0, "Bordrodan prim toplamı", pm.Expected)
	b.line(0, "Bordrodan teşvik", pm.ExpectedIncentive)
	if pm.Accrual != nil {
		b.line(0, "Tahakkuk (girilen)", *pm.Accrual)
		b.line(0, "Teşvik (girilen)", *pm.Incentive)
		b.line(0, "Fark (tahakkuk − bordro)", *pm.Difference)
	}
	b.line(b.head, "Ödenecek prim", m.Totals.PremiumPayable)

	b.newSheet("Çalışanlar", 6, 28, 14, 14, 12, 12, 8, 16, 16, 16, 16, 16, 16, 16)
	b.line(b.head, "Sıra", "Adı Soyadı", "TC Kimlik No (maskeli)", "SGK sicil no", "Giriş", "Çıkış", "Gün",
		"Brüt ücret", "SGK matrahı", "GV matrahı", "GV istisnası", "Ödenecek GV", "DV istisnası", "Ödenecek DV")
	for _, e := range m.Employees {
		days := any("")
		if e.DaysWorked != nil {
			days = *e.DaysWorked
		}
		b.line(0, e.LineNo, e.Name, e.NationalIDMasked, e.SGKNo, e.HireDate, e.ExitDate, days,
			e.Gross, e.SGKBase, e.IncomeTaxBase, e.IncomeTaxExemption, e.IncomeTax, e.StampTaxExemption, e.StampTax)
	}
}

// babs writes Form Ba and Bs in the GİB column order: sıra, unvan, ülke,
// VKN, TCKN, belge sayısı, mal ve hizmet toplam bedeli (KDV hariç).
func (b *book) babs(p Preview, x BABS) {
	for _, f := range []struct {
		name string
		form BABSForm
	}{{"Form Ba", x.BA}, {"Form Bs", x.BS}} {
		b.newSheet(f.name, 6, 40, 12, 14, 14, 12, 22)
		b.title(p, " · "+f.name)
		b.line(b.head, "Sıra", "Adı soyadı / unvanı", "Ülkesi", "Vergi kimlik no", "TC kimlik no", "Belge sayısı", "Mal ve hizmet toplam bedeli")
		for i, r := range f.form.Rows {
			vkn, tckn := "", ""
			if r.IDType == "tckn" {
				tckn = r.CounterpartyID
			} else {
				vkn = r.CounterpartyID
			}
			b.line(0, i+1, r.Name, "TÜRKİYE", vkn, tckn, r.DocumentCount, r.Amount)
		}
		b.line(b.head, "", "Toplam", "", "", "", f.form.DocumentCount, f.form.Amount)
		b.row++
		b.cell(2, "Bildirim sınırı (KDV hariç): "+x.Threshold.Dec().StringFixed(2)+" TL · sınırın altında kalan: "+strconv.FormatInt(f.form.BelowThreshold, 10), 0)
	}
}

var severityTitles = map[string]string{SeverityWarning: "Uyarı", SeverityInfo: "Bilgi"}

func (b *book) checklist(p Preview) {
	b.newSheet("Kontrol listesi", 12, 90, 10, 16, 10)
	b.line(b.head, "Önem", "Madde", "Adet", "Tutar", "Onaylandı")
	for _, c := range p.Checklist {
		amount := any("")
		if c.Amount != nil {
			amount = *c.Amount
		}
		ok := "—"
		if c.Severity == SeverityWarning {
			ok = "Hayır"
			if c.Acknowledged {
				ok = "Evet"
			}
		}
		b.line(0, severityTitles[c.Severity], c.Message, c.Count, amount, ok)
	}
	if len(p.Checklist) == 0 {
		b.line(0, "", "Kontrol listesinde madde yok.")
	}
}

// sectionTitles: the period's sources in the section summary (#710).
var sectionTitles = map[string]string{
	"retail": "Z raporları", "invoices": "Faturalar", "bank": "Banka hareketleri", "expenses": "KDV'li fişler",
}

// sections is the period's section summary (#710): per source how many
// records were processed for the return, set aside and still waiting. It
// is always live, also for a prepared or filed return.
func (b *book) sections(p Preview) {
	if len(p.Sections) == 0 {
		return
	}
	b.newSheet("Bölüm özeti", 40, 14, 14, 14, 14)
	b.line(b.head, "Bölüm", "Toplam", "İşlenen", "Yok sayılan", "Bekleyen")
	for _, x := range p.Sections {
		title := sectionTitles[x.Key]
		if title == "" {
			title = x.Key
		}
		b.line(0, title, x.Total, x.Processed, x.Ignored, max(x.Total-x.Processed-x.Ignored, 0))
	}
}
