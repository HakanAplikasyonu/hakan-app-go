package main

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestSeedBankDifference: the demo Garanti account's October statement and
// its 102.02 balance differ by two reasons (#780 C); a second run adds
// nothing.
func TestSeedBankDifference(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	atlas := demoScope(t, d, "atlas")
	flow := posting.NewWorkflow(d.Pool, nil, nil)
	rep, err := flow.Difference(ctx, posting.Subject{OrganizationID: atlas.OrganizationID, EngagementID: atlas.ID,
		TaxpayerID: atlas.TaxpayerID}, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	var garanti *posting.DifferenceAccount
	for i := range rep.Items {
		if rep.Items[i].IBAN == DemoDiffIBAN {
			garanti = &rep.Items[i]
		}
	}
	if garanti == nil {
		t.Fatalf("no Garanti account in %+v", rep.Items)
	}
	if garanti.AccountCode != "102.02" || *garanti.StatementBalance != "23150.00" || garanti.LedgerBalance != "23710.00" ||
		*garanti.Difference != "-560.00" {
		t.Fatalf("garanti %+v", garanti)
	}
	got := map[string]string{}
	for _, r := range garanti.Reasons {
		got[r.Reason] = r.Amount
	}
	if len(got) != 2 || got[posting.ReasonUnbooked] != "-650.00" || got[posting.ReasonManualLines] != "90.00" {
		t.Fatalf("reasons %v", got)
	}
	scope := atlas
	before := count(t, d, `SELECT count(*) FROM bank_transactions WHERE taxpayer_id = $1`, atlas.TaxpayerID)
	if err := seedBankDifference(ctx, d.Pool, scope, 0); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, `SELECT count(*) FROM bank_transactions WHERE taxpayer_id = $1`, atlas.TaxpayerID); n != before {
		t.Fatalf("second run added movements: %d → %d", before, n)
	}
}
