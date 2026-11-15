package extract

import (
	"fmt"
	"sort"
	"strings"
)

// Expected is the ground truth of one sample receipt.
type Expected struct {
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`
	Fields map[string]string `json:"fields"` // raw texts, normalized before comparing
}

// FieldScore is the accuracy of one field across samples.
type FieldScore struct {
	Field   string `json:"field"`
	Samples int    `json:"samples"` // samples where the field is expected
	Correct int    `json:"correct"`
	Missing int    `json:"missing"`
	Wrong   int    `json:"wrong"`
	// LowConfidenceWrong counts wrong values the model itself rated below
	// the review threshold, i.e. errors the review queue would catch.
	LowConfidenceWrong int `json:"low_confidence_wrong"`
}

// Report is an accuracy report (issue #23 acceptance: 20 sample receipts).
type Report struct {
	Samples     int          `json:"samples"`
	KindCorrect int          `json:"kind_correct"`
	Fields      []FieldScore `json:"fields"`
}

// Score compares extracted results with the ground truth. reviewBelow is
// the confidence under which a field goes to the review queue.
func Score(expected []Expected, got []Result, reviewBelow float64) Report {
	rep := Report{Samples: len(expected)}
	scores := map[string]*FieldScore{}
	for i, exp := range expected {
		var res Result
		if i < len(got) {
			res = got[i]
		}
		if res.Kind == exp.Kind {
			rep.KindCorrect++
		}
		for f, raw := range exp.Fields {
			want, ok := Normalize(f, raw)
			if !ok {
				continue
			}
			s := scores[f]
			if s == nil {
				s = &FieldScore{Field: f}
				scores[f] = s
			}
			s.Samples++
			v, present := res.Fields[f]
			switch {
			case !present || !v.Valid:
				s.Missing++
			case v.Value == want:
				s.Correct++
			default:
				s.Wrong++
				if v.Confidence < reviewBelow {
					s.LowConfidenceWrong++
				}
			}
		}
	}
	for _, s := range scores {
		rep.Fields = append(rep.Fields, *s)
	}
	sort.Slice(rep.Fields, func(i, j int) bool { return rep.Fields[i].Field < rep.Fields[j].Field })
	return rep
}

// Markdown renders the report as a table.
func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Örnek: %d · Belge türü doğru: %d\n\n", r.Samples, r.KindCorrect)
	b.WriteString("| Alan | Örnek | Doğru | Eksik | Yanlış | Yanlış ama kuyruğa düşen | Doğruluk |\n|---|---|---|---|---|---|---|\n")
	for _, s := range r.Fields {
		acc := 0.0
		if s.Samples > 0 {
			acc = float64(s.Correct) / float64(s.Samples) * 100
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %%%.0f |\n", s.Field, s.Samples, s.Correct, s.Missing, s.Wrong, s.LowConfidenceWrong, acc)
	}
	return b.String()
}
