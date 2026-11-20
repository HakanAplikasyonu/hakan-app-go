package matching

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestHistoryEngagementScoped (#565): the movement's audit history lists
// only the events written under this engagement of this organization.
func TestHistoryEngagementScoped(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000565")
	org, org2 := d.Organization(t), d.Organization(t)
	user := d.User(t)
	// The taxpayer moved from one office to the other: one active
	// engagement at a time.
	eng := d.Engagement(t, org, tp)
	d.EndEngagement(t, eng)
	eng2 := d.Engagement(t, org2, tp)
	engUUID := func(id int64) uuid.UUID {
		var u uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	sub := Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp, EngagementUUID: engUUID(eng)}
	sub2 := Subject{OrganizationID: org2, EngagementID: eng2, TaxpayerID: tp, EngagementUUID: engUUID(eng2)}

	var box int64
	if err := d.Pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, type, currency) VALUES ($1, 'bank_account', 'TRY') RETURNING id`, tp).Scan(&box); err != nil {
		t.Fatal(err)
	}
	if _, err := banking.NewService(d.Pool, nil).Ingest(ctx, tp, box, nil, []banking.Transaction{{
		SourceSystem: "bank_ziraat", SourceID: "h1", CaptureMethod: banking.CaptureAPI, TransactionDate: on(3),
		Direction: banking.DirIn, Amount: "1000", Currency: "TRY", Description: "FAST TAHSILAT",
	}}); err != nil {
		t.Fatal(err)
	}
	var tx uuid.UUID
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM bank_transactions WHERE taxpayer_id = $1 AND source_id = 'h1'`, tp).Scan(&tx); err != nil {
		t.Fatal(err)
	}

	event := func(orgID int64, action, engagement string, payload string) {
		t.Helper()
		_, err := d.Pool.Exec(ctx, `
			INSERT INTO activity_events (actor_user_id, action, resource, resource_uuid, payload, organization_id)
			VALUES ($1, $2, 'bank_transaction', $3, jsonb_set($4::jsonb, '{engagement_uuid}', to_jsonb($5::text)), $6)`,
			user, action, tx, payload, engagement, orgID)
		if err != nil {
			t.Fatal(err)
		}
	}
	event(org, ActionMatched, sub.EngagementUUID.String(), `{"invoices":["ATL2026000000101"]}`)
	event(org, ActionUnmatched, sub.EngagementUUID.String(), `{"invoices":["ATL2026000000101"]}`)
	event(org, ActionBooked, sub.EngagementUUID.String(), `{"account_code":"120.01","account_name":"Alıcılar"}`)
	event(org, "bank.transaction.other", sub.EngagementUUID.String(), `{}`) // not a history action
	event(org, ActionMatched, uuid.NewString(), `{"invoices":["X"]}`)        // another engagement
	event(org2, ActionBooked, sub2.EngagementUUID.String(), `{"account_code":"320.01"}`)

	svc := NewService(d.Pool, nil, nil, Options{FeeTolerance: decimal.RequireFromString("25")})
	got, err := svc.History(ctx, sub, tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("history = %d entries, want 3: %+v", len(got), got)
	}
	seen := map[string]HistoryEntry{}
	for _, e := range got {
		seen[e.Action] = e
		if e.ActorName == nil || *e.ActorName == "" {
			t.Errorf("%s: no actor name", e.Action)
		}
	}
	if m := seen["matched"]; len(m.Invoices) != 1 || m.Invoices[0] != "ATL2026000000101" {
		t.Errorf("matched = %+v", m)
	}
	if b := seen["booked"]; b.AccountCode == nil || *b.AccountCode != "120.01" || b.AccountName == nil || len(b.Invoices) != 0 {
		t.Errorf("booked = %+v", b)
	}
	if _, ok := seen["unmatched"]; !ok {
		t.Error("unmatched missing")
	}

	got2, err := svc.History(ctx, sub2, tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 || got2[0].Action != "booked" || *got2[0].AccountCode != "320.01" {
		t.Fatalf("other office history = %+v", got2)
	}

	// A movement of another taxpayer is not found.
	other := sub
	other.TaxpayerID = d.Taxpayer(t, "legal", "9000000566")
	if _, err := svc.History(ctx, other, tx); !errors.Is(err, ErrTransactionNotFound) {
		t.Fatalf("other taxpayer: err = %v", err)
	}
}
