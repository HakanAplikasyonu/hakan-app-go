package documents

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

const lucaPayrollCSV = "LUCA BORDRO DÖKÜMÜ\nÖRNEK GIDA LTD. ŞTİ. VKN: 1234567890\nDönem: 2026/10\n\n" +
	"Sıra No;Adı Soyadı;TC Kimlik No;SGK Sicil No;Gün;Brüt Ücret;SGK Matrahı;SGK İşçi Payı;İşsizlik İşçi Payı;" +
	"Gelir Vergisi Matrahı;Ödenecek Gelir Vergisi;Damga Vergisi;Net Ücret;SGK İşveren Payı;İşsizlik İşveren Payı;Toplam Maliyet\n" +
	"1;Ayşe Yılmaz;10000000146;3400123;30;40.000,00;40.000,00;5.600,00;400,00;34.000,00;2.100,00;153,60;31.746,40;8.200,00;800,00;49.000,00\n" +
	"2;Mehmet Demir;12345678950;3400124;30;26.005,50;26.005,50;3.640,77;260,06;22.104,67;0,00;0,00;22.104,67;5.331,13;520,11;31.856,74\n"

const lucaMizanCSV = "ÖRNEK GIDA LTD. ŞTİ. Vergi No: 1234567890\nMizan 01.01.2026 - 31.10.2026\n" +
	"Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n" +
	"100;KASA;10.000,00;4.000,00;6.000,00;0,00\n" +
	"100.01;Merkez Kasa;10.000,00;4.000,00;6.000,00;0,00\n" +
	"320;SATICILAR;0,00;5.000,00;0,00;5.000,00\n" +
	"600;YURT İÇİ SATIŞLAR;0,00;12.000,00;0,00;12.000,00\n"

func TestClassifyLucaPayroll(t *testing.T) {
	c := Classify("bordro_dokumu.csv", []byte(lucaPayrollCSV))
	if c.Kind != KindPayroll || c.Period != "2026-10" || c.SourceSystem != "luca" {
		t.Fatalf("got %+v", c)
	}
	// Only the banner is read: the employees' TCKN are not taxpayers.
	if !reflect.DeepEqual(c.TaxIDs, []string{"1234567890"}) {
		t.Fatalf("tax ids %v", c.TaxIDs)
	}
}

func TestClassifyMizan(t *testing.T) {
	c := Classify("mizan.csv", []byte(lucaMizanCSV))
	if c.Kind != KindTrialBalance || c.Period != "2026-10" {
		t.Fatalf("got %+v", c)
	}
	if !reflect.DeepEqual(c.TaxIDs, []string{"1234567890"}) {
		t.Fatalf("tax ids %v", c.TaxIDs)
	}
}

func TestClassifyAccountPlanWithoutAmounts(t *testing.T) {
	csv := "Hesap Kodu;Hesap Adı\n100;KASA\n100.01;Merkez Kasa\n320;SATICILAR\n"
	if c := Classify("plan.csv", []byte(csv)); c.Kind != KindAccountPlan || c.Period != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestClassifyPDFAndUnknown(t *testing.T) {
	if c := Classify("AWS_9876543210.pdf", []byte("%PDF-1.7\n...")); c.Kind != KindForeignInvoice ||
		!reflect.DeepEqual(c.TaxIDs, []string{"9876543210"}) {
		t.Fatalf("pdf %+v", c)
	}
	for name, data := range map[string]string{
		"notlar.csv":  "merhaba,dünya\n1,2\n",
		"rapor.docx":  "PK-not-a-sheet",
		"bos.xlsx":    "",
		"resim.gif":   "GIF89a......",
		"liste.csv":   "Ad;Soyad\nAyşe;Yılmaz\n",
		"bozuk.xlsx":  "PK\x03\x04garbage",
		"kodlar.csv":  "100;KASA\n",
		"metin.txt":   "sadece metin",
		"eski.xls":    "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1",
		"fatura.html": "<html></html>",
	} {
		if c := Classify(name, []byte(data)); c.Kind != KindUnknown {
			t.Errorf("%s: kind %q, want unknown", name, c.Kind)
		}
	}
}

func TestTaxIDsIn(t *testing.T) {
	got := TaxIDsIn("VKN 1234567890 / TCKN 10000000146, tel 05321234567 sicil 123 ve 1234567890")
	want := []string{"1234567890", "10000000146", "05321234567"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestMatchByTaxIDAndFolder(t *testing.T) {
	a := Candidate{EngagementUUID: uuid.New(), TaxID: "1234567890", Title: "Örnek Gıda Ltd. Şti."}
	b := Candidate{EngagementUUID: uuid.New(), TaxID: "2222222222", Title: "Deniz İnşaat A.Ş."}
	c := Candidate{EngagementUUID: uuid.New(), TaxID: "3333333333", Title: "Deniz Turizm A.Ş."}
	cands := []Candidate{a, b, c}

	if got, ok := ByTaxID([]string{"1234567890"}, cands); !ok || got != a {
		t.Fatalf("by tax id: %v %v", got, ok)
	}
	// Two taxpayers in one banner: unclear.
	if _, ok := ByTaxID([]string{"1234567890", "2222222222"}, cands); ok {
		t.Fatal("two taxpayers must not match")
	}
	cases := []struct {
		folder string
		want   *Candidate
	}{
		{`C:\Users\musavir\Luca\ORNEK GIDA LTD STI\Bordro`, &a},
		{"/Users/m/Luca/örnek gıda", &a},
		{"/Users/m/Mükellefler/2222222222 Deniz/mizan", &b},
		{"/Users/m/Mükellefler/Deniz İnşaat", &b},
		{"/Users/m/Mükellefler/Deniz", nil}, // two Deniz taxpayers
		{"/Users/m/Luca/Bordro", nil},
		{"", nil},
	}
	for _, tc := range cases {
		got, ok := ByFolder(tc.folder, cands)
		switch {
		case tc.want == nil && ok:
			t.Errorf("%q matched %v, want none", tc.folder, got.Title)
		case tc.want != nil && (!ok || got != *tc.want):
			t.Errorf("%q matched %v (%v), want %v", tc.folder, got.Title, ok, tc.want.Title)
		}
	}
}

func TestValidKind(t *testing.T) {
	for _, k := range []string{KindPayroll, KindTrialBalance, KindAccountPlan, KindForeignInvoice} {
		if !ValidKind(k) {
			t.Errorf("%s should be valid", k)
		}
	}
	if ValidKind(KindUnknown) || ValidKind("") {
		t.Error("unknown must not be a choice")
	}
}
