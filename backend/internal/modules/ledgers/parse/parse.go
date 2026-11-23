// Package parse reads the books an office uploads for a taxpayer (#780
// "Defterler"): the işletme defteri export of Luca (xlsx / csv), the Luca
// mizan and ücret bordrosu exports (xlsx / csv) and the e-defter files
// (yevmiye / kebir XBRL-GL XML, berat, or a zip of them).
//
// Everything is best effort and pure: no database, no storage. A file that
// cannot be read gives a Summary with Readable false and a Reason code; the
// caller stores the file anyway.
package parse

import (
	"archive/zip"
	"bytes"
	"io"
	"path"
	"strings"
)

// Kinds of a book file (ledger_files.kind).
const (
	KindIsletme  = "isletme_defteri"
	KindYevmiye  = "e_defter_yevmiye"
	KindKebir    = "e_defter_kebir"
	KindBerat    = "berat"
	KindMizan    = "mizan"  // Luca mizan dökümü
	KindBordro   = "bordro" // Luca ücret bordrosu
	KindOther    = "other"
	maxZipFiles  = 64
	maxEntrySize = 64 << 20  // one XML inside a zip, uncompressed
	maxZipTotal  = 128 << 20 // all XMLs of a zip, uncompressed
)

// Kinds lists the accepted kinds.
var Kinds = []string{KindIsletme, KindYevmiye, KindKebir, KindBerat, KindMizan, KindBordro, KindOther}

// IsBook reports whether a kind is a book that covers its period in the
// missing-period list (mizan, bordro and other files do not).
func IsBook(k string) bool {
	switch k {
	case KindIsletme, KindYevmiye, KindKebir, KindBerat:
		return true
	}
	return false
}

// ValidKind reports whether k is a known kind.
func ValidKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// MaxBytes is the upload limit of one file.
const MaxBytes = 20 << 20

// Reasons a file could not be summarised (Summary.Reason). The screen shows
// "okunamadı" with a short Turkish explanation per code.
const (
	ReasonEmpty       = "empty"
	ReasonTooLarge    = "too_large"
	ReasonLegacyXLS   = "legacy_xls"
	ReasonUnsupported = "unsupported"
	ReasonNoColumns   = "no_columns"
	ReasonNoEntries   = "no_entries"
	ReasonBadXML      = "bad_xml"
	ReasonBadZip      = "bad_zip"
	// ReasonNoMizanHeader: no HESAP KODU / BORÇ / ALACAK header row.
	ReasonNoMizanHeader = "no_mizan_header"
	// ReasonNoBordro: neither the person header nor the summary blocks of
	// an ücret bordrosu.
	ReasonNoBordro = "no_bordro"
)

// Summary is what the screen shows of one file.
type Summary struct {
	Readable bool `json:"readable"`
	// Format: xlsx, csv, xml, zip, xls.
	Format string `json:"format"`
	// Reason: why it is not readable (Reason* codes); empty when readable.
	Reason string `json:"reason,omitempty"`
	// Files: the XML names inside a zip.
	Files   []string        `json:"files,omitempty"`
	Isletme *IsletmeSummary `json:"isletme,omitempty"`
	EDefter *EDefterSummary `json:"e_defter,omitempty"`
	Mizan   *MizanSummary   `json:"mizan,omitempty"`
	Bordro  *BordroSummary  `json:"bordro,omitempty"`
}

// DetectFormat sniffs the content.
func DetectFormat(name string, data []byte) string {
	lower := strings.ToLower(name)
	switch {
	case bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}):
		return "xls"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if isXLSX(data) {
			return "xlsx"
		}
		return "zip"
	case looksXML(data):
		return "xml"
	case strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".txt"):
		return "csv"
	case bytes.IndexByte(head(data, 4096), 0) < 0 && len(data) > 0:
		return "csv"
	}
	return ""
}

func head(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

func looksXML(data []byte) bool {
	h := bytes.TrimLeft(bytes.TrimPrefix(head(data, 512), []byte("\xEF\xBB\xBF")), " \t\r\n")
	return bytes.HasPrefix(h, []byte("<?xml")) || (bytes.HasPrefix(h, []byte("<")) && bytes.Contains(head(data, 4096), []byte("xbrl")))
}

func isXLSX(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" || strings.HasPrefix(f.Name, "xl/") {
			return true
		}
	}
	return false
}

// DetectKind guesses the kind of a file from its format, name and content.
// GİB e-defter names: VKN-YYYYMM-Y-000000.xml (yevmiye), -K- (kebir),
// -YB- / -KB- (berat).
func DetectKind(name string, data []byte) string {
	switch DetectFormat(name, data) {
	case "xlsx", "csv":
		return sheetKind(name, data)
	case "xls":
		// Legacy BIFF: the content cannot be read; the name decides.
		n := strings.ToLower(name)
		switch {
		case strings.Contains(n, "bordro"):
			return KindBordro
		case strings.Contains(n, "mizan"):
			return KindMizan
		}
		return KindIsletme
	case "xml":
		return xmlKind(name, data)
	case "zip":
		best := ""
		_ = eachZipXML(data, func(n string, b []byte) bool {
			k := xmlKind(n, b)
			if rank(k) > rank(best) {
				best = k
			}
			return true
		})
		if best == "" {
			return KindOther
		}
		return best
	}
	return KindOther
}

// sheetKind tells a Luca ücret bordrosu and a mizan from an işletme
// defteri by their header text.
func sheetKind(name string, data []byte) string {
	sheets, reason := readSheets(name, data)
	if reason != "" {
		return KindIsletme
	}
	for _, sh := range sheets {
		if isBordroSheet(sh) {
			return KindBordro
		}
	}
	if _, _, _, ok := findMizan(sheets); ok {
		return KindMizan
	}
	return KindIsletme
}

func rank(k string) int {
	switch k {
	case KindYevmiye:
		return 4
	case KindKebir:
		return 3
	case KindBerat:
		return 2
	case KindOther:
		return 1
	}
	return 0
}

func xmlKind(name string, data []byte) string {
	base := strings.ToUpper(path.Base(strings.ReplaceAll(name, `\`, "/")))
	switch {
	case strings.Contains(base, "-YB-") || strings.Contains(base, "-KB-") || strings.Contains(base, "BERAT"):
		return KindBerat
	case strings.Contains(base, "-K-") || strings.Contains(base, "KEBIR"):
		return KindKebir
	case strings.Contains(base, "-Y-") || strings.Contains(base, "YEVMIYE"):
		return KindYevmiye
	}
	h := head(data, 64<<10)
	switch {
	case bytes.Contains(h, []byte("berat")) || bytes.Contains(h, []byte("Berat")):
		return KindBerat
	case bytes.Contains(h, []byte(">ledger<")) && bytes.Contains(h, []byte("gl-cor")):
		return KindKebir
	case bytes.Contains(h, []byte("gl-cor")):
		return KindYevmiye
	}
	return KindOther
}

// eachZipXML calls fn for every .xml file of a zip (bounded); fn returns
// false to stop.
func eachZipXML(data []byte, fn func(name string, b []byte) bool) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	seen, total := 0, int64(0)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		seen++
		if seen > maxZipFiles {
			break
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxEntrySize+1))
		_ = rc.Close()
		if err != nil || len(b) > maxEntrySize {
			continue
		}
		total += int64(len(b))
		if total > maxZipTotal {
			break
		}
		if !fn(f.Name, b) {
			break
		}
	}
	return nil
}

// Summarize reads a file of the given kind. It never panics on bad input
// and never returns an error: an unreadable file is Readable false.
func Summarize(kind, name string, data []byte) (s Summary) {
	defer func() {
		if r := recover(); r != nil {
			s = Summary{Format: s.Format, Reason: ReasonUnsupported}
		}
	}()
	s.Format = DetectFormat(name, data)
	switch {
	case len(data) == 0:
		s.Reason = ReasonEmpty
		return s
	case len(data) > MaxBytes:
		s.Reason = ReasonTooLarge
		return s
	}
	switch s.Format {
	case "xls":
		s.Reason = ReasonLegacyXLS
	case "xlsx", "csv":
		switch kind {
		case KindMizan:
			m, reason := Mizan(name, data)
			if reason != "" {
				s.Reason = reason
				return s
			}
			s.Readable, s.Mizan = true, &m
			return s
		case KindBordro:
			b, reason := Bordro(name, data)
			if reason != "" {
				s.Reason = reason
				return s
			}
			s.Readable, s.Bordro = true, &b
			return s
		}
		is, reason := Isletme(name, data)
		if reason != "" {
			s.Reason = reason
			return s
		}
		s.Readable, s.Isletme = true, &is
	case "xml":
		summarizeXML(&s, kind, name, data)
	case "zip":
		var files []string
		var found []byte
		foundName := ""
		err := eachZipXML(data, func(n string, b []byte) bool {
			files = append(files, path.Base(n))
			k := xmlKind(n, b)
			if (kind == KindKebir && k == KindKebir) || (kind != KindKebir && k == KindYevmiye) {
				if found == nil {
					found, foundName = b, n
				}
			}
			return true
		})
		if err != nil {
			s.Reason = ReasonBadZip
			return s
		}
		s.Files = files
		if found == nil {
			// No yevmiye / kebir inside (only berat, or other XML).
			if len(files) == 0 {
				s.Reason = ReasonNoEntries
				return s
			}
			s.Readable = true
			return s
		}
		summarizeXML(&s, kind, foundName, found)
	default:
		s.Reason = ReasonUnsupported
	}
	return s
}

func summarizeXML(s *Summary, kind, name string, data []byte) {
	k := xmlKind(name, data)
	if k == KindBerat || (kind == KindBerat && k == KindOther) {
		s.Readable = true
		return
	}
	ed, reason := EDefter(data)
	if reason != "" {
		s.Reason = reason
		return
	}
	s.Readable, s.EDefter = true, &ed
}
