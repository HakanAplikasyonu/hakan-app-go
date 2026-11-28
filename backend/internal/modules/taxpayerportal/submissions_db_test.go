package taxpayerportal

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

type recNotifier struct {
	mu  sync.Mutex
	got []notifmodel.EnqueueInput
}

func (n *recNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.got = append(n.got, in)
	return nil, nil
}

func (n *recNotifier) kinds(user int64) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, in := range n.got {
		if in.UserID != nil && *in.UserID == user {
			k, _ := in.Payload["kind"].(string)
			out = append(out, k)
		}
	}
	return out
}

// Senaryo B (#780): mükellef fişi gönderir → Gönderdiklerim'de "ulaştı" ve
// ofisin adıyla görünür, ofisin atanan üyesine bildirim gider; ofis reddedince
// fişi gönderen mükellef kullanıcısına bildirim gider ve durum "rejected" olur.
// Belge isteği yüklemesi de aynı listede, ofisin yeniden istemesi gerekçesiyle.
func TestTaxpayerSubmissionsAndNotifications(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	officeUser := e.d.User(t)
	e.d.Member(t, e.orgA, officeUser, "accountant")
	e.d.Assign(t, e.orgA, e.engA, officeUser)
	var officeName string
	if err := e.d.Pool.QueryRow(ctx, "SELECT name FROM organizations WHERE id = $1", e.orgA).Scan(&officeName); err != nil {
		t.Fatal(err)
	}

	notif := &recNotifier{}
	svc := receiptsusecase.New(e.d.Pool, storage.NewMemory(), &countingReader{}, nil, nil, nil).WithNotifier(notif, "https://panel.example")
	e.h.WithReceipts(svc)

	// Empty list: the office is known before anything was sent.
	var list SubmissionList
	rec := e.do(t, "GET", "/v1/taxpayer/submissions", e.tpA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	decode(t, rec, &list)
	if list.OfficeName != officeName || len(list.Items) != 0 {
		t.Fatalf("empty list = %+v", list)
	}

	body, ct := receiptForm(t, "gonderdiklerim-1")
	req := e.req(t, http.MethodPost, "/v1/taxpayer/receipts", body, ct)
	req.Header.Set("Idempotency-Key", "senaryo-b-1")
	rec = e.serve(req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	var capture struct {
		UUID   uuid.UUID `json:"uuid"`
		Status string    `json:"status"`
	}
	decode(t, rec, &capture)
	if got := notif.kinds(officeUser); len(got) != 1 || got[0] != receiptsusecase.NoticeReceiptSubmitted {
		t.Fatalf("office notifications = %v", got)
	}
	if got := notif.kinds(e.userA); len(got) != 0 {
		t.Fatalf("uploader must not be told about own upload: %v", got)
	}
	link := *notif.got[0].ActionURL
	if want := "/review?view=inbox"; len(link) < len(want) || !contains(link, want) {
		t.Fatalf("office link = %s", link)
	}

	// A replay (same Idempotency-Key) is not announced again.
	body, ct = receiptForm(t, "gonderdiklerim-1")
	req = e.req(t, http.MethodPost, "/v1/taxpayer/receipts", body, ct)
	req.Header.Set("Idempotency-Key", "senaryo-b-1")
	if rec = e.serve(req); rec.Code != http.StatusOK {
		t.Fatalf("replay = %d", rec.Code)
	}
	if got := notif.kinds(officeUser); len(got) != 1 {
		t.Fatalf("replay notified again: %v", got)
	}

	decode(t, e.do(t, "GET", "/v1/taxpayer/submissions", e.tpA, nil, ""), &list)
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}
	it := list.Items[0]
	if it.UUID != capture.UUID || it.Source != SourceReceipt || it.Status != SubmissionReceived || it.Kind != "fuel_receipt" ||
		it.OfficeName != officeName || it.VendorName != "Petrol A.Ş." || it.TaskUUID != nil {
		t.Fatalf("item = %+v", it)
	}

	// The office rejects: the mükellef user is told; the list says rejected.
	sc := engagementctx.Scope{ID: e.engA, OrganizationID: e.orgA, TaxpayerID: e.tpAID, TaxpayerUUID: e.tpA}
	// "diger" without words is refused and changes nothing (#780).
	if _, err := svc.DecideWithReason(ctx, sc, capture.UUID, officeUser, false, receiptsusecase.Rejection{Code: receiptsusecase.RejectOther}); !errors.Is(err, receiptsusecase.ErrInvalid) {
		t.Fatalf("diger without text: %v", err)
	}
	rej := receiptsusecase.Rejection{Code: receiptsusecase.RejectMissingInfo, Text: "Tarih okunmuyor"}
	if _, err := svc.DecideWithReason(ctx, sc, capture.UUID, officeUser, false, rej); err != nil {
		t.Fatal(err)
	}
	if got := notif.kinds(e.userA); len(got) != 1 || got[0] != receiptsusecase.NoticeReceiptRejected {
		t.Fatalf("taxpayer notifications = %v", got)
	}
	last := notif.got[len(notif.got)-1]
	if !contains(last.Body, "Gerekçe: Bilgi eksik (VKN, tarih ya da tutar): Tarih okunmuyor.") || last.Payload["reason_code"] != "eksik_bilgi" {
		t.Fatalf("reject notice = %q %v", last.Body, last.Payload)
	}
	if pv, _ := last.Payload["push_vars"].(map[string]string); pv["reason"] != "Bilgi eksik (VKN, tarih ya da tutar)" {
		t.Fatalf("push vars = %v", last.Payload["push_vars"])
	}
	decode(t, e.do(t, "GET", "/v1/taxpayer/submissions", e.tpA, nil, ""), &list)
	if list.Items[0].Status != SubmissionRejected || list.Items[0].ReasonCode != "eksik_bilgi" ||
		list.Items[0].Reason != "Bilgi eksik (VKN, tarih ya da tutar): Tarih okunmuyor" {
		t.Fatalf("after reject = %+v", list.Items[0])
	}

	// A document request upload the office asked again: rejected with its note.
	var attID int64
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO attachments (taxpayer_id, kind, storage_key, sha256, size_bytes, content_type, file_name)
		VALUES ($1, 'pdf', 'test/dekont.pdf', repeat('a', 64), 10, 'application/pdf', 'dekont.pdf') RETURNING id`, e.tpAID).Scan(&attID); err != nil {
		t.Fatal(err)
	}
	var taskID int64
	var taskUUID uuid.UUID
	if err := e.d.Pool.QueryRow(ctx, `INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, code, kind, period, status, title, note, due_at)
		VALUES ($1, $2, $3, 'DOC_REQUEST', 'document_request', '2026-10', 'open', 'Ekim banka dökümü', 'Okunmuyor, yeniden çekin', NOW() + interval '7 days')
		RETURNING id, uuid`, e.orgA, e.engA, e.tpAID).Scan(&taskID, &taskUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO document_task_uploads (organization_id, engagement_id, taxpayer_id, task_id, attachment_id, uploaded_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW() - interval '1 hour')`, e.orgA, e.engA, e.tpAID, taskID, attID, e.userA); err != nil {
		t.Fatal(err)
	}
	decode(t, e.do(t, "GET", "/v1/taxpayer/submissions", e.tpA, nil, ""), &list)
	var doc *Submission
	for i := range list.Items {
		if list.Items[i].Source == SourceDocumentRequest {
			doc = &list.Items[i]
		}
	}
	if doc == nil || doc.Status != SubmissionRejected || doc.Reason != "Okunmuyor, yeniden çekin" || doc.Title != "Ekim banka dökümü" ||
		doc.TaskUUID == nil || *doc.TaskUUID != taskUUID.String() || doc.MediaType != "application/pdf" {
		t.Fatalf("document request upload = %+v", doc)
	}
	if !list.Items[0].CreatedAt.After(time.Time{}) {
		t.Fatal("created_at missing")
	}

	// Another company sees nothing of this.
	if rec := e.do(t, "GET", "/v1/taxpayer/submissions", e.tpB, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("other company = %d", rec.Code)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
