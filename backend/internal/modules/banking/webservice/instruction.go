package webservice

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// InstructionData fills the branch instruction (20 §5.2).
type InstructionData struct {
	BankName    string
	Title       string // şirket unvanı
	TaxID       string
	TaxOffice   string
	IBANs       []string // "TR.. / TRY"
	EgressIPs   []string
	TechContact string
	Date        time.Time
}

// ErrNoEgressIPs: the instruction cannot be printed without the fixed IPs.
var ErrNoEgressIPs = fmt.Errorf("webservice: BANK_EGRESS_IPS is not configured")

// InstructionPDF renders the generic instruction letter of 20 §5.2, already
// filled in. The taxpayer signs, stamps and hands it to the branch. Banks
// with their own form get that form later (22 "Başvuru" rows).
func InstructionPDF(d InstructionData) ([]byte, error) {
	if len(d.EgressIPs) == 0 {
		return nil, ErrNoEgressIPs
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.SetMargins(22, 22, 22)
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetTitle("Hesap hareketleri web servisi talimatı", true)
	pdf.AddPage()
	font := func(style string, size float64) { pdf.SetFont(ioengine.PDFFontFamily, style, size) }

	font("", 10)
	pdf.CellFormat(0, 6, d.Date.Format("02.01.2006"), "", 1, "R", false, 0, "")
	pdf.Ln(4)
	font("B", 12)
	pdf.CellFormat(0, 7, "T.C. "+strings.ToUpper(trUpper(d.BankName))+" A.Ş.", "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 7, ".................................................. ŞUBESİ", "", 1, "L", false, 0, "")
	pdf.Ln(6)

	font("", 11)
	pdf.MultiCell(0, 6, "Bankanız nezdindeki aşağıda IBAN'ları yazılı hesaplarımızın bakiye ve hareket bilgilerinin, "+
		"online hesap hareketleri / elektronik hesap özeti web servisi üzerinden, aşağıda belirtilen sabit IP "+
		"adreslerinden sorgulanabilmesi için gerekli tanımların yapılmasını ve servis kullanıcı bilgilerinin "+
		"tarafımıza iletilmesini talep ederiz. Bu servis yalnızca bilgi sorgulama amaçlıdır; para transferi "+
		"yetkisi talep edilmemektedir.", "", "J", false)
	pdf.Ln(4)

	row := func(label, value string) {
		font("B", 10)
		pdf.CellFormat(55, 7, label, "", 0, "L", false, 0, "")
		font("", 10)
		pdf.MultiCell(0, 7, value, "", "L", false)
	}
	row("Şirket unvanı", d.Title)
	row("VKN / TCKN", d.TaxID)
	row("Vergi dairesi", orDots(d.TaxOffice))
	ibans := strings.Join(d.IBANs, "\n")
	if ibans == "" {
		ibans = "TR........................................  /  ......\nTR........................................  /  ......"
	}
	row("Hesaplar (IBAN / döviz)", ibans)
	row("Sabit IP adresleri", strings.Join(d.EgressIPs, ", "))
	row("Teknik yetkili", orDots(d.TechContact))
	row("Talep edilen hizmet", "Online hesap hareketleri / elektronik hesap özeti web servisi")
	row("Periyot", "Günlük / gün içi")
	pdf.Ln(14)

	font("", 10)
	pdf.CellFormat(0, 6, "Kaşe ve yetkili imza", "", 1, "R", false, 0, "")
	pdf.Ln(16)
	pdf.CellFormat(0, 6, "Ad soyad: ..............................................", "", 1, "R", false, 0, "")

	pdf.SetY(-34)
	font("I", 8)
	pdf.MultiCell(0, 4, "Bu servis yalnızca okuma yetkisi verir; hesaplarınızdan para çıkışı yapılamaz. Servisin ücreti "+
		"bankanız ile aranızdadır. Talimatı iptal etmek için şubenize başvurmanız yeterlidir.", "", "L", false)

	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func orDots(s string) string {
	if strings.TrimSpace(s) == "" {
		return ".................................................."
	}
	return s
}

// trUpper upper-cases with Turkish i/ı rules.
func trUpper(s string) string {
	return strings.ToUpperSpecial(trCase, s)
}
