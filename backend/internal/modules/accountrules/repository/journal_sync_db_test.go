package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

// TestAcceptedCodeReachesDraftJournal (issue #239): a corrected (or accepted)
// account code is written to the not yet approved journal draft lines booked
// from the same invoice item; approved drafts and other items stay as they are.
func TestAcceptedCodeReachesDraftJournal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.newEngagement(t, "5555555555")
	inv := f.invoice(t, "2026-10", "6666666666", "Kırtasiye A.Ş.", "Toner", "Kağıt")
	out, err := f.rules.SuggestInvoice(ctx, f.sub(), inv)
	if err != nil {
		t.Fatal(err)
	}
	line := find(t, out, usecase.TargetItem, usecase.FieldAccount)
	if line.Status != string(reviewusecase.StatusPending) {
		t.Fatalf("suggestion status = %s, want pending", line.Status)
	}
	var itemID, otherID int64
	if err := f.d.Pool.QueryRow(ctx, `SELECT target_id FROM suggestions WHERE uuid = $1`, line.UUID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := f.d.Pool.QueryRow(ctx, `SELECT id FROM invoice_items WHERE taxpayer_id = $1 AND id <> $2 ORDER BY id LIMIT 1`, f.tp, itemID).Scan(&otherID); err != nil {
		t.Fatal(err)
	}

	draft := func(status string, entry int, item int64) int64 {
		t.Helper()
		var id int64
		if err := f.d.Pool.QueryRow(ctx, `
			INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_type, entry_date, description, origin, status, approved_at)
			VALUES ($1, $2, $3, '2026-10', $4, 'mahsup', '2026-10-15', 'test', '{}', $5, CASE WHEN $5 = 'draft' THEN NULL ELSE NOW() END) RETURNING id`,
			f.org, f.eng, f.tp, entry, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.d.Pool.Exec(ctx, `
			INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, description, debit, credit, source_kind, source_id)
			VALUES ($1, $2, $3, 1, '770.99', 'gider', 100, 0, 'invoice_item', $4)`,
			f.org, f.eng, id, strconv.FormatInt(item, 10)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	open, approved, other := draft("draft", 1, itemID), draft("approved", 2, itemID), draft("draft", 3, otherID)

	rsub := reviewusecase.Subject{OrganizationID: f.org, EngagementID: f.eng, TaxpayerID: f.tp}
	if _, err := f.review.Correct(ctx, rsub, line.UUID, f.user, json.RawMessage(`{"account_code":"760.05"}`), "kırtasiye 760"); err != nil {
		t.Fatal(err)
	}
	code := func(draft int64) string {
		var c string
		if err := f.d.Pool.QueryRow(ctx, `SELECT account_code FROM journal_draft_lines WHERE journal_draft_id = $1`, draft).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if got := code(open); got != "760.05" {
		t.Fatalf("open draft line = %s, want 760.05", got)
	}
	if got := code(approved); got != "770.99" {
		t.Fatalf("approved draft line changed to %s", got)
	}
	if got := code(other); got != "770.99" {
		t.Fatalf("other item's line changed to %s", got)
	}
}
