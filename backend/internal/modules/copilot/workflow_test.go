package copilot

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

func stepOf(t *testing.T, steps []Step, key string) Step {
	t.Helper()
	for _, s := range steps {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("step %s missing", key)
	return Step{}
}

func actionOf(s Step) string {
	if s.Action == nil {
		return ""
	}
	return *s.Action
}

func TestChain(t *testing.T) {
	// An empty period: only the report is to do.
	r := Row{}
	fill(&r, Counts{})
	if r.NextStep == nil || *r.NextStep != StepReport || r.ActiveSteps != 3 || r.DoneSteps != 2 {
		t.Fatalf("empty period: %+v", r)
	}
	// Bank movements without suggestions come first after documents.
	fill(&r, Counts{BankTotal: 8, BankUnassigned: 3, BankSuggested: 2, DocsOpen: 1})
	if *r.NextStep != StepDocuments || actionOf(stepOf(t, r.Steps, StepBank)) != ActionBankSuggest {
		t.Fatalf("bank unassigned: %+v", r.Steps)
	}
	fill(&r, Counts{BankTotal: 8, BankSuggested: 2})
	if b := stepOf(t, r.Steps, StepBank); b.Status != StatusTodo || b.Count != 2 || actionOf(b) != ActionBankApprove || *r.NextStep != StepBank {
		t.Fatalf("bank suggested: %+v", b)
	}
	// Approved movements → drafts; drafts → approve; approved → transfer.
	fill(&r, Counts{BankTotal: 8, BankApproved: 8})
	if j := stepOf(t, r.Steps, StepJournals); actionOf(j) != ActionBankDrafts || *r.NextStep != StepJournals {
		t.Fatalf("journals from bank: %+v", j)
	}
	fill(&r, Counts{BankTotal: 8, JournalDraft: 2})
	if j := stepOf(t, r.Steps, StepJournals); actionOf(j) != ActionJournalsApprove || j.Count != 2 {
		t.Fatalf("drafts: %+v", j)
	}
	fill(&r, Counts{BankTotal: 8, JournalApproved: 2})
	if tr := stepOf(t, r.Steps, StepTransfer); actionOf(tr) != ActionExportTransfer || *r.NextStep != StepTransfer {
		t.Fatalf("transfer: %+v", tr)
	}
	// A queued transfer waits; a failed one blocks.
	fill(&r, Counts{BankTotal: 8, JournalExported: 2, TransferOpen: 1, ReportSent: 1})
	if tr := stepOf(t, r.Steps, StepTransfer); tr.Status != StatusWaiting || r.NextStep != nil {
		t.Fatalf("waiting: %+v next=%v", tr, r.NextStep)
	}
	fill(&r, Counts{BankTotal: 8, JournalExported: 2, TransferFailed: 1, ReportSent: 1})
	if tr := stepOf(t, r.Steps, StepTransfer); tr.Status != StatusBlocked || *r.NextStep != StepTransfer {
		t.Fatalf("blocked: %+v", tr)
	}
	// The report waits while an earlier step is open, even once sent.
	fill(&r, Counts{BankTotal: 8, JournalDraft: 2, ReportSent: 1})
	if rp := stepOf(t, r.Steps, StepReport); rp.Status != StatusWaiting || rp.Action != nil || *r.NextStep != StepJournals {
		t.Fatalf("report before journals: %+v", rp)
	}
	// Closed: everything done.
	fill(&r, Counts{BankTotal: 8, JournalExported: 2, TransferImported: 1, ReportSent: 1})
	if r.NextStep != nil || r.DoneSteps != r.ActiveSteps || r.ActiveSteps != 6 {
		t.Fatalf("closed: %+v", r)
	}
}

func TestParsePeriod(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, istanbul)
	cases := map[string]string{
		"Atlas Gıda ekim fişlerini Luca'ya aktar": "2026-10",
		"Ekim'in raporunu gönder":                 "2026-10",
		"aralık 2025 kapanışı":                    "2025-12",
		"aralık kapanışı":                         "2025-12",
		"2026-03 fişleri":                         "2026-03",
		"3/2026 fişleri":                          "2026-03",
		"geçen ayın raporları":                    "2026-10",
		"bu ay":                                   "2026-11",
		"Şubat'ı kapat":                           "2026-02",
		"fişleri onayla":                          "",
	}
	for in, want := range cases {
		if got := ParsePeriod(in, now); got != want {
			t.Errorf("ParsePeriod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchEngagements(t *testing.T) {
	refs := []EngagementRef{
		{UUID: uuid.New(), TaxpayerTitle: "Atlas Gıda Sanayi ve Ticaret A.Ş.", TaxpayerTaxID: "1234567890"},
		{UUID: uuid.New(), TaxpayerTitle: "Gıda Pazarı Ltd. Şti."},
		{UUID: uuid.New(), TaxpayerTitle: "Oğuz Yılmaz"},
		{UUID: uuid.New(), TaxpayerTitle: "Ekim Mimarlık"},
	}
	got := matchEngagements("Atlas Gıda ekim fişlerini Luca'ya aktar", refs)
	if len(got) != 1 || got[0].UUID != refs[0].UUID {
		t.Fatalf("atlas: %+v", got)
	}
	if got := matchEngagements("Atlas'ın raporunu gönder", refs); len(got) != 1 || got[0].UUID != refs[0].UUID {
		t.Fatalf("suffix: %+v", got)
	}
	if got := matchEngagements("oğuz yılmaz ekim raporu", refs); len(got) != 1 || got[0].UUID != refs[2].UUID {
		t.Fatalf("person: %+v", got)
	}
	if got := matchEngagements("1234567890 raporu", refs); len(got) != 1 || got[0].UUID != refs[0].UUID {
		t.Fatalf("tax id: %+v", got)
	}
	if got := matchEngagements("ekim fişlerini aktar", refs); len(got) != 0 {
		t.Fatalf("month is not a taxpayer: %+v", got)
	}
}

func TestParseIntent(t *testing.T) {
	cases := map[string][]string{
		"Atlas Gıda ekim fişlerini Luca'ya aktar": {ActionExportTransfer},
		"banka hareketlerini onayla":              {ActionBankApprove},
		"banka hareketleri için hesap öner":       {ActionBankSuggest},
		"ekim fişlerini oluştur":                  {ActionBankDrafts},
		"fişleri onayla ve raporu gönder":         {ActionJournalsApprove, ActionReportSend},
	}
	for in, want := range cases {
		got := parseIntent(in).actions
		if len(got) != len(want) {
			t.Errorf("%q: %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q: %v, want %v", in, got, want)
			}
		}
	}
	if it := parseIntent("Atlas ekim kapanışı"); !it.close {
		t.Fatal("close")
	}
	if it := parseIntent("tüm mükelleflerin raporunu gönder"); !it.all {
		t.Fatal("all")
	}
	if it := parseIntent("zirve'ye aktar"); it.driver != "zirve" {
		t.Fatal("driver")
	}
}

func allPerms(string) bool { return true }

func TestRulesPlanner(t *testing.T) {
	atlas := EngagementRef{UUID: uuid.New(), TaxpayerTitle: "Atlas Gıda A.Ş."}
	other := EngagementRef{UUID: uuid.New(), TaxpayerTitle: "Deniz Lojistik"}
	counts := map[uuid.UUID]Counts{
		atlas.UUID: {BankTotal: 8, BankApproved: 2, JournalDraft: 1},
		other.UUID: {ReportSent: 1},
	}
	in := PlanInput{Now: time.Date(2026, 11, 17, 10, 0, 0, 0, istanbul), Engagements: []EngagementRef{atlas, other},
		Viewer: Viewer{Has: allPerms},
		Status: func(_ context.Context, refs []EngagementRef, _ string) (map[uuid.UUID]Row, error) {
			out := map[uuid.UUID]Row{}
			for _, e := range refs {
				r := Row{EngagementUUID: e.UUID}
				fill(&r, counts[e.UUID])
				out[e.UUID] = r
			}
			return out, nil
		}}
	in.Command = "Atlas Gıda ekim fişlerini Luca'ya aktar"
	d, err := RulesPlanner{}.Plan(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ActionBankDrafts, ActionJournalsApprove, ActionExportTransfer}
	if d.Period != "2026-10" || len(d.Steps) != len(want) {
		t.Fatalf("plan: %+v", d)
	}
	for i, a := range want {
		if d.Steps[i].Action != a || d.Steps[i].EngagementUUID != atlas.UUID || d.Steps[i].Driver != "luca" {
			t.Fatalf("step %d: %+v", i, d.Steps[i])
		}
	}
	// The report of Deniz is already sent; Atlas's is planned.
	in.Command = "tüm mükelleflerin ekim raporunu gönder"
	d, _ = RulesPlanner{}.Plan(context.Background(), in)
	if len(d.Steps) != 1 || d.Steps[0].EngagementUUID != atlas.UUID || d.Steps[0].Action != ActionReportSend {
		t.Fatalf("bulk report: %+v", d)
	}
	if len(d.Notes) != 1 || d.Notes[0].Code != NoteAlreadyDone || d.Notes[0].TaxpayerTitle != other.TaxpayerTitle {
		t.Fatalf("bulk report notes: %+v", d.Notes)
	}
	// Close: every remaining step of the chain.
	in.Command = "Atlas ekim kapanışı"
	d, _ = RulesPlanner{}.Plan(context.Background(), in)
	want = []string{ActionBankDrafts, ActionJournalsApprove, ActionExportTransfer, ActionReportSend}
	if len(d.Steps) != len(want) {
		t.Fatalf("close: %+v", d.Steps)
	}
	in.Command = "Bilinmeyen Firma raporu"
	if d, _ = (RulesPlanner{}).Plan(context.Background(), in); len(d.Steps) != 0 || d.Notes[0].Code != NoteEngagementNotFound {
		t.Fatalf("unknown: %+v", d)
	}
	in.Command = "merhaba"
	if d, _ = (RulesPlanner{}).Plan(context.Background(), in); len(d.Steps) != 0 || d.Notes[0].Code != NoteNotUnderstood {
		t.Fatalf("not understood: %+v", d)
	}
}

func TestValidate(t *testing.T) {
	mine := EngagementRef{UUID: uuid.New(), TaxpayerTitle: "Atlas"}
	foreign := uuid.New()
	onlyBank := func(p string) bool { return p == rbac.PermTenantBankWrite }
	d := Validate(Draft{Period: "2026-10", Steps: []PlanStep{
		{Action: ActionBankApprove, EngagementUUID: mine.UUID},
		{Action: ActionBankApprove, EngagementUUID: mine.UUID}, // duplicate
		{Action: ActionBankApprove, EngagementUUID: foreign},   // not reachable
		{Action: ActionExportTransfer, EngagementUUID: mine.UUID, Driver: "luca"},
		{Action: "delete_everything", EngagementUUID: mine.UUID},
		{Action: ActionOpenReview, EngagementUUID: mine.UUID}, // not a plan step
	}}, Viewer{Has: onlyBank}, []EngagementRef{mine})
	if len(d.Steps) != 1 || d.Steps[0].Action != ActionBankApprove || d.Steps[0].Period != "2026-10" ||
		d.Steps[0].TaxpayerTitle != "Atlas" || d.Steps[0].Status != StepPending {
		t.Fatalf("steps: %+v", d.Steps)
	}
	codes := map[string]int{}
	for _, n := range d.Notes {
		codes[n.Code]++
	}
	if codes[NoteEngagementNotFound] != 1 || codes[NoteNoPermission] != 1 || codes[NoteNotUnderstood] != 2 {
		t.Fatalf("notes: %+v", d.Notes)
	}
}
