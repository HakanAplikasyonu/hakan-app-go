package statement

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Decode turns statement bytes into UTF-8. Order (23 §3.3): UTF-8 with or
// without BOM, then Windows-1254, then ISO-8859-9. The Turkish code pages
// differ only outside the Turkish letters; a text that is not valid UTF-8
// is read as Windows-1254 unless it uses C1 control bytes that only make
// sense in ISO-8859-9.
func Decode(b []byte) (string, string) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) {
		return string(b), "utf-8"
	}
	if s, err := charmap.Windows1254.NewDecoder().Bytes(b); err == nil && !strings.ContainsRune(string(s), utf8.RuneError) {
		return string(s), "windows-1254"
	}
	s, _ := charmap.ISO8859_9.NewDecoder().Bytes(b)
	return string(s), "iso-8859-9"
}
