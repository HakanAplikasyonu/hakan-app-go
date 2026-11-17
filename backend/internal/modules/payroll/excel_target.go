package payroll

import (
	"context"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/target"
)

// ExcelTarget writes payroll rows of the common Excel import (#395) as one
// payroll run: the same rules as the payroll upload (period, totals,
// re-import of a period replaces its run).
func (s *Service) ExcelTarget() target.Target { return excelTarget{s: s} }

type excelTarget struct{ s *Service }

// Heuristic is the payroll upload's header mapping.
func (excelTarget) Heuristic(l parse.Layout) mapping.Mapping {
	mp := HeuristicMapping(l)
	mp.SourceSystem = "unknown"
	return mp
}

// Commit writes the rows as a canonical sheet with an explicit column
// mapping and imports it as a payroll run.
func (t excelTarget) Commit(ctx context.Context, in target.Input) (target.Result, error) {
	used := map[string]bool{}
	for _, r := range in.Rows {
		for k, v := range r.Values {
			if v != "" {
				used[k] = true
			}
		}
	}
	// Only the fields the file has: an empty column would not be in the
	// sheet layout the payroll reader detects.
	var cols []target.Column
	override := map[string]string{}
	for _, f := range mapping.PayrollFields {
		if !used[f.Key] {
			continue
		}
		override[parse.ColumnLetter(len(cols))] = f.Key
		cols = append(cols, target.Column{Field: f.Key, Header: f.Key})
	}
	data, _, err := target.CSV(nil, cols, in.Rows)
	if err != nil {
		return target.Result{}, err
	}
	res, err := t.s.Import(ctx, in.Scope, in.UserID, target.CSVName(in.FileName), data, ImportOptions{Period: in.Period, Override: override})
	switch {
	case errors.Is(err, ErrPeriodRequired):
		return target.Result{}, target.Rejectf("bordro dönemi dosyada yok; içe aktarmayı dönem (YYYY-AA) seçerek yükleyin")
	case errors.Is(err, ErrMappingMissing):
		return target.Result{}, target.Rejectf("personel adı, brüt ve net ücret kolonları eşlenmeli")
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrInvalidOverride), errors.Is(err, ErrNoPayrollSheet):
		return target.Result{}, target.Rejectf("%v", err)
	case err != nil:
		return target.Result{}, err
	}
	out := target.Result{Added: res.LineCount, Errors: []target.RowError{}}
	out.Note = fmt.Sprintf("%s dönemi bordrosu yazıldı: %d personel", res.Period, res.LineCount)
	if len(res.Issues) > 0 {
		out.Note += fmt.Sprintf(", %d uyarı", len(res.Issues))
	}
	return out, nil
}
