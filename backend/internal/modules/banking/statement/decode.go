package statement

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// Decode turns statement bytes into UTF-8. Order (23 §3.3): UTF-16 with a
// byte order mark (Excel's "Unicode text" and some internet branches' .xls
// HTML), UTF-8 with or without BOM, then Windows-1254, then ISO-8859-9. The
// Turkish code pages differ only outside the Turkish letters; a text that is
// not valid UTF-8 is read as Windows-1254 unless it uses C1 control bytes
// that only make sense in ISO-8859-9.
func Decode(b []byte) (string, string) { return decode(b, false) }

// DecodeHead is Decode for the first bytes of a file (format checks, the
// report's encoding): a UTF-8 letter cut at the end still counts as UTF-8.
func DecodeHead(b []byte) (string, string) { return decode(b, true) }

func decode(b []byte, head bool) (string, string) {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		if s, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().Bytes(b[2:]); err == nil {
			return string(s), "utf-16le"
		}
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		if s, err := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(b[2:]); err == nil {
			return string(s), "utf-16be"
		}
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) || (head && validUTF8Head(b)) {
		return string(b), "utf-8"
	}
	if s, err := charmap.Windows1254.NewDecoder().Bytes(b); err == nil && !strings.ContainsRune(string(s), utf8.RuneError) {
		return string(s), "windows-1254"
	}
	s, _ := charmap.ISO8859_9.NewDecoder().Bytes(b)
	return string(s), "iso-8859-9"
}

// validUTF8Head: b would be valid UTF-8 without one letter cut off at its
// end.
func validUTF8Head(b []byte) bool {
	for k := 1; k <= utf8.UTFMax-1 && k <= len(b); k++ {
		if utf8.RuneStart(b[len(b)-k]) {
			return !utf8.FullRune(b[len(b)-k:]) && utf8.Valid(b[:len(b)-k])
		}
	}
	return false
}
