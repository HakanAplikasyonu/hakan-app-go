package communication

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Message rules of the NetGSM channel (#780 A).

var (
	// ErrInvalidPhone: not a Turkish mobile number (landline or broken).
	ErrInvalidPhone = errors.New("communication: not a mobile phone number")
	// ErrEmptyText: nothing to send.
	ErrEmptyText = errors.New("communication: message text is empty")
	// ErrCampaignText: commercial wording on the informational channel.
	ErrCampaignText = errors.New("communication: campaign wording is not allowed on the informational channel")
)

// NormalizePhone reduces a number to 10 digits (5XXXXXXXXX): +90, a leading
// 0, spaces, dashes and brackets are removed. Anything that is not a mobile
// number afterwards is ErrInvalidPhone.
func NormalizePhone(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "90"):
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	if len(d) != 10 || d[0] != '5' {
		return "", ErrInvalidPhone
	}
	return d, nil
}

var currencyReplacer = strings.NewReplacer("₺", "TL", "€", "EUR", "£", "GBP")

// NormalizeText converts currency signs NetGSM cannot carry (₺ → TL,
// € → EUR, £ → GBP), trims, and ends the text with a newline.
func NormalizeText(s string) (string, error) {
	s = strings.TrimSpace(currencyReplacer.Replace(strings.ReplaceAll(s, "\r\n", "\n")))
	if s == "" {
		return "", ErrEmptyText
	}
	return s + "\n", nil
}

var campaignPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bindirim`),
	regexp.MustCompile(`(?i)\bkampanya`),
	regexp.MustCompile(`(?i)\bpromosyon`),
	regexp.MustCompile(`(?i)\bfırsat`),
	regexp.MustCompile(`(?i)\bbedava`),
	regexp.MustCompile(`%\s?\d+`),
	regexp.MustCompile(`\d+\s?%`),
}

// CampaignWords returns the commercial phrases found in the text (empty =
// fine). Informational SMS (iysfilter 0) must not carry campaign wording.
func CampaignWords(s string) []string {
	lower := strings.ToLowerSpecial(turkishCase, s)
	out := []string{}
	for _, p := range campaignPatterns {
		if m := p.FindString(lower); m != "" {
			out = append(out, strings.TrimSpace(m))
		}
	}
	return out
}

var turkishCase = unicode.TurkishCase

// SMS length rules of the TR encoding: ç ğ ı ş Ğ İ Ş take two characters;
// a single SMS holds 155, each part of a long SMS 148; the operator adds a
// 5-character suffix.
const (
	SingleSMSChars = 155
	MultiSMSChars  = 148
	OperatorSuffix = 5
)

// Length is the SMS length of a text under an encoding.
type Length struct {
	Chars int `json:"chars"`
	Parts int `json:"parts"`
}

// SMSLength counts the characters and parts of a text.
func SMSLength(text, encoding string) Length {
	text = strings.TrimRight(text, "\n")
	if encoding == "UNICODE" {
		n := utf8.RuneCountInString(text)
		parts := 1
		if n > 70 {
			parts = (n + 66) / 67
		}
		return Length{Chars: n, Parts: parts}
	}
	n := OperatorSuffix
	for _, r := range text {
		switch r {
		case 'ç', 'ğ', 'ı', 'ş', 'Ğ', 'İ', 'Ş', '^', '{', '}', '[', ']', '~', '|', '\\':
			n += 2
		default:
			n++
		}
	}
	parts := 1
	if n > SingleSMSChars {
		parts = (n + MultiSMSChars - 1) / MultiSMSChars
	}
	return Length{Chars: n, Parts: parts}
}
