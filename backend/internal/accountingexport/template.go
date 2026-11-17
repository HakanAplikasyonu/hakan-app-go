package accountingexport

import (
	"errors"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Template is a program's import file layout, loaded from versioned YAML
// (docs/02 §3.3). Column names and order are data, never code: when the real
// template arrives only the YAML changes.
type Template struct {
	Driver     string               `yaml:"driver"`
	Name       string               `yaml:"template"`
	Version    string               `yaml:"version"`
	Verified   bool                 `yaml:"verified"`
	File       TemplateFile         `yaml:"file"`
	Limits     Limits               `yaml:"limits"`
	Columns    []Column             `yaml:"columns"`
	EntryTypes map[EntryType]string `yaml:"entry_types"`
	Notes      string               `yaml:"notes"`
}

// TemplateFile is the file container.
type TemplateFile struct {
	Format    string `yaml:"format"` // xlsx | xls | csv
	Sheet     string `yaml:"sheet"`  // "" = default first sheet
	HeaderRow int    `yaml:"header_row"`
}

// Column is one output column. Key names a value the driver knows how to fill
// (entry_no, entry_date, account, debit, credit, description …).
type Column struct {
	Key      string `yaml:"key"`
	Header   string `yaml:"header"`
	Required bool   `yaml:"required"`
	// Format is a Go time layout for dates; Decimal is the decimal separator
	// for amounts written as text ("" = numeric cell).
	Format  string `yaml:"format"`
	Decimal string `yaml:"decimal"`
	// Pattern builds a text value from placeholders (keys that support it,
	// e.g. Luca description_source: "{description} ETTN:{ettn}"). Empty
	// placeholders and the text around them are dropped by the driver.
	Pattern string `yaml:"pattern"`
}

// ErrTemplate wraps template loading errors.
var ErrTemplate = errors.New("accountingexport: invalid template")

// LoadTemplate reads and checks one template file.
func LoadTemplate(fsys fs.FS, path string) (Template, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return Template{}, fmt.Errorf("%w: %v", ErrTemplate, err)
	}
	var t Template
	dec := yaml.NewDecoder(bytesReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return Template{}, fmt.Errorf("%w: %s: %v", ErrTemplate, path, err)
	}
	if err := t.check(); err != nil {
		return Template{}, fmt.Errorf("%w: %s: %v", ErrTemplate, path, err)
	}
	return t, nil
}

func (t Template) check() error {
	if t.Driver == "" || t.Name == "" || t.Version == "" {
		return errors.New("driver, template and version are required")
	}
	if len(t.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	seen := map[string]bool{}
	for _, c := range t.Columns {
		if c.Key == "" || c.Header == "" {
			return errors.New("every column needs key and header")
		}
		if seen[c.Key] {
			return fmt.Errorf("duplicate column key %q", c.Key)
		}
		seen[c.Key] = true
	}
	if t.File.HeaderRow < 1 {
		return errors.New("file.header_row must be ≥ 1")
	}
	return nil
}
