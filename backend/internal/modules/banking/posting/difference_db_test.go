package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

func strp(s string) *string { return &s }

// Statement vs 102 (#780 C): two booked movements, one not booked and a
// manual 102 line; explained mark (who / when), its undo and a correction
// draft.
func TestBankLedgerDifference(t *testing.T) {
	f := setupFlow(t)
	ctx := context.Background()
	oct := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	in := mv("1", oct(1), banking.DirIn, "500", "HAVALE GELEN FARK", "", "", "")
	in.BalanceBefore, in.BalanceAfter = strp("1000"), strp("1500")
	out := mv("2", oct(3), banking.DirOut, "100", "EFT GIDEN FARK", "", "", "")
	out.BalanceAfter = strp("1400")
	open := mv("3", oct(6), banking.DirOut, "200", "KREDI KARTI ODEMESI FARK", "", "", "")
	open.BalanceAfter = strp("1200")
	f.ingest(t, in, out, open)
	if err := f.flow.SetLedgerAccount(ctx, f.sub, f.user, f.box.Uuid, "102.01"); err != nil {
		t.Fatal(err)
	}
	for desc, code := range map[string]string{"HAVALE GELEN FARK": "120.01", "EFT GIDEN FARK": "320.01"} {
		if err := f.flow.SetAccount(ctx, f.sub, f.user, f.item(t, "2026-10", desc).UUID, code, ""); err != nil {
			t.Fatal(err)
		}
	}
	scope := engagementctx.Scope{ID: f.sub.EngagementID, OrganizationID: f.sub.OrganizationID, TaxpayerID: f.sub.TaxpayerID}
	if _, err := f.flow.CreateDrafts(ctx, scope, f.user, "2026-10"); err != nil {
		t.Fatal(err)
	}
	js := journals.New(f.d.Pool, nil)
	thousand, fifty := decimal.NewFromInt(1000), decimal.NewFromInt(50)
	if _, err := js.CreateDraft(ctx, scope, f.user, journals.DraftInput{Type: accountingexport.EntryAcilis, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Lines: []journals.LineInput{{Account: "102.01", Debit: thousand}, {Account: "500", Credit: thousand}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateDraft(ctx, scope, f.user, journals.DraftInput{Date: oct(10), Description: "Elle banka",
		Lines: []journals.LineInput{{Account: "102.01", Debit: fifty}, {Account: "100", Credit: fifty}}}); err != nil {
		t.Fatal(err)
	}

	rep, err := f.flow.Difference(ctx, f.sub, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 1 {
		t.Fatalf("items %+v", rep.Items)
	}
	a := rep.Items[0]
	// Statement 1200; ledger 1000 + 500 − 100 + 50 = 1450.
	if a.AccountCode != "102.01" || *a.StatementBalance != "1200.00" || a.LedgerBalance != "1450.00" || *a.Difference != "-250.00" {
		t.Fatalf("account %+v", a)
	}
	if u := reasonOf(t, a, ReasonUnbooked); u.Amount != "-200.00" || len(u.Records) != 1 {
		t.Fatalf("unbooked %+v", u)
	}
	if m := reasonOf(t, a, ReasonManualLines); m.Amount != "-50.00" {
		t.Fatalf("manual %+v", m)
	}
	if hasReason(a, ReasonUnexplained) {
		t.Fatalf("unexplained %+v", a.Reasons)
	}

	// Explained, once; the mark keeps who and when; it can be taken back.
	ex, err := f.flow.Resolve(ctx, scope, f.user, ResolutionInput{CashBoxUUID: f.box.Uuid, Period: "2026-10",
		Reason: ReasonUnbooked, Action: ActionExplained, Explanation: "Kasım ekstresinde fişlenecek"})
	if err != nil || ex.Draft != nil {
		t.Fatalf("explain %+v %v", ex, err)
	}
	if _, err := f.flow.Resolve(ctx, scope, f.user, ResolutionInput{CashBoxUUID: f.box.Uuid, Period: "2026-10",
		Reason: ReasonUnbooked, Action: ActionExplained, Explanation: "yine"}); !errors.Is(err, ErrAlreadyExplained) {
		t.Fatalf("second explain err = %v", err)
	}
	rep, _ = f.flow.Difference(ctx, f.sub, "2026-10")
	u := reasonOf(t, rep.Items[0], ReasonUnbooked)
	if u.Explained == nil || u.Explained.CreatedAt.IsZero() || u.Explained.Explanation != "Kasım ekstresinde fişlenecek" || u.Open != "0.00" {
		t.Fatalf("explained %+v", u)
	}
	if rep.Items[0].OpenTotal != "-50.00" {
		t.Fatalf("open total %s", rep.Items[0].OpenTotal)
	}
	if err := f.flow.Unexplain(ctx, f.sub, ex.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.flow.Unexplain(ctx, f.sub, ex.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second unexplain err = %v", err)
	}

	// Correction: a draft 689 / 102.01 for the manual line.
	cr, err := f.flow.Resolve(ctx, scope, f.user, ResolutionInput{CashBoxUUID: f.box.Uuid, Period: "2026-10",
		Reason: ReasonManualLines, Action: ActionCorrected, Explanation: "Yanlış banka fişi", CounterAccount: "689", Amount: "-50"})
	if err != nil || cr.Draft == nil || !cr.Draft.Debit.Equal(fifty) {
		t.Fatalf("correct %+v %v", cr, err)
	}
	rep, _ = f.flow.Difference(ctx, f.sub, "2026-10")
	a = rep.Items[0]
	m := reasonOf(t, a, ReasonManualLines)
	if *a.Difference != "-200.00" || m.Amount != "-50.00" || m.Corrected != "-50.00" || m.Open != "0.00" ||
		len(m.Corrections) != 1 || *m.Corrections[0].CounterAccount != "689" || *m.Corrections[0].DraftUUID != cr.Draft.UUID {
		t.Fatalf("after correction %+v / %+v", a, m)
	}
	if hasReason(a, ReasonUnexplained) {
		t.Fatalf("unexplained after correction %+v", a.Reasons)
	}

	// Invalid input.
	for _, bad := range []ResolutionInput{
		{CashBoxUUID: f.box.Uuid, Period: "2026-10", Reason: "x", Action: ActionExplained, Explanation: "a"},
		{CashBoxUUID: f.box.Uuid, Period: "2026-10", Reason: ReasonGaps, Action: ActionExplained, Explanation: " "},
		{CashBoxUUID: f.box.Uuid, Period: "2026-10", Reason: ReasonGaps, Action: ActionCorrected, Explanation: "a", CounterAccount: "689"},
	} {
		if _, err := f.flow.Resolve(ctx, scope, f.user, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v", bad, err)
		}
	}
}
