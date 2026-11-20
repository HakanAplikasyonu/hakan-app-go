package matching

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

func TestIsCari(t *testing.T) {
	for code, want := range map[string]bool{
		"120.01.074": true, "320": true, "340.01": true, "159.02": true,
		"102.01": false, "770.01": false, "12": false, "": false,
	} {
		if got := IsCari(code); got != want {
			t.Errorf("IsCari(%q) = %v, want %v", code, got, want)
		}
	}
}

func TestLineLabelAndSide(t *testing.T) {
	if got := LineLabel("2026-10", 12, 3, "120.01.001"); got != "2026-10 #12/3 120.01.001" {
		t.Fatalf("label %q", got)
	}
	if lineSide(banking.DirIn) != "credit" || lineSide(banking.DirOut) != "debit" {
		t.Fatal("money in credits the counter account, money out debits it")
	}
}
