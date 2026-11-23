// Package xls reads a binary Excel 97-2003 workbook (BIFF8 inside an OLE
// compound file) into rows of cell texts, the same grid the xlsx and CSV
// readers give. Luca still exports some reports (ücret bordrosu, mizan) in
// this format.
//
// Only cell values are read: strings (shared string table, inline labels,
// cached formula results) and numbers. Numbers are written as plain text
// ("1234.56", "10000000146") from the stored IEEE value with the shortest
// round-trip form, so an amount is parsed as a decimal later and never goes
// through float rounding. Dates stay Excel serial numbers, like the raw
// xlsx values. Formats, styles and formulas themselves are ignored.
package xls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
	"github.com/shopspring/decimal"
)

// Sheet is one worksheet as rows of trimmed cell texts (rows are ragged:
// each ends at its last non-empty cell).
type Sheet struct {
	Name string
	Rows [][]string
}

// Errors.
var (
	// ErrNotXLS: not an OLE compound file, or no BIFF8 workbook stream in
	// it (a Word file, an Excel 5.0 / 95 workbook).
	ErrNotXLS = errors.New("xls: not an Excel 97-2003 workbook")
	// ErrEncrypted: the workbook is password protected.
	ErrEncrypted = errors.New("xls: workbook is password protected")
	// ErrCorrupt: the workbook stream is damaged.
	ErrCorrupt = errors.New("xls: damaged workbook")
	// ErrTooLarge: more cells than a report can have.
	ErrTooLarge = errors.New("xls: too many cells")
)

// Limits against a hostile file.
const (
	maxStream = 64 << 20
	maxCells  = 2_000_000
	maxSST    = 1_000_000
	// maxCol: BIFF8 has 256 columns.
	maxCol = 255
)

// Magic is the OLE compound file signature.
var Magic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// IsXLS reports whether data starts with the compound file signature.
func IsXLS(data []byte) bool { return bytes.HasPrefix(data, Magic) }

// BIFF8 record types.
const (
	recBOF        = 0x0809
	recEOF        = 0x000A
	recFILEPASS   = 0x002F
	recBOUNDSHEET = 0x0085
	recSST        = 0x00FC
	recCONTINUE   = 0x003C
	recLABELSST   = 0x00FD
	recLABEL      = 0x0204
	recRSTRING    = 0x00D6
	recNUMBER     = 0x0203
	recRK         = 0x027E
	recMULRK      = 0x00BD
	recBOOLERR    = 0x0205
	recFORMULA    = 0x0006
	recSTRING     = 0x0207

	biff8        = 0x0600
	bofGlobals   = 0x0005
	bofWorksheet = 0x0010
)

var le = binary.LittleEndian

// Read returns the worksheets of the workbook in their tab order (chart and
// macro sheets are skipped).
func Read(data []byte) (sheets []Sheet, err error) {
	defer func() {
		if r := recover(); r != nil {
			sheets, err = nil, ErrCorrupt
		}
	}()
	stream, err := workbookStream(data)
	if err != nil {
		return nil, err
	}
	return readWorkbook(stream)
}

// workbookStream returns the "Workbook" stream of the compound file.
func workbookStream(data []byte) ([]byte, error) {
	if !IsXLS(data) {
		return nil, ErrNotXLS
	}
	doc, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		if entry.Name != "Workbook" {
			continue
		}
		if entry.Size <= 0 || entry.Size > maxStream {
			return nil, ErrCorrupt
		}
		buf := make([]byte, entry.Size)
		if _, err := io.ReadFull(doc, buf); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		return buf, nil
	}
	return nil, ErrNotXLS
}

type record struct {
	typ  uint16
	data []byte
}

// nextRecord reads the record at off; ok is false at the end of the
// stream or on a record that runs past it.
func nextRecord(s []byte, off int) (record, int, bool) {
	if off < 0 || off+4 > len(s) {
		return record{}, off, false
	}
	typ, n := le.Uint16(s[off:]), int(le.Uint16(s[off+2:]))
	end := off + 4 + n
	if end > len(s) {
		return record{}, off, false
	}
	return record{typ: typ, data: s[off+4 : end]}, end, true
}

type boundSheet struct {
	name string
	pos  int
}

func readWorkbook(s []byte) ([]Sheet, error) {
	r, off, ok := nextRecord(s, 0)
	if !ok || r.typ != recBOF || len(r.data) < 4 {
		return nil, ErrNotXLS
	}
	if le.Uint16(r.data) != biff8 || le.Uint16(r.data[2:]) != bofGlobals {
		return nil, ErrNotXLS
	}
	var bounds []boundSheet
	var sst []string
globals:
	for {
		r, off, ok = nextRecord(s, off)
		if !ok {
			break
		}
		switch r.typ {
		case recEOF:
			break globals
		case recFILEPASS:
			return nil, ErrEncrypted
		case recBOUNDSHEET:
			// lbPlyPos (4), hsState (1), dt (1: 0 = worksheet), name.
			if len(r.data) < 8 || r.data[5] != 0 {
				continue
			}
			name, err := readString(&contReader{segs: [][]byte{r.data[6:]}}, true)
			if err != nil {
				return nil, ErrCorrupt
			}
			bounds = append(bounds, boundSheet{name: name, pos: int(le.Uint32(r.data))})
		case recSST:
			segs := [][]byte{r.data}
			for {
				c, next, ok := nextRecord(s, off)
				if !ok || c.typ != recCONTINUE {
					break
				}
				segs, off = append(segs, c.data), next
			}
			var err error
			if sst, err = readSST(segs); err != nil {
				return nil, err
			}
		}
	}
	out := make([]Sheet, 0, len(bounds))
	cells := 0
	for _, b := range bounds {
		rows, n, err := readSheet(s, b.pos, sst, maxCells-cells)
		if err != nil {
			return nil, err
		}
		cells += n
		out = append(out, Sheet{Name: b.name, Rows: rows})
	}
	return out, nil
}

// readSheet reads the cell records of the worksheet substream at pos.
func readSheet(s []byte, pos int, sst []string, budget int) ([][]string, int, error) {
	r, off, ok := nextRecord(s, pos)
	if !ok || r.typ != recBOF || len(r.data) < 4 || le.Uint16(r.data[2:]) != bofWorksheet {
		// Not a worksheet after all (a dialog sheet): no rows.
		return nil, 0, nil
	}
	var rows [][]string
	cells := 0
	// set stores a cell; the only error is ErrTooLarge.
	set := func(row, col int, v string) error {
		v = strings.TrimSpace(strings.ReplaceAll(v, "\u00a0", " "))
		if v == "" || col > maxCol {
			return nil
		}
		if cells++; cells > budget {
			return ErrTooLarge
		}
		for len(rows) <= row {
			rows = append(rows, []string{})
		}
		for len(rows[row]) <= col {
			rows[row] = append(rows[row], "")
		}
		rows[row][col] = v
		return nil
	}
	depth := 1
	// A FORMULA with a string result is followed by a STRING record.
	pendRow, pendCol := -1, -1
	for depth > 0 {
		r, off, ok = nextRecord(s, off)
		if !ok {
			break
		}
		switch r.typ {
		case recBOF:
			depth++ // an embedded chart substream
			continue
		case recEOF:
			depth--
			continue
		}
		if depth > 1 {
			continue
		}
		d := r.data
		if r.typ != recSTRING {
			pendRow, pendCol = -1, -1
		}
		var err error
		switch r.typ {
		case recLABELSST:
			if len(d) >= 10 {
				if i := int(le.Uint32(d[6:])); i < len(sst) {
					err = set(cellRow(d), cellCol(d), sst[i])
				}
			}
		case recLABEL, recRSTRING:
			if len(d) >= 9 {
				// A label that cannot be decoded is skipped.
				if v, e := readString(&contReader{segs: [][]byte{d[6:]}}, false); e == nil {
					err = set(cellRow(d), cellCol(d), v)
				}
			}
		case recNUMBER:
			if len(d) >= 14 {
				err = set(cellRow(d), cellCol(d), floatText(math.Float64frombits(le.Uint64(d[6:]))))
			}
		case recRK:
			if len(d) >= 10 {
				err = set(cellRow(d), cellCol(d), rkText(le.Uint32(d[6:])))
			}
		case recMULRK:
			// row, first column, (xf, rk) pairs, last column.
			if len(d) >= 12 {
				row, col := cellRow(d), cellCol(d)
				for i := 4; i+6 <= len(d)-2 && err == nil; i += 6 {
					err = set(row, col, rkText(le.Uint32(d[i+2:])))
					col++
				}
			}
		case recBOOLERR:
			if len(d) >= 8 && d[7] == 0 {
				err = set(cellRow(d), cellCol(d), boolText(d[6]))
			}
		case recFORMULA:
			// The cached result: a double, or a typed value marked by
			// 0xFFFF in its last two bytes.
			if len(d) >= 14 {
				row, col := cellRow(d), cellCol(d)
				res := d[6:14]
				if res[6] == 0xFF && res[7] == 0xFF {
					switch res[0] {
					case 0: // string: in the next STRING record
						pendRow, pendCol = row, col
					case 1: // boolean
						err = set(row, col, boolText(res[2]))
					}
				} else {
					err = set(row, col, floatText(math.Float64frombits(le.Uint64(res))))
				}
			}
		case recSTRING:
			segs := [][]byte{d}
			for {
				c, next, ok := nextRecord(s, off)
				if !ok || c.typ != recCONTINUE {
					break
				}
				segs, off = append(segs, c.data), next
			}
			if pendRow >= 0 {
				if v, e := readString(&contReader{segs: segs}, false); e == nil {
					err = set(pendRow, pendCol, v)
				}
			}
			pendRow, pendCol = -1, -1
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return rows, cells, nil
}

func cellRow(d []byte) int { return int(le.Uint16(d)) }
func cellCol(d []byte) int { return int(le.Uint16(d[2:])) }

func boolText(b byte) string {
	if b != 0 {
		return "TRUE"
	}
	return "FALSE"
}

// floatText writes an IEEE number in its shortest round-trip decimal form
// ("1234.56", not "1234.5599999999999").
func floatText(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// rkText decodes an RK number: bit 0 means "divide by 100", bit 1 means
// the upper 30 bits are a signed integer (else the upper 30 bits of an
// IEEE double).
func rkText(rk uint32) string {
	div100 := rk&1 != 0
	if rk&2 != 0 {
		n := int64(int32(rk) >> 2)
		if div100 {
			return decimal.New(n, -2).String()
		}
		return strconv.FormatInt(n, 10)
	}
	s := floatText(math.Float64frombits(uint64(rk&^3) << 32))
	if div100 && s != "" {
		d, err := decimal.NewFromString(s)
		if err != nil {
			return ""
		}
		return d.Shift(-2).String()
	}
	return s
}

// readSST reads the shared string table spread over an SST record and its
// CONTINUE records.
func readSST(segs [][]byte) ([]string, error) {
	r := &contReader{segs: segs}
	head, err := r.bytes(8)
	if err != nil {
		return nil, ErrCorrupt
	}
	n := int(le.Uint32(head[4:])) // cstUnique
	if n < 0 || n > maxSST {
		return nil, ErrTooLarge
	}
	out := make([]string, 0, min(n, 4096))
	for i := 0; i < n; i++ {
		s, err := readString(r, false)
		if err != nil {
			// A short table: keep what was read.
			break
		}
		out = append(out, s)
	}
	return out, nil
}

// contReader reads across a record and its CONTINUE records.
type contReader struct {
	segs [][]byte
	i    int
	off  int
}

func (r *contReader) avail() int {
	if r.i >= len(r.segs) {
		return 0
	}
	return len(r.segs[r.i]) - r.off
}

// advance moves to the next segment when the current one is used up.
func (r *contReader) advance() bool {
	for r.i < len(r.segs) && r.avail() == 0 {
		if r.i+1 >= len(r.segs) {
			return false
		}
		r.i, r.off = r.i+1, 0
	}
	return r.i < len(r.segs)
}

// bytes reads n bytes of non-character data (it may span segments).
func (r *contReader) bytes(n int) ([]byte, error) {
	out := make([]byte, 0, n)
	for len(out) < n {
		if !r.advance() {
			return nil, io.ErrUnexpectedEOF
		}
		k := min(n-len(out), r.avail())
		out = append(out, r.segs[r.i][r.off:r.off+k]...)
		r.off += k
	}
	return out, nil
}

func (r *contReader) skip(n int) error {
	for n > 0 {
		if !r.advance() {
			return io.ErrUnexpectedEOF
		}
		k := min(n, r.avail())
		r.off += k
		n -= k
	}
	return nil
}

// readString reads an XLUnicodeString (16-bit length) or, with short, a
// ShortXLUnicodeString (8-bit length). Character data split over a
// CONTINUE starts again with its own option byte (compressed or not).
func readString(r *contReader, short bool) (string, error) {
	var cch int
	if short {
		b, err := r.bytes(1)
		if err != nil {
			return "", err
		}
		cch = int(b[0])
	} else {
		b, err := r.bytes(2)
		if err != nil {
			return "", err
		}
		cch = int(le.Uint16(b))
	}
	fb, err := r.bytes(1)
	if err != nil {
		return "", err
	}
	flags := fb[0]
	high := flags&0x01 != 0
	runs, ext := 0, 0
	if flags&0x08 != 0 { // rich text: formatting run count
		b, err := r.bytes(2)
		if err != nil {
			return "", err
		}
		runs = int(le.Uint16(b))
	}
	if flags&0x04 != 0 { // phonetic (Far East) data size
		b, err := r.bytes(4)
		if err != nil {
			return "", err
		}
		ext = int(le.Uint32(b))
	}
	units := make([]uint16, 0, min(cch, 1024))
	for len(units) < cch {
		if r.avail() == 0 || (high && r.avail() < 2) {
			// Character data continued in the next record: it starts
			// with its own option byte (a lone byte left over from a
			// 16-bit run is padding).
			if r.i+1 >= len(r.segs) || len(r.segs[r.i+1]) == 0 {
				return "", io.ErrUnexpectedEOF
			}
			r.i, r.off = r.i+1, 1
			high = r.segs[r.i][0]&0x01 != 0
			continue
		}
		left := cch - len(units)
		seg := r.segs[r.i][r.off:]
		if high {
			k := min(left, len(seg)/2)
			if k == 0 {
				return "", io.ErrUnexpectedEOF
			}
			for j := 0; j < k; j++ {
				units = append(units, le.Uint16(seg[2*j:]))
			}
			r.off += 2 * k
		} else {
			k := min(left, len(seg))
			for j := 0; j < k; j++ {
				units = append(units, uint16(seg[j]))
			}
			r.off += k
		}
	}
	if err := r.skip(4*runs + ext); err != nil {
		return "", err
	}
	return string(utf16.Decode(units)), nil
}
