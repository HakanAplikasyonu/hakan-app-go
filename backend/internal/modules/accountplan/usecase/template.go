package usecase

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// TemplateAccount is one account to open in the target program.
type TemplateAccount struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// MissingTemplate builds an .xlsx listing accounts to open in Luca (Luca
// destek 01.01.04: hesap planı Excel şablonundan XML'e çevrilip içe alınır).
// The real column layout of Luca's template is not known yet (D18), so the
// sheet uses the two field names of the official text and says it is
// unverified.
func MissingTemplate(accounts []TemplateAccount, now time.Time) ([]byte, error) {
	if len(accounts) == 0 || len(accounts) > MaxCheckCodes {
		return nil, fmt.Errorf("%w: 1..%d accounts", ErrInvalid, MaxCheckCodes)
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Hesap Planı"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	if err := f.SetSheetRow(sheet, "A1", &[]any{"Hesap Kodu", "Hesap Adı"}); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	row := 2
	for _, a := range accounts {
		code := NormalizeCode(a.Code)
		if !ValidCode(code) {
			return nil, fmt.Errorf("%w: %q is not an account code", ErrInvalid, a.Code)
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		// Kod metin olarak yazılır: "120.10" sayıya dönüp "120.1" olmasın.
		if err := f.SetCellStr(sheet, fmt.Sprintf("A%d", row), code); err != nil {
			return nil, err
		}
		if err := f.SetCellStr(sheet, fmt.Sprintf("B%d", row), strings.TrimSpace(a.Name)); err != nil {
			return nil, err
		}
		row++
	}
	_ = f.SetColWidth(sheet, "A", "A", 18)
	_ = f.SetColWidth(sheet, "B", "B", 48)
	const info = "Bilgi"
	if _, err := f.NewSheet(info); err != nil {
		return nil, err
	}
	notes := [][]any{
		{"Şablon", "luca hesap planı (doğrulanmamış)"},
		{"Not", "Sütun başlıkları ve sırası Luca'nın indirilen şablonuyla henüz doğrulanmadı (docs/02 §1.2, D18)."},
		{"Üretim", now.UTC().Format(time.RFC3339)},
	}
	for i, n := range notes {
		if err := f.SetSheetRow(info, fmt.Sprintf("A%d", i+1), &n); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
