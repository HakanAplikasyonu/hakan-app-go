package accountingexport

import (
	"errors"
	"testing"
	"testing/fstest"
)

func csvTpl(extra string) fstest.MapFS {
	return fstest.MapFS{"t.yaml": &fstest.MapFile{Data: []byte(`driver: x
template: t
version: "1"
verified: false
csv: {` + extra + `}
columns:
  - { key: a, header: "A", required: true }
  - { key: b, header: "B" }
`)}}
}

func TestLoadCSVTemplateChecks(t *testing.T) {
	for _, bad := range []string{`delimiter: ";;"`, `quote_mode: some`, `encoding: latin5`, `line_ending: cr`, `unknown: 1`} {
		if _, err := LoadCSVTemplate(csvTpl(bad), "t.yaml"); !errors.Is(err, ErrTemplate) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, err := LoadCSVTemplate(csvTpl(""), "t.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestCSVQuoteModes(t *testing.T) {
	rows := []map[string]string{{"a": `x"y`, "b": "p;q"}, {"a": "z", "b": "line\nbreak"}}
	cases := map[string]string{
		``:                                    "A;B\r\n\"x\"\"y\";\"p;q\"\r\nz;\"line\nbreak\"",
		`quote_mode: all`:                     "\"A\";\"B\"\r\n\"x\"\"y\";\"p;q\"\r\n\"z\";\"line\nbreak\"",
		`quote: none`:                         "A;B\r\nx\"y;p q\r\nz;line break",
		`header: false, final_line_end: true`: "\"x\"\"y\";\"p;q\"\r\nz;\"line\nbreak\"\r\n",
	}
	for opts, want := range cases {
		tpl, err := LoadCSVTemplate(csvTpl(opts), "t.yaml")
		if err != nil {
			t.Fatal(err)
		}
		res, err := tpl.WriteCSV(rows)
		if err != nil || string(res.Data) != want {
			t.Errorf("%s:\n%q\nwant\n%q (%v)", opts, res.Data, want, err)
		}
	}
	tpl, _ := LoadCSVTemplate(csvTpl(`header_line_end: true`), "t.yaml")
	if res, _ := tpl.WriteCSV(nil); string(res.Data) != "A;B\r\n" {
		t.Fatalf("header line end: %q", res.Data)
	}
}
