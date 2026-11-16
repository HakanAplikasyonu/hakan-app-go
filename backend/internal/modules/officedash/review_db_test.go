package officedash

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
)

func TestSuggestionContext(t *testing.T) {
	e := setupClients(t)
	ctx := context.Background()
	h := &ReviewHandler{Q: database.QueriesFor(e.d.Pool)}
	var cust, inv, item int64
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name) VALUES ($1, 't', 'c1', 'manual', 'Kırtasiye A.Ş.') RETURNING id`, e.tps[0]).Scan(&cust); err != nil {
		t.Fatal(err)
	}
	if err := e.d.Pool.QueryRow(ctx, `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type, issue_date, period, customer_id, currency, tax_summary_source, payable_amount)
		VALUES ($1, 't', 'i1', 'manual', 'ABC2026000000001', 'incoming', 'SATIS', '2026-10-15', '2026-10', $2, 'TRY', 'items', 1200) RETURNING id`, e.tps[0], cust).Scan(&inv); err != nil {
		t.Fatal(err)
	}
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no, name, quantity, line_extension_amount, tax_rate) VALUES ($1, $2, 1, 'Toner', 2, 1000, 20) RETURNING id`, e.tps[0], inv).Scan(&item); err != nil {
		t.Fatal(err)
	}
	var sug uuid.UUID
	if err := e.d.Pool.QueryRow(ctx, `
		INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id, target_field, proposed_value, confidence, rationale, origin, rule_ref)
		VALUES ($1, $2, $3, 'account_code', 'invoice_item', $4, 'account_code', '{"account_code":"770.01"}', 0.7, 'test', 'rule', 'r1') RETURNING uuid`,
		e.org, e.engs[0], e.tps[0], item).Scan(&sug); err != nil {
		t.Fatal(err)
	}
	w := e.engagement(t, h.Context, 0, map[string]string{"suggestionUuid": sug.String()})
	var env struct{ Data ReviewContext }
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || w.Code != http.StatusOK {
		t.Fatalf("context %d %s", w.Code, w.Body)
	}
	c := env.Data
	if c.Invoice == nil || c.Invoice.Number != "ABC2026000000001" || c.Invoice.CounterpartyName != "Kırtasiye A.Ş." || c.Invoice.IssueDate != "2026-10-15" {
		t.Fatalf("invoice = %+v", c.Invoice)
	}
	if c.Item == nil || c.Item.Name != "Toner" || c.Item.LineNo != 1 || c.Receipt != nil {
		t.Fatalf("item = %+v receipt = %+v", c.Item, c.Receipt)
	}
	// Another engagement of the same office does not see it.
	if w := e.engagement(t, h.Context, 1, map[string]string{"suggestionUuid": sug.String()}); w.Code != http.StatusNotFound {
		t.Fatalf("cross-engagement = %d", w.Code)
	}
}
