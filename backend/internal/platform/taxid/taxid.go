// Package taxid validates Turkish tax identifiers: VKN (vergi kimlik
// numarası, 10 hane) and TCKN (T.C. kimlik numarası, 11 hane).
//
// Both carry check digits. Validation here is offline (format + checksum);
// it does not prove the number is registered with GİB.
package taxid

import (
	"errors"
	"strings"
)

// Type of a tax identifier.
type Type string

const (
	VKN  Type = "vkn"
	TCKN Type = "tckn"
)

// Errors returned by Parse.
var (
	ErrFormat   = errors.New("taxid: VKN 10, TCKN 11 haneli olmalı")
	ErrChecksum = errors.New("taxid: sağlama hanesi tutmuyor")
)

// Normalize removes spaces, dashes and dots people type between digit groups.
func Normalize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '\t':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// Parse normalizes s and returns its type. The number must be all digits,
// 10 (VKN) or 11 (TCKN) long, and pass its checksum.
func Parse(s string) (string, Type, error) {
	n := Normalize(s)
	for _, r := range n {
		if r < '0' || r > '9' {
			return "", "", ErrFormat
		}
	}
	switch len(n) {
	case 10:
		if !ValidVKN(n) {
			return "", "", ErrChecksum
		}
		return n, VKN, nil
	case 11:
		if !ValidTCKN(n) {
			return "", "", ErrChecksum
		}
		return n, TCKN, nil
	default:
		return "", "", ErrFormat
	}
}

// ValidVKN checks a 10-digit VKN with the GİB check-digit algorithm.
func ValidVKN(s string) bool {
	if len(s) != 10 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		d := int(s[i] - '0')
		tmp := (d + 10 - (i + 1)) % 10
		if tmp == 9 {
			sum += tmp
			continue
		}
		sum += (tmp * (1 << (9 - i))) % 9
	}
	return int(s[9]-'0') == (10-sum%10)%10
}

// ValidTCKN checks an 11-digit TCKN: first digit non-zero, 10th digit
// ((odd positions sum)*7 - (even positions sum)) mod 10, 11th digit the sum
// of the first ten mod 10.
func ValidTCKN(s string) bool {
	if len(s) != 11 || !allDigits(s) || s[0] == '0' {
		return false
	}
	var d [11]int
	for i := range d {
		d[i] = int(s[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	if ((odd*7-even)%10+10)%10 != d[9] {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += d[i]
	}
	return sum%10 == d[10]
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
