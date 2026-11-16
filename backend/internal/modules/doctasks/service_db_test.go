package doctasks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type memStore struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func (m *memStore) Upload(_ context.Context, f storage.File, path string) error {
	b, err := io.ReadAll(f.Body)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objs[path] = b
	return nil
}

func (m *memStore) Download(_ context.Context, path string) (io.ReadCloser, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objs[path]
	if !ok {
		return nil, 0, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

type fakeNotifier struct {
	mu  sync.Mutex
	got []notifmodel.EnqueueInput
}

func (f *fakeNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, in)
	return nil, nil
}

func (f *fakeNotifier) count(template string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, in := range f.got {
		if in.TemplateCode == template {
			n++
		}
	}
	return n
}

type world struct {
	d                   *testdb.DB
	svc                 *Service
	store               *memStore
	notif               *fakeNotifier
	clock               *time.Time
	scope               engagementctx.Scope
	taxpayerID, member  int64
	payment, collection int64 // bank transaction ids
	fee                 int64
}

func setupWorld(t *testing.T) *world {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	accountant := d.User(t)
	d.Member(t, org, accountant, "accountant")
	tp := d.Taxpayer(t, "legal", "9000000036")
	eng := d.Engagement(t, org, tp)
	member := d.User(t)
	if _, err := d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, tp, member); err != nil {
		t.Fatal(err)
	}
	var box int64
	if err := d.Pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Banka TL', 'TRY') RETURNING id`, tp).Scan(&box); err != nil {
		t.Fatal(err)
	}
	insert := func(src, dc, kindDetail, amount string) int64 {
		var id int64
		err := d.Pool.QueryRow(ctx, `
			INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
				booked_at, transaction_date, debit_credit, amount, currency, description, kind_detail, period, counterparty_name)
			VALUES ($1, $2, 'bank_test', $3, 'api', '2026-10-05T10:00:00Z', '2026-10-05', $4, $5, 'TRY', 'açıklama', $6, '2026-10', 'ACME LTD')
			RETURNING id`, tp, box, src, dc, amount, kindDetail).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	w := &world{d: d, store: &memStore{objs: map[string][]byte{}}, notif: &fakeNotifier{}, taxpayerID: tp, member: member}
	w.payment = insert("p-1", "B", "supplier_payment", "1250.5000")
	w.collection = insert("c-1", "A", "customer_collection", "900")
	w.fee = insert("f-1", "B", "bank_fee", "12.5")
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	w.clock = &now
	w.svc = New(d.Pool, w.store, w.notif, Config{FrontendURL: "https://app.test"}, nil).
		WithClock(func() time.Time { return *w.clock })
	w.scope = engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp, Status: engagementctx.StatusActive}
	return w
}

func (w *world) warningStatus(t *testing.T, txID int64) []string {
	t.Helper()
	rows, err := w.d.Pool.Query(context.Background(), `
		SELECT status FROM data_warnings
		WHERE engagement_id = $1 AND entity_type = 'bank_transaction' AND entity_id = $2 AND producer = $3
		ORDER BY id`, w.scope.ID, txID, Producer)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func taskFor(t *testing.T, tasks []Task, txID int64, w *world) Task {
	t.Helper()
	for _, x := range tasks {
		if x.bankTransactionID.Int64 == txID {
			return x
		}
	}
	t.Fatalf("no task for bank transaction %d", txID)
	return Task{}
}

// TestAcceptanceFlow is issue #36's acceptance: warning open → task →
// upload → match → warning auto_resolved, plus reminders and isolation.
func TestAcceptanceFlow(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	sub := subjectOf(w.scope)

	// 1. Detection raises one warning per undocumented movement.
	rep, err := w.svc.Detect(ctx, sub, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 2 || rep.Created != 2 {
		t.Fatalf("detect: %+v", rep)
	}
	if got := w.warningStatus(t, w.payment); len(got) != 1 || got[0] != "open" {
		t.Fatalf("payment warning: %v", got)
	}
	if got := w.warningStatus(t, w.fee); len(got) != 0 {
		t.Fatalf("fee warning: %v", got)
	}
	// Re-running only refreshes.
	if rep, err = w.svc.Detect(ctx, sub, "2026-10"); err != nil || rep.Created != 0 || rep.Refreshed != 2 {
		t.Fatalf("re-detect: %+v %v", rep, err)
	}

	// 2. The office opens tasks; the mükellef's member is notified once.
	gen, err := w.svc.Generate(ctx, w.scope, 0, GenerateInput{Period: "2026-10", Detect: true, Note: "Faturayı yükleyin"})
	if err != nil {
		t.Fatal(err)
	}
	if gen.Created != 2 || gen.Notified != 1 || w.notif.count(TemplateCreated) != 1 {
		t.Fatalf("generate: %+v notifications=%d", gen, w.notif.count(TemplateCreated))
	}
	if in := w.notif.got[0]; in.UserID == nil || *in.UserID != w.member || in.TemplateVars["count"] != "2" {
		t.Fatalf("notification: %+v", in)
	}
	// Two tasks: the link opens the task list; one task: the task itself (#201).
	if in := w.notif.got[0]; in.ActionURL == nil || !strings.HasSuffix(*in.ActionURL, "/tasks") {
		t.Fatalf("two-task link = %v", in.ActionURL)
	}
	var taskID int64
	var taskUUID string
	if err := w.d.Pool.QueryRow(ctx, `SELECT id, uuid::text FROM document_tasks WHERE engagement_id = $1 ORDER BY id LIMIT 1`, w.scope.ID).Scan(&taskID, &taskUUID); err != nil {
		t.Fatal(err)
	}
	if got := w.svc.singleTask(ctx, w.scope.OrganizationID, w.scope.ID, []int64{taskID}); got != taskUUID {
		t.Fatalf("singleTask = %q, want %q", got, taskUUID)
	}
	if got := w.svc.singleTask(ctx, w.scope.OrganizationID, w.scope.ID, []int64{taskID, taskID + 1}); got != "" {
		t.Fatalf("several tasks: %q", got)
	}
	if got := w.svc.singleTask(ctx, w.scope.OrganizationID+999, w.scope.ID, []int64{taskID}); got != "" {
		t.Fatalf("another office: %q", got)
	}
	if again, err := w.svc.Generate(ctx, w.scope, 0, GenerateInput{Period: "2026-10"}); err != nil || again.Created != 0 {
		t.Fatalf("generate again: %+v %v", again, err)
	}
	office, total, err := w.svc.List(ctx, w.scope, Filter{Status: StatusOpen, Limit: 20})
	if err != nil || total != 2 || len(office) != 2 {
		t.Fatalf("office list: %d %v", total, err)
	}

	// 3. The mükellef sees its tasks; another taxpayer and another office do not.
	mine, total, err := w.svc.ListMine(ctx, w.taxpayerID, Filter{Status: StatusOpen, Limit: 20})
	if err != nil || total != 2 || len(mine) != 2 {
		t.Fatalf("mine: %d %v", total, err)
	}
	payTask := taskFor(t, mine, w.payment, w)
	if payTask.Code != CodeUndocumentedPayment || payTask.OrganizationName == "" || payTask.ReminderCount != nil ||
		payTask.BankTransaction.Amount != "1250.50" || payTask.BankTransaction.Direction != "out" || payTask.Note != "Faturayı yükleyin" {
		t.Fatalf("taxpayer task view: %+v", payTask)
	}
	other := w.d.Taxpayer(t, "legal", "9000000044")
	if _, n, _ := w.svc.ListMine(ctx, other, Filter{Limit: 20}); n != 0 {
		t.Fatalf("other taxpayer sees %d tasks", n)
	}
	if _, err := w.svc.GetMine(ctx, other, payTask.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other taxpayer get: %v", err)
	}
	otherOrg := w.d.Organization(t)
	otherScope := engagementctx.Scope{ID: w.d.Engagement(t, otherOrg, other), OrganizationID: otherOrg, TaxpayerID: other}
	if _, err := w.svc.Get(ctx, otherScope, payTask.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office get: %v", err)
	}
	if _, err := w.svc.Upload(ctx, other, payTask.UUID, w.member, "x.pdf", []byte("%PDF-1.4 x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other taxpayer upload: %v", err)
	}

	// 4. Upload through the HTTP handler → match → warning auto_resolved.
	if _, err := w.svc.Upload(ctx, w.taxpayerID, payTask.UUID, w.member, "x.html", []byte("<html>")); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("html upload: %v", err)
	}
	pdf := []byte("%PDF-1.4\n% fatura\n")
	res := uploadHTTP(t, NewHandler(w.svc), w, payTask.UUID, "fatura.pdf", pdf)
	if !res.Matched || !res.WarningResolved || res.Task.Status != StatusMatched || len(res.Task.Uploads) != 1 {
		t.Fatalf("upload result: %+v", res)
	}
	if got := w.warningStatus(t, w.payment); len(got) != 1 || got[0] != "auto_resolved" {
		t.Fatalf("payment warning after upload: %v", got)
	}
	var docs int
	if err := w.d.Pool.QueryRow(ctx, `SELECT count(*) FROM bank_transaction_documents WHERE taxpayer_id = $1 AND bank_transaction_id = $2`,
		w.taxpayerID, w.payment).Scan(&docs); err != nil || docs != 1 {
		t.Fatalf("bank_transaction_documents = %d %v", docs, err)
	}
	if _, err := w.svc.Upload(ctx, w.taxpayerID, payTask.UUID, w.member, "again.pdf", pdf); !errors.Is(err, ErrClosed) {
		t.Fatalf("upload to matched task: %v", err)
	}
	// The office sees the upload and can download it.
	detail, err := w.svc.Get(ctx, w.scope, payTask.UUID)
	if err != nil || detail.Status != StatusMatched || len(detail.Uploads) != 1 {
		t.Fatalf("office detail: %+v %v", detail, err)
	}
	f, err := w.svc.OfficeFile(ctx, w.scope, payTask.UUID, detail.Uploads[0].UUID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(f.Body)
	_ = f.Body.Close()
	if !bytes.Equal(body, pdf) || f.ContentType != "application/pdf" {
		t.Fatalf("download: %q %s", body, f.ContentType)
	}
	if _, err := w.svc.OfficeFile(ctx, otherScope, payTask.UUID, detail.Uploads[0].UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office download: %v", err)
	}
	// A later detection keeps the warning resolved (document on record).
	if rep, err = w.svc.Detect(ctx, sub, "2026-10"); err != nil || rep.Created != 0 {
		t.Fatalf("detect after match: %+v %v", rep, err)
	}
	if got := w.warningStatus(t, w.payment); len(got) != 1 || got[0] != "auto_resolved" {
		t.Fatalf("payment warning reopened: %v", got)
	}

	// 5. Reminders with backoff: +3 days, then +6, then +12 (max 3).
	if r, err := w.svc.SendReminders(ctx); err != nil || r.Engagements != 0 {
		t.Fatalf("early reminders: %+v %v", r, err)
	}
	*w.clock = w.clock.Add(3*24*time.Hour + time.Minute)
	r, err := w.svc.SendReminders(ctx)
	if err != nil || r.Engagements != 1 || r.Tasks != 1 || r.Notified != 1 {
		t.Fatalf("first reminder: %+v %v", r, err)
	}
	if r, err = w.svc.SendReminders(ctx); err != nil || r.Engagements != 0 {
		t.Fatalf("repeat reminder: %+v %v", r, err)
	}
	colTask, err := w.svc.Get(ctx, w.scope, taskFor(t, office, w.collection, w).UUID)
	if err != nil || *colTask.ReminderCount != 1 || colTask.NextReminderAt == nil ||
		!colTask.NextReminderAt.Equal(w.clock.Add(6*24*time.Hour)) {
		t.Fatalf("after first reminder: %+v %v", colTask, err)
	}
	*w.clock = w.clock.Add(6 * 24 * time.Hour)
	if r, err = w.svc.SendReminders(ctx); err != nil || r.Tasks != 1 {
		t.Fatalf("second reminder: %+v %v", r, err)
	}
	*w.clock = w.clock.Add(12 * 24 * time.Hour)
	if r, err = w.svc.SendReminders(ctx); err != nil || r.Tasks != 1 {
		t.Fatalf("third reminder: %+v %v", r, err)
	}
	colTask, _ = w.svc.Get(ctx, w.scope, colTask.UUID)
	if *colTask.ReminderCount != 3 || colTask.NextReminderAt != nil || !colTask.Overdue {
		t.Fatalf("after max reminders: %+v", colTask)
	}
	if w.notif.count(TemplateReminder) != 3 {
		t.Fatalf("reminder notifications = %d", w.notif.count(TemplateReminder))
	}

	// 6. Reclassification removes the condition: warning and task close.
	if _, err := w.d.Pool.Exec(ctx, `UPDATE bank_transactions SET kind_detail = 'own_account_transfer' WHERE id = $1`, w.collection); err != nil {
		t.Fatal(err)
	}
	if rep, err = w.svc.Detect(ctx, sub, "2026-10"); err != nil || rep.AutoResolved != 1 || rep.TasksClosed != 1 {
		t.Fatalf("detect after reclassify: %+v %v", rep, err)
	}
	colTask, _ = w.svc.Get(ctx, w.scope, colTask.UUID)
	if colTask.Status != StatusCancelled || colTask.CancelReason == nil || *colTask.CancelReason != "resolved" {
		t.Fatalf("collection task: %+v", colTask)
	}
	if _, err := w.svc.Cancel(ctx, w.scope, colTask.UUID, 0, ""); !errors.Is(err, ErrClosed) {
		t.Fatalf("cancel closed task: %v", err)
	}

	// 7. Ended engagement: the mükellef no longer sees the old office's tasks.
	w.d.EndEngagement(t, w.scope.ID)
	if _, n, _ := w.svc.ListMine(ctx, w.taxpayerID, Filter{Limit: 20}); n != 0 {
		t.Fatalf("tasks of an ended engagement visible: %d", n)
	}
}

func uploadHTTP(t *testing.T, h *Handler, w *world, task uuid.UUID, name string, data []byte) UploadResult {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/taxpayer/document-tasks/"+task.String()+"/uploads", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("taskUuid", task.String())
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: w.member})
	ctx = taxpayerctx.WithScope(ctx, taxpayerctx.Scope{ID: w.taxpayerID, Role: taxpayerctx.RoleOwner})
	rec := httptest.NewRecorder()
	h.UploadMine(rec, req.WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data UploadResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}
