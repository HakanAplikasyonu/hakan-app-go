package taxreturns

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
)

// Foreign service invoices (#780 C): the "Yabancı faturalar" list feeds the
// KDV2 (sorumlu sıfatıyla KDV, yurt dışı hizmet) and the muhtasar
// (yurt dışı hizmet stopajı, KVK 30) previews. Only confirmed rows with a
// kur count; the others are reported as checks.

// Foreign invoice values (foreigninvoices: status, treatment).
const (
	foreignConfirmed     = "confirmed"
	foreignTreatmentStop = "kdv2_stopaj"
	// SourceForeignService marks a KDV2 row built from foreign invoices.
	SourceForeignService = "foreign_service"
	// WithholdingForeignService is the muhtasar row of yurt dışı hizmet
	// stopajı (KVK 30).
	WithholdingForeignService = "foreign_service"
)

// ForeignServiceLine is one foreign service invoice with its computed TL
// amounts (foreigninvoices.Compute). TL fields are nil without a kur.
type ForeignServiceLine struct {
	Status     string
	Treatment  string
	Kind       string
	VATRate    decimal.Decimal
	StopajRate decimal.Decimal
	GrossTRY   *decimal.Decimal
	KDV2TRY    *decimal.Decimal
	StopajTRY  *decimal.Decimal
}

// foreignStats counts the rows left out of the return.
type foreignStats struct {
	draft, missingRate int64
}

type foreignGroup struct {
	key       string
	kind      string
	rate      decimal.Decimal
	count     int64
	base, tax decimal.Decimal
}

// groupForeign groups the confirmed rows with a kur that pass keep, by
// key(). tax picks the TL tax of a row. Rows not counted are tallied.
func groupForeign(lines []ForeignServiceLine, keep func(ForeignServiceLine) bool,
	key func(ForeignServiceLine) (string, string, decimal.Decimal), tax func(ForeignServiceLine) *decimal.Decimal,
) ([]foreignGroup, foreignStats) {
	var st foreignStats
	m := map[string]*foreignGroup{}
	for _, l := range lines {
		if !keep(l) {
			continue
		}
		if l.Status != foreignConfirmed {
			st.draft++
			continue
		}
		t := tax(l)
		if l.GrossTRY == nil || t == nil {
			st.missingRate++
			continue
		}
		k, kind, rate := key(l)
		g := m[k]
		if g == nil {
			g = &foreignGroup{key: k, kind: kind, rate: rate}
			m[k] = g
		}
		g.count++
		g.base, g.tax = g.base.Add(*l.GrossTRY), g.tax.Add(*t)
	}
	out := make([]foreignGroup, 0, len(m))
	for _, g := range m {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out, st
}

// ForeignServiceLabel is the Turkish label of a foreign KDV2 row:
// "Sorumlu sıfatıyla KDV (yurt dışı hizmet) — Reklam (...)".
func ForeignServiceLabel(kind string) string {
	l := "Sorumlu sıfatıyla KDV (yurt dışı hizmet)"
	if k, ok := foreigninvoices.DefaultOf(kind); ok {
		return l + " — " + k.Label
	}
	return l
}
