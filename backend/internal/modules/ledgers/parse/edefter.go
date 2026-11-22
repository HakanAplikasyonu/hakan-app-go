package parse

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	maxLines    = 1_000_000
	maxAccounts = 1000
	maxText     = 200
)

// EDefterSummary: the hesap planı and per main account totals of an
// e-defter yevmiye (or kebir) XBRL-GL file.
type EDefterSummary struct {
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
	// Entries: yevmiye maddeleri (gl-cor:entryHeader); Lines: satırlar
	// (gl-cor:entryDetail).
	Entries     int    `json:"entries"`
	Lines       int    `json:"lines"`
	DebitTotal  string `json:"debit_total"`
	CreditTotal string `json:"credit_total"`
	// Accounts: main accounts (the first three digits) seen, by code.
	Accounts []Account `json:"accounts"`
}

// Account is one main account of the hesap planı with its totals.
type Account struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
	Lines  int    `json:"lines"`
}

type acc struct {
	name          string
	debit, credit decimal.Decimal
	lines         int
}

// mainCode: "100.01.001" → "100"; codes without three leading digits are
// kept whole (bounded).
func mainCode(id string) string {
	id = strings.TrimSpace(id)
	if len(id) >= 3 && isDigit(id[0]) && isDigit(id[1]) && isDigit(id[2]) {
		return id[:3]
	}
	if r := []rune(id); len(r) > 20 {
		return string(r[:20])
	}
	return id
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxText {
		return string(r[:maxText])
	}
	return s
}

// EDefter reads an XBRL-GL yevmiye / kebir file. It streams the XML and
// only looks at gl-cor elements by their local name, so the namespace
// prefix does not matter.
func EDefter(data []byte) (EDefterSummary, string) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var (
		out       EDefterSummary
		accounts  = map[string]*acc{}
		stack     []string
		inDetail  bool
		mainID    string
		mainDesc  string
		amount    string
		dc        string
		debitSum  decimal.Decimal
		creditSum decimal.Decimal
		text      strings.Builder
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if out.Lines > 0 {
				break // a truncated file: keep what was read
			}
			return EDefterSummary{}, ReasonBadXML
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
			switch t.Name.Local {
			case "entryHeader":
				out.Entries++
			case "entryDetail":
				inDetail, mainID, mainDesc, amount, dc = true, "", "", "", ""
			}
		case xml.CharData:
			if text.Len() < 4096 {
				text.Write(t)
			}
		case xml.EndElement:
			name := t.Name.Local
			v := strings.TrimSpace(text.String())
			text.Reset()
			parent := ""
			if len(stack) >= 2 {
				parent = stack[len(stack)-2]
			}
			switch name {
			case "periodCoveredStart":
				out.PeriodStart = clip(v)
			case "periodCoveredEnd":
				out.PeriodEnd = clip(v)
			}
			if inDetail {
				switch name {
				case "accountMainID":
					if mainID == "" {
						mainID = v
					}
				case "accountMainDescription":
					if mainDesc == "" {
						mainDesc = v
					}
				case "amount":
					// The line amount is a direct child of entryDetail.
					if parent == "entryDetail" {
						amount = v
					}
				case "debitCreditCode":
					dc = v
				case "entryDetail":
					inDetail = false
					if mainID != "" {
						out.Lines++
						code := mainCode(mainID)
						a := accounts[code]
						if a == nil {
							if len(accounts) >= maxAccounts {
								break
							}
							a = &acc{}
							accounts[code] = a
						}
						if a.name == "" && mainDesc != "" {
							a.name = clip(mainDesc)
						}
						a.lines++
						if d, err := decimal.NewFromString(strings.ReplaceAll(amount, ",", ".")); err == nil {
							switch strings.ToLower(strings.TrimSpace(dc)) {
							case "d", "debit", "borc", "borç", "b":
								a.debit = a.debit.Add(d)
								debitSum = debitSum.Add(d)
							case "c", "credit", "alacak", "a":
								a.credit = a.credit.Add(d)
								creditSum = creditSum.Add(d)
							}
						}
					}
				}
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if out.Lines >= maxLines {
				goto done
			}
		}
	}
done:
	if out.Lines == 0 {
		return EDefterSummary{}, ReasonNoEntries
	}
	out.DebitTotal, out.CreditTotal = debitSum.StringFixed(2), creditSum.StringFixed(2)
	codes := make([]string, 0, len(accounts))
	for c := range accounts {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	out.Accounts = make([]Account, 0, len(codes))
	for _, c := range codes {
		a := accounts[c]
		out.Accounts = append(out.Accounts, Account{Code: c, Name: a.name, Debit: a.debit.StringFixed(2), Credit: a.credit.StringFixed(2), Lines: a.lines})
	}
	return out, ""
}
