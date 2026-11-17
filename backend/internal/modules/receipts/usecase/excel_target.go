package usecase

import (
	"context"
	"errors"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/target"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
)

// ExpenseTarget writes expense rows of the common Excel import (#395) with
// the same rules as the expense sheet upload: the row key makes a file
// already imported (or exported from here) a no-op.
func (s *Service) ExpenseTarget() target.Target { return sheetTarget{s: s, dataType: mapping.Expenses} }

// RetailSaleTarget writes Z report rows of the common Excel import (#395).
func (s *Service) RetailSaleTarget() target.Target {
	return sheetTarget{s: s, dataType: mapping.RetailSales}
}

type sheetTarget struct {
	s        *Service
	dataType string
}

func (t sheetTarget) columns() []sheet.Column {
	if t.dataType == mapping.RetailSales {
		return sheet.ZColumns
	}
	return sheet.ExpenseColumns
}

// Heuristic maps the template headers and their aliases.
func (t sheetTarget) Heuristic(l parse.Layout) mapping.Mapping {
	cat, _ := mapping.CatalogFor(t.dataType)
	cols := t.columns()
	return target.MatchHeaders(t.dataType, l, func(h string) string {
		if k, ok := sheet.HeaderKey(cols, h); ok && cat.Has(k) {
			return k
		}
		return ""
	})
}

// Commit writes the rows as the module's own template and imports it.
func (t sheetTarget) Commit(ctx context.Context, in target.Input) (target.Result, error) {
	var cols []target.Column
	for _, c := range t.columns() {
		cols = append(cols, target.Column{Field: c.Key, Header: c.Header})
	}
	data, lines, err := target.CSV(nil, cols, in.Rows)
	if err != nil {
		return target.Result{}, err
	}
	name := target.CSVName(in.FileName)
	var res ImportResult
	if t.dataType == mapping.RetailSales {
		res, err = t.s.ImportRetailSales(ctx, in.Scope, name, data)
	} else {
		res, err = t.s.ImportExpenses(ctx, in.Scope, in.UserID, name, data)
	}
	if errors.Is(err, ErrInvalid) {
		return target.Result{}, target.Rejectf("%v", err)
	}
	if err != nil {
		return target.Result{}, err
	}
	out := target.Result{Added: res.Inserted, Duplicates: res.Skipped, Errors: []target.RowError{}}
	for _, e := range res.Errors {
		out.Errors = append(out.Errors, target.RowError{RowNo: lines[e.Line], Reason: e.Message})
	}
	return out, nil
}
