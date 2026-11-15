package taxpayers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
)

type onboardEnv struct {
	d    *testdb.DB
	flow *Onboarding
	mail *captureNotifier
}

func newOnboardEnv(t *testing.T, minDuration time.Duration) onboardEnv {
	t.Helper()
	d := testdb.Open(t)
	mail := &captureNotifier{}
	flow := NewOnboarding(d.Pool, mail, nil, OnboardingConfig{FrontendURL: "https://app.test", MinRequestDuration: minDuration}, nil)
	return onboardEnv{d: d, flow: flow, mail: mail}
}

func (e onboardEnv) consentToken(t *testing.T) string {
	t.Helper()
	e.mail.mu.Lock()
	defer e.mail.mu.Unlock()
	if len(e.mail.sent) == 0 {
		t.Fatal("no consent email")
	}
	link := e.mail.sent[len(e.mail.sent)-1].TemplateVars["link"]
	i := strings.LastIndex(link, "/consent/")
	if i < 0 {
		t.Fatalf("link %q", link)
	}
	return link[i+len("/consent/"):]
}

func (e onboardEnv) email(t *testing.T, user int64) string {
	t.Helper()
	var s string
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id = $1`, user).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e onboardEnv) request(t *testing.T, org int64, taxID, email string) string {
	t.Helper()
	if _, err := e.flow.Request(context.Background(), RequestInput{
		OrgID: org, OrgName: "Ofis", Kind: KindLegal, TaxID: taxID, Title: "Alfa A.Ş.", ContactEmail: email,
	}); err != nil {
		t.Fatalf("request: %v", err)
	}
	return e.consentToken(t)
}

// postRequest calls the real handler as office org.
func postRequest(t *testing.T, h *OnboardingHandler, org, user int64, body string) (int, map[string]any, time.Duration) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenant/taxpayer-requests", bytes.NewBufferString(body))
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Name: "Ofis A"})
	rec := httptest.NewRecorder()
	start := time.Now()
	h.createRequest(rec, req.WithContext(ctx))
	took := time.Since(start)
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return rec.Code, env, took
}

// normalize drops the values that are random or per-call (uuid, times,
// request id) so two answers can be compared field by field.
func normalize(env map[string]any) string {
	if d, ok := env["data"].(map[string]any); ok {
		if r, ok := d["request"].(map[string]any); ok {
			for _, k := range []string{"uuid", "created_at", "expires_at"} {
				if _, present := r[k]; present {
					r[k] = "<x>"
				}
			}
		}
	}
	if m, ok := env["meta"].(map[string]any); ok {
		for k := range m {
			m[k] = "<x>"
		}
	}
	b, _ := json.Marshal(env)
	return string(b)
}

// TestAddTaxpayerAnswerDoesNotRevealExistence is the D1b acceptance test of
// issue #7: an office adding a VKN gets the same status code, the same body
// and the same response time whether the VKN is unknown, registered with an
// active engagement at another office, already with this office, or
// registered without an engagement.
func TestAddTaxpayerAnswerDoesNotRevealExistence(t *testing.T) {
	const minDur = 120 * time.Millisecond
	e := newOnboardEnv(t, minDur)
	h := NewOnboardingHandler(e.flow, nil)
	orgA, orgB := e.d.Organization(t), e.d.Organization(t)
	userA, ownerB, tpOwner := e.d.User(t), e.d.User(t), e.d.User(t)
	e.d.Member(t, orgA, userA, "owner")
	e.d.Member(t, orgB, ownerB, "owner")

	// Registered, active at office B, with an owner user.
	atB := e.d.Taxpayer(t, KindLegal, "1234567890")
	e.d.Engagement(t, orgB, atB)
	if _, err := e.d.Pool.Exec(context.Background(), `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, atB, tpOwner); err != nil {
		t.Fatal(err)
	}
	// Registered, already with office A.
	atA := e.d.Taxpayer(t, KindLegal, "0000000001")
	e.d.Engagement(t, orgA, atA)
	// Registered, no engagement.
	e.d.Taxpayer(t, KindLegal, "1111111114")
	// Unknown: 8790033575 (valid check digit, never inserted).

	cases := map[string]string{
		"unknown":          "8790033575",
		"active elsewhere": "1234567890",
		"already ours":     "0000000001",
		"registered idle":  "1111111114",
	}
	const rounds = 12
	bodies := map[string]string{}
	medians := map[string]time.Duration{}
	for name, vkn := range cases {
		var times []time.Duration
		for i := 0; i < rounds; i++ {
			body := `{"kind":"legal","tax_id":"` + vkn + `","title":"Alfa A.Ş.","contact_email":"muhasebe@alfa.test"}`
			code, env, took := postRequest(t, h, orgA, userA, body)
			if code != http.StatusAccepted {
				t.Fatalf("%s: status %d, want 202 (%v)", name, code, env)
			}
			n := strings.ReplaceAll(normalize(env), vkn, "<vkn>")
			if prev, ok := bodies[name]; ok && prev != n {
				t.Fatalf("%s: body changed between calls:\n%s\n%s", name, prev, n)
			}
			bodies[name] = n
			times = append(times, took)
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		medians[name] = times[len(times)/2]
		if times[0] < minDur {
			t.Errorf("%s: fastest answer %v is below the padded minimum %v", name, times[0], minDur)
		}
	}
	var ref string
	for name, b := range bodies {
		if ref == "" {
			ref = b
			continue
		}
		if b != ref {
			t.Errorf("%s: body differs:\n%s\nvs\n%s", name, b, ref)
		}
	}
	var lo, hi time.Duration
	for _, m := range medians {
		if lo == 0 || m < lo {
			lo = m
		}
		if m > hi {
			hi = m
		}
	}
	t.Logf("median response times: %v", medians)
	if hi-lo > 40*time.Millisecond {
		t.Errorf("median response times differ by %v (> 40ms): %v", hi-lo, medians)
	}
	// Nothing the office can see changed for the registered taxpayers.
	var active int
	_ = e.d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM engagements WHERE taxpayer_id = $1 AND ended_at IS NULL AND organization_id = $2`, atB, orgB).Scan(&active)
	if active != 1 {
		t.Fatal("a request alone changed office B's engagement")
	}
	// Invalid input is refused before anything else: a format answer, not
	// an existence answer.
	code, _, _ := postRequest(t, h, orgA, userA, `{"kind":"legal","tax_id":"1234567891","title":"x","contact_email":"a@b.test"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad check digit: %d", code)
	}
}

// TestOfficeSeesNothingBeforeApproval: kabul kriteri 2. Onaydan önce ofisin
// engagement'ı yok; mükellef verisine yol yok. Onayla engagement açılır.
func TestOfficeSeesNothingBeforeApproval(t *testing.T) {
	e := newOnboardEnv(t, time.Millisecond)
	ctx := context.Background()
	org := e.d.Organization(t)
	staff := e.d.User(t)
	e.d.Member(t, org, staff, "owner")
	contact := e.d.User(t)

	token := e.request(t, org, "8790033575", e.email(t, contact))
	list, _, err := e.flow.List(ctx, org, 50, 0)
	if err != nil || len(list) != 1 || list[0].Status != RequestPending || list[0].EngagementUUID != nil {
		t.Fatalf("before approval: %+v %v", list, err)
	}
	var engagements, taxpayers int
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE organization_id = $1`, org).Scan(&engagements)
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM taxpayers WHERE tax_id = '8790033575'`).Scan(&taxpayers)
	if engagements != 0 || taxpayers != 0 {
		t.Fatalf("before approval: engagements=%d taxpayers=%d, want 0/0", engagements, taxpayers)
	}

	if _, err := e.flow.ApproveByLink(ctx, token, staff); !errors.Is(err, ErrInvitationEmailMismatch) {
		t.Fatalf("approve by another account: %v", err)
	}
	out, err := e.flow.ApproveByLink(ctx, token, contact)
	if err != nil || out.Status != OutcomeApproved || out.TaxpayerUUID == nil {
		t.Fatalf("approve: %+v %v", out, err)
	}
	list, _, _ = e.flow.List(ctx, org, 50, 0)
	if list[0].Status != RequestApproved || list[0].EngagementUUID == nil {
		t.Fatalf("after approval: %+v", list[0])
	}
	// The contact is now the company's owner user.
	m := NewMembers(NewMemberPostgres(e.d.Pool), nil, InvitationConfig{}, nil)
	if s, err := m.ResolveTaxpayer(ctx, *out.TaxpayerUUID, contact); err != nil || s.Role != taxpayerctx.RoleOwner {
		t.Fatalf("contact membership: %+v %v", s, err)
	}
	if _, err := e.flow.ApproveByLink(ctx, token, contact); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("second approval: %v", err)
	}
}

// TestExistingCompanyNeedsItsOwner: a registered company with owner users is
// not taken over through a link sent to an address the office chose; the
// owner approves from the mükellef panel, which also moves the company from
// its previous office (D2).
func TestExistingCompanyNeedsItsOwner(t *testing.T) {
	e := newOnboardEnv(t, time.Millisecond)
	ctx := context.Background()
	orgA, orgB := e.d.Organization(t), e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	engB := e.d.Engagement(t, orgB, tp)
	owner := e.d.User(t)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, tp, owner); err != nil {
		t.Fatal(err)
	}
	stranger := e.d.User(t)
	token := e.request(t, orgA, "1234567890", e.email(t, stranger))

	out, err := e.flow.ApproveByLink(ctx, token, stranger)
	if err != nil || out.Status != OutcomeForwarded || out.TaxpayerUUID != nil {
		t.Fatalf("stranger approval: %+v %v", out, err)
	}
	var stillB int
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE id = $1 AND ended_at IS NULL`, engB).Scan(&stillB)
	if stillB != 1 {
		t.Fatal("stranger's approval ended office B's engagement")
	}

	var tpUUID uuid.UUID
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM taxpayers WHERE id = $1`, tp).Scan(&tpUUID)
	ownerScope := taxpayerctx.Scope{ID: tp, UUID: tpUUID, TaxID: "1234567890", Role: taxpayerctx.RoleOwner}
	pending, err := e.flow.PendingForTaxpayer(ctx, ownerScope)
	if err != nil || len(pending) != 1 {
		t.Fatalf("owner pending: %+v %v", pending, err)
	}
	employee := ownerScope
	employee.Role = taxpayerctx.RoleEmployee
	if _, err := e.flow.ApproveAsOwner(ctx, employee, pending[0].UUID, owner); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("employee approval: %v", err)
	}
	out, err = e.flow.ApproveAsOwner(ctx, ownerScope, pending[0].UUID, owner)
	if err != nil || out.Status != OutcomeApproved {
		t.Fatalf("owner approval: %+v %v", out, err)
	}
	var endedB, activeA int
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE id = $1 AND ended_at IS NOT NULL`, engB).Scan(&endedB)
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE organization_id = $1 AND taxpayer_id = $2 AND ended_at IS NULL`, orgA, tp).Scan(&activeA)
	if endedB != 1 || activeA != 1 {
		t.Fatalf("office change: endedB=%d activeA=%d", endedB, activeA)
	}
}

func TestRejectCancelAndSignupGate(t *testing.T) {
	e := newOnboardEnv(t, time.Millisecond)
	ctx := context.Background()
	org := e.d.Organization(t)
	contact := e.d.User(t)
	email := e.email(t, contact)

	token := e.request(t, org, "8790033575", email)
	if err := e.flow.CheckSignupInvitation(ctx, token, strings.ToUpper(email)); err != nil {
		t.Fatalf("signup gate: %v", err)
	}
	if err := ChainGates(nil, e.flow).CheckSignupInvitation(ctx, token, email); err != nil {
		t.Fatalf("signup gate via chain: %v", err)
	}
	if err := e.flow.RejectByLink(ctx, token, contact); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if p, _ := e.flow.Preview(ctx, token); p.Status != RequestRejected {
		t.Fatalf("after reject: %s", p.Status)
	}
	if err := e.flow.CheckSignupInvitation(ctx, token, email); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("signup after reject: %v", err)
	}

	token2 := e.request(t, org, "8790033575", email)
	list, _, _ := e.flow.List(ctx, org, 50, 0)
	if err := e.flow.Cancel(ctx, org, list[0].UUID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := e.flow.ApproveByLink(ctx, token2, contact); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("approve cancelled: %v", err)
	}
	other := e.d.Organization(t)
	if err := e.flow.Cancel(ctx, other, list[1].UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel another office's request: %v", err)
	}
}
