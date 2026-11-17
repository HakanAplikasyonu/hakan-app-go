package reports

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// PDF colours. The series colours are the web report's validated
// categorical palette (light mode): revenue, expenses, net.
var (
	colBrand   = rgb{36, 72, 255}
	colRevenue = rgb{42, 120, 214}
	colExpense = rgb{235, 104, 52}
	colNet     = rgb{27, 175, 122}
	colText    = rgb{17, 17, 22}
	colMuted   = rgb{90, 90, 102}
	colLine    = rgb{227, 227, 233}
	colPanel   = rgb{248, 248, 250}
	colWhite   = rgb{255, 255, 255}
)

type rgb struct{ r, g, b int }

const (
	pageMargin = 15.0
	pageWidth  = 210.0
	contentW   = pageWidth - 2*pageMargin
)

// PDFOptions brand the document.
type PDFOptions struct {
	// OfficeName is printed in the header band and the footer.
	OfficeName string
	// Now is the print time (Europe/Istanbul is used for the date).
	Now time.Time
}

type pdfDoc struct {
	*gofpdf.Fpdf
}

func (d pdfDoc) font(style string, size float64) {
	d.SetFont(ioengine.PDFFontFamily, style, size)
}
func (d pdfDoc) text(c rgb) { d.SetTextColor(c.r, c.g, c.b) }
func (d pdfDoc) fill(c rgb) { d.SetFillColor(c.r, c.g, c.b) }
func (d pdfDoc) draw(c rgb) { d.SetDrawColor(c.r, c.g, c.b) }

// RenderPDF prints a report on A4: header, four totals, the four charts,
// the comment (approved only), the monthly table and the notes. Turkish
// number format throughout.
func RenderPDF(r Report, opt PDFOptions) ([]byte, error) {
	f := gofpdf.New("P", "mm", "A4", "")
	if err := ioengine.RegisterPDFFonts(f); err != nil {
		return nil, err
	}
	d := pdfDoc{f}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	office := strings.TrimSpace(opt.OfficeName)
	title := fmt.Sprintf("%s — %s raporu", r.Taxpayer.Title, r.Label)
	d.SetTitle(title, true)
	d.SetAuthor(office, true)
	d.SetCreator("HAKAN mükellef raporu", true)
	d.SetMargins(pageMargin, pageMargin, pageMargin)
	d.SetAutoPageBreak(true, 18)
	d.AliasNbPages("{nb}")
	d.SetFooterFunc(func() {
		d.SetY(-12)
		d.font("", 7)
		d.text(colMuted)
		left := "HAKAN ile hazırlandı"
		if office != "" {
			left = office + " · " + left
		}
		d.CellFormat(contentW/2, 4, left, "", 0, "L", false, 0, "")
		d.CellFormat(contentW/2, 4, fmt.Sprintf("Sayfa %d/{nb}", d.PageNo()), "", 0, "R", false, 0, "")
	})
	d.AddPage()

	// Header band.
	d.fill(colBrand)
	d.Rect(0, 0, pageWidth, 24, "F")
	d.text(colWhite)
	d.SetXY(pageMargin, 7)
	d.font("B", 13)
	name := office
	if name == "" {
		name = "Mali müşavirlik"
	}
	d.CellFormat(contentW*0.62, 6, name, "", 0, "L", false, 0, "")
	d.font("", 9)
	kind := "Aylık rapor"
	if r.Kind == KindYear {
		kind = "Yıllık rapor"
	}
	d.CellFormat(contentW*0.38, 6, "Mükellef raporu · "+kind, "", 1, "R", false, 0, "")
	d.SetX(pageMargin)
	d.font("", 8)
	d.CellFormat(contentW, 5, "Hazırlanma: "+trDate(opt.Now), "", 1, "R", false, 0, "")

	// Taxpayer and period.
	d.SetY(30)
	d.text(colText)
	d.font("B", 14)
	d.MultiCell(contentW, 6.5, r.Taxpayer.Title, "", "L", false)
	d.font("", 9)
	d.text(colMuted)
	d.CellFormat(contentW, 5, fmt.Sprintf("VKN/TCKN %s · Dönem: %s · Tutarlar KDV hariç, TL", r.Taxpayer.TaxID, r.Label), "", 1, "L", false, 0, "")

	// Totals.
	y := d.GetY() + 3
	prevLabel := "önceki aya göre"
	if r.Kind == KindYear {
		prevLabel = "önceki yıla göre"
	}
	vat := dec0(r.Totals.VATPayable)
	vatLabel := "Ödenecek KDV (tahmini)"
	if vat.IsNegative() {
		vatLabel = "Devreden KDV (tahmini)"
	}
	kpis := []struct{ label, value, sub string }{
		{"Ciro", TRMoney(r.Totals.Revenue), changeText(r.Change.Revenue, prevLabel)},
		{"Gider", TRMoney(r.Totals.Expenses), changeText(r.Change.Expenses, prevLabel)},
		{"Net (ciro − gider)", TRMoney(r.Totals.Net), changeText(r.Change.Net, prevLabel)},
		{vatLabel, TRMoney(vat.Abs().StringFixed(2)), "Önceki dönem devri hariç"},
	}
	boxW := (contentW - 3*3) / 4
	for i, k := range kpis {
		x := pageMargin + float64(i)*(boxW+3)
		d.fill(colPanel)
		d.draw(colLine)
		d.SetLineWidth(0.2)
		d.RoundedRect(x, y, boxW, 21, 2, "1234", "FD")
		d.SetXY(x+3, y+2.5)
		d.font("", 7.5)
		d.text(colMuted)
		d.CellFormat(boxW-6, 4, k.label, "", 2, "L", false, 0, "")
		d.SetX(x + 3)
		d.font("B", 11.5)
		d.text(colText)
		d.CellFormat(boxW-6, 7, k.value, "", 2, "L", false, 0, "")
		d.SetX(x + 3)
		d.font("", 7)
		d.text(colMuted)
		d.CellFormat(boxW-6, 4, k.sub, "", 0, "L", false, 0, "")
	}
	y += 25

	// Four charts, 2 x 2.
	cw, ch := (contentW-4)/2, 70.0
	months := r.Months
	labels := make([]string, len(months))
	rev, exp, net, vc, vd := make([]float64, len(months)), make([]float64, len(months)), make([]float64, len(months)), make([]float64, len(months)), make([]float64, len(months))
	for i, m := range months {
		labels[i] = m.Label
		rev[i], exp[i], net[i] = flt(m.Revenue), flt(m.Expenses), flt(m.Net)
		vc[i], vd[i] = flt(m.VATCalculated), flt(m.VATDeductible)
	}
	d.lineChart(pageMargin, y, cw, ch, "Gelir-gider trendi", "Aylık ciro, gider ve net", labels,
		[]series{{"Ciro", rev, colRevenue, false}, {"Gider", exp, colExpense, false}, {"Net", net, colNet, true}})
	d.barChart(pageMargin+cw+4, y, cw, ch, "Aylık ciro", "Satışlar (iadeler düşülmüş) ve Z raporları", labels,
		[]series{{"Ciro", rev, colRevenue, false}}, false)
	y += ch + 4
	d.categoryChart(pageMargin, y, cw, ch, r)
	d.barChart(pageMargin+cw+4, y, cw, ch, "KDV", "Aylık hesaplanan ve indirilecek KDV", labels,
		[]series{{"Hesaplanan", vc, colRevenue, false}, {"İndirilecek", vd, colExpense, false}}, true)
	d.SetY(y + ch + 5)

	// The comment: only an approved one reaches the taxpayer.
	if r.Commentary != nil && r.Commentary.Status == "approved" && strings.TrimSpace(r.Commentary.Body) != "" {
		d.sectionTitle("Müşavir yorumu")
		d.font("", 9.5)
		d.text(colText)
		for _, p := range strings.Split(r.Commentary.Body, "\n") {
			if strings.TrimSpace(p) == "" {
				d.Ln(1.5)
				continue
			}
			d.MultiCell(contentW, 5, strings.TrimSpace(p), "", "J", false)
		}
		d.Ln(3)
	}

	d.monthTable(r)
	d.Ln(3)
	d.font("", 7.5)
	d.text(colMuted)
	for _, n := range r.Notes {
		d.MultiCell(contentW, 3.8, "• "+n, "", "L", false)
	}
	if err := d.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := d.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type series struct {
	name   string
	values []float64
	color  rgb
	dashed bool
}

func (d pdfDoc) sectionTitle(t string) {
	if d.GetY() > 260 {
		d.AddPage()
	}
	d.font("B", 11)
	d.text(colText)
	d.CellFormat(contentW, 7, t, "", 1, "L", false, 0, "")
}

// frame draws a chart card and returns the plot area.
func (d pdfDoc) frame(x, y, w, h float64, title, sub string) (px, py, pw, ph float64) {
	d.fill(colWhite)
	d.draw(colLine)
	d.SetLineWidth(0.2)
	d.RoundedRect(x, y, w, h, 2, "1234", "FD")
	d.SetXY(x+3, y+2.5)
	d.font("B", 9.5)
	d.text(colText)
	d.CellFormat(w-6, 4.5, title, "", 2, "L", false, 0, "")
	d.SetX(x + 3)
	d.font("", 7)
	d.text(colMuted)
	d.CellFormat(w-6, 3.5, sub, "", 0, "L", false, 0, "")
	return x + 14, y + 14, w - 17, h - 26
}

// scale returns a rounded axis [lo, hi] with 4 steps.
func scale(vals ...[]float64) (lo, hi, step float64) {
	for _, vs := range vals {
		for _, v := range vs {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if hi == lo {
		hi = lo + 1
	}
	raw := (hi - lo) / 4
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*mag >= raw {
			step = m * mag
			break
		}
	}
	lo = math.Floor(lo/step) * step
	hi = math.Ceil(hi/step) * step
	return lo, hi, step
}

func (d pdfDoc) axes(px, py, pw, ph, lo, hi, step float64, labels []string) func(float64) float64 {
	yOf := func(v float64) float64 { return py + ph - (v-lo)/(hi-lo)*ph }
	d.SetLineWidth(0.15)
	d.font("", 6)
	for v := lo; v <= hi+step/2; v += step {
		yy := yOf(v)
		d.draw(colLine)
		d.Line(px, yy, px+pw, yy)
		d.text(colMuted)
		d.SetXY(px-12, yy-1.5)
		d.CellFormat(11, 3, TRCompact(v), "", 0, "R", false, 0, "")
	}
	if n := len(labels); n > 0 {
		every := 1
		if n > 6 {
			every = 2
		}
		slot := pw / float64(n)
		for i, l := range labels {
			if (n-1-i)%every != 0 {
				continue
			}
			d.SetXY(px+float64(i)*slot, py+ph+1)
			d.CellFormat(slot, 3, l, "", 0, "C", false, 0, "")
		}
	}
	return yOf
}

func (d pdfDoc) legend(x, y float64, ss []series) {
	d.font("", 6.5)
	for _, s := range ss {
		d.fill(s.color)
		d.Rect(x, y+0.6, 2.2, 2.2, "F")
		d.text(colMuted)
		d.SetXY(x+3, y)
		w := d.GetStringWidth(s.name) + 2
		d.CellFormat(w, 3.4, s.name, "", 0, "L", false, 0, "")
		x += w + 5
	}
}

func (d pdfDoc) lineChart(x, y, w, h float64, title, sub string, labels []string, ss []series) {
	px, py, pw, ph := d.frame(x, y, w, h, title, sub)
	vals := make([][]float64, 0, len(ss))
	for _, s := range ss {
		vals = append(vals, s.values)
	}
	lo, hi, step := scale(vals...)
	yOf := d.axes(px, py, pw, ph, lo, hi, step, labels)
	slot := pw / float64(max(1, len(labels)))
	for _, s := range ss {
		d.draw(s.color)
		d.SetLineWidth(0.6)
		if s.dashed {
			d.SetDashPattern([]float64{1.4, 1}, 0)
		}
		for i := 1; i < len(s.values); i++ {
			d.Line(px+slot*(float64(i)-0.5), yOf(s.values[i-1]), px+slot*(float64(i)+0.5), yOf(s.values[i]))
		}
		d.SetDashPattern([]float64{}, 0)
	}
	d.legend(x+3, y+h-6, ss)
}

func (d pdfDoc) barChart(x, y, w, h float64, title, sub string, labels []string, ss []series, withLegend bool) {
	px, py, pw, ph := d.frame(x, y, w, h, title, sub)
	vals := make([][]float64, 0, len(ss))
	for _, s := range ss {
		vals = append(vals, s.values)
	}
	lo, hi, step := scale(vals...)
	yOf := d.axes(px, py, pw, ph, lo, hi, step, labels)
	slot := pw / float64(max(1, len(labels)))
	gap := 0.4
	barW := math.Min(5, (slot*0.7-gap*float64(len(ss)-1))/float64(len(ss)))
	group := barW*float64(len(ss)) + gap*float64(len(ss)-1)
	for si, s := range ss {
		d.fill(s.color)
		for i, v := range s.values {
			bx := px + slot*float64(i) + (slot-group)/2 + float64(si)*(barW+gap)
			top, base := yOf(math.Max(v, 0)), yOf(math.Min(v, 0))
			if base-top > 0.05 {
				d.Rect(bx, top, barW, base-top, "F")
			}
		}
	}
	if withLegend {
		d.legend(x+3, y+h-6, ss)
	}
}

func (d pdfDoc) categoryChart(x, y, w, h float64, r Report) {
	px, py, pw, ph := d.frame(x, y, w, h, "Gider kategorileri", r.Label+" giderlerinin dağılımı")
	cats := r.Categories
	if len(cats) == 0 {
		d.font("", 8)
		d.text(colMuted)
		d.SetXY(px, py+ph/2-2)
		d.CellFormat(pw, 4, "Bu dönemde gider kaydı yok.", "", 0, "C", false, 0, "")
		return
	}
	if len(cats) > 6 {
		rest := Category{Label: "Diğer"}
		sum, share := decimal.Zero, decimal.Zero
		for _, c := range cats[5:] {
			sum, share = sum.Add(dec0(c.Amount)), share.Add(dec0(c.Share))
		}
		rest.Amount, rest.Share = sum.StringFixed(2), share.StringFixed(1)
		cats = append(append([]Category{}, cats[:5]...), rest)
	}
	labelW := 30.0
	maxV := 0.0
	for _, c := range cats {
		maxV = math.Max(maxV, flt(c.Amount))
	}
	if maxV <= 0 {
		maxV = 1
	}
	row := (ph + 8) / float64(len(cats))
	barH := math.Min(5, row*0.6)
	barMax := pw - labelW + 10 - 14
	for i, c := range cats {
		ry := py - 2 + float64(i)*row
		d.font("", 6.8)
		d.text(colText)
		d.SetXY(px-11, ry+(row-3)/2)
		d.CellFormat(labelW-2, 3, fit(d, c.Label, labelW-2), "", 0, "R", false, 0, "")
		bw := math.Max(0.4, flt(c.Amount)/maxV*barMax)
		d.fill(colExpense)
		d.Rect(px-11+labelW, ry+(row-barH)/2, bw, barH, "F")
		d.text(colMuted)
		d.SetXY(px-11+labelW+bw+1, ry+(row-3)/2)
		d.CellFormat(14, 3, "%"+strings.Replace(c.Share, ".", ",", 1), "", 0, "L", false, 0, "")
	}
}

func (d pdfDoc) monthTable(r Report) {
	if d.GetY() > 230 {
		d.AddPage()
	}
	d.sectionTitle("Aylık tablo")
	cols := []string{"Ay", "Ciro", "Gider", "Net", "Hesaplanan KDV", "İndirilecek KDV"}
	widths := []float64{20, 32, 32, 32, 32, 32}
	d.font("B", 7.5)
	d.text(colMuted)
	d.draw(colLine)
	d.SetLineWidth(0.2)
	for i, c := range cols {
		align := "R"
		if i == 0 {
			align = "L"
		}
		d.CellFormat(widths[i], 5.5, c, "B", 0, align, false, 0, "")
	}
	d.Ln(-1)
	d.font("", 7.5)
	d.text(colText)
	for _, m := range r.Months {
		vals := []string{MonthName(m.Period), TRMoney(m.Revenue), TRMoney(m.Expenses), TRMoney(m.Net), TRMoney(m.VATCalculated), TRMoney(m.VATDeductible)}
		for i, v := range vals {
			align := "R"
			if i == 0 {
				align = "L"
			}
			d.CellFormat(widths[i], 5, v, "B", 0, align, false, 0, "")
		}
		d.Ln(-1)
	}
}

func fit(d pdfDoc, s string, w float64) string {
	if d.GetStringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && d.GetStringWidth(string(r)+"…") > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func changeText(p *string, label string) string {
	if p == nil {
		return ""
	}
	v := dec0(*p)
	sign := ""
	if v.IsPositive() {
		sign = "+"
	} else if v.IsNegative() {
		sign = "−"
	}
	return sign + "%" + strings.Replace(v.Abs().StringFixed(1), ".", ",", 1) + " " + label
}

// TRMoney formats a decimal string the Turkish way: "₺95.250,75".
func TRMoney(s string) string {
	v := dec0(s)
	neg := v.IsNegative()
	parts := strings.SplitN(v.Abs().StringFixed(2), ".", 2)
	in := parts[0]
	var b strings.Builder
	for i, c := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := "₺" + b.String() + "," + parts[1]
	if neg {
		out = "−" + out
	}
	return out
}

// TRCompact is an axis label: 1,2 Mn · 450 B · 900.
func TRCompact(v float64) string {
	a := math.Abs(v)
	sign := ""
	if v < 0 {
		sign = "−"
	}
	num := func(x float64) string {
		s := fmt.Sprintf("%.1f", x)
		s = strings.TrimSuffix(s, ".0")
		return strings.Replace(s, ".", ",", 1)
	}
	switch {
	case a >= 1e6:
		return sign + num(a/1e6) + " Mn"
	case a >= 1e3:
		return sign + num(a/1e3) + " B"
	default:
		return sign + num(a)
	}
}

var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*3600)
}()

func trDate(t time.Time) string {
	t = t.In(istanbul)
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

func dec0(s string) decimal.Decimal {
	v, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Zero
	}
	return v
}

func flt(s string) float64 {
	f, _ := dec0(s).Float64()
	return f
}
