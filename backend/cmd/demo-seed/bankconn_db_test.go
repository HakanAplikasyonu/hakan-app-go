package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice/vakifbank"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestDemoBankConnection (#518): Atlas Gıda'nın Vakıfbank web servisi
// bağlantısı şubede bekler; kimlik bilgisi yoktur (işçi bankayı çağırmaz).
func TestDemoBankConnection(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	if err := seed(ctx, d.Pool, "Demo-Test-1!", ""); err != nil {
		t.Fatal(err)
	}
	atlas := demoScope(t, d, "atlas")
	var state, status string
	var hasCreds bool
	var raw []byte
	if err := d.Pool.QueryRow(ctx, `SELECT setup_state, status, credentials_ref IS NOT NULL, meta
		FROM source_connections WHERE taxpayer_id = $1 AND source_system = $2`,
		atlas.TaxpayerID, vakifbank.SourceSystem).Scan(&state, &status, &hasCreds, &raw); err != nil {
		t.Fatal(err)
	}
	if state != "submitted_to_bank" || status != "pending" || hasCreds {
		t.Errorf("state=%s status=%s creds=%v, want submitted_to_bank/pending/false", state, status, hasCreds)
	}
	var m webservice.Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.BankID != "vakifbank" || len(m.IBANs) != 1 || m.IBANs[0] != DemoVakifbankIBAN || m.SubmittedAt == nil {
		t.Errorf("meta = %+v", m)
	}
}
