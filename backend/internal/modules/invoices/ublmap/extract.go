package ublmap

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	ubltr "github.com/furkanmeclis/go-ubltr"
)

// PayloadKind is what the first bytes say a payload is (`37` §3.4).
type PayloadKind string

const (
	PayloadXML     PayloadKind = "xml"
	PayloadZIP     PayloadKind = "zip"
	PayloadPDF     PayloadKind = "pdf"
	PayloadBase64  PayloadKind = "base64"
	PayloadUnknown PayloadKind = "unknown"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Sniff classifies a payload by its first bytes. A BOM is skipped; text that
// is entirely base64 alphabet is reported as PayloadBase64.
func Sniff(b []byte) PayloadKind {
	b = bytes.TrimPrefix(b, utf8BOM)
	t := bytes.TrimLeftFunc(b, unicode.IsSpace)
	switch {
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return PayloadZIP
	case bytes.HasPrefix(t, []byte("<")):
		return PayloadXML
	case bytes.HasPrefix(b, []byte("%PDF")):
		return PayloadPDF
	case looksBase64(t):
		return PayloadBase64
	}
	return PayloadUnknown
}

func looksBase64(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	for _, c := range b {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '+', c == '/', c == '=', c == '\r', c == '\n', c == ' ', c == '\t':
		default:
			return false
		}
	}
	return true
}

// Limits bound what Extract will unpack (ZIP bombs, oversized uploads).
type Limits struct {
	MaxFileBytes  int64 // per XML document, after decompression
	MaxTotalBytes int64 // all documents together
	MaxEntries    int   // ZIP entries
}

// DefaultLimits: 20 MB per file (`02` §7), 500 MB total, 5000 entries.
var DefaultLimits = Limits{MaxFileBytes: 20 << 20, MaxTotalBytes: 500 << 20, MaxEntries: 5000}

// Item is one document found in a payload. Err is set when that one entry
// could not be read; the other items are still usable.
type Item struct {
	Name string     // file name inside the ZIP (or the given name)
	Kind ubltr.Kind // from ubltr.Detect; envelopes are unpacked
	XML  []byte
	Err  error
}

var (
	ErrUnknownPayload = errors.New("ublmap: payload_unknown")
	ErrDTD            = errors.New("ublmap: DTD / DOCTYPE is not allowed")
	ErrTooLarge       = errors.New("ublmap: payload exceeds size limits")
	ErrNotUBL         = errors.New("ublmap: not a UBL-TR document")
)

// Extract turns an uploaded or downloaded payload into UBL documents
// (`31` §1): raw XML, base64 text, ZIP with one or many XML files, or a GİB
// envelope (StandardBusinessDocument) whose documents are unpacked. PDFs and
// unknown bytes return an error; callers keep the raw bytes as an attachment.
func Extract(name string, data []byte, lim Limits) ([]Item, error) {
	if lim.MaxFileBytes <= 0 {
		lim = DefaultLimits
	}
	var total int64
	return extract(name, data, lim, &total, 0)
}

func extract(name string, data []byte, lim Limits, total *int64, depth int) ([]Item, error) {
	switch Sniff(data) {
	case PayloadXML:
		return xmlItems(name, data, lim, total)
	case PayloadZIP:
		if depth > 1 {
			return nil, fmt.Errorf("%w: nested archive", ErrUnknownPayload)
		}
		return zipItems(data, lim, total, depth)
	case PayloadBase64:
		raw, err := base64.StdEncoding.DecodeString(stripSpace(data))
		if err != nil || depth > 1 {
			return nil, ErrUnknownPayload
		}
		return extract(name, raw, lim, total, depth+1)
	case PayloadPDF:
		return nil, fmt.Errorf("%w: pdf", ErrNotUBL)
	}
	return nil, ErrUnknownPayload
}

func xmlItems(name string, data []byte, lim Limits, total *int64) ([]Item, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	if int64(len(data)) > lim.MaxFileBytes {
		return nil, ErrTooLarge
	}
	*total += int64(len(data))
	if *total > lim.MaxTotalBytes {
		return nil, ErrTooLarge
	}
	if hasDTD(data) {
		return nil, ErrDTD
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotUBL, err)
	}
	root, err := wellFormed(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotUBL, err)
	}
	if want, ok := rootNamespace[kind]; ok && root.Space != want {
		return nil, fmt.Errorf("%w: root %s is not in namespace %s", ErrNotUBL, root.Local, want)
	}
	if kind != ubltr.KindEnvelope {
		return []Item{{Name: name, Kind: kind, XML: data}}, nil
	}
	docs, err := ubltr.EnvelopeDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("ublmap: envelope: %w", err)
	}
	out := make([]Item, 0, len(docs))
	for i, d := range docs {
		out = append(out, Item{Name: fmt.Sprintf("%s#%d", name, i+1), Kind: d.Kind, XML: d.XML})
	}
	return out, nil
}

func zipItems(data []byte, lim Limits, total *int64, depth int) ([]Item, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: zip: %v", ErrUnknownPayload, err)
	}
	if len(zr.File) > lim.MaxEntries {
		return nil, ErrTooLarge
	}
	var out []Item
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := path.Base(f.Name)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		ext := strings.ToLower(path.Ext(base))
		if ext != ".xml" && ext != ".zip" {
			continue
		}
		b, err := readEntry(f, lim.MaxFileBytes)
		if err != nil {
			out = append(out, Item{Name: f.Name, Err: err})
			continue
		}
		items, err := extract(f.Name, b, lim, total, depth+1)
		if errors.Is(err, ErrTooLarge) && *total > lim.MaxTotalBytes {
			return nil, err
		}
		if err != nil {
			out = append(out, Item{Name: f.Name, Err: err})
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func readEntry(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) { //nolint:gosec // max is positive
		return nil, ErrTooLarge
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrTooLarge
	}
	return b, nil
}

var rootNamespace = map[ubltr.Kind]string{
	ubltr.KindInvoice:    "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2",
	ubltr.KindCreditNote: "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2",
}

// wellFormed reads every token so a truncated or broken file fails here, per
// file, instead of later in the mapper. It returns the root element name.
func wellFormed(data []byte) (xml.Name, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root xml.Name
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if root.Local == "" {
				return root, errors.New("no root element")
			}
			return root, nil
		}
		if err != nil {
			return root, err
		}
		if se, ok := tok.(xml.StartElement); ok && root.Local == "" {
			root = se.Name
		}
	}
}

// hasDTD rejects any DOCTYPE (`37` §3.1). encoding/xml never fetches external
// entities, but a DTD has no place in UBL-TR and is refused outright.
func hasDTD(b []byte) bool {
	head := b
	if len(head) > 4096 {
		head = head[:4096]
	}
	return bytes.Contains(bytes.ToUpper(head), []byte("<!DOCTYPE")) || bytes.Contains(b, []byte("<!ENTITY"))
}

func stripSpace(b []byte) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, string(b))
}
