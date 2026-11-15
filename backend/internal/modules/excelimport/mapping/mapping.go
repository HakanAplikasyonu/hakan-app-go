package mapping

import "fmt"

// Origins of a column mapping.
const (
	OriginTemplate = "template"
	OriginAI       = "ai"
	OriginUser     = "user"
)

// ReviewBelow is the confidence under which a column is asked to the
// accountant (04 §4.3 rule 2).
const ReviewBelow = 0.80

// ColumnMap maps one sheet column to a target field ("" = unmapped; the
// value still goes to raw_payload).
type ColumnMap struct {
	Column      string  `json:"column"`
	Header      string  `json:"header"`
	Field       string  `json:"field"`
	Confidence  float64 `json:"confidence"`
	Origin      string  `json:"origin"`
	NeedsReview bool    `json:"needs_review"`
}

// Mapping is the column mapping of one import.
type Mapping struct {
	DataType     string      `json:"data_type"`
	SourceSystem string      `json:"source_system"`
	HeaderRow    int         `json:"header_row"`
	Columns      []ColumnMap `json:"columns"`
	Warnings     []string    `json:"warnings"`
}

// NeedsReview reports whether the accountant must look at the mapping:
// a low-confidence column or a missing required field.
func (m Mapping) NeedsReview() bool {
	for _, c := range m.Columns {
		if c.NeedsReview {
			return true
		}
	}
	return len(MissingRequired(m.DataType, m)) > 0
}

// Validate checks fields and duplicates. Two columns may not map to one field.
func (m Mapping) Validate() error {
	if !Supported(m.DataType) {
		return fmt.Errorf("data_type %q is not supported", m.DataType)
	}
	seen := map[string]string{}
	for _, c := range m.Columns {
		if c.Field == "" {
			continue
		}
		if !ValidField(m.DataType, c.Field) {
			return fmt.Errorf("column %s: unknown field %q", c.Column, c.Field)
		}
		if prev, ok := seen[c.Field]; ok {
			return fmt.Errorf("field %q is mapped twice (%s, %s)", c.Field, prev, c.Column)
		}
		seen[c.Field] = c.Column
	}
	return nil
}

// FieldColumns returns field -> column letter.
func (m Mapping) FieldColumns() map[string]string {
	out := map[string]string{}
	for _, c := range m.Columns {
		if c.Field != "" {
			out[c.Field] = c.Column
		}
	}
	return out
}
