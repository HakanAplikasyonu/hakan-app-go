// Package xlstest writes small binary Excel 97-2003 (.xls, BIFF8) workbooks
// for tests: an OLE compound file holding one "Workbook" stream. It is for
// made-up fixtures only (a single FAT sector, so a workbook stream of at
// most about 60 KB).
package xlstest

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

// Cell kinds besides string (shared string table) and float64 / int
// (RK, MULRK or NUMBER, as Excel picks).
type (
	// Label is written as an inline LABEL record.
	Label string
	// FormulaNum is a FORMULA cell with a cached number.
	FormulaNum float64
	// FormulaStr is a FORMULA cell with a cached string (STRING record).
	FormulaStr string
)

// Sheet is one worksheet; Rows[i][j] is the cell at row i, column j (nil
// or "" leaves it empty).
type Sheet struct {
	Name string
	Rows [][]any
}

// Book is a workbook. MaxRecord limits a record's data (BIFF8: 8224); a
// smaller value splits the shared string table over more CONTINUE records.
type Book struct {
	Sheets    []Sheet
	MaxRecord int
}

// Write returns the .xls bytes of the sheets.
func Write(sheets ...Sheet) []byte { return Book{Sheets: sheets}.Bytes() }

var le = binary.LittleEndian

const (
	xf         = 0x0F
	endOfChain = 0xFFFFFFFE
	freeSect   = 0xFFFFFFFF
	fatSect    = 0xFFFFFFFD
	noStream   = 0xFFFFFFFF
	sectorSize = 512
)

func record(out []byte, typ uint16, data []byte) []byte {
	out = le.AppendUint16(out, typ)
	out = le.AppendUint16(out, uint16(len(data)))
	return append(out, data...)
}

// units returns the UTF-16 units of s and whether they all fit one byte
// (a "compressed" string).
func units(s string) ([]uint16, bool) {
	u := utf16.Encode([]rune(s))
	for _, c := range u {
		if c > 0xFF {
			return u, false
		}
	}
	return u, true
}

func appendChars(b []byte, u []uint16, compressed bool) []byte {
	for _, c := range u {
		if compressed {
			b = append(b, byte(c))
		} else {
			b = le.AppendUint16(b, c)
		}
	}
	return b
}

// xlString: 16-bit length, option byte, characters.
func xlString(s string) []byte {
	u, comp := units(s)
	b := le.AppendUint16(nil, uint16(len(u)))
	if comp {
		b = append(b, 0)
	} else {
		b = append(b, 1)
	}
	return appendChars(b, u, comp)
}

// shortString: 8-bit length, option byte, characters (sheet names).
func shortString(s string) []byte {
	u, comp := units(s)
	b := []byte{byte(len(u)), 0}
	if !comp {
		b[1] = 1
	}
	return appendChars(b, u, comp)
}

func bof(dt uint16) []byte {
	b := le.AppendUint16(nil, 0x0600)
	b = le.AppendUint16(b, dt)
	b = le.AppendUint16(b, 0x0DBB)
	b = le.AppendUint16(b, 0x07CC)
	b = le.AppendUint32(b, 0)
	return le.AppendUint32(b, 0x0006)
}

func cellHead(row, col int) []byte {
	b := le.AppendUint16(nil, uint16(row))
	b = le.AppendUint16(b, uint16(col))
	return le.AppendUint16(b, xf)
}

// rk encodes v as an RK number when it can be stored exactly.
func rk(v float64) (uint32, bool) {
	if v == math.Trunc(v) && math.Abs(v) < 1<<29 {
		return uint32(int32(v))<<2 | 2, true
	}
	bits := math.Float64bits(v)
	if bits&0x3_FFFF_FFFF == 0 {
		return uint32(bits >> 32), true
	}
	if c := math.Round(v * 100); c/100 == v && math.Abs(c) < 1<<29 {
		return uint32(int32(c))<<2 | 3, true
	}
	return 0, false
}

func number(c any) (float64, bool) {
	switch v := c.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// sst builds the shared string table records, splitting long strings
// across CONTINUE records the way Excel does (a continued character run
// starts with its own option byte).
type sst struct {
	max   int
	recs  [][]byte
	index map[string]int
	total int
}

func (t *sst) add(s string) int {
	t.total++
	if i, ok := t.index[s]; ok {
		return i
	}
	i := len(t.index)
	t.index[s] = i
	u, comp := units(s)
	size := 1
	if !comp {
		size = 2
	}
	cur := &t.recs[len(t.recs)-1]
	if t.max-len(*cur) < 3+size {
		t.recs = append(t.recs, nil)
		cur = &t.recs[len(t.recs)-1]
	}
	*cur = le.AppendUint16(*cur, uint16(len(u)))
	if comp {
		*cur = append(*cur, 0)
	} else {
		*cur = append(*cur, 1)
	}
	for len(u) > 0 {
		room := (t.max - len(*cur)) / size
		if room == 0 {
			t.recs = append(t.recs, nil)
			cur = &t.recs[len(t.recs)-1]
			*cur = append(*cur, byte(size-1))
			continue
		}
		k := min(room, len(u))
		*cur = appendChars(*cur, u[:k], comp)
		u = u[k:]
	}
	return i
}

// Bytes returns the .xls file.
func (b Book) Bytes() []byte {
	maxRec := b.MaxRecord
	if maxRec <= 0 || maxRec > 8224 {
		maxRec = 8224
	}
	strs := &sst{max: maxRec, recs: [][]byte{make([]byte, 8)}, index: map[string]int{}}
	// Worksheet substreams first: they fill the string table.
	subs := make([][]byte, len(b.Sheets))
	for si, sh := range b.Sheets {
		out := record(nil, 0x0809, bof(0x0010))
		for r, row := range sh.Rows {
			for c := 0; c < len(row); c++ {
				// A run of RK numbers is one MULRK record.
				n := 0
				for c+n < len(row) {
					v, ok := number(row[c+n])
					if !ok {
						break
					}
					if _, ok := rk(v); !ok {
						break
					}
					n++
				}
				if n >= 2 {
					d := le.AppendUint16(nil, uint16(r))
					d = le.AppendUint16(d, uint16(c))
					for k := 0; k < n; k++ {
						v, _ := number(row[c+k])
						x, _ := rk(v)
						d = le.AppendUint16(d, xf)
						d = le.AppendUint32(d, x)
					}
					d = le.AppendUint16(d, uint16(c+n-1))
					out = record(out, 0x00BD, d)
					c += n - 1
					continue
				}
				switch v := row[c].(type) {
				case nil:
				case string:
					if v == "" {
						continue
					}
					out = record(out, 0x00FD, le.AppendUint32(cellHead(r, c), uint32(strs.add(v))))
				case Label:
					out = record(out, 0x0204, append(cellHead(r, c), xlString(string(v))...))
				case float64, int:
					f, _ := number(v)
					if x, ok := rk(f); ok {
						out = record(out, 0x027E, le.AppendUint32(cellHead(r, c), x))
					} else {
						out = record(out, 0x0203, le.AppendUint64(cellHead(r, c), math.Float64bits(f)))
					}
				case FormulaNum:
					d := le.AppendUint64(cellHead(r, c), math.Float64bits(float64(v)))
					d = le.AppendUint16(d, 0) // grbit
					d = le.AppendUint32(d, 0) // chn
					d = le.AppendUint16(d, 0) // cce: no parsed formula
					out = record(out, 0x0006, d)
				case FormulaStr:
					d := append(cellHead(r, c), 0, 0, 0, 0, 0, 0, 0xFF, 0xFF)
					d = le.AppendUint16(d, 0)
					d = le.AppendUint32(d, 0)
					d = le.AppendUint16(d, 0)
					out = record(out, 0x0006, d)
					out = record(out, 0x0207, xlString(string(v)))
				default:
					panic("xlstest: unsupported cell type")
				}
			}
		}
		subs[si] = record(out, 0x000A, nil)
	}
	le.PutUint32(strs.recs[0][0:], uint32(strs.total))
	le.PutUint32(strs.recs[0][4:], uint32(len(strs.index)))

	glob := record(nil, 0x0809, bof(0x0005))
	var patch []int // offsets of the BOUNDSHEET positions
	for _, sh := range b.Sheets {
		patch = append(patch, len(glob)+4)
		glob = record(glob, 0x0085, append([]byte{0, 0, 0, 0, 0, 0}, shortString(sh.Name)...))
	}
	glob = record(glob, 0x00FC, strs.recs[0])
	for _, c := range strs.recs[1:] {
		glob = record(glob, 0x003C, c)
	}
	glob = record(glob, 0x000A, nil)
	stream := glob
	for i, s := range subs {
		le.PutUint32(stream[patch[i]:], uint32(len(stream)))
		stream = append(stream, s...)
	}
	// Streams under 4096 bytes would live in the mini stream; pad after
	// the last EOF so the workbook uses normal sectors.
	for len(stream) < 4096 {
		stream = append(stream, 0)
	}
	return compound(stream)
}

// compound wraps the stream in an OLE compound file (version 3): sector 0
// is the FAT, sector 1 the directory, then the workbook stream.
func compound(stream []byte) []byte {
	n := (len(stream) + sectorSize - 1) / sectorSize
	if 2+n > sectorSize/4 {
		panic("xlstest: workbook too large")
	}
	out := make([]byte, sectorSize*(3+n))
	h := out[:sectorSize]
	copy(h, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	le.PutUint16(h[24:], 0x003E) // minor version
	le.PutUint16(h[26:], 3)      // major version
	le.PutUint16(h[28:], 0xFFFE) // byte order
	le.PutUint16(h[30:], 9)      // sector shift (512)
	le.PutUint16(h[32:], 6)      // mini sector shift (64)
	le.PutUint32(h[44:], 1)      // FAT sectors
	le.PutUint32(h[48:], 1)      // first directory sector
	le.PutUint32(h[56:], 4096)   // mini stream cutoff
	le.PutUint32(h[60:], endOfChain)
	le.PutUint32(h[68:], endOfChain)
	for i := 0; i < 109; i++ {
		le.PutUint32(h[76+4*i:], freeSect)
	}
	le.PutUint32(h[76:], 0) // the FAT is sector 0

	fat := out[sectorSize : 2*sectorSize]
	for i := 0; i < sectorSize/4; i++ {
		le.PutUint32(fat[4*i:], freeSect)
	}
	le.PutUint32(fat[0:], fatSect)
	le.PutUint32(fat[4:], endOfChain)
	for i := 0; i < n; i++ {
		next := uint32(2 + i + 1)
		if i == n-1 {
			next = endOfChain
		}
		le.PutUint32(fat[4*(2+i):], next)
	}

	dir := out[2*sectorSize : 3*sectorSize]
	dirEntry(dir[0:128], "Root Entry", 5, 1, endOfChain, 0)
	dirEntry(dir[128:256], "Workbook", 2, noStream, 2, uint32(len(stream)))
	for _, e := range [][]byte{dir[256:384], dir[384:512]} {
		le.PutUint32(e[68:], noStream)
		le.PutUint32(e[72:], noStream)
		le.PutUint32(e[76:], noStream)
	}
	copy(out[3*sectorSize:], stream)
	return out
}

func dirEntry(b []byte, name string, typ byte, child, start, size uint32) {
	u := utf16.Encode([]rune(name))
	for i, c := range u {
		le.PutUint16(b[2*i:], c)
	}
	le.PutUint16(b[64:], uint16(2*(len(u)+1)))
	b[66] = typ
	b[67] = 1 // black
	le.PutUint32(b[68:], noStream)
	le.PutUint32(b[72:], noStream)
	le.PutUint32(b[76:], child)
	le.PutUint32(b[116:], start)
	le.PutUint32(b[120:], size)
}
