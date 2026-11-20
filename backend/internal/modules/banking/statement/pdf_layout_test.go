package statement

import "testing"

// Sağa hizalı geniş tutar başlığının solundan başlar (önceki kolonun
// altına taşar); sağ kenarı tutar başlığıyla hizalıdır.
func TestPDFRightAlignedAmounts(t *testing.T) {
	run := func(x, end float64, s string) pdfText { return pdfText{x: x, end: end, s: s} }
	rows := [][]pdfText{
		{run(40, 80, "Tarih"), run(100, 150, "Açıklama"), run(300, 330, "Tutar"), run(380, 420, "Bakiye")},
		{run(40, 90, "01.10.2026"), run(100, 200, "FAST GELEN YILDIZ"), run(270, 330, "-12.500,00"), run(360, 420, "1.000.000,00")},
		{run(40, 90, "02.10.2026"), run(100, 180, "EFT GIDEN 2024"), run(305, 330, "5,00"), run(372, 420, "999.995,00")},
	}
	got := tableFromRuns(rows, nil)
	want := [][]string{
		{"Tarih", "Açıklama", "Tutar", "Bakiye"},
		{"01.10.2026", "FAST GELEN YILDIZ", "-12.500,00", "1.000.000,00"},
		{"02.10.2026", "EFT GIDEN 2024", "5,00", "999.995,00"},
	}
	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("row %d col %d: %q, want %q (%q)", i, j, got[i][j], want[i][j], got[i])
			}
		}
	}
	// Tutar olmayan metin sola hizalı kalır.
	if rightAligned([]float64{40, 100}, []float64{80, 150}, run(100, 140, "KIRA")) != -1 {
		t.Fatal("text run must keep the start-based column")
	}
}
