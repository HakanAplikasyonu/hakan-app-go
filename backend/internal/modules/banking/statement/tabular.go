package statement

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Excel / CSV statements (23 §4). The 04 Excel engine with AI column mapping
// (#22) is not in the repository yet; until then columns are found by the
// header names banks use in their "Excel'e aktar" output. Per-bank templates
// are added to bankHeaders once real files are collected (23 §6).

type column int

const (
	colDate column = iota
	colValueDate
	colDescription
	colAmount
	colDebit
	colCredit
	colBalance
	colReceipt
	colReference
	colChannel
	colCounterparty
	colTime
	colCounterpartyIBAN
	colCounterpartyTaxID
)

// headerNames are matched on the normalized header text (banking.NormalizeDescription).
var headerNames = map[column][]string{
	colDate:         {"TARIH", "ISLEM TARIHI", "TARIH SAAT", "ISLEM TARIHI SAATI", "TRANSACTION DATE", "DATE"},
	colValueDate:    {"VALOR", "VALOR TARIHI", "VALUE DATE"},
	colDescription:  {"ACIKLAMA", "ISLEM ACIKLAMASI", "ACIKLAMALAR", "ISLEM", "DESCRIPTION"},
	colAmount:       {"TUTAR", "ISLEM TUTARI", "TUTAR TL", "AMOUNT"},
	colDebit:        {"BORC", "BORC TUTARI", "CIKAN", "CIKAN TUTAR", "DEBIT"},
	colCredit:       {"ALACAK", "ALACAK TUTARI", "GIREN", "GIREN TUTAR", "CREDIT"},
	colBalance:      {"BAKIYE", "BAKIYE TL", "ISLEM SONRASI BAKIYE", "BALANCE"},
	colReceipt:      {"DEKONT NO", "DEKONT", "FIS NO", "RECEIPT NO"},
	colReference:    {"REFERANS", "REFERANS NO", "REF NO", "REFERENCE"},
	colChannel:      {"KANAL", "ISLEM KANALI", "CHANNEL"},
	colCounterparty: {"KARSI TARAF", "KARSI TARAF ADI", "GONDEREN ALICI", "ALICI GONDEREN", "COUNTERPARTY"},
	colTime:         {"SAAT", "ISLEM SAATI", "TIME"},
	// Issue #383: columns some banks add.
	colCounterpartyIBAN:  {"KARSI TARAF IBAN", "KARSI IBAN", "GONDEREN IBAN", "ALICI IBAN", "COUNTERPARTY IBAN"},
	colCounterpartyTaxID: {"KARSI TARAF VKN", "GONDEREN ALICI VKN", "VKN TCKN", "KARSI TARAF VKN TCKN"},
}

var reIBANLoose = regexp.MustCompile(`TR\d{2}(?:\s?\d{4}){5}\s?\d{2}`)

// ReadXLSX returns the first sheet's cells as text.
func ReadXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("excel: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("excel: no sheet")
	}
	return f.GetRows(sheets[0], excelize.Options{RawCellValue: false})
}

// ReadCSV reads semicolon, comma or tab separated text.
func ReadCSV(text string) ([][]string, error) {
	// The separator is the one used most on a line of the first 20: banks
	// put a title line without separators above the table (#383).
	sep, best := ',', 0
	for i, line := range strings.SplitN(text, "\n", 21) {
		if i == 20 {
			break
		}
		for _, c := range []rune{',', ';', '\t'} {
			if n := strings.Count(line, string(c)); n > best {
				sep, best = c, n
			}
		}
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.ReadAll()
}

// ParseTabular finds the account and the header row, maps the columns and
// reads the movements. format is "excel" or "csv".
func ParseTabular(rows [][]string, format string) (Statement, error) {
	return ParseTabularWith(rows, format, nil)
}

// ParseTabularWith is ParseTabular with the office's saved templates; they
// are tried before the built-in ones and the header synonyms (#383).
func ParseTabularWith(rows [][]string, format string, tpls []Template) (Statement, error) {
	st := Statement{Format: format}
	hdr, cols, match := findHeader(rows, format, tpls)
	// Account: an IBAN in the header area (23 §4): the first 10 rows, or
	// every row above the movement table when the title block is longer.
	for i := 0; i < len(rows) && (i < 10 || i < hdr) && st.AccountIBAN == ""; i++ {
		if i == hdr {
			continue
		}
		for _, cell := range rows[i] {
			if m := reIBANLoose.FindString(strings.ToUpper(cell)); m != "" {
				if iban := banking.NormalizeIBAN(m); iban != "" {
					st.AccountIBAN = iban
					break
				}
			}
		}
	}
	if hdr < 0 {
		return st, fmt.Errorf("%w (%s: header row with date, description and amount columns not found)", ErrLayoutUnknown, format)
	}
	st.Template = match
	switch {
	case match != nil && match.BankID != "":
		st.BankID = match.BankID
	case st.AccountIBAN != "":
		st.BankID = BankFromIBAN(st.AccountIBAN)
	}
	if st.BankID == "" {
		st.BankID = bankFromText(rows, hdr)
	}
	for r := hdr + 1; r < len(rows); r++ {
		row := rows[r]
		if blank(row) {
			continue
		}
		l, err := readRow(row, cols)
		if err != nil {
			if isFooter(row) {
				break
			}
			st.Errors = append(st.Errors, RowError{Row: r + 1, Reason: err.Error()})
			continue
		}
		l.Raw = map[string]any{"row": r + 1, "cells": row}
		st.Lines = append(st.Lines, l)
	}
	if len(st.Lines) == 0 {
		return st, fmt.Errorf("%s: no movements read", format)
	}
	orderByBalance(&st)
	finishTabular(&st)
	return st, nil
}

func findHeader(rows [][]string, format string, tpls []Template) (int, map[column]int, *TemplateMatch) {
	for i := 0; i < len(rows) && i < 40; i++ {
		if cols, m := matchTemplate(rows[i], format, tpls); m != nil {
			return i, cols, m
		}
	}
	for i := 0; i < len(rows) && i < 40; i++ {
		cols := synonymColumns(rows[i])
		_, hasDate := cols[colDate]
		_, hasDesc := cols[colDescription]
		_, hasAmt := cols[colAmount]
		_, hasDebit := cols[colDebit]
		_, hasCredit := cols[colCredit]
		if hasDate && hasDesc && (hasAmt || (hasDebit && hasCredit)) {
			return i, cols, &TemplateMatch{Source: SourceSynonyms, Fingerprint: Fingerprint(rows[i])}
		}
	}
	return -1, nil, nil
}

// synonymColumns maps a row's cells by the header names banks use.
func synonymColumns(row []string) map[column]int {
	cols := map[column]int{}
	for j, cell := range row {
		n := banking.NormalizeDescription(cell)
		if n == "" {
			continue
		}
		for c, names := range headerNames {
			if _, done := cols[c]; done {
				continue
			}
			for _, name := range names {
				if n == name {
					cols[c] = j
					break
				}
			}
		}
	}
	return cols
}

func cell(row []string, cols map[column]int, c column) string {
	j, ok := cols[c]
	if !ok || j >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[j])
}

func readRow(row []string, cols map[column]int) (Line, error) {
	var l Line
	date, hasTime, err := parseDate(cell(row, cols, colDate))
	if err != nil {
		return l, err
	}
	// An Excel date cell carries a 00:00 clock; the time column wins then.
	midnight := date.Hour() == 0 && date.Minute() == 0 && date.Second() == 0
	if t := cell(row, cols, colTime); t != "" && (!hasTime || midnight) {
		if h, m, ok := parseClock(t); ok {
			date = time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, time.UTC)
			hasTime = true
		}
	}
	l.TransactionDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if hasTime {
		l.BookedAt = date
		l.HasTime = true
	}
	if v := cell(row, cols, colValueDate); v != "" {
		if vd, _, err := parseDate(v); err == nil {
			vd = time.Date(vd.Year(), vd.Month(), vd.Day(), 0, 0, 0, 0, time.UTC)
			l.ValueDate = &vd
		}
	}
	// Direction: the filled one of separate debit / credit columns, else the
	// sign of the single amount column (23 §4).
	debit, credit := cell(row, cols, colDebit), cell(row, cols, colCredit)
	var amt decimal.Decimal
	switch {
	case nonZero(debit):
		amt, err = parseAmount(debit)
		l.Direction = banking.DirOut
	case nonZero(credit):
		amt, err = parseAmount(credit)
		l.Direction = banking.DirIn
	default:
		amt, err = parseAmount(cell(row, cols, colAmount))
		l.Direction = banking.DirIn
		if amt.IsNegative() {
			l.Direction = banking.DirOut
		}
	}
	if err != nil {
		return l, err
	}
	l.Amount = amt.Abs().String()
	if b := cell(row, cols, colBalance); b != "" {
		bal, err := parseAmount(b)
		if err != nil {
			return l, fmt.Errorf("bakiye: %w", err)
		}
		s := bal.String()
		l.BalanceAfter = &s
	}
	l.Description = cell(row, cols, colDescription)
	l.ReceiptNumber = cell(row, cols, colReceipt)
	l.BankReference = cell(row, cols, colReference)
	l.Channel = cell(row, cols, colChannel)
	l.CounterpartyName = cell(row, cols, colCounterparty)
	if l.CounterpartyName == "" {
		// #780: like MT940 :86:, "GONDEREN: X" / "ALICI: X" in the
		// description names the counterparty; the invoice matcher scores
		// it (without it an Excel movement is never "related").
		l.CounterpartyName = partyFromDescription(l.Description)
	}
	compact := strings.ReplaceAll(strings.ToUpper(l.Description), " ", "")
	l.CounterpartyIBAN = findIBAN(compact)
	if v := banking.NormalizeIBAN(cell(row, cols, colCounterpartyIBAN)); v != "" {
		l.CounterpartyIBAN = v
	}
	if m := reTaxID.FindStringSubmatch(l.Description); m != nil {
		l.CounterpartyTaxID = m[1]
	}
	if vkn, tckn := banking.SplitTaxID(cell(row, cols, colCounterpartyTaxID)); vkn+tckn != "" {
		l.CounterpartyTaxID = vkn + tckn
	}
	return l, nil
}

// reNameTail: words after the name in a description ("... A.S. FATURA
// 2026-118", "... ACIKLAMA: KIRA").
var reNameTail = regexp.MustCompile(`(?i)\s+(?:FATURA|FAT\.?|FTR|AC[Iİ]KLAMA|A[ÇC]IKLAMA|REF(?:ERANS)?)\b.*$`)

// partyFromDescription: the sender / receiver named in a description, ""
// when none is written.
func partyFromDescription(desc string) string {
	m := reParty.FindStringSubmatch(desc)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(reNameTail.ReplaceAllString(strings.TrimSpace(m[1]), ""))
}

var dateLayouts = []string{
	"02.01.2006 15:04:05", "02.01.2006 15:04", "02.01.2006",
	"02/01/2006 15:04:05", "02/01/2006 15:04", "02/01/2006",
	"2006-01-02 15:04:05", "2006-01-02T15:04:05.000", "2006-01-02T15:04:05", "2006-01-02",
	"02-01-2006", "02.01.06", "01-02-06",
}

func parseDate(s string) (time.Time, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, fmt.Errorf("tarih boş")
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, strings.Contains(layout, "15:04"), nil
		}
	}
	// Excel serial date (an .xlsx raw value or a binary .xls number). The
	// 1900 system counts a 29 February 1900 that never was, so from serial
	// 61 on day N is 1899-12-30 + N; bank dates (> 20000, after 1954) are
	// all past it. A fraction is the time of day, rounded to the second
	// (46296.375 is 09:00, not 08:59:59.999).
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 20000 && f < 80000 {
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		return base.Add(time.Duration(math.Round(f*86400)) * time.Second), f != math.Trunc(f), nil
	}
	return time.Time{}, false, fmt.Errorf("tarih okunamadı: %q", s)
}

// parseClock reads a time cell: "09:12" (text) or an Excel time, the
// fraction of a day a binary .xls gives ("0.3833333333333333" is 09:12).
func parseClock(s string) (hour, minute int, ok bool) {
	if tt, err := time.Parse("15:04", s[:min(5, len(s))]); err == nil {
		return tt.Hour(), tt.Minute(), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f >= 1 {
		return 0, 0, false
	}
	mins := int(math.Round(f*86400)) / 60
	if mins >= 24*60 {
		return 0, 0, false
	}
	return mins / 60, mins % 60, true
}

var reThousandsDot = regexp.MustCompile(`^-?\d{1,3}(\.\d{3})+$`)

// reDebitCreditMark: "B" / "BORÇ" (money out) or "A" / "ALACAK" (money
// in) after the number, as some statements print the direction in the
// amount cell ("1.234,56 B", "1.234,56 (A)", "1.234,56 (BORÇ)"; the PDF
// reader keeps these runs, reAmountRun). A currency code has three letters
// and is not taken for it. Matched on the upper-cased cell.
var reDebitCreditMark = regexp.MustCompile(`^(.*\d)\s*(?:TL|TRY)?\s*\(?\s*(A|B|ALACAK|BORÇ|BORC)\s*\)?$`)

// minusSigns: the minus a PDF or a word processor writes instead of "-".
var minusSigns = strings.NewReplacer("\u2212", "-", "\u2013", "-", "\u2012", "-", "\u2010", "-")

// parseAmount reads "1.234,56", "-1.234,56", "1,234.56", "1234.56", "1234,5",
// "(1.234,56)", "1.234,56 B" / "1.234,56 A" (borç / alacak) and a trailing
// currency. The decimal separator is the last one of "," and "." when both
// appear; a lone "." followed by groups of three digits is a thousands
// separator.
func parseAmount(s string) (decimal.Decimal, error) {
	v := minusSigns.Replace(strings.TrimSpace(s))
	neg := false
	if m := reDebitCreditMark.FindStringSubmatch(strings.ToUpper(v)); m != nil {
		v = strings.TrimSpace(m[1])
		if m[2] != "A" && m[2] != "ALACAK" {
			if strings.HasPrefix(v, "-") || (strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")")) {
				return decimal.Decimal{}, fmt.Errorf("tutar okunamadı: %q", s)
			}
			neg = true
		}
	}
	if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") {
		neg, v = true, strings.Trim(v, "()")
	}
	v = strings.TrimSpace(strings.TrimRight(strings.ToUpper(v), "ABCDEFGHIJKLMNOPQRSTUVWXYZ₺$€ "))
	v = strings.ReplaceAll(v, " ", "")
	if strings.HasSuffix(v, "-") {
		neg, v = true, strings.TrimSuffix(v, "-")
	}
	v = strings.TrimPrefix(v, "+")
	if v == "" {
		return decimal.Decimal{}, fmt.Errorf("tutar boş")
	}
	dot, comma := strings.LastIndex(v, "."), strings.LastIndex(v, ",")
	switch {
	case dot >= 0 && comma >= 0 && comma > dot:
		v = strings.ReplaceAll(v, ".", "")
		v = strings.Replace(v, ",", ".", 1)
	case dot >= 0 && comma >= 0:
		v = strings.ReplaceAll(v, ",", "")
	case comma >= 0:
		v = strings.Replace(strings.ReplaceAll(v, ".", ""), ",", ".", 1)
	case reThousandsDot.MatchString(v):
		v = strings.ReplaceAll(v, ".", "")
	}
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("tutar okunamadı: %q", s)
	}
	if neg {
		d = d.Neg()
	}
	return d, nil
}

func nonZero(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	d, err := parseAmount(s)
	return err == nil && !d.IsZero()
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// isFooter: totals and closing lines after the table end the reading.
func isFooter(row []string) bool {
	for _, c := range row {
		n := banking.NormalizeDescription(c)
		if strings.HasPrefix(n, "TOPLAM") || strings.HasPrefix(n, "DONEM SONU") || strings.HasPrefix(n, "KAPANIS") {
			return true
		}
	}
	return false
}

// orderByBalance: banks export newest first or oldest first. With a balance
// column the order whose running balance adds up wins (23 §4, §7 test 9).
func orderByBalance(st *Statement) {
	if len(st.Lines) < 2 {
		return
	}
	if st.Lines[0].BalanceAfter == nil {
		// No balance to tell: a file whose first date is after its last is
		// newest first (otherwise the report's from/to and the bank
		// sequence came out backwards).
		if st.Lines[0].TransactionDate.After(st.Lines[len(st.Lines)-1].TransactionDate) {
			reverseLines(st)
		}
		return
	}
	if CheckRunning(st.Lines) == nil {
		return
	}
	rev := reversed(st.Lines)
	if CheckRunning(rev) == nil {
		st.Lines = rev
		return
	}
	// Neither order adds up: try newest-first by date as the tie-breaker.
	if st.Lines[0].TransactionDate.After(st.Lines[len(st.Lines)-1].TransactionDate) {
		st.Lines = rev
	}
}

func reversed(lines []Line) []Line {
	rev := make([]Line, len(lines))
	for i, l := range lines {
		rev[len(lines)-1-i] = l
	}
	return rev
}

func reverseLines(st *Statement) { st.Lines = reversed(st.Lines) }

func finishTabular(st *Statement) {
	seen := map[string]int{}
	for i := range st.Lines {
		l := &st.Lines[i]
		seq := int32(i + 1)
		l.BankSequence = &seq
		l.CaptureMethod = banking.CaptureExcel
		// Rows have no bank key: hash the row (20 §7.3) plus how many times
		// the same row already appeared, so two identical payments on one
		// day stay two movements while a re-upload keeps the same keys.
		bal := ""
		if l.BalanceAfter != nil {
			bal = *l.BalanceAfter
		}
		key := strings.Join([]string{st.AccountIBAN, l.TransactionDate.Format("2006-01-02"), l.BookedAt.Format("15:04"),
			string(l.Direction), l.Amount, bal, banking.NormalizeDescription(l.Description)}, "|")
		seen[key]++
		sum := sha256.Sum256([]byte(key + "|" + strconv.Itoa(seen[key])))
		l.SourceID = "row:" + hex.EncodeToString(sum[:])
	}
}
