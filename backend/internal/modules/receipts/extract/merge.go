package extract

// Merge joins the reads of a document photographed in several parts (a
// long Z report: parça 1 has the date, Z no and ÖKC, the last part the
// totals and the KDV breakdown) into one result. For every field the valid
// value with the highest confidence wins; on a tie the earlier part wins.
// The kind is the first part's unless it is "other"; the model is the
// first part's.
func Merge(parts ...Result) Result {
	if len(parts) == 0 {
		return Result{Fields: map[string]Value{}}
	}
	out := Result{Kind: parts[0].Kind, KindConfidence: parts[0].KindConfidence, Model: parts[0].Model, Fields: map[string]Value{}}
	for _, p := range parts {
		if out.Kind == KindOther && p.Kind != "" && p.Kind != KindOther {
			out.Kind, out.KindConfidence = p.Kind, p.KindConfidence
		}
		if p.Kind == out.Kind && p.KindConfidence > out.KindConfidence {
			out.KindConfidence = p.KindConfidence
		}
		for name, v := range p.Fields {
			cur, ok := out.Fields[name]
			switch {
			case !ok:
				out.Fields[name] = v
			case v.Valid && !cur.Valid:
				out.Fields[name] = v
			case v.Valid == cur.Valid && v.Confidence > cur.Confidence:
				out.Fields[name] = v
			}
		}
	}
	return out
}
