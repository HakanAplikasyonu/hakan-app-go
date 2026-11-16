package doctasks

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestUploadInTaxpayerScopeNotifiesOffice: in the API the mükellef's upload
// runs in the taxpayer's row level security scope (issue #166), where office
// tables are not visible. The office must still be told (issue #240).
func TestUploadInTaxpayerScopeNotifiesOffice(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	creator := w.d.User(t)
	w.d.Member(t, w.scope.OrganizationID, creator, "owner")
	task, err := w.svc.CreateRequest(ctx, w.scope, creator, RequestInput{Title: "Kira kontratı"})
	if err != nil {
		t.Fatal(err)
	}
	scope := rls.Scope{UserID: w.member, TaxpayerIDs: []int64{w.taxpayerID}}
	err = rls.WithScope(ctx, w.d.Pool, scope, func(ctx context.Context, _ pgx.Tx) error {
		_, err := w.svc.Upload(ctx, w.taxpayerID, task.UUID, w.member, "k.pdf", []byte("%PDF-1.4 k"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := w.notif.count(TemplateSubmitted); n != 1 {
		t.Fatalf("office notifications in taxpayer scope = %d, want 1", n)
	}
}
