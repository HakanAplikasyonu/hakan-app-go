package posting

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

func day(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func codes(f []ZFinding) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, x := range f {
		out[x.Code] = x.Params
	}
	return out
}

func TestContinuity(t *testing.T) {
	got := Continuity([]ZDay{
		{ID: 1, Date: day("2026-10-01"), Number: "0100"},
		{ID: 2, Date: day("2026-10-02"), Number: "0101"},
		{ID: 3, Date: day("2026-10-03"), Number: "0104"},   // 102-103 atlandı
		{ID: 4, Date: day("2026-10-06"), Number: "0105"},   // 4-5 Ekim yok
		{ID: 5, Date: day("2026-10-07"), Number: "Z 0105"}, // aynı numara başka gün
		{ID: 6, Number: "0200"},                            // tarihsiz: atlanır
	}, MaxGapDays)
	if len(got[1]) != 0 || len(got[2]) != 0 {
		t.Fatalf("clean days flagged: %v %v", got[1], got[2])
	}
	if p := codes(got[3])[CodeZNumberGap]; p == nil || p["from"] != "102" || p["to"] != "103" || p["missing"] != int64(2) {
		t.Fatalf("gap %v", got[3])
	}
	if p := codes(got[4])[CodeZDayGap]; p == nil || p["days"] != 2 || p["previous_date"] != "2026-10-03" {
		t.Fatalf("day gap %v", got[4])
	}
	if p := codes(got[5])[CodeZNumberDuplicate]; p == nil || p["first_date"] != "2026-10-06" {
		t.Fatalf("duplicate %v", got[5])
	}
	if len(got[6]) != 0 {
		t.Fatal("undated report checked")
	}
	// Sıra girdiden bağımsız.
	again := Continuity([]ZDay{{ID: 2, Date: day("2026-10-02"), Number: "11"}, {ID: 1, Date: day("2026-10-01"), Number: "10"}}, 0)
	if len(again[1])+len(again[2]) != 0 {
		t.Fatalf("unsorted input: %v", again)
	}
}

func TestZEntry(t *testing.T) {
	zplan := []engine.PlanAccount{
		{Code: "100.01", Name: "MERKEZ KASA", IsLeaf: true},
		{Code: "108.01", Name: "POS ALACAKLARI", IsLeaf: true},
		{Code: "600.01", Name: "YURT İÇİ SATIŞLAR %20", IsLeaf: true},
		{Code: "600.02", Name: "YURT İÇİ SATIŞLAR %10", IsLeaf: true},
		{Code: "391.01", Name: "HESAPLANAN KDV %20", IsLeaf: true},
		{Code: "391.02", Name: "HESAPLANAN KDV %10", IsLeaf: true},
	}
	card := d("7000")
	e, err := ZEntry(ZInput{SourceID: "retail_sale:x", Date: day("2026-10-15"), ZNumber: "0123", OKC: "AB123",
		Total: d("12100.03"), Card: &card, Plan: zplan, Lines: []ZVatLine{
			{Rate: d("20"), Base: d("9000"), Tax: d("1800")},
			{Rate: d("10"), Base: d("1000"), Tax: d("100")},
			{Rate: d("1"), Base: d("198"), Tax: d("2")},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Balanced() {
		t.Fatalf("unbalanced %v", e.Lines)
	}
	want := map[string]string{}
	for _, l := range e.Lines {
		want[l.Account] = l.String()
	}
	for acc, line := range map[string]string{
		"100.01": "100.01 D5100.03 C0.00", "108.01": "108.01 D7000.00 C0.00",
		"600.01": "600.01 D0.00 C9000.03", // fark en büyük matraha
		"391.01": "391.01 D0.00 C1800.00", "600.02": "600.02 D0.00 C1000.00", "391.02": "391.02 D0.00 C100.00",
		"600": "600 D0.00 C198.00", "391": "391 D0.00 C2.00",
	} {
		if want[acc] != line {
			t.Errorf("%s = %q, want %q", acc, want[acc], line)
		}
	}
	// %1: plan oranla bölünmüş ama %1 hesabı yok, ana hesap (600 / 391).
	if len(e.Lines) != 8 {
		t.Fatalf("lines %d: %v", len(e.Lines), e.Lines)
	}
	// Kırılım yok: tek satır, tamamı kasa.
	e, err = ZEntry(ZInput{Total: d("1200"), Tax: d("200")})
	if err != nil || len(e.Lines) != 3 || e.Lines[0].String() != "100 D1200.00 C0.00" || e.Lines[1].String() != "600 D0.00 C1000.00" {
		t.Fatalf("no breakdown %v %v", e.Lines, err)
	}
	if _, err := ZEntry(ZInput{Total: d("100"), Lines: []ZVatLine{{Rate: d("20"), Base: d("50"), Tax: d("10")}}}); err != ErrZBreakdown {
		t.Fatalf("mismatch accepted: %v", err)
	}
	_ = decimal.Zero
}
