package journals

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// invoice inserts an invoice of the engagement's taxpayer and returns its id.
func (e env) invoice(t *testing.T, ettn, number, sourceSystem string) int64 {
	t.Helper()
	var id int64
	var ettnArg any
	if ettn != "" {
		ettnArg = ettn
	}
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, ettn, invoice_number,
			direction, type, issue_date, period, currency, tax_summary_source)
		VALUES ($1, $2, $3, 'api', $4, $5, 'incoming', 'ALIS', DATE '2026-10-03', '2026-10', 'TRY', 'header_estimate')
		RETURNING id`, e.scope.TaxpayerID, sourceSystem, uuid.NewString(), ettnArg, number).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) invoiceItem(t *testing.T, invoiceID int64) int64 {
	t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no) VALUES ($1, $2, 1) RETURNING id`,
		e.scope.TaxpayerID, invoiceID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// invoiceDraft is an approved purchase entry booked from invoice id.
func (e env) invoiceDraft(t *testing.T, kind string, id int64) Draft {
	t.Helper()
	src := strconv.FormatInt(id, 10)
	d := e.draft(t, oct,
		LineInput{Account: "153", Debit: dec("100"), SourceKind: kind, SourceID: src},
		LineInput{Account: "191", Debit: dec("20"), SourceKind: kind, SourceID: src},
		LineInput{Account: "320.01", Credit: dec("120"), SourceKind: kind, SourceID: src})
	// Aynı belgeden ikinci fiş bu testlerde bilerek kurulur; mükerrer
	// engelini (#710) açıkça "yine de gönder" ile geçer.
	if err := e.svc.ApproveWith(context.Background(), e.scope, e.actor, d.UUID,
		Confirmation{SendDuplicate: true, Reason: "test: aynı belgeden ikinci fiş"}); err != nil {
		t.Fatal(err)
	}
	return d
}

func issueCodes(is []accountingexport.Issue) map[string]int {
	out := map[string]int{}
	for _, i := range is {
		out[i.Code]++
	}
	return out
}

const (
	ettnA = "0f3c1a2e-1111-4a5b-8c9d-00000000000a"
	ettnB = "0f3c1a2e-1111-4a5b-8c9d-00000000000b"
	ettnC = "0f3c1a2e-1111-4a5b-8c9d-00000000000c"
)

// The same ETTN under a second entry: the second export leaves it out with
// DOCUMENT_ALREADY_EXPORTED until the accountant confirms the document; the
// confirmation is recorded with the document → file trail.
func TestDuplicateETTNNeedsConfirmation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	inv := e.invoice(t, ettnA, "ABC2026000000001", "entegrator_izibiz")
	first := e.invoiceDraft(t, "invoice", inv)
	b1, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "luca", Drafts: []uuid.UUID{first.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	if issueCodes(b1.Issues)[accountingexport.IssueDocumentAlreadyExported] != 0 || len(b1.Excluded) != 0 {
		t.Fatalf("first export: %+v", b1)
	}
	docs, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Batch: &b1.UUID, Limit: 50})
	if err != nil || len(docs) != 1 || docs[0].ETTN != ettnA || docs[0].FileName != "luca_fis.xlsx" ||
		docs[0].DocumentNo != "ABC2026000000001" || len(docs[0].ConfirmedCodes) != 0 || docs[0].Draft != first.UUID {
		t.Fatalf("trail: %+v %v", docs, err)
	}

	// A second entry from the same invoice (an item line this time).
	second := e.invoiceDraft(t, "invoice_item", e.invoiceItem(t, inv))
	in := ExportInput{Driver: "luca", Drafts: []uuid.UUID{second.UUID}}
	chk, err := e.svc.Check(ctx, e.scope, in)
	if err != nil {
		t.Fatal(err)
	}
	if !chk.Blocked || chk.Entries != 0 || len(chk.Excluded) != 1 || chk.Excluded[0].Draft != second.UUID ||
		!slices.Equal(chk.Excluded[0].Documents, []string{ettnA}) {
		t.Fatalf("check: %+v", chk)
	}
	var dup accountingexport.Issue
	for _, i := range chk.Issues {
		if i.Code == accountingexport.IssueDocumentAlreadyExported {
			dup = i
		}
	}
	if dup.Severity != accountingexport.SeverityWarning || dup.Params["previous_batch"] != b1.UUID.String() ||
		dup.Params["previous_file"] != "luca_fis.xlsx" || dup.Params["ettn"] != ettnA || dup.Params["excluded"] != true {
		t.Fatalf("duplicate issue: %+v", dup)
	}
	// Without confirmation nothing goes out and the draft stays approved.
	var be *BlockedError
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); !errors.As(err, &be) {
		t.Fatalf("unconfirmed export: %v", err)
	}
	if got, _ := e.svc.GetDraft(ctx, e.scope, second.UUID); got.Status != "approved" {
		t.Fatalf("draft after refused export: %s", got.Status)
	}
	// Confirmed: exported, the confirmation is kept.
	in.ConfirmDocuments = []string{" " + ettnA + " "}
	b2, err := e.svc.Export(ctx, e.scope, e.actor, in)
	if err != nil || b2.Entries != 1 {
		t.Fatalf("confirmed export: %+v %v", b2, err)
	}
	if c := issueCodes(b2.Issues); c[accountingexport.IssueDocumentAlreadyExported] != 1 {
		t.Fatalf("confirmed issues: %+v", b2.Issues)
	}
	all, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Q: ettnA[:8], Limit: 50})
	if err != nil || len(all) != 2 {
		t.Fatalf("history by ETTN: %+v %v", all, err)
	}
	if all[0].Batch != b2.UUID || !slices.Equal(all[0].ConfirmedCodes, []string{accountingexport.IssueDocumentAlreadyExported}) || all[0].ConfirmedAt == nil {
		t.Fatalf("confirmed trail: %+v", all[0])
	}

	// A reversed export no longer counts: after reversing both entries the
	// first one goes out again without a warning.
	for _, d := range []Draft{first, second} {
		if err := e.svc.Reverse(ctx, e.scope, e.actor, d.UUID, "Luca'ya yüklenmedi"); err != nil {
			t.Fatal(err)
		}
	}
	b3, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "luca", Drafts: []uuid.UUID{first.UUID}})
	if err != nil || issueCodes(b3.Issues)[accountingexport.IssueDocumentAlreadyExported] != 0 {
		t.Fatalf("after reversal: %+v %v", b3, err)
	}
	// Another program does not count either.
	if chk, err := e.svc.Check(ctx, e.scope, ExportInput{Driver: "zirve", Drafts: []uuid.UUID{second.UUID}}); err != nil || len(chk.Excluded) != 0 {
		t.Fatalf("zirve check: %+v %v", chk, err)
	}
}

// A package where only part of the documents went out before: the new
// entries are exported, the repeated one is left out and listed.
func TestPartialRepeatPackage(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	invA := e.invoice(t, ettnA, "A-1", "entegrator_izibiz")
	invB := e.invoice(t, ettnB, "B-1", "entegrator_izibiz")
	old := e.invoiceDraft(t, "invoice", invA)
	if _, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "luca", Drafts: []uuid.UUID{old.UUID}}); err != nil {
		t.Fatal(err)
	}
	repeat := e.invoiceDraft(t, "invoice", invA)
	fresh := e.invoiceDraft(t, "invoice", invB)
	plain := e.draft(t, oct, debit("770", "5"), credit("102", "5"))
	e.approve(t, plain)
	b, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "luca", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Entries != 2 || len(b.Excluded) != 1 || b.Excluded[0].Draft != repeat.UUID {
		t.Fatalf("partial: entries %d excluded %+v", b.Entries, b.Excluded)
	}
	for _, d := range []Draft{fresh, plain} {
		if got, _ := e.svc.GetDraft(ctx, e.scope, d.UUID); got.Status != "exported" {
			t.Fatalf("%d not exported: %s", d.EntryNo, got.Status)
		}
	}
	if got, _ := e.svc.GetDraft(ctx, e.scope, repeat.UUID); got.Status != "approved" {
		t.Fatalf("repeat exported: %s", got.Status)
	}
	docs, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Batch: &b.UUID, Limit: 50})
	if err != nil || len(docs) != 1 || docs[0].ETTN != ettnB {
		t.Fatalf("documents of the package: %+v %v", docs, err)
	}
}

// Two entries of one package from the same invoice: the second is a repeat.
func TestSameETTNTwiceInOnePackage(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	inv := e.invoice(t, ettnC, "C-1", "entegrator_izibiz")
	a := e.invoiceDraft(t, "invoice", inv)
	b := e.invoiceDraft(t, "invoice", inv)
	chk, err := e.svc.Check(ctx, e.scope, ExportInput{Driver: "luca", Drafts: []uuid.UUID{a.UUID, b.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	if chk.Entries != 1 || len(chk.Excluded) != 1 || chk.Excluded[0].Draft != b.UUID {
		t.Fatalf("check: %+v", chk)
	}
}

// Luca fetches İşNet invoices itself: off → exported, office setting on →
// left out with PROGRAM_PULLS_DOCUMENT, taxpayer override [] → exported
// again, confirmed → exported with the code recorded. Other programs and
// other integrators are not affected.
func TestLucaPullSetting(t *testing.T) {
	e := setup(t)
	ctx := e.scoped(t) // export_settings is enforced
	isnet := e.invoice(t, ettnA, "ISN-1", "entegrator_isnet")
	other := e.invoice(t, ettnB, "IZB-1", "entegrator_izibiz")
	dIsnet := e.invoiceDraft(t, "invoice", isnet)
	dOther := e.invoiceDraft(t, "invoice", other)
	in := ExportInput{Driver: "luca", Drafts: []uuid.UUID{dIsnet.UUID, dOther.UUID}}

	// Off (no settings saved).
	chk, err := e.svc.Check(ctx, e.scope, in)
	if err != nil || chk.Entries != 2 || len(chk.Excluded) != 0 {
		t.Fatalf("off: %+v %v", chk, err)
	}
	// Office: Luca pulls İşNet.
	if _, err := e.svc.SaveOfficeSettings(ctx, e.scope.OrganizationID, e.actor, OfficeSettings{LucaPullIntegrators: []string{"isnet", "ISNET"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveOfficeSettings(ctx, e.scope.OrganizationID, e.actor, OfficeSettings{LucaPullIntegrators: []string{"izibiz"}}); !errors.Is(err, ErrInvalidIntegrator) {
		t.Fatalf("unknown integrator: %v", err)
	}
	st, err := e.svc.GetSettings(ctx, e.scope)
	if err != nil || st.LucaPullIntegrators != nil || !slices.Equal(st.EffectiveLucaPullIntegrators, []string{"isnet"}) {
		t.Fatalf("settings after office: %+v %v", st, err)
	}
	chk, err = e.svc.Check(ctx, e.scope, in)
	if err != nil || chk.Entries != 1 || len(chk.Excluded) != 1 || chk.Excluded[0].Draft != dIsnet.UUID {
		t.Fatalf("office on: %+v %v", chk, err)
	}
	if issueCodes(chk.Issues)[accountingexport.IssueProgramPullsDocument] != 1 {
		t.Fatalf("office on issues: %+v", chk.Issues)
	}
	// Zirve is not affected.
	if z, err := e.svc.Check(ctx, e.scope, ExportInput{Driver: "zirve", Drafts: in.Drafts}); err != nil || len(z.Excluded) != 0 {
		t.Fatalf("zirve: %+v %v", z, err)
	}
	// Taxpayer override: Luca pulls nothing for this taxpayer.
	st, err = e.svc.SaveSettings(ctx, e.scope, e.actor, SettingsInput{BlockOnUnknownAccount: true, LucaPullIntegrators: json.RawMessage(`[]`)})
	if err != nil || st.LucaPullIntegrators == nil || len(*st.LucaPullIntegrators) != 0 || len(st.EffectiveLucaPullIntegrators) != 0 {
		t.Fatalf("override: %+v %v", st, err)
	}
	if chk, err = e.svc.Check(ctx, e.scope, in); err != nil || len(chk.Excluded) != 0 {
		t.Fatalf("override off: %+v %v", chk, err)
	}
	// Omitted keeps the override; null returns to the office setting.
	if st, err = e.svc.SaveSettings(ctx, e.scope, e.actor, SettingsInput{BlockOnUnknownAccount: false}); err != nil || st.LucaPullIntegrators == nil || st.BlockOnUnknownAccount {
		t.Fatalf("omitted: %+v %v", st, err)
	}
	if st, err = e.svc.SaveSettings(ctx, e.scope, e.actor, SettingsInput{BlockOnUnknownAccount: true, LucaPullIntegrators: json.RawMessage(`null`)}); err != nil || st.LucaPullIntegrators != nil {
		t.Fatalf("null: %+v %v", st, err)
	}
	if _, err = e.svc.SaveSettings(ctx, e.scope, e.actor, SettingsInput{LucaPullIntegrators: json.RawMessage(`["logo"]`)}); !errors.Is(err, ErrInvalidIntegrator) {
		t.Fatalf("bad override: %v", err)
	}
	// Confirmed: both go out, the code is recorded.
	in.ConfirmDocuments = []string{ettnA}
	b, err := e.svc.Export(ctx, e.scope, e.actor, in)
	if err != nil || b.Entries != 2 || len(b.Excluded) != 0 {
		t.Fatalf("confirmed: %+v %v", b, err)
	}
	docs, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Q: "ISN-1", Limit: 10})
	if err != nil || len(docs) != 1 || !slices.Equal(docs[0].ConfirmedCodes, []string{accountingexport.IssueProgramPullsDocument}) {
		t.Fatalf("trail: %+v %v", docs, err)
	}
}

// An invoice from Excel takes the taxpayer's only integrator connection.
func TestLucaPullFallbackIntegrator(t *testing.T) {
	e := setup(t)
	ctx := e.scoped(t)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO source_connections (taxpayer_id, source_system, status)
		VALUES ($1, 'entegrator_isnet', 'active')`, e.scope.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	inv := e.invoice(t, ettnA, "XL-1", "excel_import")
	d := e.invoiceDraft(t, "invoice", inv)
	if _, err := e.svc.SaveOfficeSettings(ctx, e.scope.OrganizationID, e.actor, OfficeSettings{LucaPullIntegrators: []string{"isnet"}}); err != nil {
		t.Fatal(err)
	}
	chk, err := e.svc.Check(ctx, e.scope, ExportInput{Driver: "luca", Drafts: []uuid.UUID{d.UUID}})
	if err != nil || len(chk.Excluded) != 1 {
		t.Fatalf("fallback: %+v %v", chk, err)
	}
}

// Two exports of the same package at once: the draft uniqueness keeps one
// file; the other fails cleanly (already exported, or held back by the
// guard when it saw the first one's documents) and no document row is left
// behind.
func TestConcurrentExportsOfSameDocument(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	d := e.invoiceDraft(t, "invoice", e.invoice(t, ettnA, "A-1", "entegrator_izibiz"))
	in := ExportInput{Driver: "luca", Drafts: []uuid.UUID{d.UUID}}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Export(ctx, e.scope, e.actor, in)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if be := (*BlockedError)(nil); !errors.Is(err, ErrAlreadyExported) && !errors.As(err, &be) {
			t.Fatalf("unexpected: %v", err)
		}
	}
	docs, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Limit: 10})
	if ok != 1 || err != nil || len(docs) != 1 {
		t.Fatalf("ok=%d docs=%d err=%v", ok, len(docs), err)
	}
}
