package luca

// Luca "Hesap Planı Aktar" file (issue #464): the office's account plan, or
// only the accounts Luca does not have yet (new cari sub accounts), as CSV.
// The layout is data in templates/hesap_plani_<version>.yaml (unverified:
// another team's observation of the Luca screen).

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// AccountPlanTemplateDefault is the default hesap planı template version.
const AccountPlanTemplateDefault = "unverified-1"

// AccountPlanContentType of the produced file.
const AccountPlanContentType = "text/csv"

// AccountPlanTemplate loads templates/hesap_plani_<version>.yaml ("" =
// default).
func AccountPlanTemplate(version string) (ae.CSVTemplate, error) {
	if version == "" {
		version = AccountPlanTemplateDefault
	}
	t, err := ae.LoadCSVTemplate(templates, fmt.Sprintf("templates/hesap_plani_%s.yaml", version))
	if err != nil {
		return ae.CSVTemplate{}, err
	}
	if t.Driver != Name || t.Name != "hesap_plani" {
		return ae.CSVTemplate{}, fmt.Errorf("%w: not a luca hesap_plani template", ae.ErrTemplate)
	}
	for _, c := range t.Columns {
		if !planKeys[c.Key] {
			return ae.CSVTemplate{}, fmt.Errorf("%w: unknown column key %q", ae.ErrTemplate, c.Key)
		}
	}
	return t, nil
}

// AccountPlanVersions lists the embedded hesap planı template versions.
func AccountPlanVersions() []string {
	matches, _ := fs.Glob(templates, "templates/hesap_plani_*.yaml")
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(m, "templates/hesap_plani_"), ".yaml"))
	}
	sort.Strings(out)
	return out
}

var planKeys = map[string]bool{
	"code": true, "name": true, "tax_no": true, "tax_office": true, "tckn": true, "address": true,
	"currency": true, "email": true, "vat_rate": true, "vat_account": true,
}

// PlanAccount is one row of the hesap planı file.
type PlanAccount struct {
	Code       string
	Name       string
	TaxNo      string
	TaxOffice  string
	TCKN       string
	Address    string
	Currency   string
	Email      string
	VATRate    string
	VATAccount string
}

func (a PlanAccount) values() map[string]string {
	return map[string]string{
		"code": a.Code, "name": a.Name, "tax_no": a.TaxNo, "tax_office": a.TaxOffice, "tckn": a.TCKN,
		"address": a.Address, "currency": a.Currency, "email": a.Email, "vat_rate": a.VATRate, "vat_account": a.VATAccount,
	}
}

// WriteAccountPlan writes the accounts in the template layout, sorted by
// code. A missing Hesap Kodu / Hesap Adı is a *ae.FieldError.
func WriteAccountPlan(t ae.CSVTemplate, accounts []PlanAccount) (ae.CSVResult, error) {
	sorted := append([]PlanAccount(nil), accounts...)
	sort.SliceStable(sorted, func(i, j int) bool { return CompareCodes(sorted[i].Code, sorted[j].Code) < 0 })
	rows := make([]map[string]string, len(sorted))
	for i, a := range sorted {
		rows[i] = a.values()
	}
	return t.WriteCSV(rows)
}

// CompareCodes orders account codes segment by segment, numerically when
// both segments are numbers ("120.2" < "120.10").
func CompareCodes(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, ex := strconv.Atoi(as[i])
		y, ey := strconv.Atoi(bs[i])
		switch {
		case ex == nil && ey == nil && x != y:
			if x < y {
				return -1
			}
			return 1
		case (ex != nil || ey != nil) && as[i] != bs[i]:
			return strings.Compare(as[i], bs[i])
		}
	}
	return len(as) - len(bs)
}

// ErrNoFreeCode: every code of the sub account level is taken.
var ErrNoFreeCode = errors.New("luca: no free sub account code under the main account")

// ErrInvalidMain: the main account is not a three digit code.
var ErrInvalidMain = errors.New("luca: main account must be three digits")

// SubaccountLayout is how sub accounts under one main account are coded:
// the widths of the levels below the main code (120.01.001 → [2 3]).
type SubaccountLayout []int

// DefaultSegments reads the template's default layout ("2,3").
func DefaultSegments(t ae.CSVTemplate) SubaccountLayout {
	var out SubaccountLayout
	for _, p := range strings.Split(t.Options["default_segments"], ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 && n <= 6 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return SubaccountLayout{2, 3}
	}
	return out
}

// NextSubaccount picks the next free cari sub account code under main.
//
// The level count and digit widths come from the plan: the deepest codes
// under main that are all digits decide them (120.01.001 → two levels of 2
// and 3 digits), and the new code goes under the parent of the latest such
// code (highest by code order) as its last segment + 1, zero padded. With no
// sub account under main the fallback layout is used under the first group
// ("120.01.001"). taken holds every code that may not be used (the plan's
// codes and codes already handed out); a full level is ErrNoFreeCode.
// parent is the code the new account hangs under (it may be missing from
// the plan when the fallback layout is used).
func NextSubaccount(main string, plan []string, taken map[string]bool, fallback SubaccountLayout) (code, parent string, err error) {
	if len(main) != 3 || strings.Trim(main, "0123456789") != "" {
		return "", "", ErrInvalidMain
	}
	// Deepest all-digit codes under main.
	var deepest []string
	depth := 0
	for _, c := range plan {
		if !strings.HasPrefix(c, main+".") {
			continue
		}
		segs := strings.Split(c, ".")
		digits := true
		for _, s := range segs[1:] {
			if s == "" || strings.Trim(s, "0123456789") != "" {
				digits = false
				break
			}
		}
		if !digits {
			continue
		}
		switch d := len(segs) - 1; {
		case d > depth:
			depth, deepest = d, []string{c}
		case d == depth:
			deepest = append(deepest, c)
		}
	}
	width := 0
	if len(deepest) > 0 {
		sort.Slice(deepest, func(i, j int) bool { return CompareCodes(deepest[i], deepest[j]) < 0 })
		latest := deepest[len(deepest)-1]
		parent = latest[:strings.LastIndex(latest, ".")]
		width = len(latest) - len(parent) - 1
	} else {
		if len(fallback) == 0 {
			fallback = SubaccountLayout{2, 3}
		}
		parent = main
		for _, w := range fallback[:len(fallback)-1] {
			parent += "." + fmt.Sprintf("%0*d", w, 1)
		}
		width = fallback[len(fallback)-1]
	}
	// Next number after the highest used under parent (plan and taken).
	highest := 0
	consider := func(c string) {
		if !strings.HasPrefix(c, parent+".") {
			return
		}
		last := c[len(parent)+1:]
		if strings.Contains(last, ".") {
			return
		}
		if n, err := strconv.Atoi(last); err == nil && n > highest {
			highest = n
		}
	}
	for _, c := range plan {
		consider(c)
	}
	for c := range taken {
		consider(c)
	}
	limit := 1
	for range width {
		limit *= 10
	}
	for n := highest + 1; n < limit; n++ {
		c := fmt.Sprintf("%s.%0*d", parent, width, n)
		if !taken[c] {
			return c, parent, nil
		}
	}
	return "", parent, ErrNoFreeCode
}
