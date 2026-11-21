package posting

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal { v := d(s); return &v }

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func reasonOf(t *testing.T, a DifferenceAccount, r string) DifferenceReason {
	t.Helper()
	for _, x := range a.Reasons {
		if x.Reason == r {
			return x
		}
	}
	t.Fatalf("reason %s missing in %+v", r, a.Reasons)
	return DifferenceReason{}
}

func hasReason(a DifferenceAccount, r string) bool {
	for _, x := range a.Reasons {
		if x.Reason == r {
			return true
		}
	}
	return false
}

func bankLine(id int64, code, date string, signed string) diffLine {
	return diffLine{DraftUUID: uuid.New(), Code: code, Date: day(date), Period: date[:7], EntryType: "mahsup",
		Signed: d(signed), SourceKind: DraftSourceKind, SourceID: strconv.FormatInt(id, 10)}
}

var (
	boxA = diffBox{ID: 1, UUID: uuid.New(), Name: "Ziraat", Code: "102.01"}
	boxB = diffBox{ID: 2, UUID: uuid.New(), Name: "Garanti", Code: "102.02"}
)

func TestDifferenceBalanced(t *testing.T) {
	txs := []diffTx{
		{ID: 10, Box: 1, Date: day("2026-10-01"), Period: "2026-10", Signed: d("500"), Before: dp("1000"), After: dp("1500")},
		{ID: 11, Box: 1, Date: day("2026-10-02"), Period: "2026-10", Signed: d("-100"), After: dp("1400")},
	}
	lines := []diffLine{
		{Code: "102.01", Date: day("2026-01-01"), EntryType: "acilis", Signed: d("1000")},
		bankLine(10, "102.01", "2026-10-01", "500"),
		bankLine(11, "102.01", "2026-10-02", "-100"),
	}
	a := computeDifference(boxA, []diffBox{boxA, boxB}, txs, lines, nil)
	if !a.Balanced || *a.Difference != "0.00" || a.LedgerBalance != "1400.00" || *a.StatementBalance != "1400.00" {
		t.Fatalf("not balanced: %+v", a)
	}
	if len(a.Reasons) != 0 || a.OpenTotal != "0.00" {
		t.Fatalf("reasons on a balanced account: %+v", a.Reasons)
	}
}

// Two reasons (the demo shape): a movement not booked yet and a manual
// line on 102 without a bank movement.
func TestDifferenceUnbookedAndManual(t *testing.T) {
	txs := []diffTx{
		{ID: 10, Box: 1, Date: day("2026-10-01"), Period: "2026-10", Signed: d("500"), Before: dp("1000"), After: dp("1500")},
		{ID: 11, Box: 1, Date: day("2026-10-05"), Period: "2026-10", Signed: d("-200"), After: dp("1300")},
	}
	draft := uuid.New()
	lines := []diffLine{
		{Code: "102.01", Date: day("2026-01-01"), EntryType: "acilis", Signed: d("1000")},
		bankLine(10, "102.01", "2026-10-01", "500"),
		{DraftUUID: draft, EntryNo: 7, Code: "102.01", Date: day("2026-10-10"), Period: "2026-10", EntryType: "mahsup", Signed: d("75")},
	}
	a := computeDifference(boxA, []diffBox{boxA}, txs, lines, nil)
	if *a.Difference != "-275.00" {
		t.Fatalf("difference %v", *a.Difference)
	}
	u := reasonOf(t, a, ReasonUnbooked)
	if u.Amount != "-200.00" || len(u.Records) != 1 || u.Records[0].Kind != "bank_transaction" {
		t.Fatalf("unbooked %+v", u)
	}
	m := reasonOf(t, a, ReasonManualLines)
	if m.Amount != "-75.00" || m.Records[0].DraftUUID == nil || *m.Records[0].DraftUUID != draft || m.Records[0].EntryNo != 7 {
		t.Fatalf("manual %+v", m)
	}
	if hasReason(a, ReasonUnexplained) {
		t.Fatalf("unexplained left: %+v", a.Reasons)
	}
	if a.OpenTotal != "-275.00" {
		t.Fatalf("open total %s", a.OpenTotal)
	}
}

func TestDifferenceOtherAccountGapDuplicateOpening(t *testing.T) {
	txs := []diffTx{
		// A: opening 900 on the statement, ledger opening 1000 → −100.
		{ID: 10, Box: 1, Date: day("2026-03-01"), Period: "2026-03", Signed: d("100"), Before: dp("900"), After: dp("1000")},
		// Booked to B's code by mistake → +50 other account.
		{ID: 11, Box: 1, Date: day("2026-03-02"), Period: "2026-03", Signed: d("50"), After: dp("1050")},
		// A possible duplicate the bank did not count, booked → −30.
		{ID: 12, Box: 1, Date: day("2026-03-02"), Period: "2026-03", Signed: d("30"), Flagged: true},
		// Gap: next statement opens at 1250 (200 of movements missing).
		{ID: 13, Box: 1, Date: day("2026-03-20"), Period: "2026-03", Signed: d("-50"), Before: dp("1250"), After: dp("1200")},
		// B's movement booked to A's code → −40 other account.
		{ID: 20, Box: 2, Date: day("2026-03-03"), Period: "2026-03", Signed: d("40"), After: dp("40")},
	}
	lines := []diffLine{
		{Code: "102.01", Date: day("2026-01-01"), EntryType: "acilis", Signed: d("1000")},
		bankLine(10, "102.01", "2026-03-01", "100"),
		bankLine(11, "102.02", "2026-03-02", "50"),
		bankLine(12, "102.01", "2026-03-02", "30"),
		bankLine(13, "102.01", "2026-03-20", "-50"),
		bankLine(20, "102.01", "2026-03-03", "40"),
	}
	a := computeDifference(boxA, []diffBox{boxA, boxB}, txs, lines, nil)
	// S = 1200; L = 1000 + 100 + 30 − 50 + 40 = 1120.
	if *a.Difference != "80.00" || a.LedgerBalance != "1120.00" {
		t.Fatalf("difference %v ledger %v", *a.Difference, a.LedgerBalance)
	}
	want := map[string]string{ReasonOpening: "-100.00", ReasonOtherAccount: "10.00", ReasonGaps: "200.00", ReasonDuplicates: "-30.00"}
	for r, v := range want {
		if got := reasonOf(t, a, r).Amount; got != v {
			t.Errorf("%s = %s, want %s", r, got, v)
		}
	}
	if hasReason(a, ReasonUnexplained) {
		t.Fatalf("unexplained left: %+v", reasonOf(t, a, ReasonUnexplained))
	}
	g := reasonOf(t, a, ReasonGaps).Records[0]
	if g.Kind != "gap" || g.Date != "2026-03-02" || g.ToDate != "2026-03-20" {
		t.Fatalf("gap record %+v", g)
	}
	if len(reasonOf(t, a, ReasonOtherAccount).Records) != 2 {
		t.Fatalf("other account records %+v", reasonOf(t, a, ReasonOtherAccount).Records)
	}
}

// A transfer between own accounts: the outgoing leg books both 102 lines;
// neither account shows a reason.
func TestDifferenceTransferBetweenOwnAccounts(t *testing.T) {
	txs := []diffTx{
		{ID: 10, Box: 1, Date: day("2026-10-01"), Period: "2026-10", Signed: d("-300"), Before: dp("1000"), After: dp("700"), Partner: 20},
		{ID: 20, Box: 2, Date: day("2026-10-01"), Period: "2026-10", Signed: d("300"), Before: dp("0"), After: dp("300"), Partner: 10},
	}
	lines := []diffLine{
		{Code: "102.01", Date: day("2026-01-01"), EntryType: "acilis", Signed: d("1000")},
		bankLine(10, "102.01", "2026-10-01", "-300"),
		bankLine(10, "102.02", "2026-10-01", "300"),
	}
	for _, b := range []diffBox{boxA, boxB} {
		a := computeDifference(b, []diffBox{boxA, boxB}, txs, lines, nil)
		if !a.Balanced || len(a.Reasons) != 0 {
			t.Fatalf("%s: %+v", b.Name, a)
		}
	}
}

// A correction entry and an explained mark close the open amount; the
// raw reason amounts stay.
func TestDifferenceCorrectionAndExplained(t *testing.T) {
	txs := []diffTx{
		{ID: 10, Box: 1, Date: day("2026-10-01"), Period: "2026-10", Signed: d("500"), Before: dp("1000"), After: dp("1500")},
		{ID: 11, Box: 1, Date: day("2026-10-05"), Period: "2026-10", Signed: d("-200"), After: dp("1300")},
	}
	lines := []diffLine{
		{Code: "102.01", Date: day("2026-01-01"), EntryType: "acilis", Signed: d("1000")},
		bankLine(10, "102.01", "2026-10-01", "500"),
		{Code: "102.01", Date: day("2026-10-10"), EntryType: "mahsup", Signed: d("75")},
		{Code: "102.01", Date: day("2026-10-31"), EntryType: "mahsup", Signed: d("-75"), SourceKind: "manual", SourceID: CorrectionSourcePrefix + ReasonManualLines},
	}
	res := map[string][]DifferenceResolution{
		ReasonUnbooked: {{UUID: uuid.New(), Action: ActionExplained, Explanation: "Kasım fişinde", CreatedBy: "Hakan"}},
	}
	a := computeDifference(boxA, []diffBox{boxA}, txs, lines, res)
	if *a.Difference != "-200.00" {
		t.Fatalf("difference %s", *a.Difference)
	}
	m := reasonOf(t, a, ReasonManualLines)
	if m.Amount != "-75.00" || m.Corrected != "-75.00" || m.Open != "0.00" {
		t.Fatalf("manual %+v", m)
	}
	u := reasonOf(t, a, ReasonUnbooked)
	if u.Explained == nil || u.Explained.CreatedBy != "Hakan" || u.Open != "0.00" {
		t.Fatalf("unbooked %+v", u)
	}
	if hasReason(a, ReasonUnexplained) || a.OpenTotal != "0.00" {
		t.Fatalf("open %s reasons %+v", a.OpenTotal, a.Reasons)
	}
}

func TestDifferenceNoStatementBalance(t *testing.T) {
	txs := []diffTx{{ID: 10, Box: 1, Date: day("2026-10-01"), Period: "2026-10", Signed: d("500")}}
	a := computeDifference(boxA, []diffBox{boxA}, txs, nil, nil)
	if a.StatementBalance != nil || a.Difference != nil || a.Balanced {
		t.Fatalf("%+v", a)
	}
}

func TestDifferenceLineOwnersAndSharedCode(t *testing.T) {
	parent := diffBox{ID: 3, Code: "102"}
	if got := lineOwners("102.01.5", []diffBox{parent, boxA}); len(got) != 1 || got[0] != boxA.ID {
		t.Fatalf("owners %v", got)
	}
	if got := lineOwners("102.09", []diffBox{parent, boxA}); len(got) != 1 || got[0] != parent.ID {
		t.Fatalf("owners %v", got)
	}
	twin := diffBox{ID: 4, Code: "102.01"}
	if a := computeDifference(boxA, []diffBox{boxA, twin}, nil, nil, nil); !a.SharedCode {
		t.Fatal("shared code not flagged")
	}
	if normalizeLedgerCode(" 102 01 ") != "102.01" || normalizeLedgerCode("102-01") != "102.01" {
		t.Fatal("normalize")
	}
}

func TestCorrectionLines(t *testing.T) {
	in := ResolutionInput{Reason: ReasonUnexplained, Explanation: "Banka masrafı", CounterAccount: "780", Amount: "-12.5"}
	lines, amt, err := correctionLines("102.01", in)
	if err != nil || amt.String() != "-12.5" || len(lines) != 2 {
		t.Fatalf("%v %v %v", lines, amt, err)
	}
	if !lines[0].Credit.Equal(d("12.5")) || lines[0].Account != "102.01" || lines[0].SourceID != "bankfark:unexplained" ||
		!lines[1].Debit.Equal(d("12.5")) || lines[1].Account != "780" || lines[1].SourceKind != "" {
		t.Fatalf("lines %+v", lines)
	}
	for _, bad := range []ResolutionInput{
		{Reason: ReasonUnexplained, Explanation: "x", CounterAccount: "780", Amount: "0"},
		{Reason: ReasonUnexplained, Explanation: "x", CounterAccount: "102.01.1", Amount: "5"},
		{Reason: ReasonUnexplained, Explanation: "x", CounterAccount: "abc", Amount: "5"},
	} {
		if _, _, err := correctionLines("102.01", bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
