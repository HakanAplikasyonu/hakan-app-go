package lucakoza

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// The adapter is read-only: besides signing in and choosing the
// company/branch it may only call Listele* / Getir* endpoints. This test
// reads every Koza endpoint written in the package's source (any "*.do"
// string) and fails on a write endpoint (Ekle*, Sil*, Guncelle*, Kaydet* …).

var doRe = regexp.MustCompile(`([A-Za-z0-9_]+)\.do\b`)

// allowedEndpoint: sign-in, the company list and the company/branch switch,
// and reads.
func allowedEndpoint(name string) bool {
	switch name {
	case "Giris", "YdlUserResponsibilityOrgSs", "GuncelleYtkSirketSubeDegistir":
		return true
	}
	return strings.HasPrefix(name, "Listele") || strings.HasPrefix(name, "Getir")
}

func TestNoWriteEndpoints(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, m := range doRe.FindAllStringSubmatch(s, -1) {
				found++
				if !allowedEndpoint(m[1]) {
					t.Errorf("%s: %s.do is not a read endpoint; the Luca Koza adapter must stay read-only", fset.Position(lit.Pos()), m[1])
				}
			}
			return true
		})
	}
	if found < len(endpoints) {
		t.Fatalf("found %d endpoints in the source, want at least %d", found, len(endpoints))
	}
	for _, ep := range endpoints {
		m := doRe.FindStringSubmatch(ep)
		if m == nil || !allowedEndpoint(m[1]) {
			t.Errorf("endpoint %q is not allowed", ep)
		}
	}
	// The rule itself: write endpoints are refused.
	for _, bad := range []string{"EkleFtrSsFaturaBaslik", "SilFtrSsFaturaBaslik", "GuncelleFtrSsFaturaBaslik", "KaydetFtrSsFaturaBaslik"} {
		if allowedEndpoint(bad) {
			t.Errorf("%s allowed", bad)
		}
	}
}

// A full read calls only allowed endpoints on the fake server.
func TestReadCallsOnlyAllowedEndpoints(t *testing.T) {
	ctx := context.Background()
	f := testfakes.LucaKozaSample()
	c := fakeClient(t, f)
	login(t, c)
	if _, err := c.Companies(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SelectCompany(ctx, testfakes.LucaKozaCompanyID); err != nil {
		t.Fatal(err)
	}
	from, to := oct()
	if _, err := c.Read(ctx, from, to); err != nil {
		t.Fatal(err)
	}
	for _, ep := range f.Calls {
		if !allowedEndpoint(strings.TrimSuffix(ep, ".do")) {
			t.Errorf("called %s", ep)
		}
	}
	if len(f.Calls) != 7 { // login, companies, select, 4 lists
		t.Errorf("calls = %v", f.Calls)
	}
}
