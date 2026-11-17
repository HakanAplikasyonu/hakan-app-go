package payroll

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Import errors.
var (
	ErrNoPayrollSheet  = errors.New("payroll: no sheet looks like a payroll")
	ErrMappingMissing  = errors.New("payroll: required columns are not mapped")
	ErrPeriodRequired  = errors.New("payroll: period is not in the file; send period=YYYY-MM")
	ErrInvalidOverride = errors.New("payroll: column mapping is invalid")
)

// rule maps a header to a field when every token is a word of the
// normalized header (tokens of four or more letters may be a word prefix).
type rule struct {
	field  string
	tokens [][]string
	not    []string // a header with one of these words is not this field
}

var codeWords = []string{"no", "kod", "kodu", "numara", "numarasi", "sira", "sicil"}

// rules are tried in order; more specific ones first (işsizlik before SGK,
// matrah and istisna before the tax itself).
var rules = []rule{
	{"national_id", [][]string{{"tckn"}, {"tc"}, {"kimlik"}, {"tc", "no"}}, nil},
	{"sgk_no", [][]string{{"sicil"}, {"sgk", "no"}, {"ssk", "no"}}, nil},
	{"hire_date", [][]string{{"giris"}, {"ise", "giris"}}, nil},
	{"exit_date", [][]string{{"cikis"}, {"isten", "cikis"}}, nil},
	{"period", [][]string{{"donem"}, {"bordro", "ay"}}, nil},
	{"unemployment_employer", [][]string{{"issizlik", "isveren"}}, nil},
	{"unemployment_employee", [][]string{{"issizlik", "isci"}, {"issizlik"}}, nil},
	{"sgk_incentive", [][]string{{"tesvik"}}, nil},
	{"sgk_base", [][]string{{"sgk", "matrah"}, {"ssk", "matrah"}, {"prime", "esas"}, {"pek"}}, nil},
	{"sgk_employer", [][]string{{"isveren", "sgk"}, {"isveren", "ssk"}, {"isveren", "prim"}, {"isveren", "pay"}}, nil},
	{"sgk_employee", [][]string{{"isci", "sgk"}, {"isci", "ssk"}, {"isci", "prim"}, {"isci", "pay"}, {"sgk", "kesinti"}}, nil},
	{"income_tax_exemption", [][]string{{"gelir", "istisna"}, {"gv", "istisna"}, {"gelir", "vergisi", "istisna"}}, nil},
	{"stamp_tax_exemption", [][]string{{"damga", "istisna"}, {"dv", "istisna"}}, nil},
	{"income_tax_base", [][]string{{"gelir", "matrah"}, {"gv", "matrah"}}, nil},
	{"agi", [][]string{{"agi"}, {"asgari", "gecim"}}, nil},
	{"income_tax", [][]string{{"gelir", "vergi"}, {"gv"}}, nil},
	{"stamp_tax", [][]string{{"damga"}, {"dv"}}, nil},
	{"employer_cost", [][]string{{"maliyet"}, {"isveren", "toplam"}}, nil},
	{"net", [][]string{{"net"}, {"ele", "gecen"}, {"odenecek", "ucret"}}, nil},
	{"gross", [][]string{{"brut"}, {"toplam", "kazanc"}, {"kazanc", "toplam"}}, nil},
	{"other_deductions", [][]string{{"diger", "kesinti"}, {"ozel", "kesinti"}, {"avans"}, {"icra"}, {"sendika"}}, nil},
	{"employee_name", [][]string{{"ad", "soyad"}, {"adi", "soyadi"}, {"personel"}, {"calisan"}, {"isim"}, {"adsoyad"}}, codeWords},
	{"first_name", [][]string{{"adi"}, {"ad"}}, codeWords},
	{"last_name", [][]string{{"soyadi"}, {"soyad"}}, codeWords},
	{"job_title", [][]string{{"gorev"}, {"unvan"}, {"meslek"}}, nil},
	{"department", [][]string{{"bolum"}, {"departman"}}, nil},
	{"days_worked", [][]string{{"gun"}, {"gunu"}, {"prim", "gunu"}}, nil},
}

// Words that make a column a worse candidate for its field (a cumulative
// or calculated tax beside the payable one).
var weaker = []string{"kumulatif", "devreden", "hesaplanan", "onceki", "matrah"}

func words(h string) []string { return strings.Fields(parse.Normalize(h)) }

func hasToken(ws []string, tok string) bool {
	for _, w := range ws {
		if w == tok || (len(tok) >= 4 && strings.HasPrefix(w, tok)) {
			return true
		}
	}
	return false
}

func matchRule(ws []string) (string, bool) {
	for _, r := range rules {
		excluded := false
		for _, n := range r.not {
			if hasToken(ws, n) {
				excluded = true
			}
		}
		if excluded {
			continue
		}
		for _, alt := range r.tokens {
			ok := true
			for _, t := range alt {
				if !hasToken(ws, t) {
					ok = false
					break
				}
			}
			if ok {
				return r.field, true
			}
		}
	}
	return "", false
}

// HeuristicMapping maps the columns by header words. Two columns of one
// field: the one without "kümülatif / hesaplanan" wins.
func HeuristicMapping(l parse.Layout) mapping.Mapping {
	mp := mapping.Mapping{DataType: mapping.Payroll, HeaderRow: l.HeaderRow, SourceSystem: SourceGeneric}
	best := map[string]int{} // field -> column index in mp.Columns
	score := func(ws []string) int {
		s := 0
		for _, w := range weaker {
			if hasToken(ws, w) {
				s--
			}
		}
		if hasToken(ws, "odenecek") {
			s++
		}
		return s
	}
	for _, c := range l.Columns {
		cm := mapping.ColumnMap{Column: c.Letter, Header: c.Header, Origin: mapping.OriginTemplate}
		ws := words(c.Header)
		if f, ok := matchRule(ws); ok {
			if prev, dup := best[f]; dup {
				if score(ws) > score(words(mp.Columns[prev].Header)) {
					mp.Columns[prev].Field, mp.Columns[prev].Confidence = "", 0
					best[f] = len(mp.Columns)
					cm.Field, cm.Confidence = f, 0.9
				}
			} else {
				best[f] = len(mp.Columns)
				cm.Field, cm.Confidence = f, 0.9
			}
		}
		mp.Columns = append(mp.Columns, cm)
	}
	return mp
}

// Missing lists the unmet required fields: a name and gross and net.
func Missing(mp mapping.Mapping) []string {
	have := mp.FieldColumns()
	var out []string
	if have["employee_name"] == "" && (have["first_name"] == "" || have["last_name"] == "") {
		out = append(out, "employee_name")
	}
	for _, f := range []string{"gross", "net"} {
		if have[f] == "" {
			out = append(out, f)
		}
	}
	return out
}

// ApplyOverride replaces the field of the named columns (letter -> field,
// "" to unmap). The result may not map a field twice.
func ApplyOverride(mp mapping.Mapping, override map[string]string) (mapping.Mapping, error) {
	if len(override) == 0 {
		return mp, nil
	}
	cols := make([]mapping.ColumnMap, len(mp.Columns))
	copy(cols, mp.Columns)
	known := map[string]bool{}
	for i := range cols {
		known[cols[i].Column] = true
		if f, ok := override[cols[i].Column]; ok {
			if f != "" && !mapping.PayrollField(f) {
				return mp, fmt.Errorf("%w: unknown field %q", ErrInvalidOverride, f)
			}
			cols[i].Field, cols[i].Confidence, cols[i].Origin, cols[i].NeedsReview = f, 1, mapping.OriginUser, false
		}
	}
	for letter := range override {
		if !known[letter] {
			return mp, fmt.Errorf("%w: no column %s", ErrInvalidOverride, letter)
		}
	}
	seen := map[string]bool{}
	for _, c := range cols {
		if c.Field == "" {
			continue
		}
		if seen[c.Field] {
			return mp, fmt.Errorf("%w: field %q is mapped twice", ErrInvalidOverride, c.Field)
		}
		seen[c.Field] = true
	}
	mp.Columns = cols
	return mp, nil
}

// Parsed is a read payroll file.
type Parsed struct {
	Sheet        parse.Sheet
	Layout       parse.Layout
	Mapping      mapping.Mapping
	SourceSystem string
	Period       string // "" when the file does not say
}

// ReadFile reads the workbook and picks the sheet that maps best.
func ReadFile(name string, data []byte) (Parsed, error) {
	_, sheets, err := parse.Read(name, data)
	if err != nil {
		return Parsed{}, err
	}
	var best Parsed
	bestMissing := 99
	for _, s := range sheets {
		l, ok := parse.DetectLayout(s)
		if !ok {
			continue
		}
		mp := HeuristicMapping(l)
		if m := len(Missing(mp)); m < bestMissing {
			best, bestMissing = Parsed{Sheet: s, Layout: l, Mapping: mp}, m
		}
	}
	if bestMissing == 99 {
		return Parsed{}, ErrNoPayrollSheet
	}
	best.SourceSystem = DetectSource(name, best.Sheet, best.Layout)
	best.Mapping.SourceSystem = best.SourceSystem
	best.Period = DetectPeriod(best.Sheet, best.Layout)
	return best, nil
}

// DetectSource looks for the program name in the file name and the rows
// above the header (Luca, Zirve and Logo print it in the report banner).
func DetectSource(name string, s parse.Sheet, l parse.Layout) string {
	text := parse.Normalize(name)
	for i := 0; i < len(s.Rows) && i < l.HeaderRow; i++ {
		text += " " + parse.Normalize(strings.Join(s.Rows[i], " "))
	}
	ws := strings.Fields(text)
	for _, sys := range []string{SourceLuca, SourceZirve, SourceLogo} {
		for _, w := range ws {
			if strings.HasPrefix(w, sys) {
				return sys
			}
		}
	}
	return SourceGeneric
}

var (
	periodNumRe = regexp.MustCompile(`\b(20[0-9]{2})\s*[-/.]\s*(0?[1-9]|1[0-2])\b|\b(0?[1-9]|1[0-2])\s*[-/.]\s*(20[0-9]{2})\b`)
	months      = map[string]int{"ocak": 1, "subat": 2, "mart": 3, "nisan": 4, "mayis": 5, "haziran": 6, "temmuz": 7,
		"agustos": 8, "eylul": 9, "ekim": 10, "kasim": 11, "aralik": 12}
	yearRe = regexp.MustCompile(`^20[0-9]{2}$`)
)

// DetectPeriod reads "Dönem: 2026/10", "10.2026" or "Ekim 2026" above the
// header.
func DetectPeriod(s parse.Sheet, l parse.Layout) string {
	for i := 0; i < len(s.Rows) && i < l.HeaderRow-1; i++ {
		if p := periodIn(strings.Join(s.Rows[i], " ")); p != "" {
			return p
		}
	}
	return ""
}

func periodIn(text string) string {
	if m := periodNumRe.FindStringSubmatch(text); m != nil {
		y, mo := m[1], m[2]
		if y == "" {
			y, mo = m[4], m[3]
		}
		n, _ := strconv.Atoi(mo)
		return fmt.Sprintf("%s-%02d", y, n)
	}
	ws := strings.Fields(parse.Normalize(text))
	for i, w := range ws {
		if mo, ok := months[w]; ok {
			for _, n := range []int{i + 1, i - 1} {
				if n >= 0 && n < len(ws) && yearRe.MatchString(ws[n]) {
					return fmt.Sprintf("%s-%02d", ws[n], mo)
				}
			}
		}
	}
	return ""
}

// amountFields are the money columns of a line.
var amountFields = []string{"gross", "sgk_base", "sgk_employee", "unemployment_employee", "income_tax_base", "income_tax",
	"income_tax_exemption", "stamp_tax", "stamp_tax_exemption", "agi", "other_deductions", "net", "sgk_employer",
	"unemployment_employer", "sgk_incentive", "employer_cost"}

func (l *Line) amount(field string) *decimal.Decimal {
	switch field {
	case "gross":
		return &l.Gross
	case "sgk_base":
		return &l.SGKBase
	case "sgk_employee":
		return &l.SGKEmployee
	case "unemployment_employee":
		return &l.UnemploymentEmployee
	case "income_tax_base":
		return &l.IncomeTaxBase
	case "income_tax":
		return &l.IncomeTax
	case "income_tax_exemption":
		return &l.IncomeTaxExemption
	case "stamp_tax":
		return &l.StampTax
	case "stamp_tax_exemption":
		return &l.StampTaxExemption
	case "agi":
		return &l.AGI
	case "other_deductions":
		return &l.OtherDeductions
	case "net":
		return &l.Net
	case "sgk_employer":
		return &l.SGKEmployer
	case "unemployment_employer":
		return &l.UnemploymentEmployer
	case "sgk_incentive":
		return &l.SGKIncentive
	case "employer_cost":
		return &l.EmployerCost
	}
	return nil
}

// Lines turns the data block into payroll lines with the row checks.
// Summary rows ("Toplam") and rows without a name or a national id are
// skipped. A "period" column, when mapped, may name the period.
func Lines(p Parsed) ([]Line, []Issue, string) {
	cols := map[string]int{}
	for i, c := range p.Mapping.Columns {
		if c.Field != "" && i < len(p.Layout.Columns) {
			cols[c.Field] = p.Layout.Columns[i].Index
		}
	}
	first, last := p.Layout.FirstDataRow-1, p.Layout.LastDataRow-1
	if last >= len(p.Sheet.Rows) {
		last = len(p.Sheet.Rows) - 1
	}
	cell := func(row []string, field string) string {
		i, ok := cols[field]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	styles := map[string]parse.NumberStyle{}
	for _, f := range amountFields {
		if _, ok := cols[f]; !ok {
			continue
		}
		var vals []string
		for r := first; r <= last && r >= 0; r++ {
			vals = append(vals, cell(p.Sheet.Rows[r], f))
		}
		styles[f] = parse.DetectNumberStyle(vals)
	}
	_, hasNet := cols["net"]
	_, hasCost := cols["employer_cost"]
	var lines []Line
	var issues []Issue
	period := ""
	seen := map[string]int{}
	for r := first; r <= last && r >= 0; r++ {
		row := p.Sheet.Rows[r]
		if parse.IsTotalRow(row) {
			issues = append(issues, Issue{r + 1, IssueRowTotalsSkipped, "Toplam satırı atlandı"})
			continue
		}
		name := cell(row, "employee_name")
		if name == "" {
			name = strings.TrimSpace(cell(row, "first_name") + " " + cell(row, "last_name"))
		}
		rawID := cell(row, "national_id")
		if name == "" && rawID == "" {
			continue
		}
		l := Line{Row: r + 1, EmployeeName: strings.Join(strings.Fields(name), " ")}
		if rawID != "" {
			if id, ok := NationalID(rawID); ok {
				l.nationalID, l.NationalIDMasked = id, MaskNationalID(id)
				if prev, dup := seen[id]; dup {
					issues = append(issues, Issue{r + 1, IssueDuplicateTCKN, fmt.Sprintf("Aynı TCKN %d. satırda da var", prev)})
				}
				seen[id] = r + 1
			} else {
				issues = append(issues, Issue{r + 1, IssueInvalidTCKN, "TCKN geçersiz, personel adla eşlendi"})
			}
		}
		l.SGKNo = cell(row, "sgk_no")
		l.JobTitle = cell(row, "job_title")
		l.Department = cell(row, "department")
		if d, ok := parse.ParseDate(cell(row, "hire_date")); ok {
			l.HireDate = &d
		}
		if d, ok := parse.ParseDate(cell(row, "exit_date")); ok {
			l.ExitDate = &d
		}
		if v := cell(row, "days_worked"); v != "" {
			if n, ok := parseDays(v); ok {
				l.DaysWorked = &n
			}
		}
		for f, st := range styles {
			v, neg, ok := parse.ParseAmount(cell(row, f), st)
			if !ok {
				continue
			}
			if neg {
				issues = append(issues, Issue{r + 1, IssueNegativeSkipped, fmt.Sprintf("%s eksi; mutlak değeri alındı", f)})
			}
			*l.amount(f) = v.Round(2)
		}
		if period == "" {
			period = periodIn(cell(row, "period"))
		}
		l.Complete(hasNet, hasCost)
		issues = append(issues, l.Check()...)
		lines = append(lines, l)
	}
	if len(lines) == 0 {
		issues = append(issues, Issue{0, IssueNoRows, "Bordroda personel satırı bulunamadı"})
	}
	return lines, issues, period
}

func parseDays(v string) (int32, bool) {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f > 31 {
		return 0, false
	}
	return int32(f), true
}

// periodStart is the first day of a YYYY-MM period.
func periodStart(period string) time.Time {
	t, _ := time.Parse("2006-01", period)
	return t
}
