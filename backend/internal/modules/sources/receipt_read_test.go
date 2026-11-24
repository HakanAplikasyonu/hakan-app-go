package sources

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Fişten okunan oran satırları (#780): toplam tutarsa okunanlar yazılır,
// tutmazsa programın tek oranı kalır; aynı fiş yeniden okunmaz. Uydurma
// tutarlar.
func TestNextReceiptRead(t *testing.T) {
	bim := []ReceiptReadLine{{Rate: d("1"), Base: d("99.01"), Tax: d("0.99")}, {Rate: d("20"), Base: d("50"), Tax: d("10")}}
	single := []ExpenseVatLine{{Rate: d("1"), Base: d("158.42"), Tax: d("1.58")}}
	e := Expense{Amount: d("160.00"), VatLines: single, ReceiptKey: "https://cdn.example.test/r/1.jpg"}

	// Yeni fiş: sıraya girer, programın satırı kalır.
	next, lines := NextReceiptRead(ReceiptReadState{}, false, e)
	if next == nil || next.Status != ReceiptPending || next.Key != e.ReceiptKey || len(lines) != 1 {
		t.Fatalf("new: %+v %+v", next, lines)
	}
	// Okunmuş ve tutan fiş: senkron yeniden yazsa da okunan satırlar kalır.
	read := ReceiptReadState{Key: e.ReceiptKey, Status: ReceiptRead, Lines: bim}
	next, lines = NextReceiptRead(read, true, e)
	if next.Status != ReceiptRead || len(lines) != 2 || !lines[1].Rate.Equal(d("20")) {
		t.Fatalf("kept: %+v %+v", next, lines)
	}
	// Tutar değişti: artık tutmuyor, programın oranı.
	changed := e
	changed.Amount = d("200")
	next, lines = NextReceiptRead(read, true, changed)
	if next.Status != ReceiptMismatch || len(lines) != 1 {
		t.Fatalf("changed: %+v %+v", next, lines)
	}
	// Okunamayan fiş yeniden denenmez.
	next, _ = NextReceiptRead(ReceiptReadState{Key: e.ReceiptKey, Status: ReceiptUnreadable}, true, e)
	if next.Status != ReceiptUnreadable {
		t.Fatalf("unreadable: %+v", next)
	}
	// Fiş değişti: yeniden sıraya.
	other := e
	other.ReceiptKey = "https://cdn.example.test/r/2.jpg"
	next, _ = NextReceiptRead(read, true, other)
	if next.Status != ReceiptPending || next.Key != other.ReceiptKey {
		t.Fatalf("other receipt: %+v", next)
	}
	// Fiş yok: durum silinir.
	none := e
	none.ReceiptKey = ""
	if next, lines = NextReceiptRead(read, true, none); next != nil || len(lines) != 1 {
		t.Fatalf("no receipt: %+v %+v", next, lines)
	}

	// Durum ocr_fields içinde gidip gelir.
	back, ok := ParseReceiptRead(read.OcrFields())
	if !ok || back.Status != ReceiptRead || len(back.Lines) != 2 || !back.Lines[0].Base.Equal(d("99.01")) {
		t.Fatalf("round trip: %+v %v", back, ok)
	}
	if _, ok := ParseReceiptRead([]byte(`{"date":{"value":"2026-10-01"}}`)); ok {
		t.Fatal("photo OCR fields are not a receipt read")
	}
}

func TestReceiptMatches(t *testing.T) {
	lines := []ReceiptReadLine{{Rate: d("1"), Base: d("99.01"), Tax: d("0.99")}, {Rate: d("20"), Base: d("50"), Tax: d("10")}}
	for _, c := range []struct {
		amount string
		want   bool
	}{{"160.00", true}, {"160.30", true}, {"160.40", false}, {"150", false}, {"0", false}} {
		if got := ReceiptMatches(lines, d(c.amount)); got != c.want {
			t.Errorf("%s: %v", c.amount, got)
		}
	}
	// Büyük tutarda %0,2 pay: 10.000 TL'de 20 TL.
	big := []ReceiptReadLine{{Rate: d("20"), Base: d("8333.33"), Tax: d("1666.67")}}
	if !ReceiptMatches(big, d("10015")) || ReceiptMatches(big, d("10025")) {
		t.Error("percentage tolerance")
	}
	if ReceiptMatches(nil, d("10")) {
		t.Error("no lines must not match")
	}
}
