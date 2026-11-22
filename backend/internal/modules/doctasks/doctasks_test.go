package doctasks

import (
	"bufio"
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	apidocs "github.com/HakanAplikasyonu/hakan-app-go/backend/docs"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

func TestFinding(t *testing.T) {
	t.Parallel()
	base := Candidate{ID: 1, Period: "2026-10", Amount: "100.00", Currency: "TRY", TransactionDate: "2026-10-03"}
	cases := []struct {
		name string
		mod  func(c *Candidate)
		want string
	}{
		{"supplier payment out", func(c *Candidate) { c.DebitCredit, c.KindDetail = "B", "supplier_payment" }, CodeUndocumentedPayment},
		{"customer collection in", func(c *Candidate) { c.DebitCredit, c.KindDetail = "A", "customer_collection" }, CodeUninvoicedCollection},
		{"supplier payment in is not a payment", func(c *Candidate) { c.DebitCredit, c.KindDetail = "A", "supplier_payment" }, ""},
		{"bank fee", func(c *Candidate) { c.DebitCredit, c.KindDetail = "B", "bank_fee" }, ""},
		{"documented", func(c *Candidate) { c.DebitCredit, c.KindDetail, c.HasDocument = "B", "supplier_payment", true }, ""},
		{"matched in banking", func(c *Candidate) { c.DebitCredit, c.KindDetail, c.MatchStatus = "B", "supplier_payment", "matched" }, ""},
		{"ignored", func(c *Candidate) { c.DebitCredit, c.KindDetail, c.MatchStatus = "A", "customer_collection", "ignored" }, ""},
		{"suggested still raises", func(c *Candidate) {
			c.DebitCredit, c.KindDetail, c.MatchStatus = "A", "customer_collection", "suggested"
		}, CodeUninvoicedCollection},
		{"cancelled", func(c *Candidate) { c.DebitCredit, c.KindDetail, c.IsCancelled = "B", "supplier_payment", true }, ""},
	}
	for _, tc := range cases {
		c := base
		tc.mod(&c)
		f := Finding(c)
		got := ""
		if f != nil {
			got = f.Code
			if f.Params["amount"] != "100.00" {
				t.Errorf("%s: params %v", tc.name, f.Params)
			}
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSniff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []byte
		kind string
		ok   bool
	}{
		{"pdf", []byte("%PDF-1.7\n1 0 obj"), "pdf", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, "image", true},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), "image", true},
		{"heic", append([]byte{0, 0, 0, 0x18}, []byte("ftypheic\x00\x00\x00\x00")...), "image", true},
		{"html", []byte("<html><script>alert(1)</script>"), "", false},
		{"xml", []byte("<?xml version=\"1.0\"?><Invoice/>"), "", false},
	}
	for _, tc := range cases {
		_, kind, _, ok := sniff(tc.data)
		if ok != tc.ok || kind != tc.kind {
			t.Errorf("%s: kind=%q ok=%v", tc.name, kind, ok)
		}
	}
}

func TestCleanName(t *testing.T) {
	t.Parallel()
	if got := cleanName(`..\..\etc/passwd`, ".pdf"); got != "passwd" {
		t.Fatalf("got %q", got)
	}
	if got := cleanName("", ".jpg"); got != "document.jpg" {
		t.Fatalf("got %q", got)
	}
}

type fakeOrgResolver struct{}

func (fakeOrgResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, nil
}

type fakeEngResolver struct{}

func (fakeEngResolver) ResolveEngagement(context.Context, int64, uuid.UUID, int64) (engagementctx.Access, error) {
	return engagementctx.Access{}, nil
}

type fakeTpResolver struct{}

func (fakeTpResolver) ResolveTaxpayer(context.Context, uuid.UUID, int64) (taxpayerctx.Scope, error) {
	return taxpayerctx.Scope{}, nil
}

// TestRoutesDocumented: httpserver's route-table test mounts modules without
// a tenancy resolver, so it never sees tenant routes. This one mounts the
// module with tenancy on and checks every route against openapi.yaml.
func TestRoutesDocumented(t *testing.T) {
	t.Parallel()
	r := module.NewRouter(module.Deps{Resolver: fakeOrgResolver{}})
	if err := r.Mount(Module{Engagements: fakeEngResolver{}, Taxpayers: fakeTpResolver{}}); err != nil {
		t.Fatal(err)
	}
	raw, err := apidocs.FS.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pathRe := regexp.MustCompile(`^  (/[^:\s]+):\s*$`)
	opRe := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	spec := map[string]bool{}
	var current string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if m := pathRe.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := opRe.FindStringSubmatch(line); m != nil && current != "" {
			spec[m[1]+" "+current] = true
		}
	}
	routes := r.Routes()
	if len(routes) != 15 {
		t.Fatalf("routes = %d, want 15", len(routes))
	}
	for _, rt := range routes {
		if !spec[strings.ToLower(rt.Method)+" "+rt.Path] {
			t.Errorf("%s is not in openapi.yaml", rt.Pattern)
		}
	}
}
