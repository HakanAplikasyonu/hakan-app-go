// Package vakifbank is the Vakıfbank "Online Hesap Hareketleri" web service
// (22 §1, document v1.20, 01.07.2019): SOAP 1.2 / WCF with WS-Addressing,
// credentials in the body, calls only from the fixed IPs the branch defined.
//
// ⚠️ The service was not reachable from development (IP-restricted): the
// namespaces and the response shape follow the document and community code
// (22 §1 "Örnek zarf"); they are constants here so the first real answer can
// correct them in one place (CLAUDE.md §6).
package vakifbank

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
)

const (
	// TestEndpoint is the bank's test system (22 §1).
	TestEndpoint = "https://vbtestservice.vakifbank.com.tr/HesapHareketleri.OnlineEkstre/SOnlineEkstreServis.svc"
	// ProductionEndpoint ⚠️ comes from community code; the bank confirms it
	// after the test phase (22 §1 "Doğrulanacaklar").
	ProductionEndpoint = "https://vbservice.vakifbank.com.tr/HesapHareketleri.OnlineEkstre/SOnlineEkstreServis.svc"

	nsService = "Peak.Integration.ExternalInbound.Ekstre"
	nsDTO     = "http://schemas.datacontract.org/2004/07/Peak.Integration.ExternalInbound.Ekstre.DataTransferObjects"
	actionGet = nsService + "/ISOnlineEkstreServis/GetirHareket"

	// SourceSystem of movements read from this service.
	SourceSystem = "bank_vakifbank"
	// maxBody bounds the answer (20 §4 madde 8: answers can be large).
	maxBody = 64 << 20
)

var istanbul = mustLoad("Europe/Istanbul")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}

// Client implements webservice.Adapter.
type Client struct {
	Endpoint string
	// HTTP is the transport. In production it must leave from the fixed
	// egress IPs (20 §4); the caller injects that client.
	HTTP *http.Client
}

// New returns a client for endpoint (empty: TestEndpoint).
func New(endpoint string, hc *http.Client) *Client {
	if endpoint == "" {
		endpoint = TestEndpoint
	}
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{Endpoint: endpoint, HTTP: hc}
}

var _ webservice.Adapter = (*Client)(nil)

func (c *Client) BankID() string       { return "vakifbank" }
func (c *Client) BankName() string     { return "Vakıfbank" }
func (c *Client) SourceSystem() string { return SourceSystem }

// Limits: 365 days of history (VBH0010); the document gives no range limit,
// the planner uses 7-day windows (20 §7.1).
func (c *Client) Limits() webservice.Limits {
	return webservice.Limits{MaxRangeDays: 7, MaxHistoryDays: 365}
}

func (c *Client) CredentialFields() []webservice.CredentialField {
	return []webservice.CredentialField{
		{Key: "customer_no", Label: "Müşteri No", Pattern: `^\d{1,12}$`, Help: "Vakıfbank müşteri numaranız"},
		{Key: "username", Label: "Kurum Kullanıcı Adı", Help: "Bankanın talimat sonrası ilettiği servis kullanıcısı"},
		{Key: "password", Label: "Şifre", Secret: true, Help: "Bankanın ilettiği servis şifresi"},
	}
}

var accountNoRe = regexp.MustCompile(`^00158\d{12}$`)

// Fetch runs GetirHareket for one window.
func (c *Client) Fetch(ctx context.Context, creds webservice.Credentials, w webservice.Window) (webservice.Result, error) {
	if creds["customer_no"] == "" || creds["username"] == "" || creds["password"] == "" {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonBadCredentials, Message: "missing credentials"}
	}
	if w.Account != "" && !accountNoRe.MatchString(w.Account) {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonInvalidRequest, Message: "account number must start with 00158 and have 17 digits"}
	}
	body := c.envelope(creds, w)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return webservice.Result{}, err
	}
	req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+actionGet+`"`)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonBankError, Message: "transport: " + err.Error()}
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonBankError, Message: "read: " + err.Error()}
	}
	if res.StatusCode >= 400 && !bytes.Contains(raw, []byte("Envelope")) {
		// The firewall answers "The requested URL was rejected" to calls
		// without WS-Addressing or from an unknown IP (22 §1).
		reason := webservice.ReasonBankError
		if bytes.Contains(raw, []byte("rejected")) || res.StatusCode == http.StatusForbidden {
			reason = webservice.ReasonIPRejected
		}
		return webservice.Result{}, &webservice.Error{Reason: reason, Code: fmt.Sprintf("HTTP %d", res.StatusCode)}
	}
	return parse(raw)
}

func (c *Client) envelope(creds webservice.Credentials, w webservice.Window) []byte {
	var b bytes.Buffer
	esc := func(s string) string {
		var x bytes.Buffer
		_ = xml.EscapeText(&x, []byte(s))
		return x.String()
	}
	b.WriteString(`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:peak="` + nsService + `" xmlns:dto="` + nsDTO + `">`)
	b.WriteString(`<soap:Header xmlns:wsa="http://www.w3.org/2005/08/addressing">`)
	b.WriteString(`<wsa:Action>` + actionGet + `</wsa:Action><wsa:To>` + esc(c.Endpoint) + `</wsa:To></soap:Header>`)
	b.WriteString(`<soap:Body><peak:GetirHareket><peak:sorgu>`)
	b.WriteString(`<dto:MusteriNo>` + esc(creds["customer_no"]) + `</dto:MusteriNo>`)
	b.WriteString(`<dto:KurumKullanici>` + esc(creds["username"]) + `</dto:KurumKullanici>`)
	b.WriteString(`<dto:Sifre>` + esc(creds["password"]) + `</dto:Sifre>`)
	b.WriteString(`<dto:SorguBaslangicTarihi>` + w.Start.In(istanbul).Format("2006-01-02 15:04") + `</dto:SorguBaslangicTarihi>`)
	b.WriteString(`<dto:SorguBitisTarihi>` + w.End.In(istanbul).Format("2006-01-02 15:04") + `</dto:SorguBitisTarihi>`)
	if w.Account != "" {
		b.WriteString(`<dto:HesapNo>` + esc(w.Account) + `</dto:HesapNo>`)
	}
	b.WriteString(`</peak:sorgu></peak:GetirHareket></soap:Body></soap:Envelope>`)
	return b.Bytes()
}

// ---------------------------------------------------------------------------
// Answer (22 §1 "Yanıt → eşleme"). Element names are matched without
// namespace, as WCF prefixes vary.

type envelope struct {
	Body struct {
		Fault *struct {
			Reason string `xml:"Reason>Text"`
			Code   string `xml:"Code>Value"`
		} `xml:"Fault"`
		Resp struct {
			Result result `xml:"GetirHareketResult"`
		} `xml:"GetirHareketResponse"`
	} `xml:"Body"`
}

type result struct {
	IslemKodu       string     `xml:"IslemKodu"`
	IslemAciklamasi string     `xml:"IslemAciklamasi"`
	Hesaplar        []hesapXML `xml:"Hesaplar>DtoEkstreHesap"`
}

type hesapXML struct {
	HesapNo              string       `xml:"HesapNo"`
	HesapNoIban          string       `xml:"HesapNoIban"`
	DovizTipi            string       `xml:"DovizTipi"`
	SubeKodu             string       `xml:"SubeKodu"`
	SubeAdi              string       `xml:"SubeAdi"`
	MusteriUnvani        string       `xml:"MusteriUnvani"`
	VergiKimlikNumarasi  string       `xml:"VergiKimlikNumarasi"`
	HesapRumuzu          string       `xml:"HesapRumuzu"`
	HesapTipi            string       `xml:"HesapTipi"`
	HesapDurumu          string       `xml:"HesapDurumu"`
	AcilisBakiye         string       `xml:"AcilisBakiye"`
	CariBakiye           string       `xml:"CariBakiye"`
	KullanilabilirBakiye string       `xml:"KullanilabilirBakiye"`
	Hareketler           []hareketXML `xml:"Hareketler>DtoEkstreHareket"`
}

type hareketXML struct {
	IslemNo           string `xml:"IslemNo"`
	IslemTarihi       string `xml:"IslemTarihi"`
	Tutar             string `xml:"Tutar"`
	BorcAlacak        string `xml:"BorcAlacak"`
	Aciklama          string `xml:"Aciklama"`
	IslemAdi          string `xml:"IslemAdi"`
	IslemOncesiBakiye string `xml:"IslemOncesiBakiye"`
	// The document spells it with a dotless ı (22 §1); both are accepted.
	IslemSonrasiBakiye  string  `xml:"IslemSonrasıBakiye"`
	IslemSonrasiBakiye2 string  `xml:"IslemSonrasiBakiye"`
	IslemKodu           string  `xml:"IslemKodu"`
	IslemKanal          string  `xml:"IslemKanal"`
	IslemYeri           string  `xml:"IslemYeri"`
	Detaylar            details `xml:"Detaylar"`
}

// details is the key–value list; WCF writes a Dictionary as repeated
// <KeyValueOf…><Key/><Value/></KeyValueOf…> ⚠️. Any child with Key and Value
// children is read, whatever its name.
type details map[string]string

func (d *details) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	*d = details{}
	var item struct {
		Key   string `xml:"Key"`
		Value string `xml:"Value"`
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			item.Key, item.Value = "", ""
			if err := dec.DecodeElement(&item, &t); err != nil {
				return err
			}
			if item.Key != "" {
				(*d)[item.Key] = strings.TrimSpace(item.Value)
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

// codes maps the bank's result codes to reasons (22 §1 "Hata kodları").
var codes = map[string]webservice.Reason{
	"VBH0002": webservice.ReasonBankError,
	"VBH0003": webservice.ReasonInvalidRequest,
	"VBH0004": webservice.ReasonInvalidRequest,
	"VBH0005": webservice.ReasonBadCredentials,
	"VBH0006": webservice.ReasonInvalidRequest,
	"VBH0007": webservice.ReasonInvalidRequest,
	"VBH0008": webservice.ReasonInvalidRequest,
	"VBH0009": webservice.ReasonIPRejected,
	"VBH0010": webservice.ReasonInvalidRequest,
	"VBH0011": webservice.ReasonInstructionMissing,
	"VBH0012": webservice.ReasonBadCredentials,
	"VBH0013": webservice.ReasonInstructionMissing,
	"VBH0014": webservice.ReasonRateLimited,
}

// ErrTooOld is VBH0010: the window starts more than 365 days ago.
var ErrTooOld = errors.New("vakifbank: window older than 365 days")

func parse(raw []byte) (webservice.Result, error) {
	var env envelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonBankError, Message: "unreadable answer: " + err.Error()}
	}
	if f := env.Body.Fault; f != nil {
		return webservice.Result{}, &webservice.Error{Reason: webservice.ReasonBankError, Code: f.Code, Message: f.Reason}
	}
	r := env.Body.Resp.Result
	code := strings.TrimSpace(r.IslemKodu)
	if code != "" && code != "VBB0001" {
		reason, ok := codes[code]
		if !ok {
			reason = webservice.ReasonBankError
		}
		be := &webservice.Error{Reason: reason, Code: code, Message: r.IslemAciklamasi}
		if code == "VBH0010" {
			return webservice.Result{}, fmt.Errorf("%w: %w", ErrTooOld, be)
		}
		return webservice.Result{}, be
	}
	out := webservice.Result{}
	for _, h := range r.Hesaplar {
		acc, err := mapAccount(h)
		if err != nil {
			return webservice.Result{}, err
		}
		out.Accounts = append(out.Accounts, acc)
	}
	return out, nil
}

func mapAccount(h hesapXML) (webservice.Account, error) {
	acc := webservice.Account{
		Number:     strings.TrimSpace(h.HesapNo),
		IBAN:       strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(h.HesapNoIban)), " ", ""),
		Currency:   currency(h.DovizTipi),
		BranchCode: strings.TrimSpace(h.SubeKodu),
		BranchName: strings.TrimSpace(h.SubeAdi),
		Nickname:   strings.TrimSpace(h.HesapRumuzu),
		Active:     !strings.EqualFold(strings.TrimSpace(h.HesapDurumu), "false"),
		OwnerTaxID: strings.TrimSpace(h.VergiKimlikNumarasi),
		OwnerName:  strings.TrimSpace(h.MusteriUnvani),
		Opening:    amount(h.AcilisBakiye),
		Balance:    amount(h.CariBakiye),
		Available:  amount(h.KullanilabilirBakiye),
	}
	switch strings.TrimSpace(h.HesapTipi) {
	case "1":
		acc.Kind = "demand"
	case "2":
		acc.Kind = "term"
	}
	for _, m := range h.Hareketler {
		t, err := mapMovement(m, acc.Currency)
		if err != nil {
			return acc, err
		}
		acc.Movements = append(acc.Movements, t)
	}
	return acc, nil
}

// currency: the document shows ISO codes; "TL" is normalized ⚠️.
func currency(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "", "TL", "YTL":
		return "TRY"
	}
	return s
}

// amount reads "1234.56" or "1234,56" (no grouping in the document's samples).
func amount(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil
	}
	v := d.StringFixed(2)
	return &v
}

func mapMovement(m hareketXML, cur string) (banking.Transaction, error) {
	at, err := parseTime(m.Detaylar["IslemTarihZamani"], m.IslemTarihi)
	if err != nil {
		return banking.Transaction{}, &webservice.Error{Reason: webservice.ReasonBankError, Message: "movement " + m.IslemNo + ": " + err.Error()}
	}
	amt := amount(m.Tutar)
	if amt == nil {
		return banking.Transaction{}, &webservice.Error{Reason: webservice.ReasonBankError, Message: "movement " + m.IslemNo + ": unreadable amount"}
	}
	abs := strings.TrimPrefix(*amt, "-")
	dir := banking.DirIn
	switch strings.ToUpper(strings.TrimSpace(m.BorcAlacak)) {
	case "B":
		dir = banking.DirOut
	case "A":
	default:
		if strings.HasPrefix(*amt, "-") {
			dir = banking.DirOut
		}
	}
	after := m.IslemSonrasiBakiye
	if after == "" {
		after = m.IslemSonrasiBakiye2
	}
	d := m.Detaylar
	t := banking.Transaction{
		SourceSystem:     SourceSystem,
		SourceID:         strings.TrimSpace(m.IslemNo),
		CaptureMethod:    banking.CaptureAPI,
		BookedAt:         at,
		HasTime:          true,
		TransactionDate:  time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		Direction:        dir,
		Amount:           abs,
		Currency:         cur,
		BalanceBefore:    amount(m.IslemOncesiBakiye),
		BalanceAfter:     amount(after),
		Description:      strings.TrimSpace(m.Aciklama),
		DescriptionExtra: strings.TrimSpace(m.IslemAdi),
		BankReference:    first(d["Id"], d["HavaleReferansNo"]),
		BankTxnCode:      strings.TrimSpace(m.IslemKodu),
		Channel:          strings.TrimSpace(strings.Join(nonEmpty(m.IslemKanal, m.IslemYeri), " / ")),
		EFTQueryNumber:   d["EftSorguNumarasi"],
		PaymentPurpose:   d["EftIslemTuru"],
		Raw:              map[string]any{"vakifbank": m.raw()},
	}
	// Counterparty: the sender on credits, the receiver on debits (22 §1).
	if dir == banking.DirIn {
		t.CounterpartyName = d["GonderenAdi"]
		t.CounterpartyTaxID = d["GonderenKimlikNumarasi"]
		t.CounterpartyIBAN = first(d["GonderenIbanKumarasi"], d["GonderenIbanNumarasi"])
		t.CounterpartyBankCode = d["GonderenBankaKodu"]
	} else {
		t.CounterpartyName = first(d["AliciAdi"], d["TosAliciAdSoyad"])
		t.CounterpartyTaxID = first(d["AliciKimlikNumarasi"], d["TosAliciVergiNo"], d["TosAliciTcKimlikNo"])
		t.CounterpartyIBAN = first(d["AliciIbanKumarasi"], d["AliciIbanNumarasi"])
		t.CounterpartyBankCode = d["AliciBankaKodu"]
	}
	return t, nil
}

func (m hareketXML) raw() map[string]any {
	out := map[string]any{
		"IslemNo": m.IslemNo, "IslemTarihi": m.IslemTarihi, "Tutar": m.Tutar, "BorcAlacak": m.BorcAlacak,
		"IslemKodu": m.IslemKodu, "IslemKanal": m.IslemKanal, "IslemYeri": m.IslemYeri,
	}
	if len(m.Detaylar) > 0 {
		dd := map[string]any{}
		for k, v := range m.Detaylar {
			dd[k] = v
		}
		out["Detaylar"] = dd
	}
	return out
}

// parseTime prefers the precise IslemTarihZamani, then IslemTarihi
// ("yyyy-MM-dd HH:mm:ss"); both are Türkiye wall clock.
func parseTime(precise, plain string) (time.Time, error) {
	for _, s := range []string{precise, plain} {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "02.01.2006 15:04:05"} {
			if t, err := time.ParseInLocation(layout, s, istanbul); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unreadable date %q", plain)
}

func first(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func nonEmpty(vs ...string) []string {
	var out []string
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
