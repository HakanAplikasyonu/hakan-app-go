package foreigninvoices

import (
	"context"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// Column labels are Turkish text (i18n.Translate returns an unknown key
// unchanged).
var exportColumns = []ioengine.Column{
	{Key: "vendor", LabelKey: "Satıcı", Type: ioengine.ColumnTypeString},
	{Key: "country", LabelKey: "Ülke", Type: ioengine.ColumnTypeString},
	{Key: "invoice_no", LabelKey: "Fatura no", Type: ioengine.ColumnTypeString},
	{Key: "invoice_date", LabelKey: "Fatura tarihi", Type: ioengine.ColumnTypeString},
	{Key: "kind", LabelKey: "İşlem türü", Type: ioengine.ColumnTypeString},
	{Key: "amount", LabelKey: "Tutar", Type: ioengine.ColumnTypeString},
	{Key: "currency", LabelKey: "Döviz", Type: ioengine.ColumnTypeString},
	{Key: "rate", LabelKey: "Kur", Type: ioengine.ColumnTypeString},
	{Key: "gross", LabelKey: "Brüt (döviz)", Type: ioengine.ColumnTypeString},
	{Key: "stopaj_rate", LabelKey: "Stopaj oranı", Type: ioengine.ColumnTypeString},
	{Key: "gross_try", LabelKey: "Matrah (TL)", Type: ioengine.ColumnTypeString},
	{Key: "stopaj_try", LabelKey: "Stopaj (TL)", Type: ioengine.ColumnTypeString},
	{Key: "kdv2_try", LabelKey: "KDV2 (TL)", Type: ioengine.ColumnTypeString},
	{Key: "status", LabelKey: "Durum", Type: ioengine.ColumnTypeString},
}

func orDash(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return *s
}

// Dataset is the period's list as rows, with a totals row at the end.
func Dataset(l List) ioengine.Dataset {
	ds := ioengine.Dataset{Resource: "foreign_invoices", Columns: exportColumns, Rows: make([]map[string]any, 0, len(l.Items)+1)}
	for _, it := range l.Items {
		status := "Taslak"
		if it.Status == StatusConfirmed {
			status = "Onaylandı"
		}
		if it.MissingRate {
			status += " · kur eksik"
		}
		stopajRate := "—"
		if it.Treatment == TreatmentKDV2Stopaj {
			stopajRate = "%" + it.StopajRate
			if it.GrossUp {
				stopajRate += " (brütleştirme)"
			}
		}
		ds.Rows = append(ds.Rows, map[string]any{
			"vendor": it.Vendor, "country": it.Country, "invoice_no": it.InvoiceNo,
			"invoice_date": orDash(it.InvoiceDate), "kind": it.ServiceKindLabel,
			"amount": it.Amount, "currency": it.Currency, "rate": orDash(it.ExchangeRate),
			"gross": it.Gross, "stopaj_rate": stopajRate, "gross_try": orDash(it.GrossTRY),
			"stopaj_try": orDash(it.StopajTRY), "kdv2_try": orDash(it.KDV2TRY), "status": status,
		})
	}
	total := map[string]any{
		"vendor": fmt.Sprintf("Toplam (%d fatura)", l.Totals.Count), "gross_try": l.Totals.GrossTRY,
		"stopaj_try": l.Totals.StopajTRY, "kdv2_try": l.Totals.KDV2TRY,
	}
	if l.Totals.MissingRate > 0 {
		total["status"] = fmt.Sprintf("%d faturada kur eksik (toplama girmedi)", l.Totals.MissingRate)
	}
	ds.Rows = append(ds.Rows, total)
	return ds
}

// Export renders the period's list as .xlsx or .pdf with the office
// letterhead. It returns the bytes, a file name and the content type.
func (s *Service) Export(ctx context.Context, sub Subject, period, format string) ([]byte, string, string, error) {
	if format == "" {
		format = string(ioengine.ExportXLSX)
	}
	f := ioengine.ExportFormat(format)
	if f != ioengine.ExportXLSX && f != ioengine.ExportPDF {
		return nil, "", "", fmt.Errorf("%w: format must be xlsx or pdf", ErrInvalid)
	}
	l, err := s.List(ctx, sub, period)
	if err != nil {
		return nil, "", "", err
	}
	var lh *ioengine.Letterhead
	if settings, err := s.q.GetAppSettings(ctx); err == nil {
		h := ioengine.LetterheadFromSettings(settings)
		lh = &h
	}
	title := "Yabancı hizmet faturaları ve KDV2 — " + l.Period
	data, err := ioengine.EncodeExport(f, Dataset(l), "tr", lh, title)
	if err != nil {
		return nil, "", "", fmt.Errorf("export foreign invoices: %w", err)
	}
	name := fmt.Sprintf("yabanci-faturalar-%s.%s", l.Period, ioengine.FileExtForExport(f))
	return data, name, ioengine.ContentTypeForExport(f), nil
}
