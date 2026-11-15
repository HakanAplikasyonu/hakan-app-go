package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	docPath = "../../../../../docs/kaynak/musavir-entegrasyon-paketi/05-uyari-kodlari.md"
	// The catalog is seeded by *_warning_rules.up.sql; later migrations add
	// codes with their own INSERT INTO warning_codes (e.g. *_vat_forecast).
	seedGlob = "../../../../migrations/*.up.sql"
)

// docCodes parses 05 §3: table rows "| `CODE` | `severity` | ..." and the
// §3.4 BNK list, whose levels are given in prose (BNK_GAP and
// BNK_BALANCE_MISMATCH blocker, the rest warning).
func docCodes(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read 05: %v", err)
	}
	text := string(raw)
	start := strings.Index(text, "## 3. ")
	end := strings.Index(text, "## 4. ")
	if start < 0 || end < start {
		t.Fatal("05: §3 not found")
	}
	sec := text[start:end]
	out := map[string]string{}
	row := regexp.MustCompile("(?m)^\\| `([A-Z0-9_]+)` \\| `(info|warning|blocker)` \\|")
	for _, m := range row.FindAllStringSubmatch(sec, -1) {
		out[m[1]] = m[2]
	}
	bankStart := strings.Index(sec, "### 3.4")
	for _, m := range regexp.MustCompile("`(BNK_[A-Z_]+)`").FindAllStringSubmatch(sec[bankStart:], -1) {
		if _, ok := out[m[1]]; ok {
			continue
		}
		out[m[1]] = "warning"
	}
	out["BNK_GAP"] = "blocker"
	out["BNK_BALANCE_MISMATCH"] = "blocker"
	// BNK_PAYMENT_METHOD_MISMATCH is named in §3.3 prose too; it is a bank code.
	return out
}

func seedCodes(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(seedGlob)
	out := map[string]string{}
	re := regexp.MustCompile(`\('([A-Z0-9_]+)',\s*'[0-9.]+',\s*'(info|warning|blocker)'`)
	seeds := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "INSERT INTO warning_codes") {
			continue
		}
		seeds++
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			if _, dup := out[m[1]]; dup {
				t.Errorf("warning code %s seeded twice (%s)", m[1], filepath.Base(f))
			}
			out[m[1]] = m[2]
		}
	}
	if seeds == 0 {
		t.Fatalf("no migration seeds warning_codes (glob %s)", seedGlob)
	}
	return out
}

func TestCatalogMatchesDocAndSeed(t *testing.T) {
	doc := docCodes(t)
	seed := seedCodes(t)
	if len(doc) < 45 {
		t.Fatalf("parsed only %d codes from 05; parser broken?", len(doc))
	}
	added := map[string]bool{
		"UBL_SCHEMATRON_FAIL": true, "ACCOUNT_NOT_IN_TARGET_PLAN": true,
		// docs/00 K9 (T10, issue #33)
		"VAT_PAYABLE_THRESHOLD": true, "VAT_DEADLINE_NEAR": true,
	}
	for code, sev := range doc {
		if _, ok := Lookup(code); !ok {
			t.Errorf("05 code %s missing from catalog", code)
		}
		if seed[code] != sev {
			t.Errorf("%s: seed severity %q, 05 suggests %q", code, seed[code], sev)
		}
	}
	for _, c := range Codes {
		if _, ok := doc[c.Code]; !ok && !added[c.Code] {
			t.Errorf("catalog code %s is not in 05 (add it to 05 first)", c.Code)
		}
		if _, ok := seed[c.Code]; !ok {
			t.Errorf("catalog code %s missing from warning_codes seed", c.Code)
		}
	}
	if len(seed) != len(Codes) {
		t.Errorf("seed has %d codes, catalog %d", len(seed), len(Codes))
	}
}

func TestCatalogEntries(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Codes {
		if seen[c.Code] {
			t.Errorf("duplicate %s", c.Code)
		}
		seen[c.Code] = true
		if len(c.EntityTypes) == 0 {
			t.Errorf("%s has no entity types", c.Code)
		}
		for _, e := range c.EntityTypes {
			if e != AnyEntity && !ValidEntityType(e) {
				t.Errorf("%s: unknown entity type %q", c.Code, e)
			}
		}
	}
	c, _ := Lookup("LINE_TAXABLE_MISMATCH")
	if !c.Allows(EntityInvoiceItem) || c.Allows(EntityInvoice) {
		t.Fatal("Allows broken")
	}
	c, _ = Lookup("DELETED_AT_SOURCE")
	if !c.Allows(EntityBankTransaction) {
		t.Fatal("AnyEntity broken")
	}
	if !SeverityBlocker.Valid() || Severity("x").Valid() || !StatusAutoResolved.Valid() || Status("x").Valid() {
		t.Fatal("Valid broken")
	}
}
