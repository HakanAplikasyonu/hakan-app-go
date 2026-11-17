package mapping

// Bordro, masraf, Z raporu ve banka hareketi veri türleri (#395). Fatura
// dışındaki türlerin satırları içe aktarma akışında (yükleme → eşleme →
// doğrulama → önizleme) ortak kurallarla denetlenir; yazma işini sahibi
// olan modül yapar (excelimport/target).
const (
	Expenses         = "expenses"
	RetailSales      = "retail_sales"
	BankTransactions = "bank_transactions"
)

// Field kinds: how a cell is checked and normalised before it reaches the
// owning module (dates as YYYY-MM-DD, amounts as signed decimals).
const (
	KindText  = "text"
	KindDate  = "date"
	KindMoney = "money"
	KindRate  = "rate"
	KindInt   = "int"
)

// Catalog describes one data type: its target fields, the kind of each and
// the required groups (each group needs at least one mapped field).
type Catalog struct {
	DataType string     `json:"data_type"`
	Label    string     `json:"label"`
	Fields   []Field    `json:"fields"`
	Required [][]string `json:"required"`
	// TotalField is summed into the preview total ("" = no total).
	TotalField string `json:"total_field,omitempty"`
	kinds      map[string]string
}

// Kind returns the kind of field f (text when unknown).
func (c Catalog) Kind(f string) string {
	if k, ok := c.kinds[f]; ok {
		return k
	}
	return KindText
}

// Has reports whether f is a field of the catalog.
func (c Catalog) Has(f string) bool {
	for _, x := range c.Fields {
		if x.Key == f {
			return true
		}
	}
	return false
}

type kf struct {
	key, desc, kind string
}

func catalog(dt, label, total string, required [][]string, fields ...kf) Catalog {
	c := Catalog{DataType: dt, Label: label, Required: required, TotalField: total, kinds: map[string]string{}}
	for _, f := range fields {
		c.Fields = append(c.Fields, Field{Key: f.key, Description: f.desc, Kind: f.kind})
		c.kinds[f.key] = f.kind
	}
	return c
}

// Masraf alanları: anahtarlar receipts/sheet.ExpenseColumns ile aynıdır.
var expenseCatalog = catalog(Expenses, "Masraf fişleri", "total",
	[][]string{{"date"}, {"total"}},
	kf{"date", "fiş / belge tarihi", KindDate},
	kf{"vendor_name", "satıcı unvanı / firma", KindText},
	kf{"vendor_tax_number", "satıcı VKN veya TCKN", KindText},
	kf{"receipt_number", "fiş / belge numarası", KindText},
	kf{"description", "açıklama", KindText},
	kf{"category", "masraf kategorisi", KindText},
	kf{"account_code", "hesap kodu (770, 760 ...)", KindText},
	kf{"tax_rate", "KDV oranı", KindRate},
	kf{"tax_amount", "KDV tutarı", KindMoney},
	kf{"total", "KDV dahil toplam", KindMoney},
	kf{"payment_method", "ödeme şekli (nakit, kart, havale)", KindText},
	kf{"vehicle_plate", "araç plakası", KindText},
)

// Z raporu alanları: anahtarlar receipts/sheet.ZColumns ile aynıdır.
var retailCatalog = catalog(RetailSales, "Z raporları", "total",
	[][]string{{"date"}, {"total"}},
	kf{"date", "Z raporu tarihi", KindDate},
	kf{"okc", "ÖKC (yazar kasa) seri numarası", KindText},
	kf{"z_number", "Z numarası", KindText},
	kf{"receipt_count", "fiş sayısı", KindInt},
	kf{"cash", "nakit tahsilat", KindMoney},
	kf{"card", "kredi kartı tahsilatı", KindMoney},
	kf{"other", "diğer ödeme", KindMoney},
	kf{"base_0", "%0 KDV matrahı", KindMoney},
	kf{"vat_0", "%0 KDV", KindMoney},
	kf{"base_1", "%1 KDV matrahı", KindMoney},
	kf{"vat_1", "%1 KDV", KindMoney},
	kf{"base_10", "%10 KDV matrahı", KindMoney},
	kf{"vat_10", "%10 KDV", KindMoney},
	kf{"base_20", "%20 KDV matrahı", KindMoney},
	kf{"vat_20", "%20 KDV", KindMoney},
	kf{"tax_amount", "toplam KDV", KindMoney},
	kf{"total", "Z toplamı (KDV dahil)", KindMoney},
)

// Banka hareketi alanları (23 §4 ekstre kolonları).
var bankCatalog = catalog(BankTransactions, "Banka hareketleri", "",
	[][]string{{"date"}, {"description"}, {"amount", "debit", "credit"}},
	kf{"account_iban", "hesabın IBAN'ı (ekstre sahibi hesap)", KindText},
	kf{"date", "işlem tarihi", KindDate},
	kf{"value_date", "valör tarihi", KindDate},
	kf{"time", "işlem saati", KindText},
	kf{"description", "işlem açıklaması", KindText},
	kf{"amount", "tutar (işaretli: eksi çıkış)", KindMoney},
	kf{"debit", "borç / çıkan tutar", KindMoney},
	kf{"credit", "alacak / giren tutar", KindMoney},
	kf{"balance", "işlem sonrası bakiye", KindMoney},
	kf{"receipt_number", "dekont numarası", KindText},
	kf{"reference", "banka referans numarası", KindText},
	kf{"channel", "işlem kanalı", KindText},
	kf{"counterparty", "karşı taraf adı", KindText},
	kf{"counterparty_iban", "karşı taraf IBAN", KindText},
	kf{"counterparty_tax_id", "karşı taraf VKN / TCKN", KindText},
)

var payrollCatalog = func() Catalog {
	kinds := map[string]string{
		"employee_name": KindText, "first_name": KindText, "last_name": KindText, "national_id": KindText,
		"sgk_no": KindText, "job_title": KindText, "department": KindText, "period": KindText,
		"hire_date": KindDate, "exit_date": KindDate, "days_worked": KindInt,
	}
	fields := make([]kf, 0, len(PayrollFields))
	for _, f := range PayrollFields {
		k, ok := kinds[f.Key]
		if !ok {
			k = KindMoney
		}
		fields = append(fields, kf{f.Key, f.Description, k})
	}
	return catalog(Payroll, "Bordro", "net",
		[][]string{{"employee_name", "first_name"}, {"gross"}, {"net"}}, fields...)
}()

var invoiceKinds = map[string]string{
	"issue_date": KindDate, "due_date": KindDate,
	"amounts.tax_exclusive_amount": KindMoney, "amounts.tax_amount": KindMoney,
	"amounts.withholding_amount": KindMoney, "amounts.payable_amount": KindMoney,
}

func invoiceCatalog(dt, label string) Catalog {
	c := Catalog{DataType: dt, Label: label, Required: invoiceRequired, TotalField: "amounts.payable_amount", kinds: invoiceKinds}
	for _, f := range InvoiceFields {
		f.Kind = c.Kind(f.Key)
		c.Fields = append(c.Fields, f)
	}
	return c
}

var catalogs = map[string]Catalog{
	OutgoingInvoices: invoiceCatalog(OutgoingInvoices, "Satış faturaları"),
	IncomingInvoices: invoiceCatalog(IncomingInvoices, "Alış faturaları"),
	Payroll:          payrollCatalog,
	Expenses:         expenseCatalog,
	RetailSales:      retailCatalog,
	BankTransactions: bankCatalog,
}

// CatalogFor returns the catalog of a supported data type.
func CatalogFor(dt string) (Catalog, bool) {
	c, ok := catalogs[dt]
	return c, ok
}

// Catalogs returns every catalog in SupportedDataTypes order.
func Catalogs() []Catalog {
	out := make([]Catalog, 0, len(SupportedDataTypes))
	for _, dt := range SupportedDataTypes {
		out = append(out, catalogs[dt])
	}
	return out
}

// IsInvoice reports whether dt is committed by the invoice import itself.
func IsInvoice(dt string) bool { return dt == OutgoingInvoices || dt == IncomingInvoices }
