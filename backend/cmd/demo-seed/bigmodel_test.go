package main

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
)

// Aynı gün aynı veri: model iki kez kurulduğunda belgeler birebir aynıdır;
// fişler dengelidir.
func TestBigModelDeterministic(t *testing.T) {
	sum := func() (n int, total kurus, vouchers int) {
		w := newBigWorld(scaleBig, testNow)
		for _, tp := range demo.GenerateTaxpayers(1, 10) {
			c := &bigClient{BigTaxpayer: tp}
			m := buildClient(w, c)
			tax := taxDue{kdv: m.kdvPayable, muh: map[string]kurus{}, filed: map[string]bool{}}
			m.buildBank(w, w.cps, tax)
			m.buildVouchers(w, tax)
			n += len(m.invoices) + len(m.bank) + len(m.zs) + len(m.expenses)
			for i := range m.invoices {
				total += m.invoices[i].payable()
			}
			for _, v := range m.vouchers {
				var d, cr kurus
				for _, l := range v.lines {
					d, cr = d+l.debit, cr+l.credit
				}
				if d != cr {
					t.Fatalf("%s %s: borç %d ≠ alacak %d", tp.Title, v.desc, d, cr)
				}
			}
			vouchers += len(m.vouchers)
		}
		return
	}
	n1, t1, v1 := sum()
	n2, t2, v2 := sum()
	if n1 != n2 || t1 != t2 || v1 != v2 || n1 == 0 {
		t.Fatalf("not deterministic: %d/%d/%d vs %d/%d/%d", n1, t1, v1, n2, t2, v2)
	}
}
