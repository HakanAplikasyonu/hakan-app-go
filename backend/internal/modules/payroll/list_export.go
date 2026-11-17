package payroll

import (
	"context"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Bordro ekranlarının liste dışa aktarımları (#395): personel, bordro
// dönemleri ve SGK primleri, ekrandaki süzgeçle .xlsx olarak. Excel yazımı
// masraf / Z / banka dışa aktarımıyla ortak (receipts/sheet). TC kimlik
// numarası listede olduğu gibi maskeli kalır.

// listExportLimit bounds one list export.
const listExportLimit = 10_000

var employeeColumns = []sheet.Column{
	{Key: "employee_name", Header: "Adı Soyadı", Width: 26},
	{Key: "national_id", Header: "TC Kimlik No", Width: 14},
	{Key: "sgk_no", Header: "SGK Sicil No", Width: 14},
	{Key: "job_title", Header: "Görevi", Width: 18},
	{Key: "department", Header: "Bölüm", Width: 16},
	{Key: "hire_date", Header: "İşe Giriş Tarihi", Width: 12},
	{Key: "exit_date", Header: "İşten Çıkış Tarihi", Width: 12},
	{Key: "exit_reason", Header: "Çıkış Nedeni", Width: 18},
	{Key: "status", Header: "Durum", Width: 10},
}

var runColumns = []sheet.Column{
	{Key: "period", Header: "Dönem", Width: 10},
	{Key: "employee_count", Header: "Personel", Width: 10},
	{Key: "gross", Header: "Brüt Ücret", Width: 14, Money: true},
	{Key: "net", Header: "Net Ücret", Width: 14, Money: true},
	{Key: "sgk_employee", Header: "SGK İşçi Payı", Width: 14, Money: true},
	{Key: "unemployment_employee", Header: "İşsizlik İşçi Payı", Width: 14, Money: true},
	{Key: "income_tax", Header: "Gelir Vergisi", Width: 14, Money: true},
	{Key: "stamp_tax", Header: "Damga Vergisi", Width: 14, Money: true},
	{Key: "sgk_employer", Header: "SGK İşveren Payı", Width: 14, Money: true},
	{Key: "unemployment_employer", Header: "İşsizlik İşveren Payı", Width: 14, Money: true},
	{Key: "sgk_incentive", Header: "SGK Teşvik", Width: 14, Money: true},
	{Key: "employer_cost", Header: "İşverene Maliyet", Width: 14, Money: true},
	{Key: "net_paid", Header: "Ödenen Net", Width: 14, Money: true},
	{Key: "payment_status", Header: "Ödeme Durumu", Width: 14},
	{Key: "sgk_status", Header: "SGK Durumu", Width: 14},
	{Key: "source", Header: "Kaynak", Width: 14},
}

var premiumColumns = []sheet.Column{
	{Key: "period", Header: "Dönem", Width: 10},
	{Key: "accrual", Header: "Tahakkuk", Width: 14, Money: true},
	{Key: "incentive", Header: "Teşvik", Width: 14, Money: true},
	{Key: "payable", Header: "Ödenecek", Width: 14, Money: true},
	{Key: "due_date", Header: "Son Ödeme", Width: 12},
	{Key: "document_no", Header: "Tahakkuk Fişi No", Width: 16},
	{Key: "paid", Header: "Ödenen", Width: 14, Money: true},
	{Key: "paid_at", Header: "Ödeme Tarihi", Width: 12},
	{Key: "status", Header: "Durum", Width: 12},
	{Key: "note", Header: "Not", Width: 24},
}

var statusLabels = map[string]string{
	"active": "Çalışıyor", "exited": "Ayrıldı",
	"paid": "Ödendi", "partial": "Kısmen ödendi", "unpaid": "Ödenmedi",
	"accrued": "Tahakkuk etti", "overdue": "Gecikmiş",
}

func label(s string) string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return s
}

func trDate(s *string) string {
	if s == nil || *s == "" {
		return ""
	}
	if d, err := time.Parse(time.DateOnly, *s); err == nil {
		return d.Format("02.01.2006")
	}
	return *s
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func amount(d decimal.Decimal) string { return d.StringFixed(2) }

// ExportEmployees writes the filtered employee list.
func (s *Service) ExportEmployees(ctx context.Context, taxpayerID int64, f EmployeeFilter) ([]byte, error) {
	f.Limit, f.Offset = listExportLimit, 0
	p, err := s.ListEmployees(ctx, taxpayerID, f)
	if err != nil {
		return nil, err
	}
	rows := make([]sheet.Row, 0, len(p.Items))
	for _, e := range p.Items {
		rows = append(rows, sheet.Row{
			"employee_name": e.FullName, "national_id": str(e.NationalIDMasked), "sgk_no": str(e.SGKNo),
			"job_title": str(e.JobTitle), "department": str(e.Department),
			"hire_date": trDate(e.HireDate), "exit_date": trDate(e.ExitDate), "exit_reason": str(e.ExitReason),
			"status": label(e.Status),
		})
	}
	return sheet.Write("Personel", employeeColumns, rows)
}

// ExportRuns writes the payroll runs of the year ("" = all), one row each.
func (s *Service) ExportRuns(ctx context.Context, scope engagementctx.Scope, year string) ([]byte, error) {
	p, err := s.ListRuns(ctx, scope, year, listExportLimit, 0)
	if err != nil {
		return nil, err
	}
	rows := make([]sheet.Row, 0, len(p.Items))
	for _, r := range p.Items {
		t := r.Totals
		row := sheet.Row{
			"period": r.Period, "employee_count": strconv.Itoa(int(t.EmployeeCount)),
			"gross": amount(t.Gross), "net": amount(t.Net), "sgk_employee": amount(t.SGKEmployee),
			"unemployment_employee": amount(t.UnemploymentEmployee), "income_tax": amount(t.IncomeTax),
			"stamp_tax": amount(t.StampTax), "sgk_employer": amount(t.SGKEmployer),
			"unemployment_employer": amount(t.UnemploymentEmployer), "sgk_incentive": amount(t.SGKIncentive),
			"employer_cost": amount(t.EmployerCost), "net_paid": amount(r.NetPaid),
			"payment_status": label(r.PaymentStatus), "source": r.SourceSystem,
		}
		if r.SGK != nil {
			row["sgk_status"] = label(r.SGK.Status)
		}
		rows = append(rows, row)
	}
	return sheet.Write("Bordro dönemleri", runColumns, rows)
}

// ExportPremiums writes the SGK accruals of the year ("" = all).
func (s *Service) ExportPremiums(ctx context.Context, taxpayerID int64, year string) ([]byte, error) {
	p, err := s.ListPremiums(ctx, taxpayerID, year, listExportLimit, 0)
	if err != nil {
		return nil, err
	}
	rows := make([]sheet.Row, 0, len(p.Items))
	for _, x := range p.Items {
		due := x.DueDate
		rows = append(rows, sheet.Row{
			"period": x.Period, "accrual": amount(x.Accrual), "incentive": amount(x.Incentive),
			"payable": amount(x.Payable), "due_date": trDate(&due), "document_no": str(x.DocumentNo),
			"paid": amount(x.Paid), "paid_at": trDate(x.PaidAt), "status": label(x.Status), "note": str(x.Note),
		})
	}
	return sheet.Write("SGK primleri", premiumColumns, rows)
}
