package httpserver

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	apidocs "github.com/HakanAplikasyonu/hakan-app-go/backend/docs"
)

// taxIDInputAllowed lists the only non-platform operations that may take a
// VKN/TCKN as input. Each one must answer the same way whether or not the id
// is on the platform (D1b) and has a test proving it:
//
//	POST /v1/tenant/taxpayer-requests  internal/modules/taxpayers
//	  TestAddTaxpayerAnswerDoesNotRevealExistence (#7)
//	  TestTaxIDExistenceSideChannels (#171)
//	PUT /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/counterparty-tax-id
//	  internal/modules/invoices/office
//	  TestCounterpartyTaxIDAnswerDoesNotRevealExistence
//
// Platform operations (/v1/platform/*) are the platform admin's catalog and
// are outside D1b. Add to this list only together with such a test.
var taxIDInputAllowed = map[string]bool{
	"post /v1/tenant/taxpayer-requests": true,
	"put /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/counterparty-tax-id": true,
}

// forbiddenRoutes were removed because they revealed whether a tax id is
// registered (#63's 409 on engagement create, removed in #96). An office
// gets an engagement only through the taxpayer's consent.
var forbiddenRoutes = []string{
	"POST /v1/tenant/engagements",
}

var taxIDFields = map[string]bool{"tax_id": true, "vkn": true, "tckn": true, "tax_number": true, "identity_number": true}

// TestNoTaxIDExistenceEndpoint (#171): no office- or taxpayer-facing
// operation takes a tax id except the allowed ones, and those never declare
// a 409 (an "already exists" answer is an existence answer).
func TestNoTaxIDExistenceEndpoint(t *testing.T) {
	t.Parallel()
	raw, err := apidocs.FS.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	schemas, _ := dig(spec, "components", "schemas").(map[string]any)
	params, _ := dig(spec, "components", "parameters").(map[string]any)
	paths, _ := spec["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("no paths in openapi.yaml")
	}

	var found []string
	for path, item := range paths {
		if strings.HasPrefix(path, "/v1/platform/") {
			continue
		}
		ops, _ := item.(map[string]any)
		for method, op := range ops {
			opm, ok := op.(map[string]any)
			if !ok || !isHTTPMethod(method) {
				continue
			}
			key := method + " " + path
			takes := false
			for _, p := range append(asList(ops["parameters"]), asList(opm["parameters"])...) {
				pm := resolve(p, params)
				if name, _ := pm["name"].(string); taxIDFields[strings.ToLower(name)] {
					takes = true
				}
			}
			if content, ok := dig(opm, "requestBody", "content").(map[string]any); ok {
				for _, media := range content {
					if s, ok := dig(media, "schema").(map[string]any); ok && hasTaxIDField(s, schemas, map[string]bool{}) {
						takes = true
					}
				}
			}
			if !takes {
				continue
			}
			found = append(found, key)
			if !taxIDInputAllowed[key] {
				t.Errorf("%s takes a tax id: answer it the same way for known and unknown ids (D1b), test that, then add it to taxIDInputAllowed", key)
			}
			if responses, ok := opm["responses"].(map[string]any); ok {
				if _, has := responses["409"]; has {
					t.Errorf("%s declares 409: a conflict answer reveals that the tax id exists (D1b)", key)
				}
			}
		}
	}
	sort.Strings(found)
	t.Logf("operations taking a tax id: %v", found)
	for k := range taxIDInputAllowed {
		if !contains(found, k) {
			t.Errorf("%s is allowed but no longer takes a tax id; drop it from the list", k)
		}
	}
}

// TestForbiddenTaxIDRoutesStayRemoved scans the route registrations in the
// Go sources, which also covers routes missing from openapi.yaml.
func TestForbiddenTaxIDRoutesStayRemoved(t *testing.T) {
	t.Parallel()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..")
	routeRe := regexp.MustCompile(`"((?:GET|POST|PUT|PATCH|DELETE) /v1/[^"]*)"`)
	seen := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range routeRe.FindAllStringSubmatch(string(b), -1) {
			seen++
			for _, f := range forbiddenRoutes {
				if m[1] == f {
					t.Errorf("%s: route %q is back; it revealed whether a tax id is registered (D1b, #63/#96)", p, f)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 50 {
		t.Fatalf("only %d route registrations found; the scan is broken", seen)
	}
}

func hasTaxIDField(s map[string]any, schemas map[string]any, seen map[string]bool) bool {
	if ref, ok := s["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if seen[name] {
			return false
		}
		seen[name] = true
		target, _ := schemas[name].(map[string]any)
		return target != nil && hasTaxIDField(target, schemas, seen)
	}
	if props, ok := s["properties"].(map[string]any); ok {
		for name, sub := range props {
			if taxIDFields[strings.ToLower(name)] {
				return true
			}
			if sm, ok := sub.(map[string]any); ok && hasTaxIDField(sm, schemas, seen) {
				return true
			}
		}
	}
	for _, k := range []string{"allOf", "oneOf", "anyOf"} {
		for _, sub := range asList(s[k]) {
			if sm, ok := sub.(map[string]any); ok && hasTaxIDField(sm, schemas, seen) {
				return true
			}
		}
	}
	if items, ok := s["items"].(map[string]any); ok {
		return hasTaxIDField(items, schemas, seen)
	}
	return false
}

func resolve(v any, params map[string]any) map[string]any {
	m, _ := v.(map[string]any)
	if ref, ok := m["$ref"].(string); ok {
		target, _ := params[strings.TrimPrefix(ref, "#/components/parameters/")].(map[string]any)
		return target
	}
	return m
}

func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func isHTTPMethod(m string) bool {
	switch m {
	case "get", "post", "put", "patch", "delete":
		return true
	}
	return false
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
