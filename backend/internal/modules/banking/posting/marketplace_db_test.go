package posting

import (
	"context"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Marketplace payouts through Suggest: the plan's channel cari, the missing
// cari warning and the office's own alias winning over the table.
func TestSuggestMarketplaceDB(t *testing.T) {
	f := setupAliases(t)
	ctx := context.Background()
	var imp int64
	if err := f.d.Pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('b', 64)) RETURNING id`, f.sub.OrganizationID, f.sub.EngagementID).Scan(&imp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Pool.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf) VALUES
		($1, $2, $3, '120', 'ALICILAR', '120', 1, false),
		($1, $2, $3, '120.01', 'YURT İÇİ ALICILAR', '120', 2, false),
		($1, $2, $3, '120.01.001', 'ABC GIDA SAN. TİC. A.Ş.', '120', 3, true),
		($1, $2, $3, '120.01.002', 'TRENDYOL', '120', 3, true)`, f.sub.OrganizationID, f.sub.EngagementID, imp); err != nil {
		t.Fatal(err)
	}
	in := func(desc, name string) Movement {
		return Movement{Direction: banking.DirIn, Detail: banking.DetailCustomerCollection, Description: desc,
			Counterparty: Counterparty{Name: name, Direction: banking.DirIn}}
	}

	s, err := Suggest(ctx, f.q, f.sub, in("EFT GELEN TRENDYOL HAKEDIS", ""))
	if err != nil || s == nil || s.AccountCode != "120.01.002" || s.Source != SourceCounterpartyAccount || s.Marketplace != "trendyol" ||
		!strings.HasPrefix(s.Reason, "Açıklamada 'TRENDYOL' geçiyor → Trendyol carisi önerildi") {
		t.Fatalf("trendyol %+v %v", s, err)
	}
	s, err = Suggest(ctx, f.q, f.sub, in("HEPSIBURADA SATIS ODEMESI", ""))
	if err != nil || s == nil || s.AccountCode != "120" || s.Source != SourceKindDefault ||
		!strings.Contains(s.Reason, "pazaryeri carisi tanımlı değil") {
		t.Fatalf("hepsiburada %+v %v", s, err)
	}
	// Money out naming a channel is not a payout.
	out := in("TRENDYOL REKLAM ODEMESI", "")
	out.Direction, out.Detail = banking.DirOut, banking.DetailSupplierPayment
	if s, err = Suggest(ctx, f.q, f.sub, out); err != nil || s == nil || s.Marketplace != "" || s.AccountCode != "320" {
		t.Fatalf("outgoing %+v %v", s, err)
	}
	// The office books the channel's payer elsewhere once: its alias wins.
	payer := Counterparty{Name: "DSM Grup Danışmanlık İletişim ve Satış Tic. A.Ş.", Direction: banking.DirIn}
	if err := Learn(ctx, f.q, f.sub, payer, Decision{Correction: true, AccountCode: "120.03.001", AccountName: "TRENDYOL TEMLİK"}); err != nil {
		t.Fatal(err)
	}
	s, err = Suggest(ctx, f.q, f.sub, in("TEMLIK DSM GRUP ODEMESI", payer.Name))
	if err != nil || s == nil || s.Source != SourceAlias || s.AccountCode != "120.03.001" || s.Marketplace != "" {
		t.Fatalf("alias %+v %v", s, err)
	}
}
