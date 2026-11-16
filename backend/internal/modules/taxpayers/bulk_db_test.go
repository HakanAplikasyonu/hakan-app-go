package taxpayers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestImportUploadAndSend (issue #273): a list is stored with its row
// report; sending opens one add-mükellef request (consent email) per valid
// row, once, inside the office's request scope; another office cannot see
// the batch.
func TestImportUploadAndSend(t *testing.T) {
	e := newOnboardEnv(t, 5*time.Millisecond)
	im := NewImporter(e.d.Pool, e.flow)
	orgA, orgB := e.d.Organization(t), e.d.Organization(t)
	user := e.d.User(t)
	e.d.Member(t, orgA, user, "owner")

	var b strings.Builder
	b.WriteString("Unvan;VKN;E-posta\n")
	valid := []string{"1234567890", "1111111114", "0000000001", "8790033575", "10000000146"}
	for i, id := range valid {
		fmt.Fprintf(&b, "Firma %d;%s;f%d@example.test\n", i, id, i)
	}
	b.WriteString("Bozuk;1234567891;x@example.test\n")
	ctx := context.Background()
	batch, err := im.Upload(ctx, orgA, user, "liste.csv", []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if batch.Total != 6 || batch.Valid != 5 || batch.Invalid != 1 || batch.SentAt != nil || len(batch.Rows) != 6 {
		t.Fatalf("batch = %+v", batch)
	}

	// Send in the office's request scope, as the API does.
	reqCtx := authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: user})
	reqCtx = orgctx.WithScope(reqCtx, orgctx.Scope{ID: orgA, Name: "Ofis A", Permissions: []string{rbac.PermTenantEngagementsReadAll}})
	var sent ImportBatch
	err = rls.WithScope(reqCtx, e.d.Pool, rls.FromRequest(reqCtx), func(ctx context.Context, _ pgx.Tx) error {
		var err error
		sent, err = im.Send(ctx, orgA, "Ofis A", user, batch.UUID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent.SentAt == nil || len(sent.Outcome) != 6 {
		t.Fatalf("sent = %+v", sent)
	}
	counts := map[string]int{}
	for _, o := range sent.Outcome {
		counts[o.Status]++
		if o.Status == "requested" && o.RequestUUID == nil {
			t.Errorf("row %d requested without a request uuid", o.Row)
		}
	}
	if counts["requested"] != 5 || counts["skipped"] != 1 {
		t.Fatalf("outcome counts %v", counts)
	}
	reqs, total, err := e.flow.List(ctx, orgA, 50, 0)
	if err != nil || total != 5 || len(reqs) != 5 {
		t.Fatalf("office requests %d %d %v", total, len(reqs), err)
	}
	e.mail.mu.Lock()
	emails := len(e.mail.sent)
	e.mail.mu.Unlock()
	if emails != 5 {
		t.Fatalf("consent emails = %d, want 5", emails)
	}

	// Once only.
	if _, err := im.Send(ctx, orgA, "Ofis A", user, batch.UUID); !errors.Is(err, ErrImportSent) {
		t.Fatalf("second send: %v", err)
	}
	// Another office does not see it.
	if _, err := im.Get(ctx, orgB, batch.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office get: %v", err)
	}
	list, err := im.List(ctx, orgA)
	if err != nil || len(list) != 1 || list[0].SentAt == nil {
		t.Fatalf("list = %+v %v", list, err)
	}
}
