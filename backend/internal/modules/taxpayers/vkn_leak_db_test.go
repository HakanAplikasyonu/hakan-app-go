package taxpayers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

// TestTaxIDExistenceSideChannels (#171) extends the D1b test of #7 to every
// answer an office gets about a VKN/TCKN it adds: the create response
// (status, headers, body, duration), the consent email the platform sends,
// the office's request list and the cancel answer. More stored states than
// #7: an ended engagement elsewhere, another office's pending request, an
// individual (TCKN) and the same id typed with separators.
func TestTaxIDExistenceSideChannels(t *testing.T) {
	const minDur = 120 * time.Millisecond
	e := newOnboardEnv(t, minDur)
	h := NewOnboardingHandler(e.flow, nil)
	ctx := context.Background()
	orgA, orgB, orgC := e.d.Organization(t), e.d.Organization(t), e.d.Organization(t)
	userA, owner := e.d.User(t), e.d.User(t)
	e.d.Member(t, orgA, userA, "owner")

	// Stored states (every id has a valid check digit).
	active := e.d.Taxpayer(t, KindLegal, "1234567890") // active at B, has an owner
	e.d.Engagement(t, orgB, active)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, active, owner); err != nil {
		t.Fatal(err)
	}
	ended := e.d.Taxpayer(t, KindLegal, "1111111114") // only an ended engagement at C
	e.d.EndEngagement(t, e.d.Engagement(t, orgC, ended))
	e.d.Taxpayer(t, KindIndividual, "10000000146") // registered individual, idle
	// Another office already asked for 0000000001 (no taxpayer row yet).
	if _, err := e.flow.Request(ctx, RequestInput{OrgID: orgB, OrgName: "B", Kind: KindLegal, TaxID: "0000000001", Title: "X", ContactEmail: "x@b.test"}); err != nil {
		t.Fatal(err)
	}

	type kase struct{ name, kind, typed, canonical string }
	cases := []kase{
		{"unknown VKN", KindLegal, "8790033575", "8790033575"},
		{"active elsewhere, has owner", KindLegal, "1234567890", "1234567890"},
		{"ended elsewhere", KindLegal, "1111111114", "1111111114"},
		{"pending at another office", KindLegal, "0000000001", "0000000001"},
		{"registered individual", KindIndividual, "10000000146", "10000000146"},
		{"unknown TCKN", KindIndividual, "10000000078", "10000000078"},
		{"active, typed with separators", KindLegal, "123 456-78.90", "1234567890"},
	}

	const rounds = 8
	var refBody, refHeaders, refMail, refList string
	medians := map[string]time.Duration{}
	for _, c := range cases {
		var times []time.Duration
		for i := 0; i < rounds; i++ {
			e.mail.mu.Lock()
			before := len(e.mail.sent)
			e.mail.mu.Unlock()

			body := fmt.Sprintf(`{"kind":%q,"tax_id":%q,"title":"Alfa","contact_email":"muhasebe@alfa.test"}`, c.kind, c.typed)
			rec, took := serveCreate(h, orgA, userA, body)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("%s: status %d (%s)", c.name, rec.Code, rec.Body.String())
			}
			times = append(times, took)

			// Body: identical once per-call values and the id itself are masked.
			var env map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			got := strings.ReplaceAll(normalize(env), c.canonical, "<id>")
			got = strings.ReplaceAll(got, `"kind":"`+c.kind+`"`, `"kind":"<k>"`)
			if refBody == "" {
				refBody = got
			} else if got != refBody {
				t.Errorf("%s: body differs\n got %s\nwant %s", c.name, got, refBody)
			}

			// Headers: same set and values (Content-Length may follow the id length only).
			hdr := headerShape(rec.Header(), len(c.canonical))
			if refHeaders == "" {
				refHeaders = hdr
			} else if hdr != refHeaders {
				t.Errorf("%s: headers differ\n got %s\nwant %s", c.name, hdr, refHeaders)
			}

			// Exactly one consent email, to the address the office typed, same template.
			e.mail.mu.Lock()
			sent := e.mail.sent[before:]
			e.mail.mu.Unlock()
			if len(sent) != 1 {
				t.Fatalf("%s: %d emails, want 1", c.name, len(sent))
			}
			m := sent[0]
			recipient := ""
			if m.Recipient != nil {
				recipient = *m.Recipient
			}
			keys := make([]string, 0, len(m.TemplateVars))
			for k := range m.TemplateVars {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			mail := fmt.Sprintf("%s|%s|%v|uid=%v", m.TemplateCode, recipient, keys, m.UserID != nil)
			if refMail == "" {
				refMail = mail
			} else if mail != refMail {
				t.Errorf("%s: email differs\n got %s\nwant %s", c.name, mail, refMail)
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		medians[c.name] = times[len(times)/2]
		if times[0] < minDur {
			t.Errorf("%s: fastest answer %v below the padded minimum %v", c.name, times[0], minDur)
		}
	}

	var lo, hi time.Duration
	for _, m := range medians {
		if lo == 0 || m < lo {
			lo = m
		}
		hi = max(hi, m)
	}
	t.Logf("median response times: %v", medians)
	if hi-lo > 40*time.Millisecond {
		t.Errorf("median response times differ by %v (> 40ms)", hi-lo)
	}

	// The office's list: every request reads the same (status, no engagement).
	items, _, err := e.flow.List(ctx, orgA, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(cases)*rounds {
		t.Fatalf("list has %d requests, want %d", len(items), len(cases)*rounds)
	}
	for _, it := range items {
		shape := fmt.Sprintf("status=%s engagement=%v", it.Status, it.EngagementUUID)
		if refList == "" {
			refList = shape
		} else if shape != refList {
			t.Errorf("list entry for %s differs: %s vs %s", it.TaxID, shape, refList)
		}
	}
	if refList != "status="+RequestPending+" engagement=<nil>" {
		t.Errorf("list entries = %s, want pending without engagement", refList)
	}

	// Cancel answers 204 for every one of them.
	for _, it := range items[:len(cases)] {
		if err := e.flow.Cancel(ctx, orgA, it.UUID); err != nil {
			t.Errorf("cancel %s: %v", it.TaxID, err)
		}
	}

	// Nothing about the stored taxpayers changed.
	var n int
	if err := e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE organization_id = $1`, orgA).Scan(&n); err != nil || n != 0 {
		t.Fatalf("office A engagements = %d (%v), want 0 before any approval", n, err)
	}
}

func serveCreate(h *OnboardingHandler, org, user int64, body string) (*httptest.ResponseRecorder, time.Duration) {
	req := httptest.NewRequest(http.MethodPost, "/v1/tenant/taxpayer-requests", strings.NewReader(body))
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: user})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Name: "Ofis A"})
	rec := httptest.NewRecorder()
	start := time.Now()
	h.createRequest(rec, req.WithContext(ctx))
	return rec, time.Since(start)
}

// headerShape renders the response headers for comparison. Content-Length is
// reported relative to the length of the id echoed in the body, so a 10- and
// an 11-digit id compare equal; request ids are masked.
func headerShape(h http.Header, idLen int) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := strings.Join(h.Values(k), ",")
		switch strings.ToLower(k) {
		case "x-request-id", "date":
			v = "<x>"
		case "content-length":
			var n int
			_, _ = fmt.Sscan(v, &n)
			v = fmt.Sprint(n - idLen)
		}
		fmt.Fprintf(&b, "%s=%s;", k, v)
	}
	return b.String()
}
