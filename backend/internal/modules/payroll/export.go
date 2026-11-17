package payroll

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// exportColumns are the sheet columns; the TCKN stays masked.
var exportColumns = []struct {
	header string
	value  func(LineView) any
}{
	{"Sıra", func(l LineView) any { return l.Row }},
	{"Adı Soyadı", func(l LineView) any { return l.EmployeeName }},
	{"TC Kimlik No", func(l LineView) any { return l.NationalIDMasked }},
	{"Gün", func(l LineView) any {
		if l.DaysWorked == nil {
			return ""
		}
		return *l.DaysWorked
	}},
	{"Brüt Ücret", func(l LineView) any { return money(l.Gross) }},
	{"SGK Matrahı", func(l LineView) any { return money(l.SGKBase) }},
	{"SGK İşçi Payı", func(l LineView) any { return money(l.SGKEmployee) }},
	{"İşsizlik İşçi Payı", func(l LineView) any { return money(l.UnemploymentEmployee) }},
	{"Gelir Vergisi Matrahı", func(l LineView) any { return money(l.IncomeTaxBase) }},
	{"Gelir Vergisi İstisnası", func(l LineView) any { return money(l.IncomeTaxExemption) }},
	{"Ödenecek Gelir Vergisi", func(l LineView) any { return money(l.IncomeTax) }},
	{"Damga Vergisi İstisnası", func(l LineView) any { return money(l.StampTaxExemption) }},
	{"Ödenecek Damga Vergisi", func(l LineView) any { return money(l.StampTax) }},
	{"AGİ", func(l LineView) any { return money(l.AGI) }},
	{"Diğer Kesintiler", func(l LineView) any { return money(l.OtherDeductions) }},
	{"Net Ücret", func(l LineView) any { return money(l.Net) }},
	{"SGK İşveren Payı", func(l LineView) any { return money(l.SGKEmployer) }},
	{"İşsizlik İşveren Payı", func(l LineView) any { return money(l.UnemploymentEmployer) }},
	{"SGK Teşvik", func(l LineView) any { return money(l.SGKIncentive) }},
	{"İşverene Maliyet", func(l LineView) any { return money(l.EmployerCost) }},
}

func money(d decimal.Decimal) float64 {
	f, _ := d.Round(2).Float64()
	return f
}

// ExportRun writes the run as an .xlsx workbook (one sheet, a total row).
func (s *Service) ExportRun(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) ([]byte, string, error) {
	r, err := s.GetRun(ctx, scope, id)
	if err != nil {
		return nil, "", err
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := "Bordro " + r.Period
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, "", err
	}
	_ = f.SetCellValue(sheet, "A1", r.Period+" dönemi bordrosu")
	head, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#EEF2F7"}, Pattern: 1}})
	moneyStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: ptr("#,##0.00")})
	for j, c := range exportColumns {
		cell, _ := excelize.CoordinatesToCellName(j+1, 3)
		_ = f.SetCellValue(sheet, cell, c.header)
		_ = f.SetCellStyle(sheet, cell, cell, head)
	}
	for i, l := range r.Lines {
		for j, c := range exportColumns {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+4)
			_ = f.SetCellValue(sheet, cell, c.value(l))
		}
	}
	last := len(r.Lines) + 4
	_ = f.SetCellValue(sheet, "B"+itoa(last), "Toplam")
	for j := 5; j <= len(exportColumns); j++ {
		col, _ := excelize.ColumnNumberToName(j)
		_ = f.SetCellFormula(sheet, col+itoa(last), "SUM("+col+"4:"+col+itoa(last-1)+")")
		_ = f.SetCellStyle(sheet, col+"4", col+itoa(last), moneyStyle)
	}
	_ = f.SetColWidth(sheet, "B", "B", 28)
	_ = f.SetColWidth(sheet, "C", "C", 14)
	_ = f.SetColWidth(sheet, "E", "T", 16)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "bordro-" + r.Period + ".xlsx", nil
}

func ptr[T any](v T) *T { return &v }

func itoa(n int) string {
	return decimal.NewFromInt(int64(n)).String()
}
