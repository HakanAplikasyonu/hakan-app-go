package taxpayers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

// captureNotifier records enqueued emails so tests can read the link.
type captureNotifier struct {
	mu   sync.Mutex
	sent []notifmodel.EnqueueInput
}

func (c *captureNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, in)
	return nil, nil
}

func (c *captureNotifier) lastToken(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) == 0 {
		t.Fatal("no invitation email enqueued")
	}
	link := c.sent[len(c.sent)-1].TemplateVars["link"]
	i := strings.LastIndex(link, "/invite/m/")
	if i < 0 {
		t.Fatalf("unexpected link %q", link)
	}
	return link[i+len("/invite/m/"):]
}

type memberEnv struct {
	d       *testdb.DB
	members *Members
	mail    *captureNotifier
}

func newMemberEnv(t *testing.T) memberEnv {
	t.Helper()
	d := testdb.Open(t)
	mail := &captureNotifier{}
	m := NewMembers(NewMemberPostgres(d.Pool), mail, InvitationConfig{FrontendURL: "https://app.test"}, nil)
	return memberEnv{d: d, members: m, mail: mail}
}

func (e memberEnv) userEmail(t *testing.T, id int64) string {
	t.Helper()
	var email string
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id = $1`, id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

func (e memberEnv) taxpayerUUID(t *testing.T, id int64) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT uuid FROM taxpayers WHERE id = $1`, id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

func (e memberEnv) invite(t *testing.T, org, eng, tp int64, email, role string) string {
	t.Helper()
	if _, err := e.members.Invite(context.Background(), InviteInput{
		OrgID: org, OrgName: "Ofis", EngagementID: eng, TaxpayerID: tp, Email: email, Role: role,
	}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	return e.mail.lastToken(t)
}

// TestOfficeInvitationCreatesTaxpayerMembership: ofis daveti -> kayıt ->
// mükellef üyeliği -> X-Taxpayer çözümü (issue #4 kabul kriteri 1, backend).
func TestOfficeInvitationCreatesTaxpayerMembership(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	invitee := e.d.User(t)
	email := e.userEmail(t, invitee)

	token := e.invite(t, org, eng, tp, strings.ToUpper(email), "")

	// Kayıt kapısı: davet edilen adres geçer, başka adres geçmez.
	if err := e.members.CheckSignupInvitation(ctx, token, email); err != nil {
		t.Fatalf("signup gate: %v", err)
	}
	if err := e.members.CheckSignupInvitation(ctx, token, "baska@example.test"); !errors.Is(err, ErrInvitationEmailMismatch) {
		t.Fatalf("signup gate other email: %v", err)
	}
	prev, err := e.members.Preview(ctx, token)
	if err != nil || prev.Status != InvitationPending || prev.Taxpayer.Title == "" {
		t.Fatalf("preview = %+v, %v", prev, err)
	}

	// Yanlış hesap kabul edemez.
	other := e.d.User(t)
	if _, err := e.members.Accept(ctx, token, other); !errors.Is(err, ErrInvitationEmailMismatch) {
		t.Fatalf("accept by other: %v", err)
	}
	acc, err := e.members.Accept(ctx, token, invitee)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if acc.TaxpayerUUID != e.taxpayerUUID(t, tp) || acc.Role != taxpayerctx.RoleOwner {
		t.Fatalf("accepted = %+v", acc)
	}
	// Tek kullanımlık.
	if _, err := e.members.Accept(ctx, token, invitee); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("second accept: %v", err)
	}
	scope, err := e.members.ResolveTaxpayer(ctx, acc.TaxpayerUUID, invitee)
	if err != nil || scope.ID != tp || scope.Role != taxpayerctx.RoleOwner {
		t.Fatalf("resolve = %+v, %v", scope, err)
	}
	if _, err := e.members.ResolveTaxpayer(ctx, acc.TaxpayerUUID, other); !errors.Is(err, taxpayerctx.ErrNotMember) {
		t.Fatalf("resolve other: %v", err)
	}
	if _, err := e.members.ResolveTaxpayer(ctx, uuid.New(), invitee); !errors.Is(err, taxpayerctx.ErrNotMember) {
		t.Fatalf("resolve unknown: %v", err)
	}
}

// TestUserWithTwoCompaniesResolvesEachIndependently: iki şirketi (farklı
// ofislerde) olan kullanıcı ikisini de /me'de görür, her X-Taxpayer kendi
// şirketine çözülür (kabul kriteri 2, backend).
func TestUserWithTwoCompaniesResolvesEachIndependently(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	orgA, orgB := e.d.Organization(t), e.d.Organization(t)
	tpA := e.d.Taxpayer(t, KindLegal, "1234567890")
	tpB := e.d.Taxpayer(t, KindIndividual, "10000000146")
	engA := e.d.Engagement(t, orgA, tpA)
	engB := e.d.Engagement(t, orgB, tpB)
	u := e.d.User(t)
	email := e.userEmail(t, u)

	if _, err := e.members.Accept(ctx, e.invite(t, orgA, engA, tpA, email, "owner"), u); err != nil {
		t.Fatal(err)
	}
	if _, err := e.members.Accept(ctx, e.invite(t, orgB, engB, tpB, email, "employee"), u); err != nil {
		t.Fatal(err)
	}
	list, err := e.members.ListForUser(ctx, u)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	a, err := e.members.ResolveTaxpayer(ctx, e.taxpayerUUID(t, tpA), u)
	if err != nil || a.ID != tpA || a.Role != "owner" {
		t.Fatalf("A = %+v %v", a, err)
	}
	b, err := e.members.ResolveTaxpayer(ctx, e.taxpayerUUID(t, tpB), u)
	if err != nil || b.ID != tpB || b.Role != "employee" || b.TaxID != "10000000146" {
		t.Fatalf("B = %+v %v", b, err)
	}
}

// TestTaxpayerUserHasNoOfficeAccess: mükellef kullanıcısı ofisin /t/* kapsamına
// giremez (kabul kriteri 3). Ofis çözümleyicisi ErrNotMember döner; bu
// RequireOrganization'da 403 NO_TENANT_MEMBERSHIP olur.
func TestTaxpayerUserHasNoOfficeAccess(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	u := e.d.User(t)
	if _, err := e.members.Accept(ctx, e.invite(t, org, eng, tp, e.userEmail(t, u), ""), u); err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(e.d.Pool, db.New(e.d.Pool)), nil)
	var slug string
	if err := e.d.Pool.QueryRow(ctx, `SELECT slug FROM organizations WHERE id = $1`, org).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.Resolve(ctx, slug, u); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("office resolve for taxpayer user: %v, want ErrNotMember", err)
	}
	// Ofis üyesi olsa bile mükellef kapsamı ayrı: ofis sahibi mükellef üyesi değildir.
	owner := e.d.User(t)
	e.d.Member(t, org, owner, "owner")
	if _, err := e.members.ResolveTaxpayer(ctx, e.taxpayerUUID(t, tp), owner); !errors.Is(err, taxpayerctx.ErrNotMember) {
		t.Fatalf("office owner as taxpayer: %v", err)
	}
}

func TestInvitationOfEndedEngagementIsUnavailable(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	u := e.d.User(t)
	token := e.invite(t, org, eng, tp, e.userEmail(t, u), "")
	e.d.EndEngagement(t, eng)
	if p, err := e.members.Preview(ctx, token); err != nil || p.Status != InvitationRevoked {
		t.Fatalf("preview after end = %+v, %v", p, err)
	}
	if _, err := e.members.Accept(ctx, token, u); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("accept after end: %v", err)
	}
	if err := e.members.CheckSignupInvitation(ctx, token, e.userEmail(t, u)); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("signup after end: %v", err)
	}
}

func TestReinviteReplacesOpenInvitationAndRevoke(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	first := e.invite(t, org, eng, tp, "a@example.test", "")
	second := e.invite(t, org, eng, tp, "a@example.test", "employee")
	if p, _ := e.members.Preview(ctx, first); p.Status != InvitationRevoked {
		t.Fatalf("first after re-invite = %s", p.Status)
	}
	list, err := e.members.ListInvitations(ctx, org, eng)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v %v", list, err)
	}
	if err := e.members.RevokeInvitation(ctx, org, eng, list[0].UUID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if p, _ := e.members.Preview(ctx, second); p.Status != InvitationRevoked {
		t.Fatalf("second after revoke = %s", p.Status)
	}
	// Başka ofis bu engagement'ın davetini iptal edemez.
	other := e.d.Organization(t)
	if err := e.members.RevokeInvitation(ctx, other, eng, list[1].UUID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("revoke from other org: %v", err)
	}
}

func TestChainGates(t *testing.T) {
	e := newMemberEnv(t)
	ctx := context.Background()
	org := e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	eng := e.d.Engagement(t, org, tp)
	token := e.invite(t, org, eng, tp, "a@example.test", "")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(e.d.Pool, db.New(e.d.Pool)), nil)
	orgSvc.SetInvitations(orgrepository.NewPostgres(e.d.Pool, db.New(e.d.Pool)), nil, orgusecase.InvitationConfig{}, nil)
	gate := ChainGates(orgSvc, e.members)
	if err := gate.CheckSignupInvitation(ctx, token, "a@example.test"); err != nil {
		t.Fatalf("taxpayer token through chain: %v", err)
	}
	if err := gate.CheckSignupInvitation(ctx, "bilinmeyen-token", "a@example.test"); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
}
