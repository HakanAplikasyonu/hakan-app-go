// Package sheet is the Excel channel of expenses and Z reports (issue #384):
// the office's list as .xlsx, and a fixed-column template read back from
// .xlsx / .csv. The export is also the template: a file exported, edited
// and uploaded again imports only its new rows (each row's content is its
// key).
package sheet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// MaxRows bounds one import.
const MaxRows = 5000

// ErrNoHeader: no row with the template's required headers.
var ErrNoHeader = errors.New("sheet: header row not found (Tarih and Toplam columns are required)")

// Column is one template column: Header is written, Aliases are accepted.
type Column struct {
	Key     string
	Header  string
	Aliases []string
	Width   float64
	Money   bool
	// Identity: part of the row key (import idempotency). Only source data
	// that every file carries the same way; the office's choices (category,
	// account) and derived values are left out.
	Identity bool
}

// Expense columns.
var ExpenseColumns = []Column{
	{Key: "date", Header: "Tarih", Aliases: []string{"fiş tarihi", "belge tarihi"}, Width: 12, Identity: true},
	{Key: "vendor_name", Header: "Satıcı", Aliases: []string{"satıcı unvanı", "firma", "unvan"}, Width: 30, Identity: true},
	{Key: "vendor_tax_number", Header: "VKN / TCKN", Aliases: []string{"vkn", "tckn", "vergi no"}, Width: 14, Identity: true},
	{Key: "receipt_number", Header: "Fiş No", Aliases: []string{"belge no", "fiş numarası"}, Width: 12, Identity: true},
	{Key: "description", Header: "Açıklama", Width: 30},
	{Key: "category", Header: "Kategori", Width: 22},
	{Key: "account_code", Header: "Hesap Kodu", Aliases: []string{"hesap"}, Width: 14},
	{Key: "tax_rate", Header: "KDV Oranı", Aliases: []string{"kdv %", "oran"}, Width: 10},
	{Key: "tax_amount", Header: "KDV", Aliases: []string{"kdv tutarı"}, Width: 14, Money: true},
	{Key: "total", Header: "Toplam", Aliases: []string{"tutar", "kdv dahil toplam", "genel toplam"}, Width: 14, Money: true, Identity: true},
	{Key: "payment_method", Header: "Ödeme", Aliases: []string{"ödeme şekli", "ödeme yöntemi"}, Width: 12},
	{Key: "vehicle_plate", Header: "Plaka", Width: 12},
	{Key: "status", Header: "Durum", Width: 12},
	{Key: "journal", Header: "Yevmiye No", Width: 12},
}

// ZRates are the template's VAT rate columns.
var ZRates = []string{"0", "1", "10", "20"}

// ZColumns are the Z report columns.
var ZColumns = func() []Column {
	cols := []Column{
		{Key: "date", Header: "Tarih", Aliases: []string{"z tarihi"}, Width: 12, Identity: true},
		{Key: "okc", Header: "ÖKC Seri No", Aliases: []string{"ökc", "okc", "cihaz seri no", "seri no"}, Width: 16, Identity: true},
		{Key: "z_number", Header: "Z No", Aliases: []string{"z numarası", "z rapor no"}, Width: 10, Identity: true},
		{Key: "receipt_count", Header: "Fiş Sayısı", Width: 10},
		{Key: "cash", Header: "Nakit", Width: 14, Money: true},
		{Key: "card", Header: "Kart", Aliases: []string{"kredi kartı", "pos"}, Width: 14, Money: true},
		{Key: "other", Header: "Diğer Ödeme", Width: 14, Money: true},
	}
	for _, r := range ZRates {
		cols = append(cols,
			Column{Key: "base_" + r, Header: "%" + r + " Matrah", Width: 14, Money: true},
			Column{Key: "vat_" + r, Header: "%" + r + " KDV", Width: 12, Money: true})
	}
	return append(cols,
		Column{Key: "tax_amount", Header: "Toplam KDV", Width: 14, Money: true},
		Column{Key: "total", Header: "Toplam", Aliases: []string{"z toplamı", "genel toplam"}, Width: 14, Money: true, Identity: true},
		Column{Key: "status", Header: "Durum", Width: 12},
		Column{Key: "warnings", Header: "Uyarılar", Width: 30},
		Column{Key: "journal", Header: "Yevmiye No", Width: 12},
	)
}()

// Row is one data row keyed by Column.Key (cell text as written).
type Row map[string]string

// Write renders rows as one sheet with a bold, frozen header.
func Write(sheetName string, cols []Column, rows []Row) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName(f.GetSheetName(0), sheetName); err != nil {
		return nil, err
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"EEF2F6"}, Pattern: 1}})
	money, _ := f.NewStyle(&excelize.Style{NumFmt: 4}) // #,##0.00
	for i, c := range cols {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, c.Header)
		_ = f.SetCellStyle(sheetName, cell, cell, bold)
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheetName, col, col, c.Width)
	}
	for ri, r := range rows {
		for ci, c := range cols {
			v, ok := r[c.Key]
			if !ok || v == "" {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(ci+1, ri+2)
			if c.Money {
				if d, err := decimal.NewFromString(v); err == nil {
					f64, _ := d.Float64()
					_ = f.SetCellValue(sheetName, cell, f64)
					_ = f.SetCellStyle(sheetName, cell, cell, money)
					continue
				}
			}
			_ = f.SetCellStr(sheetName, cell, v)
		}
	}
	_ = f.SetPanes(sheetName, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Parsed is one data row read from a file.
type Parsed struct {
	Line int // 1-based row in the sheet
	Row  Row
	// Key identifies the row's content (import idempotency).
	Key string
}

// HeaderKey returns the column key whose header or alias is h.
func HeaderKey(cols []Column, h string) (string, bool) {
	n := parse.Normalize(h)
	if n == "" {
		return "", false
	}
	for _, c := range cols {
		if parse.Normalize(c.Header) == n {
			return c.Key, true
		}
		for _, a := range c.Aliases {
			if parse.Normalize(a) == n {
				return c.Key, true
			}
		}
	}
	return "", false
}

// Read finds the header row (the first with Tarih and Toplam) in the first
// sheet that has one and returns the data rows below it. Empty rows and
// "Toplam" summary rows are skipped.
func Read(name string, data []byte, cols []Column) ([]Parsed, error) {
	_, sheets, err := parse.Read(name, data)
	if err != nil {
		return nil, err
	}
	want := map[string]string{}
	for _, c := range cols {
		want[parse.Normalize(c.Header)] = c.Key
		for _, a := range c.Aliases {
			want[parse.Normalize(a)] = c.Key
		}
	}
	for _, sh := range sheets {
		for hi, hdr := range sh.Rows {
			idx := map[int]string{}
			found := map[string]bool{}
			for j, h := range hdr {
				if k, ok := want[parse.Normalize(h)]; ok && !found[k] {
					idx[j], found[k] = k, true
				}
			}
			if !found["date"] || !found["total"] {
				continue
			}
			var out []Parsed
			for ri := hi + 1; ri < len(sh.Rows); ri++ {
				raw := sh.Rows[ri]
				if parse.IsTotalRow(raw) {
					continue
				}
				row := Row{}
				for j, k := range idx {
					if j < len(raw) {
						if v := strings.TrimSpace(raw[j]); v != "" {
							row[k] = v
						}
					}
				}
				if len(row) == 0 {
					continue
				}
				if len(out) >= MaxRows {
					return nil, fmt.Errorf("sheet: more than %d rows", MaxRows)
				}
				out = append(out, Parsed{Line: ri + 1, Row: row, Key: key(row, cols)})
			}
			return out, nil
		}
	}
	return nil, ErrNoHeader
}

// key hashes the row's identity columns in their parsed form, so the same
// receipt typed "1.200" in a CSV and written 1200 by the export is one row.
func key(r Row, cols []Column) string {
	h := sha256.New()
	for _, c := range cols {
		if !c.Identity {
			continue
		}
		v := strings.TrimSpace(r[c.Key])
		switch {
		case c.Key == "date":
			if d, ok := Date(v); ok {
				v = d.Format(time.DateOnly)
			}
		case c.Money:
			if d, ok := Amount(v); ok {
				v = d.StringFixed(2)
			}
		default:
			v = parse.Normalize(v)
		}
		_, _ = fmt.Fprintf(h, "%s=%s\x1f", c.Key, v)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Date reads a date cell.
func Date(s string) (time.Time, bool) { return parse.ParseDate(s) }

// Amount reads a money cell (TR or EN style, raw xlsx numbers).
func Amount(s string) (decimal.Decimal, bool) {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero, false
	}
	style := parse.DetectNumberStyle([]string{s})
	if style == parse.StyleAuto && trThousands.MatchString(strings.TrimSpace(s)) {
		// "1.200" in a Turkish sheet is one thousand two hundred.
		style = parse.StyleTR
	}
	d, neg, ok := parse.ParseAmount(s, style)
	return d, ok && !neg
}

var trThousands = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{3})+$`)

// Rate reads a VAT rate ("%20", "20", "0,20").
func Rate(s string) (decimal.Decimal, bool) { return parse.Rate(s) }

// TaxID reads a VKN / TCKN.
func TaxID(s string) (string, bool) {
	vkn, tckn, ok := parse.TaxID(s)
	if !ok {
		return "", false
	}
	if vkn != "" {
		return vkn, true
	}
	return tckn, true
}

// PaymentMethod maps an Ödeme cell to expenses.payment_method.
func PaymentMethod(s string) (string, bool) {
	n := parse.Normalize(s)
	switch {
	case n == "":
		return "", true
	case strings.Contains(n, "kart") || strings.Contains(n, "card"):
		return "credit_card", true
	case strings.Contains(n, "nakit") || n == "cash" || n == "kasa":
		return "cash", true
	case strings.Contains(n, "havale") || strings.Contains(n, "eft") || strings.Contains(n, "banka"):
		return "bank_transfer", true
	case strings.Contains(n, "cek"):
		return "check", true
	case strings.Contains(n, "senet"):
		return "promissory_note", true
	case strings.Contains(n, "veresiye") || strings.Contains(n, "vadeli") || strings.Contains(n, "acik hesap"):
		return "credit", true
	}
	return "", false
}

// PaymentLabel is the Ödeme cell of a payment method.
func PaymentLabel(m string) string {
	return map[string]string{"cash": "Nakit", "credit_card": "Kredi kartı", "bank_transfer": "Havale / EFT",
		"check": "Çek", "promissory_note": "Senet", "credit": "Veresiye"}[m]
}

// StatusLabel is the Durum cell of an approval status.
func StatusLabel(s string) string {
	return map[string]string{"pending": "Bekliyor", "approved": "Onaylı", "rejected": "Reddedildi"}[s]
}
