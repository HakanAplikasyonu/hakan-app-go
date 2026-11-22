package exchangerates

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// DefaultBaseURL is the TCMB rates feed: today.xml and the archive
// YYYYMM/DDMMYYYY.xml.
const DefaultBaseURL = "https://www.tcmb.gov.tr/kurlar"

// ErrNotPublished: TCMB has no bulletin for that day (weekend or holiday,
// the archive answers 404).
var ErrNotPublished = errors.New("exchangerates: no TCMB bulletin for that day")

// Bulletin is one TCMB daily bulletin.
type Bulletin struct {
	Date  time.Time
	Rates []Rate
}

type xmlBulletin struct {
	XMLName    xml.Name      `xml:"Tarih_Date"`
	Tarih      string        `xml:"Tarih,attr"`
	Date       string        `xml:"Date,attr"`
	Currencies []xmlCurrency `xml:"Currency"`
}

type xmlCurrency struct {
	Code            string `xml:"CurrencyCode,attr"`
	Kod             string `xml:"Kod,attr"`
	Unit            string `xml:"Unit"`
	ForexBuying     string `xml:"ForexBuying"`
	ForexSelling    string `xml:"ForexSelling"`
	BanknoteBuying  string `xml:"BanknoteBuying"`
	BanknoteSelling string `xml:"BanknoteSelling"`
}

// Parse reads a TCMB bulletin (today.xml or an archive day). Currencies
// without any value and malformed numbers are left out; a bulletin
// without a date or without currencies is an error.
func Parse(data []byte) (Bulletin, error) {
	var x xmlBulletin
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	if err := dec.Decode(&x); err != nil {
		return Bulletin{}, fmt.Errorf("exchangerates: parse TCMB xml: %w", err)
	}
	d, err := bulletinDate(x)
	if err != nil {
		return Bulletin{}, err
	}
	b := Bulletin{Date: d}
	for _, c := range x.Currencies {
		code := strings.ToUpper(strings.TrimSpace(c.Code))
		if code == "" {
			code = strings.ToUpper(strings.TrimSpace(c.Kod))
		}
		if len(code) != 3 {
			continue
		}
		unit, err := strconv.Atoi(strings.TrimSpace(c.Unit))
		if err != nil || unit <= 0 {
			unit = 1
		}
		r := Rate{
			Date: d, Currency: code, Unit: unit,
			ForexBuying: num(c.ForexBuying), ForexSelling: num(c.ForexSelling),
			BanknoteBuying: num(c.BanknoteBuying), BanknoteSelling: num(c.BanknoteSelling),
		}
		if r.ForexBuying == nil && r.ForexSelling == nil && r.BanknoteBuying == nil && r.BanknoteSelling == nil {
			continue
		}
		b.Rates = append(b.Rates, r)
	}
	if len(b.Rates) == 0 {
		return Bulletin{}, errors.New("exchangerates: TCMB bulletin has no rates")
	}
	return b, nil
}

// bulletinDate reads Tarih="21.11.2026" or, failing that, Date="11/21/2026".
func bulletinDate(x xmlBulletin) (time.Time, error) {
	if t, err := time.Parse("02.01.2006", strings.TrimSpace(x.Tarih)); err == nil {
		return t, nil
	}
	if t, err := time.Parse("01/02/2006", strings.TrimSpace(x.Date)); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("exchangerates: TCMB bulletin has no date (%q)", x.Tarih)
}

// num parses a TCMB value ("34.5012"); empty or invalid is nil.
func num(s string) *decimal.Decimal {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil || !d.IsPositive() {
		return nil
	}
	return &d
}

// charsetReader accepts the ISO-8859-9 / windows-1254 declarations of older
// archive files (the Turkish currency names); UTF-8 passes through.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "":
		return in, nil
	case "iso-8859-9", "windows-1254", "latin5", "cp1254":
		data, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(latin5ToUTF8(data)), nil
	}
	return nil, fmt.Errorf("exchangerates: unsupported charset %q", label)
}

// latin5 differs from Latin-1 in six letters.
var latin5 = map[byte]rune{0xD0: 'Ğ', 0xDD: 'İ', 0xDE: 'Ş', 0xF0: 'ğ', 0xFD: 'ı', 0xFE: 'ş'}

func latin5ToUTF8(in []byte) []byte {
	out := make([]byte, 0, len(in)+len(in)/8)
	for _, b := range in {
		r, ok := latin5[b]
		if !ok {
			r = rune(b)
		}
		out = utf8.AppendRune(out, r)
	}
	return out
}

// Doer sends HTTP requests (*http.Client).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads the TCMB feed. HTTP is the worker's outbound client (the
// egress proxy setting, EGRESS_PROXY_URL / EGRESS_PROXY_HOSTS).
type Client struct {
	HTTP    Doer
	BaseURL string
}

// Today fetches today.xml (the latest bulletin; before 15:30 the previous
// business day's).
func (c Client) Today(ctx context.Context) (Bulletin, error) {
	return c.get(ctx, c.base()+"/today.xml")
}

// Day fetches the archive bulletin of d; ErrNotPublished on 404.
func (c Client) Day(ctx context.Context, d time.Time) (Bulletin, error) {
	return c.get(ctx, ArchiveURL(c.base(), d))
}

// ArchiveURL is base/YYYYMM/DDMMYYYY.xml.
func ArchiveURL(base string, d time.Time) string {
	return fmt.Sprintf("%s/%s/%s.xml", strings.TrimRight(base, "/"), d.Format("200601"), d.Format("02012006"))
}

func (c Client) base() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c Client) get(ctx context.Context, url string) (Bulletin, error) {
	if c.HTTP == nil {
		return Bulletin{}, errors.New("exchangerates: no HTTP client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Bulletin{}, err
	}
	req.Header.Set("Accept", "application/xml, text/xml")
	req.Header.Set("User-Agent", "hakan-app-rates/1.0")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Bulletin{}, fmt.Errorf("exchangerates: GET %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		return Bulletin{}, ErrNotPublished
	}
	if res.StatusCode != http.StatusOK {
		return Bulletin{}, fmt.Errorf("exchangerates: GET %s: status %d", url, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return Bulletin{}, fmt.Errorf("exchangerates: read %s: %w", url, err)
	}
	return Parse(data)
}
