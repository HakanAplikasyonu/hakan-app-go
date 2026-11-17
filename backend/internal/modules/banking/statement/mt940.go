// Package statement reads bank statement files (MT940/MT942, Excel/CSV, text
// PDF) into banking.Transaction rows (issue #19,
// docs/kaynak/musavir-entegrasyon-paketi/23-banka-dosya-ice-aktarma.md).
package statement

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Balance is a signed statement balance (negative: debit / overdraft).
type Balance struct {
	Date     time.Time
	Currency string
	Amount   decimal.Decimal
}

// Statement is one account's block of a file (one :20: block in MT940, one
// sheet in Excel). A file may hold several (23 §3.3).
type Statement struct {
	Format string // mt940 | mt942 | excel | csv | pdf
	// AccountIBAN is the account found in the file (23 §2 step 2); empty when
	// the file names only a bank account number (AccountRef).
	AccountIBAN string
	AccountRef  string
	Reference   string
	Currency    string
	Opening     *Balance
	Closing     *Balance
	Available   *Balance
	Lines       []Line
	// Errors are rows that could not be read; they are reported, not stored.
	Errors []RowError
	// BankID is the bank the file belongs to (template, IBAN or bank name in
	// the header area; #383). Empty when unknown.
	BankID string
	// Template says how the columns of a table statement were found.
	Template *TemplateMatch
}

// Line is one movement plus what the parser learned about it.
type Line struct {
	banking.Transaction
	// Reversal: RC / RD storno line (23 §3.2).
	Reversal bool
}

// RowError is a row the parser skipped.
type RowError struct {
	Row    int
	Reason string
}

var (
	reTag     = regexp.MustCompile(`^:(\d{2}[A-Z]?):`)
	reBalance = regexp.MustCompile(`^([CD])(\d{6})([A-Z]{3})(\d+,\d*)$`)
	// :61: valör YYMMDD, kayıt MMDD?, (R)C/D, currency letter?, amount, type+code, refs
	re61       = regexp.MustCompile(`^(\d{6})(\d{4})?(RC|RD|C|D)([A-Z])?(\d+,\d*)([NFS])([A-Z0-9]{3})(.*)$`)
	reIBAN     = regexp.MustCompile(`TR\d{24}`)
	reTaxID    = regexp.MustCompile(`(?i)\b(?:VKN|TCKN|T\.?C\.?\s*K(?:IMLIK|İMLİK)?\s*NO|VERG[Iİ]\s*NO)\s*[:.]?\s*(\d{10,11})\b`)
	reParty    = regexp.MustCompile(`(?i)\b(?:G[OÖ]NDEREN|G[OÖ]ND\.?|ALICI(?:\s*ADI)?)\s*[:.]\s*([^/;|:]+?)\s*(?:$|/|;|\||\s{2,}|\b(?:VKN|TCKN|IBAN|AC[Iİ]KLAMA|SORGU)\b)`)
	reEFTQuery = regexp.MustCompile(`(?i)\bSORGU\s*NO\s*[:.]?\s*(\d+)\b`)
	reSwiftHdr = regexp.MustCompile(`\{[1-3]:[^}]*\}|\{4:|-\}|\{5:.*?\}`)
)

// ParseMT940 reads MT940 and MT942 text (already decoded to UTF-8).
func ParseMT940(text string) ([]Statement, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = reSwiftHdr.ReplaceAllString(text, "\n")

	type field struct {
		tag   string
		value string
		line  int
	}
	var fields []field
	for i, raw := range strings.Split(text, "\n") {
		l := strings.TrimRight(raw, " \t")
		if l == "" {
			continue
		}
		if m := reTag.FindStringSubmatch(l); m != nil {
			fields = append(fields, field{tag: m[1], value: l[len(m[0]):], line: i + 1})
			continue
		}
		if len(fields) > 0 {
			fields[len(fields)-1].value += "\n" + l
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("mt940: no SWIFT tags found")
	}

	var out []Statement
	var cur *Statement
	var last *Line
	flush := func() {
		if cur != nil && (len(cur.Lines) > 0 || cur.Opening != nil || cur.AccountRef != "") {
			out = append(out, *cur)
		}
	}
	for _, f := range fields {
		switch f.tag {
		case "20":
			flush()
			cur = &Statement{Format: "mt940", Reference: strings.TrimSpace(f.value)}
			last = nil
		case "21", "28", "28C", "65", "34F", "90D", "90C":
			// statement / page numbers, forward balances, MT942 totals: kept raw only.
		case "13D":
			if cur != nil {
				cur.Format = "mt942"
			}
		case "25":
			if cur == nil {
				cur = &Statement{Format: "mt940"}
			}
			cur.AccountRef = strings.TrimSpace(f.value)
			if iban := banking.NormalizeIBAN(cur.AccountRef); iban != "" {
				cur.AccountIBAN = iban
			} else if m := reIBAN.FindString(strings.ReplaceAll(cur.AccountRef, " ", "")); m != "" {
				cur.AccountIBAN = banking.NormalizeIBAN(m)
			}
		case "60F", "60M", "62F", "62M", "64":
			if cur == nil {
				continue
			}
			b, err := parseBalance(strings.TrimSpace(f.value))
			if err != nil {
				cur.Errors = append(cur.Errors, RowError{Row: f.line, Reason: err.Error()})
				continue
			}
			switch f.tag[:2] {
			case "60":
				if cur.Opening == nil {
					cur.Opening = &b
				}
			case "62":
				cur.Closing = &b
			case "64":
				cur.Available = &b
			}
			if cur.Currency == "" {
				cur.Currency = b.Currency
			}
		case "61":
			if cur == nil {
				continue
			}
			l, err := parse61(f.value, cur.Currency)
			if err != nil {
				cur.Errors = append(cur.Errors, RowError{Row: f.line, Reason: err.Error()})
				last = nil
				continue
			}
			cur.Lines = append(cur.Lines, l)
			last = &cur.Lines[len(cur.Lines)-1]
		case "86":
			if last != nil {
				apply86(last, f.value)
			}
		}
	}
	flush()
	for i := range out {
		finishMT940(&out[i])
	}
	return out, nil
}

func parseBalance(v string) (Balance, error) {
	m := reBalance.FindStringSubmatch(v)
	if m == nil {
		return Balance{}, fmt.Errorf("mt940: bad balance %q", v)
	}
	d, err := time.Parse("060102", m[2])
	if err != nil {
		return Balance{}, fmt.Errorf("mt940: bad balance date %q", m[2])
	}
	amt, err := commaDecimal(m[4])
	if err != nil {
		return Balance{}, err
	}
	if m[1] == "D" {
		amt = amt.Neg()
	}
	return Balance{Date: d, Currency: banking.NormalizeCurrency(m[3]), Amount: amt}, nil
}

func commaDecimal(s string) (decimal.Decimal, error) {
	s = strings.TrimSuffix(s, ",")
	d, err := decimal.NewFromString(strings.Replace(s, ",", ".", 1))
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("mt940: bad amount %q", s)
	}
	return d, nil
}

func parse61(v, currency string) (Line, error) {
	first, extra, _ := strings.Cut(v, "\n")
	m := re61.FindStringSubmatch(strings.TrimSpace(first))
	if m == nil {
		return Line{}, fmt.Errorf("mt940: bad :61: line %q", first)
	}
	valueDate, err := time.Parse("060102", m[1])
	if err != nil {
		return Line{}, fmt.Errorf("mt940: bad value date %q", m[1])
	}
	txDate := valueDate
	if m[2] != "" {
		mm, _ := strconv.Atoi(m[2][:2])
		dd, _ := strconv.Atoi(m[2][2:])
		year := valueDate.Year()
		vm := int(valueDate.Month())
		// 23 §3.2: year from the value date; across a year end the entry
		// month is far from the value month.
		switch {
		case mm > vm && mm-vm > 6:
			year--
		case vm > mm && vm-mm > 6:
			year++
		}
		txDate = time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
		if txDate.Month() != time.Month(mm) {
			return Line{}, fmt.Errorf("mt940: bad entry date %q", m[2])
		}
	}
	amt, err := commaDecimal(m[5])
	if err != nil {
		return Line{}, err
	}
	l := Line{}
	l.TransactionDate = txDate
	vd := valueDate
	l.ValueDate = &vd
	l.Amount = amt.String()
	l.Currency = currency
	l.MT940Code = m[7]
	l.CaptureMethod = banking.CaptureMT940
	switch m[3] {
	case "C":
		l.Direction = banking.DirIn
	case "D":
		l.Direction = banking.DirOut
	case "RC": // alacak iptali: para çıkar
		l.Direction, l.Reversal = banking.DirOut, true
	case "RD": // borç iptali: para girer
		l.Direction, l.Reversal = banking.DirIn, true
	}
	refs := m[8]
	cust, bankRef, _ := strings.Cut(refs, "//")
	cust = strings.TrimSpace(cust)
	l.Raw = map[string]any{"mt940_61": first, "type": m[6]}
	if cust != "" && !strings.EqualFold(cust, "NONREF") {
		l.Raw["customer_reference"] = cust
	}
	if l.Reversal {
		l.Raw["reversal"] = true
	}
	if b := strings.TrimSpace(bankRef); b != "" {
		l.BankReference = b
	}
	if e := strings.TrimSpace(extra); e != "" {
		l.Raw["mt940_61_extra"] = e
	}
	return l, nil
}

// apply86 joins the :86: lines into the description and lifts the fields a
// Turkish bank usually embeds; nothing is invented (23 §3.2).
func apply86(l *Line, v string) {
	desc := strings.Join(strings.Fields(strings.ReplaceAll(v, "\n", " ")), " ")
	l.Description = desc
	l.Raw["mt940_86"] = v
	compact := strings.ReplaceAll(desc, " ", "")
	l.CounterpartyIBAN = findIBAN(compact)
	if m := reTaxID.FindStringSubmatch(desc); m != nil {
		l.CounterpartyTaxID = m[1]
	}
	if m := reParty.FindStringSubmatch(desc); m != nil {
		l.CounterpartyName = strings.TrimSpace(m[1])
	}
	if m := reEFTQuery.FindStringSubmatch(desc); m != nil {
		l.EFTQueryNumber = m[1]
	}
}

// finishMT940 links storno pairs and fills the source keys.
func finishMT940(s *Statement) {
	for i := range s.Lines {
		l := &s.Lines[i]
		if l.Currency == "" {
			l.Currency = s.Currency
		}
		seq := int32(i + 1)
		l.BankSequence = &seq
		if !l.Reversal {
			continue
		}
		// The original is the earlier line with the same amount in the
		// opposite direction of the reversal (23 §3.2).
		for j := i - 1; j >= 0; j-- {
			o := &s.Lines[j]
			if !o.Reversal && !o.IsCancelled && o.Amount == l.Amount && o.Direction != l.Direction {
				o.IsCancelled, l.IsCancelled = true, true
				l.Raw["reverses_sequence"] = j + 1
				break
			}
		}
	}
	// Running balance from the opening balance: MT940 lines carry none, but
	// :60F: + the signed amounts give it, and it is what lets a movement that
	// also came from the bank service merge instead of duplicating (21 §5
	// rule 2, 23 §7 test 6). Marked as derived in the raw payload.
	if s.Opening != nil {
		run := s.Opening.Amount
		for i := range s.Lines {
			l := &s.Lines[i]
			run = run.Add(signed(*l))
			v := run.String()
			l.BalanceAfter = &v
			l.Raw["balance_after_derived"] = true
		}
	}
	for i := range s.Lines {
		l := &s.Lines[i]
		vd := ""
		if l.ValueDate != nil {
			vd = l.ValueDate.Format("2006-01-02")
		}
		// 23 §3.5: sha256(iban | transaction_date | value_date | D/C | amount | code | bank ref | seq)
		sum := sha256.Sum256([]byte(strings.Join([]string{
			accountKey(s), l.TransactionDate.Format("2006-01-02"), vd, string(l.Direction), l.Amount,
			l.MT940Code, l.BankReference, strconv.Itoa(i + 1),
		}, "|")))
		l.SourceID = "mt940:" + hex.EncodeToString(sum[:])
	}
}

func accountKey(s *Statement) string {
	if s.AccountIBAN != "" {
		return s.AccountIBAN
	}
	return s.AccountRef
}

// findIBAN returns the first TR IBAN in s whose check digits are valid.
func findIBAN(s string) string {
	for i := 0; i+26 <= len(s); i++ {
		if s[i] == 'T' && s[i+1] == 'R' {
			if iban := banking.NormalizeIBAN(s[i : i+26]); iban != "" {
				return iban
			}
		}
	}
	return ""
}
