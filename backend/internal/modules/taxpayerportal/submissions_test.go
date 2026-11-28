package taxpayerportal

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

func TestReceiptStatus(t *testing.T) {
	for in, want := range map[string]string{
		"draft": SubmissionReceived, "duplicate": SubmissionDuplicate, "failed": SubmissionUnreadable,
		"approved": SubmissionApproved, "rejected": SubmissionRejected,
	} {
		if got := receiptStatus(in); got != want {
			t.Errorf("%s = %s, want %s", in, got, want)
		}
	}
}

func TestTaskUploadStatus(t *testing.T) {
	up := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	ts := func(tm time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: tm, Valid: true} }
	row := func(status string, updated time.Time) db.ListTaxpayerSubmittedTaskUploadsRow {
		return db.ListTaxpayerSubmittedTaskUploadsRow{TaskStatus: status, CreatedAt: ts(up), TaskUpdatedAt: ts(updated),
			Note: "Daha net çekin", CancelReason: "Gerek kalmadı"}
	}
	cases := []struct {
		r              db.ListTaxpayerSubmittedTaskUploadsRow
		status, reason string
	}{
		{row("submitted", up), SubmissionReceived, ""},
		{row("matched", up.Add(time.Hour)), SubmissionApproved, ""},
		{row("cancelled", up.Add(time.Hour)), SubmissionCancelled, "Gerek kalmadı"},
		{row("open", up.Add(time.Hour)), SubmissionRejected, "Daha net çekin"},
		{row("open", up), SubmissionReceived, ""},
	}
	for _, c := range cases {
		s, r := taskUploadStatus(c.r)
		if s != c.status || r != c.reason {
			t.Errorf("%s = %s %q, want %s %q", c.r.TaskStatus, s, r, c.status, c.reason)
		}
	}
	if requestTitle("", "uninvoiced_collection") != "Tahsilat için fatura" || requestTitle("Kira", "document_request") != "Kira" {
		t.Fatal("requestTitle")
	}
}
