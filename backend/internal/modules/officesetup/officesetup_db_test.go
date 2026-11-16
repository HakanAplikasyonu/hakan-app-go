package officesetup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestSetupState (issue #273): a new office starts with every step
// pending; a step that changes status gets a timestamp, one that keeps it
// keeps its timestamp; settling every step completes the wizard; the rest
// of the office settings is untouched; bad input is refused.
func TestSetupState(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	if _, err := d.Pool.Exec(ctx, `UPDATE organizations SET settings = '{"locale":"tr"}' WHERE id = $1`, org); err != nil {
		t.Fatal(err)
	}
	svc := New(d.Pool)
	st, err := svc.Get(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Steps) != len(Steps) || st.Steps["office"].Status != StatusPending || st.CompletedAt != nil {
		t.Fatalf("new state = %+v", st)
	}

	t1 := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	st.Steps["office"] = StepState{Status: StatusDone}
	st.Steps["team"] = StepState{Status: StatusSkipped}
	st.AccountingProgram = "luca"
	st, err = svc.Put(ctx, org, st, t1)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Steps["office"].UpdatedAt; got == nil || !got.Equal(t1) || st.Steps["taxpayers"].UpdatedAt != nil {
		t.Fatalf("after first put = %+v", st.Steps)
	}

	t2 := t1.Add(time.Hour)
	for _, k := range []string{"taxpayers", "accounting", "integrations", "invites"} {
		st.Steps[k] = StepState{Status: StatusDone}
	}
	st, err = svc.Put(ctx, org, st, t2)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Steps["office"].UpdatedAt.Equal(t1) || !st.Steps["invites"].UpdatedAt.Equal(t2) || st.CompletedAt == nil || !st.CompletedAt.Equal(t2) {
		t.Fatalf("after completing = %+v completed=%v", st.Steps, st.CompletedAt)
	}
	// Stored, and the other settings are kept.
	again, err := svc.Get(ctx, org)
	if err != nil || again.AccountingProgram != "luca" || again.CompletedAt == nil {
		t.Fatalf("reload = %+v %v", again, err)
	}
	var locale string
	if err := d.Pool.QueryRow(ctx, `SELECT settings->>'locale' FROM organizations WHERE id = $1`, org).Scan(&locale); err != nil || locale != "tr" {
		t.Fatalf("other settings = %q %v", locale, err)
	}
	// Replacing the settings object (PATCH /v1/tenant/settings) keeps it.
	if _, err := d.Pool.Exec(ctx, `UPDATE organizations SET settings = '{}' WHERE id = $1`, org); err != nil {
		t.Fatal(err)
	}
	if kept, err := svc.Get(ctx, org); err != nil || kept.CompletedAt == nil {
		t.Fatalf("after settings replace = %+v %v", kept, err)
	}
	// Completion is kept when re-saved; reopening a step clears it.
	if st, err = svc.Put(ctx, org, st, t2.Add(time.Hour)); err != nil || !st.CompletedAt.Equal(t2) {
		t.Fatalf("re-save: %v %v", st.CompletedAt, err)
	}
	st.Steps["invites"] = StepState{Status: StatusPending}
	if st, err = svc.Put(ctx, org, st, t2.Add(2*time.Hour)); err != nil || st.CompletedAt != nil || st.Steps["invites"].UpdatedAt != nil {
		t.Fatalf("reopen: %+v %v", st, err)
	}

	for name, bad := range map[string]State{
		"unknown step":   {Steps: map[string]StepState{"payroll": {Status: StatusDone}}},
		"unknown status": {Steps: map[string]StepState{"office": {Status: "half"}}},
		"program":        {AccountingProgram: "logo"},
	} {
		if _, err := svc.Put(ctx, org, bad, t1); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
