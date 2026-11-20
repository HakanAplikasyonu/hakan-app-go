package handler

import (
	"bytes"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

func strp(s string) *string { return &s }

// Mizan dışa aktarımı (#669 E): işaretli bakiye borç / alacak bakiyesine
// ayrılır; karşılaştırma varsa önceki bakiye ve değişim sütunları eklenir.
func TestTrialBalanceDataset(t *testing.T) {
	p := "2026-10"
	rep := usecase.TrialBalanceReport{
		TrialBalance: usecase.TrialBalance{Period: &p},
		ComparedTo:   &usecase.TrialBalance{},
		Lines: []usecase.TrialBalanceLine{
			{Code: "100", Name: "Kasa", Debit: "500.00", Credit: "200.00", Balance: "300.00", PreviousBalance: strp("100.00"), Change: strp("200.00")},
			{Code: "320", Name: "Satıcılar", Debit: "0.00", Credit: "750.00", Balance: "-750.00"},
			{Code: "102", Name: "Bankalar", Debit: "10.00", Credit: "10.00", Balance: "0.00"},
		},
	}
	ds := TrialBalanceDataset(rep)
	if len(ds.Columns) != 8 || len(ds.Rows) != 3 {
		t.Fatalf("columns %d rows %d", len(ds.Columns), len(ds.Rows))
	}
	if ds.Rows[0]["debit_balance"] != "300.00" || ds.Rows[0]["credit_balance"] != "" || ds.Rows[0]["change"] != "200.00" {
		t.Errorf("kasa %+v", ds.Rows[0])
	}
	if ds.Rows[1]["credit_balance"] != "750.00" || ds.Rows[1]["debit_balance"] != "" || ds.Rows[1]["previous_balance"] != nil {
		t.Errorf("satıcılar %+v", ds.Rows[1])
	}
	if ds.Rows[2]["debit_balance"] != "" || ds.Rows[2]["credit_balance"] != "" {
		t.Errorf("zero balance %+v", ds.Rows[2])
	}
	rep.ComparedTo = nil
	if n := len(TrialBalanceDataset(rep).Columns); n != 6 {
		t.Errorf("no comparison: %d columns", n)
	}

	xlsx, err := ioengine.EncodeXLSX(ds, "tr", nil)
	if err != nil || !bytes.HasPrefix(xlsx, []byte("PK")) {
		t.Fatalf("xlsx: %v", err)
	}
	pdf, err := ioengine.EncodePDF(ds, "tr", nil, "Mizan 2026-10")
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("pdf: %v", err)
	}
}
