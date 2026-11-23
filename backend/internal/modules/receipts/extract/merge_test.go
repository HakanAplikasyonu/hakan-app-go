package extract

import "testing"

// Parçalı Z raporu: üst parçada tarih ve Z no, alt parçada toplam ve KDV;
// birleşik okuma ikisini de taşır, çakışan alanda güveni yüksek olan kalır.
func TestMergeParts(t *testing.T) {
	top := Result{Kind: KindZReport, KindConfidence: 0.9, Model: "m1", Fields: map[string]Value{
		FieldDate:      {Value: "2026-10-31", Confidence: 0.95, Valid: true},
		FieldZNumber:   {Value: "0412", Confidence: 0.9, Valid: true},
		FieldTotal:     {Value: "", Confidence: 0.1, Valid: false},
		FieldOKCSerial: {Value: "AB1", Confidence: 0.6, Valid: true},
	}}
	bottom := Result{Kind: KindOther, KindConfidence: 0.4, Model: "m2", Fields: map[string]Value{
		FieldTotal:     {Value: "5900.00", Confidence: 0.93, Valid: true},
		FieldTax:       {Value: "900.00", Confidence: 0.92, Valid: true},
		FieldOKCSerial: {Value: "AB12345678", Confidence: 0.9, Valid: true},
	}}
	m := Merge(top, bottom)
	if m.Kind != KindZReport || m.Model != "m1" {
		t.Fatalf("kind/model = %s %s", m.Kind, m.Model)
	}
	for f, want := range map[string]string{
		FieldDate: "2026-10-31", FieldZNumber: "0412", FieldTotal: "5900.00", FieldTax: "900.00", FieldOKCSerial: "AB12345678",
	} {
		if got := m.Get(f); got != want {
			t.Errorf("%s = %q, want %q", f, got, want)
		}
	}
	if one := Merge(top); one.Get(FieldZNumber) != "0412" {
		t.Fatalf("single part = %+v", one)
	}
	if empty := Merge(); empty.Fields == nil {
		t.Fatal("empty merge has nil fields")
	}
}
