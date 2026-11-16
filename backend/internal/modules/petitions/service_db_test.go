package petitions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// reqCtx is what the middleware chain puts on an office request.
func reqCtx(org, user, eng, tp int64) (context.Context, engagementctx.Scope) {
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsWrite, rbac.PermTenantEngagementsReadAll}})
	e := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	return engagementctx.WithScope(ctx, e), e
}

func TestPetitionsStoredPerEngagementUnderRLS(t *testing.T) {
	d := testdb.Open(t)
	svc := New(d.Pool)
	orgA, orgB := d.Organization(t), d.Organization(t)
	userA, userB := d.User(t), d.User(t)
	d.Member(t, orgA, userA, "owner")
	d.Member(t, orgB, userB, "owner")
	if _, err := d.Pool.Exec(context.Background(), `UPDATE users SET smmm_registry_no = '34-777' WHERE id = $1`, userA); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(context.Background(), `UPDATE organizations SET name = 'Örnek SMMM' WHERE id = $1`, orgA); err != nil {
		t.Fatal(err)
	}
	tpA := d.Taxpayer(t, "legal", "1234567890")
	if _, err := d.Pool.Exec(context.Background(), `UPDATE taxpayers SET title = 'Alfa Gıda A.Ş.', tax_office = 'KADIKÖY' WHERE id = $1`, tpA); err != nil {
		t.Fatal(err)
	}
	tpB := d.Taxpayer(t, "individual", "10000000146")
	engA := d.Engagement(t, orgA, tpA)
	engB := d.Engagement(t, orgB, tpB)

	ctxA, scopeA := reqCtx(orgA, userA, engA, tpA)
	prev, err := svc.Preview(ctxA, scopeA, userA, Input{TemplateKey: "vd_borc_durum_yazisi", Fields: map[string]string{"amac": "bankaya"}})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(prev.Body, "1234567890 VKN numaralı mükellefi Alfa Gıda A.Ş.") ||
		prev.Recipient != "KADIKÖY VERGİ DAİRESİ MÜDÜRLÜĞÜNE" || !strings.Contains(prev.Signer, "34-777") ||
		!strings.Contains(prev.Signer, "Örnek SMMM") || !prev.Draft {
		t.Fatalf("preview fields: %+v", prev)
	}
	var n int
	_ = d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM petitions`).Scan(&n)
	if n != 0 {
		t.Fatal("preview stored a petition")
	}

	p, err := svc.Create(ctxA, scopeA, userA, Input{TemplateKey: "vd_borc_durum_yazisi", Fields: map[string]string{"amac": "bankaya"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.Get(ctxA, scopeA, p.UUID)
	if err != nil || got.Rendered.Body != prev.Body || got.TemplateVersion != 1 || !got.Draft {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, err := svc.List(ctxA, scopeA, 50, 0)
	if err != nil || len(list) != 1 || list[0].CreatedBy == nil {
		t.Fatalf("list: %+v %v", list, err)
	}

	// Office B cannot read office A's petition, even naming its uuid and
	// office A's engagement id in the parameters: RLS follows the request.
	ctxB, _ := reqCtx(orgB, userB, engB, tpB)
	if _, err := svc.Get(ctxB, scopeA, p.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("office B reading office A's petition: %v", err)
	}
	_, scopeB := reqCtx(orgB, userB, engB, tpB)
	if list, _ := svc.List(ctxB, scopeB, 50, 0); len(list) != 0 {
		t.Fatalf("office B list: %+v", list)
	}
	// Individual taxpayer: TCKN wording.
	pb, err := svc.Preview(ctxB, scopeB, userB, Input{TemplateKey: "vd_mukellefiyet_yazisi", Fields: map[string]string{"adet": "2"}})
	if err != nil || !strings.Contains(pb.Body, "10000000146 TCKN") {
		t.Fatalf("individual preview: %+v %v", pb, err)
	}
	if _, err := svc.Create(ctxA, scopeA, userA, Input{TemplateKey: "yok"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown template: %v", err)
	}
}
