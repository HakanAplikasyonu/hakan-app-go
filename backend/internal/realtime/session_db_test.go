package realtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/identity"
	authmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	authusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	orgmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/password"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/realtime"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const testPassword = "Sifre-12345!"

// mailbox records enqueued emails so the test can read invitation links.
type mailbox struct {
	mu   sync.Mutex
	sent []notifmodel.EnqueueInput
}

func (m *mailbox) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, in)
	return nil, nil
}

func (m *mailbox) lastToken(t *testing.T, marker string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no invitation email enqueued")
	}
	link := m.sent[len(m.sent)-1].TemplateVars["link"]
	i := strings.LastIndex(link, marker)
	if i < 0 {
		t.Fatalf("unexpected invitation link %q", link)
	}
	return link[i+len(marker):]
}

// TestTenancyUsersGetConnectionToken (#141): ofis sahibi, mükellef daveti ve
// ofis davetiyle kayıt olan kullanıcılar global rol taşımaz; yine de oturum
// uçlarına (burada realtime connection-token) 200 ile erişmeli. Gerçek akış:
// kimlik doğrulama, identity loader, auth.session kontrolü.
func TestTenancyUsersGetConnectionToken(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	queries := database.NewQueries(d.Pool)
	repo := authrepo.NewPostgres(d.Pool, queries)

	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := authusecase.New(repo, tokens)

	mail := &mailbox{}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(d.Pool, queries), nil)
	orgSvc.SetInvitations(orgrepository.NewPostgres(d.Pool, queries), mail, orgusecase.InvitationConfig{
		TTL: time.Hour, FrontendURL: "https://app.test",
	}, nil)
	members := taxpayers.NewMembers(taxpayers.NewMemberPostgres(d.Pool), mail,
		taxpayers.InvitationConfig{TTL: time.Hour, FrontendURL: "https://app.test"}, nil)
	uc.SetInvitationGate(taxpayers.ChainGates(orgSvc, members))

	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{Enabled: true, TokenHMAC: "test-hmac-secret", WSURL: "ws://rt.test"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	realtime.RegisterRoutes(mux, realtime.NewHandler(issuer, nil), tokens, identity.Loader{UC: uc})

	connect := func(t *testing.T, access string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/realtime/connection-token", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	login := func(t *testing.T, email string) string {
		t.Helper()
		tk, err := uc.Login(ctx, email, testPassword, "", authmodel.SessionMeta{})
		if err != nil {
			t.Fatalf("login %s: %v", email, err)
		}
		return tk.AccessToken
	}
	hash, err := password.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}

	// Ofis sahibi: demo-seed gibi kullanıcı + ofis oluşturma; global rol yok.
	owner, err := repo.CreateUser(ctx, authmodel.User{
		Email: "sahip@ofis141.test", PasswordHash: hash, Name: "Sahip", Surname: "Ofis", Status: "active",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: "Ofis 141", Slug: "ofis-141", OwnerUserUUID: owner.UUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerAccess := login(t, owner.Email)

	// Mükellef kullanıcısı: ilişkiden gelen davetle kayıt + kabul.
	tp := d.Taxpayer(t, taxpayers.KindLegal, "1410000001")
	eng := d.Engagement(t, org.ID, tp)
	if _, err := members.Invite(ctx, taxpayers.InviteInput{
		OrgID: org.ID, OrgName: org.Name, EngagementID: eng, TaxpayerID: tp,
		Email: "mukellef@ofis141.test", InviterUserID: owner.ID,
	}); err != nil {
		t.Fatalf("taxpayer invite: %v", err)
	}
	tpToken := mail.lastToken(t, "/invite/m/")
	tpUser, err := uc.Register(ctx, authmodel.RegisterInput{
		Email: "mukellef@ofis141.test", Password: testPassword, Name: "Mükellef", Surname: "Kullanıcı", InvitationToken: tpToken,
	})
	if err != nil {
		t.Fatalf("taxpayer register: %v", err)
	}
	if _, err := members.Accept(ctx, tpToken, tpUser.ID); err != nil {
		t.Fatalf("taxpayer accept: %v", err)
	}

	// Ofis davetiyle kayıt olan personel.
	if _, err := orgSvc.CreateInvitation(ctx, orgmodel.CreateInvitationInput{
		OrgID: org.ID, OrgName: org.Name, InviterUserID: owner.ID, InviterName: "Sahip",
		InviterRoleKey: orgctx.RoleOwner, Email: "personel@ofis141.test", RoleKey: orgctx.RoleMember,
	}); err != nil {
		t.Fatalf("office invite: %v", err)
	}
	staffToken := mail.lastToken(t, "/invite/")
	staff, err := uc.Register(ctx, authmodel.RegisterInput{
		Email: "personel@ofis141.test", Password: testPassword, Name: "Personel", Surname: "Ofis", InvitationToken: staffToken,
	})
	if err != nil {
		t.Fatalf("office register: %v", err)
	}
	if _, err := orgSvc.AcceptInvitation(ctx, staffToken, staff.ID); err != nil {
		t.Fatalf("office accept: %v", err)
	}

	staffAccess := login(t, staff.Email)
	for _, c := range []struct{ name, access string }{
		{"ofis sahibi", ownerAccess},
		{"mükellef kullanıcısı", login(t, tpUser.Email)},
		{"davetle kayıt olan personel", staffAccess},
	} {
		if code := connect(t, c.access); code != http.StatusOK {
			t.Errorf("%s: connection-token = %d, want 200", c.name, code)
		}
	}

	// /auth/me izin listesi de aynı taban izni gösterir (frontend buna bakar).
	me, err := uc.Me(ctx, tpUser.UUID, nil)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if !contains(me.Permissions, "auth.session") {
		t.Errorf("me.permissions = %v, auth.session yok", me.Permissions)
	}

	// Devre dışı kullanıcı taban izni almaz: elindeki token reddedilir.
	if _, err := d.Pool.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, staff.ID); err != nil {
		t.Fatal(err)
	}
	if code := connect(t, staffAccess); code == http.StatusOK {
		t.Errorf("disabled user got connection token")
	}
	if code := connect(t, ownerAccess); code != http.StatusOK {
		t.Errorf("owner after staff disabled: %d", code)
	}

	// Anonim istek hâlâ 401.
	if code := connect(t, "gecersiz"); code != http.StatusUnauthorized {
		t.Errorf("invalid token: %d, want 401", code)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
