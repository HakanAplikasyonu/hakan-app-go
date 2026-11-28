package journals

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// Çift aktarım koruması Z raporu ve banka hareketinde iki yönlü: yevmiye
// dışa aktarımı (fiş dosyası) belgeyi Luca listelerinin anahtarıyla
// ("z:<uuid>", "bank:<uuid>") yazar ve liste dosyasıyla gitmiş belgeyi
// onaylanmadıkça dışarıda bırakır. Uydurma veriler.
func TestBankAndZReportDoubleExportGuard(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// Z raporu önce fiş dosyasıyla gider: belge izinde "z:<uuid>" (ETTN değil).
	z := uuid.New()
	zsrc := "retail_sale:" + z.String()
	zd := e.draft(t, oct,
		LineInput{Account: "100.01", Debit: dec("1200"), SourceKind: "receipt", SourceID: zsrc},
		LineInput{Account: "600.01", Credit: dec("1000"), SourceKind: "receipt", SourceID: zsrc},
		LineInput{Account: "391.01", Credit: dec("200"), SourceKind: "receipt", SourceID: zsrc})
	e.approve(t, zd)
	b1, err := e.svc.Export(ctx, e.scope, e.actor, ExportInput{Driver: "luca", Drafts: []uuid.UUID{zd.UUID}})
	if err != nil || b1.Entries != 1 {
		t.Fatalf("z export: %+v %v", b1, err)
	}
	docs, err := e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Batch: &b1.UUID, Limit: 50})
	if err != nil || len(docs) != 1 || docs[0].DocumentKey != ZReportDocumentKey(z) || docs[0].ETTN != "" {
		t.Fatalf("z trail: %+v %v", docs, err)
	}
	// Aynı Z raporundan ikinci fiş: dışarıda kalır (liste dosyası da bu
	// kaydı ListLiveDocumentExports ile okur).
	zd2 := e.draft(t, oct,
		LineInput{Account: "100.01", Debit: dec("1200"), SourceKind: "receipt", SourceID: zsrc},
		LineInput{Account: "600.01", Credit: dec("1200"), SourceKind: "receipt", SourceID: zsrc})
	// Aynı belgeden ikinci fiş bilerek kurulur: mükerrer engelini (#710)
	// "yine de gönder" ile geçer.
	if err := e.svc.ApproveWith(ctx, e.scope, e.actor, zd2.UUID,
		Confirmation{SendDuplicate: true, Reason: "test: aynı Z raporundan ikinci fiş"}); err != nil {
		t.Fatal(err)
	}
	chk, err := e.svc.Check(ctx, e.scope, ExportInput{Driver: "luca", Drafts: []uuid.UUID{zd2.UUID}})
	if err != nil || !chk.Blocked || len(chk.Excluded) != 1 || !slices.Equal(chk.Excluded[0].Documents, []string{ZReportDocumentKey(z)}) {
		t.Fatalf("second z check: %+v %v", chk, err)
	}

	// Banka hareketi önce "Luca'ya aktar (Excel)" liste dosyasıyla gitti.
	var box, tx int64
	var txUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Deneme Banka TL', 'TRY') RETURNING id`,
		e.scope.TaxpayerID).Scan(&box); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
			booked_at, transaction_date, debit_credit, amount, currency, description, period, counterparty_name)
		VALUES ($1, $2, 'bank_test', 'b-guard-1', 'api', '2026-10-05', '2026-10-05', 'A', 2360, 'TRY', 'Tahsilat', '2026-10', 'Deneme Müşteri Ltd.')
		RETURNING id, uuid`, e.scope.TaxpayerID, box).Scan(&tx, &txUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO expense_export_documents (organization_id, engagement_id, export_uuid, driver, period, file_name,
			document_key, source_kind, source_uuid)
		VALUES ($1, $2, $3, 'luca', '2026-10', 'banka_luca_fis_2026-10.xlsx', $4, 'bank_transaction', $5)`,
		e.scope.OrganizationID, e.scope.ID, uuid.New(), BankDocumentKey(txUUID), txUUID); err != nil {
		t.Fatal(err)
	}
	src := strconv.FormatInt(tx, 10)
	bd := e.draft(t, oct,
		LineInput{Account: "102.01", Debit: dec("2360"), SourceKind: BankSourceKind, SourceID: src},
		LineInput{Account: "120.01", Credit: dec("2360"), SourceKind: BankSourceKind, SourceID: src})
	e.approve(t, bd)
	in := ExportInput{Driver: "luca", Drafts: []uuid.UUID{bd.UUID}}
	chk, err = e.svc.Check(ctx, e.scope, in)
	if err != nil || !chk.Blocked || len(chk.Excluded) != 1 || !slices.Equal(chk.Excluded[0].Documents, []string{BankDocumentKey(txUUID)}) {
		t.Fatalf("bank check: %+v %v", chk, err)
	}
	var dup accountingexport.Issue
	for _, i := range chk.Issues {
		if i.Code == accountingexport.IssueDocumentAlreadyExported {
			dup = i
		}
	}
	if dup.Params["previous_file"] != "banka_luca_fis_2026-10.xlsx" || dup.Params["excluded"] != true {
		t.Fatalf("bank issue: %+v", dup)
	}
	var be *BlockedError
	if _, err := e.svc.Export(ctx, e.scope, e.actor, in); !errors.As(err, &be) {
		t.Fatalf("unconfirmed bank export: %v", err)
	}
	// Onaylanınca gider; belge banka anahtarıyla ve onay koduyla yazılır.
	in.ConfirmDocuments = []string{BankDocumentKey(txUUID)}
	b2, err := e.svc.Export(ctx, e.scope, e.actor, in)
	if err != nil || b2.Entries != 1 {
		t.Fatalf("confirmed bank export: %+v %v", b2, err)
	}
	docs, err = e.svc.ListDocuments(ctx, e.scope, DocumentFilter{Batch: &b2.UUID, Limit: 50})
	if err != nil || len(docs) != 1 || docs[0].DocumentKey != BankDocumentKey(txUUID) ||
		!slices.Equal(docs[0].ConfirmedCodes, []string{accountingexport.IssueDocumentAlreadyExported}) {
		t.Fatalf("bank trail: %+v %v", docs, err)
	}
}
