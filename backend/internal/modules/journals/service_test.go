package journals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// groupingDriver writes "no;date;account;debit;credit" rows, 50 entries per
// file, and splits mahsup entries above 400 lines (the Luca rules, without the
// Luca template).
type groupingDriver struct{}

func (groupingDriver) Info() accountingexport.DriverInfo {
	return accountingexport.DriverInfo{Name: "journal_test", TemplateVer: "t1", Limits: accountingexport.Limits{MaxEntriesPerFile: 50, MaxLinesPerEntry: 400}}
}

func (g groupingDriver) Validate(_ context.Context, b accountingexport.Batch) []accountingexport.Issue {
	issues := accountingexport.CommonIssues(b)
	entries, _ := accountingexport.SelectEntries(b)
	for _, e := range entries {
		if e.Type == accountingexport.EntryMahsup {
			if _, ok := accountingexport.SplitEntry(e, 400); !ok {
				issues = append(issues, accountingexport.Issue{Code: accountingexport.IssueSplitImpossible, Severity: accountingexport.SeverityBlocker, EntryNo: e.No})
			}
		}
	}
	return issues
}

func (g groupingDriver) Export(ctx context.Context, b accountingexport.Batch, w accountingexport.FileWriter) (accountingexport.Result, error) {
	entries, issues := accountingexport.SelectEntries(b)
	var out []accountingexport.Entry
	res := accountingexport.Result{Issues: issues}
	for _, e := range entries {
		pieces, _ := accountingexport.SplitEntry(e, 400)
		out = append(out, pieces...)
		res.EntryIDs = append(res.EntryIDs, e.ID)
	}
	for i, group := range accountingexport.SplitIntoFiles(out, 50) {
		var sb strings.Builder
		for _, e := range group {
			for _, l := range e.Lines {
				fmt.Fprintf(&sb, "%s;%s;%s;%s;%s\n", e.No, e.Date.Format("02.01.2006"), l.Account, l.Debit.StringFixed(2), l.Credit.StringFixed(2))
			}
		}
		f, err := accountingexport.WriteFile(w, fmt.Sprintf("fis-%d.txt", i+1), "text/plain", []byte(sb.String()), len(group))
		if err != nil {
			return res, err
		}
		res.Files = append(res.Files, f)
	}
	return res, nil
}

func init() {
	accountingexport.Register("journal_test", func(accountingexport.Config) (accountingexport.Driver, error) { return groupingDriver{}, nil })
}

type env struct {
	pool  *pgxpool.Pool
	svc   *Service
	store *storage.Memory
	scope engagementctx.Scope
	actor int64
}

func setup(t *testing.T) env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	user := d.User(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, org, tp)
	store := storage.NewMemory()
	return env{pool: d.Pool, svc: New(d.Pool, store), store: store, scope: engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}, actor: user}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var oct = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

func (e env) draft(t *testing.T, date time.Time, lines ...LineInput) Draft {
	t.Helper()
	d, err := e.svc.CreateDraft(context.Background(), e.scope, e.actor, DraftInput{Date: date, Description: "test", Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e env) approve(t *testing.T, d Draft) {
	t.Helper()
	if err := e.svc.Approve(context.Background(), e.scope, e.actor, d.UUID); err != nil {
		t.Fatal(err)
	}
}

func debit(acc, amt string) LineInput  { return LineInput{Account: acc, Debit: dec(amt)} }
func credit(acc, amt string) LineInput { return LineInput{Account: acc, Credit: dec(amt)} }

// docs/02 §5 test 1: dengeli 3 fişlik batch → dosya; Fiş No + Tarih gruplaması.
func TestExportThreeBalancedEntries(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	d1 := e.draft(t, oct, debit("770.01", "100"), credit("320.01", "100"))
	d2 := e.draft(t, oct.AddDate(0, 0, 1), debit("153", "50.5"), debit("191.20", "10.1"), credit("320.02", "60.6"))
	d3 := e.draft(t, oct.AddDate(0, 0, 2), debit("102", "30"), credit("120", "30"))
	for _, d := range []Draft{d1, d2, d3} {
		e.approve(t, d)
	}
	if d1.EntryNo != 1 || d2.EntryNo != 2 || d3.EntryNo != 3 || d1.Period != "2026-10" {
		t.Fatalf("numbering: %d %d %d %s", d1.EntryNo, d2.EntryNo, d3.EntryNo, d1.Period)
	}
	b, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Files) != 1 || b.Entries != 3 || len(b.Files[0].SHA256) != 64 {
		t.Fatalf("batch: %+v", b)
	}
	rc, _, err := e.store.Download(ctx, b.Files[0].StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	buf := new(strings.Builder)
	_, _ = fmt.Fprint(buf, readAll(t, rc))
	want := "1;15.10.2026;770.01;100.00;0.00\n1;15.10.2026;320.01;0.00;100.00\n" +
		"2;16.10.2026;153;50.50;0.00\n2;16.10.2026;191.20;10.10;0.00\n2;16.10.2026;320.02;0.00;60.60\n" +
		"3;17.10.2026;102;30.00;0.00\n3;17.10.2026;120;0.00;30.00\n"
	if buf.String() != want {
		t.Fatalf("file:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// docs/02 §5 test 3: 450 satırlık mahsup → dengeli iki fiş; bölünemeyen → blocker.
func TestExportSplitsLongEntry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var lines []LineInput
	for i := range 225 {
		amt := fmt.Sprint(i + 1)
		lines = append(lines, debit("770", amt), credit("102", amt))
	}
	long := e.draft(t, oct, lines...)
	e.approve(t, long)
	b, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{long.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	if b.Entries != 1 || b.Files[0].Entries != 2 {
		t.Fatalf("split: %+v", b)
	}

	var bad []LineInput
	for range 449 {
		bad = append(bad, debit("770", "1"))
	}
	bad = append(bad, credit("102", "449"))
	stuck := e.draft(t, oct, bad...)
	e.approve(t, stuck)
	_, err = e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{stuck.UUID}})
	var be *BlockedError
	if !errors.As(err, &be) || be.Issues[0].Code != accountingexport.IssueSplitImpossible {
		t.Fatalf("unsplittable: %v", err)
	}
}

// docs/02 §5 test 5: aynı fişin ikinci kez çıktıya alınması reddediliyor.
func TestSecondExportRejectedUntilReversed(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	d := e.draft(t, oct, debit("770", "10"), credit("320", "10"))
	e.approve(t, d)
	in := ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{d.UUID}}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); !errors.Is(err, ErrAlreadyExported) {
		t.Fatalf("second export: %v", err)
	}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Period: "2026-10"}); !errors.Is(err, ErrNothingToExport) {
		t.Fatalf("period export after all exported: %v", err)
	}
	if err := e.svc.Reverse(ctx, e.scope, e.actor, d.UUID, "  "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("reverse without reason: %v", err)
	}
	if err := e.svc.Reverse(ctx, e.scope, e.actor, d.UUID, "Luca'da hesap kodu düzeltildi"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); err != nil {
		t.Fatalf("export after reverse: %v", err)
	}
}

// TestUnapprove is the #566 "onaylanmış fişi gerekçesiyle geri alma": only
// an approved, not yet exported entry goes back to draft; the reason is
// required and stays on the entry; an exported entry must be reversed first.
func TestUnapprove(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	d := e.draft(t, oct, debit("770", "10"), credit("320", "10"))
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, d.UUID, "yanlış fiş"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("unapprove a draft: %v", err)
	}
	e.approve(t, d)
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, d.UUID, "   "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("unapprove without reason: %v", err)
	}
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, uuid.New(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unapprove unknown: %v", err)
	}
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, d.UUID, " Hesap kodu yanlış, düzeltilecek "); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.GetDraft(ctx, e.scope, d.UUID)
	if err != nil || got.Status != "draft" || got.ApprovedAt != nil || got.ApprovalReverted == nil ||
		got.ApprovalReverted.Reason != "Hesap kodu yanlış, düzeltilecek" {
		t.Fatalf("after unapprove: %+v %+v %v", got.Draft, got.ApprovalReverted, err)
	}
	// A draft again: it is not exportable until it is approved again.
	in := ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{d.UUID}}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("export of an unapproved entry: %v", err)
	}
	e.approve(t, d)
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); err != nil {
		t.Fatal(err)
	}
	// Exported: the approval cannot be taken back; reverse the export first.
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, d.UUID, "geç kalındı"); !errors.Is(err, ErrAlreadyExported) {
		t.Fatalf("unapprove an exported entry: %v", err)
	}
	if err := e.svc.Reverse(ctx, e.scope, e.actor, d.UUID, "aktarım geri alındı"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Unapprove(ctx, e.scope, e.actor, d.UUID, "aktarım geri alındıktan sonra düzeltme"); err != nil {
		t.Fatalf("unapprove after reverse: %v", err)
	}
}

func TestApproveRefusesUnbalanced(t *testing.T) {
	e := setup(t)
	d := e.draft(t, oct, debit("770", "10"), credit("320", "9.99"))
	if err := e.svc.Approve(context.Background(), e.scope, e.actor, d.UUID); !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("approve: %v", err)
	}
	if _, err := e.svc.Export(context.Background(), e.scope, e.actor, ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{d.UUID}}); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("export draft: %v", err)
	}
	if _, err := e.svc.CreateDraft(context.Background(), e.scope, e.actor, DraftInput{Date: oct, Lines: []LineInput{{Account: "770", Debit: dec("1"), Credit: dec("1")}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("line with both sides: %v", err)
	}
}

func TestInvoicesReachLucaDirectly(t *testing.T) {
	e := setup(t)
	inv := e.draft(t, oct, LineInput{Account: "120", Debit: dec("120"), SourceKind: "invoice", SourceID: uuid.NewString()}, credit("600", "100"), credit("391", "20"))
	bank := e.draft(t, oct, debit("102", "120"), credit("120", "120"))
	e.approve(t, inv)
	e.approve(t, bank)
	// export_settings is enforced (issue #166): the settings and the export
	// run in the engagement's row level security scope, as in the API.
	ctx := e.scoped(t)
	if _, err := e.svc.q.UpsertExportSettings(ctx, dbSettings(e.scope, true)); err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Entries != 1 || len(b.Issues) != 1 || b.Issues[0].Code != accountingexport.IssueInvoiceEntryExcluded {
		t.Fatalf("batch: %+v", b)
	}
	// The invoice entry is still approved and can go out when chosen.
	b, err = e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{inv.UUID}, IncludeInvoiceEntries: true})
	if err != nil || b.Entries != 1 {
		t.Fatalf("include: %+v %v", b, err)
	}
}

func readAll(t *testing.T, rc interface {
	Read([]byte) (int, error)
	Close() error
}) string {
	t.Helper()
	defer func() { _ = rc.Close() }()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := rc.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func dbSettings(s engagementctx.Scope, direct bool) db.UpsertExportSettingsParams {
	return db.UpsertExportSettingsParams{OrganizationID: s.OrganizationID, EngagementID: s.ID, InvoicesReachLucaDirectly: direct, BlockOnUnknownAccount: true}
}

// The real excel_formula driver runs through the service: the workbook is
// stored and the batch records the driver.
func TestExportWithExcelFormulaDriver(t *testing.T) {
	e := setup(t)
	d := e.draft(t, oct, debit("770", "10"), credit("320", "10"))
	e.approve(t, d)
	b, err := e.svc.Export(context.Background(), e.scope, e.actor, ExportInput{Driver: "excel_formula", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Driver != "excel_formula" || !b.TemplateVerified || len(b.Files) != 1 || b.Files[0].Name != "kontrol_2026-10.xlsx" {
		t.Fatalf("batch: %+v", b)
	}
}

// The luca driver runs through the service with Luca's own fiş layout
// (verified-1, Oğuz'un geri bildirimi): the batch records a verified
// template and no "template unverified" warning.
func TestExportWithLucaDriver(t *testing.T) {
	e := setup(t)
	d := e.draft(t, oct, debit("770", "10"), credit("320", "10"))
	e.approve(t, d)
	b, err := e.svc.Export(context.Background(), e.scope, e.actor, ExportInput{Driver: "luca", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Driver != "luca" || !b.TemplateVerified || b.TemplateVersion != "verified-1" || len(b.Files) != 1 || b.Files[0].Name != "luca_fis_2026-10.xlsx" {
		t.Fatalf("batch: %+v", b)
	}
	for _, i := range b.Issues {
		if i.Code == accountingexport.IssueTemplateUnverified {
			t.Fatalf("issues: %+v", b.Issues)
		}
	}
}

// rlsScope is the scope rls.Middleware builds for the engagement.
func (e env) rlsScope() rls.Scope {
	return rls.Scope{OrganizationID: e.scope.OrganizationID, UserID: e.actor, EngagementsReadAll: true, TaxpayerIDs: []int64{e.scope.TaxpayerID}}
}

// scoped opens the engagement's row level security transaction for the rest
// of the test and returns its context; it commits at cleanup.
func (e env) scoped(t *testing.T) context.Context {
	t.Helper()
	ready := make(chan context.Context)
	done := make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		errc <- rls.WithScope(context.Background(), e.pool, e.rlsScope(), func(ctx context.Context, _ pgx.Tx) error {
			ready <- ctx
			<-done
			return nil
		})
	}()
	ctx := <-ready
	t.Cleanup(func() {
		close(done)
		if err := <-errc; err != nil {
			t.Errorf("scope: %v", err)
		}
	})
	return ctx
}

// TestCancelExport is the #710 office decision: an entry already
// transferred to the program is cancelled completely, without a correction
// entry. It leaves its export and goes back to draft in one step with the
// reason, can be corrected, approved and exported again; only an exported
// entry can be cancelled and the reason is required.
func TestCancelExport(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	d := e.draft(t, oct, debit("770", "10"), credit("320", "10"))
	e.approve(t, d)
	if err := e.svc.CancelExport(ctx, e.scope, e.actor, d.UUID, "x"); !errors.Is(err, ErrNotExported) {
		t.Fatalf("cancel of an approved entry: %v", err)
	}
	in := ExportInput{Driver: "journal_test", Drafts: []uuid.UUID{d.UUID}}
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CancelExport(ctx, e.scope, e.actor, d.UUID, "  "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("cancel without reason: %v", err)
	}
	if err := e.svc.CancelExport(ctx, e.scope, e.actor, uuid.New(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel unknown: %v", err)
	}
	if err := e.svc.CancelExport(ctx, e.scope, e.actor, d.UUID, " Luca'da silindi, tutar düzeltilecek "); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.GetDraft(ctx, e.scope, d.UUID)
	if err != nil || got.Status != "draft" || got.ApprovalReverted == nil ||
		got.ApprovalReverted.Reason != "Luca'da silindi, tutar düzeltilecek" {
		t.Fatalf("after cancel: %+v %+v %v", got.Draft, got.ApprovalReverted, err)
	}
	var reversed int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM export_batch_items WHERE journal_draft_id = (SELECT id FROM journal_drafts WHERE uuid = $1)
		AND reversed_at IS NOT NULL AND reversal_reason = 'Luca''da silindi, tutar düzeltilecek'`, d.UUID).Scan(&reversed); err != nil || reversed != 1 {
		t.Fatalf("export item reversed: %d %v", reversed, err)
	}
	// A second cancel finds no live export; approved again it exports again.
	if err := e.svc.CancelExport(ctx, e.scope, e.actor, d.UUID, "x"); !errors.Is(err, ErrNotExported) {
		t.Fatalf("second cancel: %v", err)
	}
	e.approve(t, d)
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); err != nil {
		t.Fatalf("export after cancel: %v", err)
	}
}
