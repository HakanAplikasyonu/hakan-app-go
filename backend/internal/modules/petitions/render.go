package petitions

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"
)

// ErrInvalid is a validation error (unknown template, missing field ...).
var ErrInvalid = errors.New("petitions: invalid request")

// Taxpayer, Office and Signer are filled in from the engagement and the user.
type Taxpayer struct {
	Title     string
	TaxID     string
	TaxIDType string // "VKN" or "TCKN"
	TaxOffice string
}

// Office is the accounting office.
type Office struct{ Name string }

// Signer is the user who prepares the petition.
type Signer struct {
	Name           string
	SMMMRegistryNo string
}

// Data is the input of a template.
type Data struct {
	Taxpayer Taxpayer
	Office   Office
	Signer   Signer
	Date     string
	F        map[string]string
}

// Rendered is a petition ready to preview or print.
type Rendered struct {
	TemplateKey     string    `json:"template_key"`
	TemplateVersion int       `json:"template_version"`
	Title           string    `json:"title"`
	Recipient       string    `json:"recipient"`
	Subject         string    `json:"subject"`
	Body            string    `json:"body"`
	Date            string    `json:"date"`
	Petitioner      string    `json:"petitioner"`
	Signer          string    `json:"signer"`
	Draft           bool      `json:"draft"`
	DraftNotice     string    `json:"draft_notice,omitempty"`
	RenderedAt      time.Time `json:"rendered_at"`
}

var funcs = template.FuncMap{
	"orDots": func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "...................."
		}
		return s
	},
}

// Render validates fields and fills the template.
func Render(key string, fields map[string]string, base Data, now time.Time) (Rendered, map[string]string, error) {
	t, ok := Lookup(key)
	if !ok {
		return Rendered{}, nil, fmt.Errorf("%w: unknown template %q", ErrInvalid, key)
	}
	clean, err := checkFields(t, fields)
	if err != nil {
		return Rendered{}, nil, err
	}
	base.F = clean
	if base.Date == "" {
		base.Date = now.Format("02.01.2006")
	}
	fill := func(name, text string) (string, error) {
		tpl, err := template.New(name).Funcs(funcs).Option("missingkey=error").Parse(text)
		if err != nil {
			return "", fmt.Errorf("petitions: template %s/%s: %w", key, name, err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, base); err != nil {
			return "", fmt.Errorf("petitions: template %s/%s: %w", key, name, err)
		}
		return strings.TrimSpace(buf.String()), nil
	}
	out := Rendered{
		TemplateKey: t.Key, TemplateVersion: t.Version, Title: t.Title,
		Date: base.Date, Draft: t.Draft, RenderedAt: now.UTC(),
	}
	if out.Recipient, err = fill("recipient", t.Recipient); err != nil {
		return Rendered{}, nil, err
	}
	if out.Subject, err = fill("subject", t.Subject); err != nil {
		return Rendered{}, nil, err
	}
	if out.Body, err = fill("body", t.Body); err != nil {
		return Rendered{}, nil, err
	}
	out.Petitioner = strings.TrimSpace(base.Taxpayer.Title)
	signer := strings.TrimSpace(base.Signer.Name)
	if no := strings.TrimSpace(base.Signer.SMMMRegistryNo); no != "" {
		signer += " (SMMM oda sicil no: " + no + ")"
	}
	if office := strings.TrimSpace(base.Office.Name); office != "" {
		signer += ", " + office
	}
	out.Signer = strings.Trim(signer, ", ")
	if out.Draft {
		out.DraftNotice = DraftNotice
	}
	return out, clean, nil
}

// checkFields keeps declared fields only, trims them and enforces
// required, length and date rules. Control characters other than newlines
// in textareas are refused.
func checkFields(t Template, in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(t.Fields))
	for k := range in {
		if !declared(t, k) {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalid, k)
		}
	}
	for _, f := range t.Fields {
		v := strings.TrimSpace(in[f.Key])
		if f.Type != FieldTextarea {
			v = strings.Join(strings.Fields(v), " ")
		}
		if v == "" {
			if f.Required {
				return nil, fmt.Errorf("%w: %s is required", ErrInvalid, f.Label)
			}
			out[f.Key] = ""
			continue
		}
		if f.MaxLen > 0 && utf8.RuneCountInString(v) > f.MaxLen {
			return nil, fmt.Errorf("%w: %s is too long (max %d)", ErrInvalid, f.Label, f.MaxLen)
		}
		for _, r := range v {
			if r < 0x20 && r != '\n' && (r != '\r' || f.Type != FieldTextarea) {
				return nil, fmt.Errorf("%w: %s has control characters", ErrInvalid, f.Label)
			}
		}
		if f.Type == FieldDate {
			d, err := time.Parse("2006-01-02", v)
			if err != nil {
				return nil, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrInvalid, f.Label)
			}
			v = d.Format("02.01.2006")
		}
		out[f.Key] = strings.ReplaceAll(v, "\r\n", "\n")
	}
	return out, nil
}

func declared(t Template, key string) bool {
	for _, f := range t.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}
