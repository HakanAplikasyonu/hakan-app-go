package doctasks

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TestDocumentRequestFlow (issue #240): the office opens a request by hand,
// the mükellef sees it and uploads, the office hears about it, asks again,
// then accepts it. Bank tasks are unaffected (TestAcceptanceFlow).
func TestDocumentRequestFlow(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	creator, assignee := w.d.User(t), w.d.User(t)
	w.d.Member(t, w.scope.OrganizationID, creator, "owner")
	w.d.Member(t, w.scope.OrganizationID, assignee, "member")
	w.d.Assign(t, w.scope.OrganizationID, w.scope.ID, assignee)

	if _, err := w.svc.CreateRequest(ctx, w.scope, creator, RequestInput{Title: "  "}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("empty title: %v", err)
	}
	if _, err := w.svc.CreateRequest(ctx, w.scope, creator, RequestInput{Title: "Kira kontratı", DueDate: "2026-10-01"}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("past due date: %v", err)
	}

	task, err := w.svc.CreateRequest(ctx, w.scope, creator, RequestInput{Title: "Kira kontratı", Note: "2026 yılı", DueDate: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Kind != KindDocumentRequest || task.Code != CodeDocumentRequest || task.Status != StatusOpen || task.Period != "2026-11" ||
		task.Title != "Kira kontratı" || task.RequestCount == nil || *task.RequestCount != 1 {
		t.Fatalf("created: %+v", task)
	}
	if task.DueAt.In(istanbul).Format("2006-01-02 15:04") != "2026-11-20 23:59" {
		t.Fatalf("due at = %s", task.DueAt.In(istanbul))
	}
	// Stand-in bank transaction for clients built for bank tasks (#200).
	if b := task.BankTransaction; b.UUID != task.UUID || b.Amount != "0" || b.Description != "Kira kontratı" || b.Date != "2026-11-20" {
		t.Fatalf("stand-in bank transaction: %+v", b)
	}
	if w.notif.count(TemplateRequested) != 1 {
		t.Fatalf("requested notifications = %d", w.notif.count(TemplateRequested))
	}
	if in := w.notif.got[len(w.notif.got)-1]; *in.UserID != w.member || in.TemplateVars["title"] != "Kira kontratı" {
		t.Fatalf("notification: %+v", in)
	}

	// The mükellef sees it among its tasks.
	mine, err := w.svc.GetMine(ctx, w.taxpayerID, task.UUID)
	if err != nil || mine.Title != "Kira kontratı" || mine.Kind != KindDocumentRequest || mine.OrganizationName == "" {
		t.Fatalf("mükellef view: %+v %v", mine, err)
	}

	// Upload → submitted (no bank match); the office is told, once per person.
	res, err := w.svc.Upload(ctx, w.taxpayerID, task.UUID, w.member, "kontrat.pdf", []byte("%PDF-1.4 kontrat"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched || res.Task.Status != StatusSubmitted || len(res.Task.Uploads) != 1 {
		t.Fatalf("upload: %+v", res)
	}
	var notified []int64
	for _, in := range w.notif.got {
		if in.TemplateCode == TemplateSubmitted {
			notified = append(notified, *in.UserID)
			if in.Payload["task_uuid"] != task.UUID.String() || in.Payload["engagement_uuid"] == "" || in.Payload["kind"] != "doc_task_submitted" {
				t.Fatalf("submitted payload: %+v", in.Payload)
			}
		}
	}
	if !slices.Contains(notified, creator) || !slices.Contains(notified, assignee) || len(notified) != 2 {
		t.Fatalf("office recipients = %v, want creator %d and assignee %d", notified, creator, assignee)
	}

	// Ask again: back to open, counted, mükellef notified again.
	if _, err := w.svc.Rerequest(ctx, w.scope, task.UUID, RerequestInput{}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("rerequest without note: %v", err)
	}
	again, err := w.svc.Rerequest(ctx, w.scope, task.UUID, RerequestInput{Note: "İmzalı sayfa eksik", DueDays: 3})
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != StatusOpen || *again.RequestCount != 2 || again.Note != "İmzalı sayfa eksik" || w.notif.count(TemplateRequested) != 2 {
		t.Fatalf("rerequest: %+v requested=%d", again, w.notif.count(TemplateRequested))
	}
	if _, err := w.svc.Rerequest(ctx, w.scope, task.UUID, RerequestInput{Note: "x"}); !errors.Is(err, ErrNotSubmitted) {
		t.Fatalf("rerequest an open task: %v", err)
	}

	// Second upload, then the office accepts it.
	if _, err := w.svc.Upload(ctx, w.taxpayerID, task.UUID, w.member, "imza.jpg", []byte("\xff\xd8\xff\xe0 jpeg")); err != nil {
		t.Fatal(err)
	}
	done, err := w.svc.Complete(ctx, w.scope, task.UUID)
	if err != nil || done.Status != StatusMatched || done.MatchedAt == nil || len(done.Uploads) != 2 {
		t.Fatalf("complete: %+v %v", done, err)
	}
	if _, err := w.svc.Complete(ctx, w.scope, task.UUID); !errors.Is(err, ErrClosed) {
		t.Fatalf("complete twice: %v", err)
	}
	if mine, _ := w.svc.GetMine(ctx, w.taxpayerID, task.UUID); mine.Status != StatusMatched {
		t.Fatalf("mükellef sees %s", mine.Status)
	}

	// Another office cannot reach it.
	otherOrg := w.d.Organization(t)
	other := w.d.Taxpayer(t, "legal", "9000000051")
	otherScope := w.scope
	otherScope.OrganizationID, otherScope.ID, otherScope.TaxpayerID = otherOrg, w.d.Engagement(t, otherOrg, other), other
	if _, err := w.svc.Complete(ctx, otherScope, task.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office complete: %v", err)
	}
}
