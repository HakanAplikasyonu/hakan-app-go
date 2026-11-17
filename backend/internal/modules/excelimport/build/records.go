package build

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Records converts the data rows of a non-invoice data type (payroll,
// expenses, Z reports, bank movements; #395). Each mapped cell is checked by
// its kind and normalised (dates YYYY-MM-DD, amounts signed with two
// decimals); a row without one of the required groups is an error. The
// owning module applies its own rules when the rows are written.
func Records(sheet parse.Sheet, l parse.Layout, m mapping.Mapping, opt Options) []Row {
	cat, _ := mapping.CatalogFor(m.DataType)
	cols := m.FieldColumns()
	idx := map[string]int{}
	headers := map[int]string{}
	for _, c := range l.Columns {
		idx[c.Letter] = c.Index
		headers[c.Index] = c.Header
	}
	value := func(r []string, field string) string {
		letter, ok := cols[field]
		if !ok {
			return ""
		}
		if i := idx[letter]; i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	var dataRows [][]string
	for i := l.FirstDataRow - 1; i < l.LastDataRow && i < len(sheet.Rows); i++ {
		if i >= 0 {
			dataRows = append(dataRows, sheet.Rows[i])
		}
	}
	styles := map[string]parse.NumberStyle{}
	for field := range cols {
		if cat.Kind(field) != mapping.KindMoney {
			continue
		}
		var vals []string
		for _, r := range dataRows {
			if v := value(r, field); v != "" {
				vals = append(vals, v)
			}
		}
		styles[field] = parse.DetectNumberStyle(vals)
	}
	// A signed amount is the norm in a bank statement; elsewhere a minus is
	// worth a look.
	warnNegative := m.DataType != mapping.BankTransactions

	out := make([]Row, 0, len(dataRows))
	for k, r := range dataRows {
		row := Row{RowNo: l.FirstDataRow + k, Raw: map[string]string{}, Record: map[string]string{}}
		for i, v := range r {
			if h, ok := headers[i]; ok && v != "" {
				row.Raw[h] = v
			}
		}
		if len(row.Raw) == 0 || parse.IsTotalRow(r) {
			continue
		}
		b := builder{row: &row, value: func(f string) string { return value(r, f) }, styles: styles}
		for _, f := range cat.Fields {
			v := b.value(f.Key)
			if v == "" {
				continue
			}
			switch f.Kind {
			case mapping.KindDate:
				d, ok := parse.ParseDate(v)
				if !ok {
					b.err(CodeBadDate, f.Key, v)
					continue
				}
				row.Record[f.Key] = d.Format(time.DateOnly)
			case mapping.KindMoney:
				d, neg, ok := parse.ParseAmount(v, styles[f.Key])
				if !ok {
					b.err(CodeBadNumber, f.Key, v)
					continue
				}
				if neg {
					d = d.Neg()
					if warnNegative {
						b.warn(CodeNegative, f.Key, v)
					}
				}
				row.Record[f.Key] = d.StringFixed(2)
			case mapping.KindRate:
				d, ok := parse.Rate(v)
				if !ok {
					b.err(CodeBadNumber, f.Key, v)
					continue
				}
				row.Record[f.Key] = d.String()
			case mapping.KindInt:
				n, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(v, ".", ""), " ", ""))
				if err != nil || n < 0 {
					b.err(CodeBadNumber, f.Key, v)
					continue
				}
				row.Record[f.Key] = strconv.Itoa(n)
			default:
				row.Record[f.Key] = v
			}
		}
		for _, g := range cat.Required {
			hit := false
			for _, f := range g {
				if value(r, f) != "" {
					hit = true
				}
			}
			if !hit {
				b.err(CodeRequired, strings.Join(g, "|"), "")
			}
		}
		if opt.Period != "" {
			if d, ok := row.Record["date"]; ok && !strings.HasPrefix(d, opt.Period) {
				b.warn(CodeOutOfPeriod, "date", d)
			}
		}
		if cat.TotalField != "" {
			if v, ok := row.Record[cat.TotalField]; ok {
				if d, err := decimal.NewFromString(v); err == nil {
					row.amount = &d
				}
			}
		}
		row.Result = ResultOK
		for _, msg := range row.Messages {
			if msg.Severity == ResultError {
				row.Result = ResultError
				break
			}
			row.Result = ResultWarning
		}
		if row.Messages == nil {
			row.Messages = []Message{}
		}
		out = append(out, row)
	}
	return out
}
