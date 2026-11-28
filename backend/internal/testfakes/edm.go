package testfakes

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EDM web-service user the fake accepts (test values, not a real account).
const (
	EDMUsername = "edm_ws"
	EDMPassword = "edm-fake-pw"
	edmSession  = "edm-fake-session"
)

// EDMInvoice is one invoice of the fake's inbox or outbox.
type EDMInvoice struct {
	UUID, ID, Direction string // Direction: IN or OUT
	Sender, Receiver    string
	Supplier, Customer  string
	IssueDate, CDate    string
	Amount              string
	Status              string // e.g. "RECEIVE - SUCCEED"
	EArchive            bool
	UBL                 []byte
}

// EDM fakes the EDM Bilişim EFaturaEDM SOAP service (Login → SESSION_ID,
// GetInvoice with DIRECTION / HEADER_ONLY / OFFSET / LIMIT /
// CR_START_DATE / CR_END_DATE, Logout). A wrong user answers the
// RequestFault the real service gives; a call with an unknown session
// answers a session fault. CDATE values without a zone are Türkiye time;
// the creation window is inclusive at both ends.
type EDM struct {
	mu       sync.Mutex
	Invoices []EDMInvoice
	// Down answers 503 to every call (service not answering).
	Down     bool
	Logins   int
	Logouts  int
	Requests int
	LastBody string
	// Calls logs every GetInvoice: "list IN 2026-11-01T00:00:00+03:00 0"
	// (direction, CR_START_DATE, OFFSET) or "fetch <UUID>".
	Calls []string
}

// EDMSample is a fake with one incoming and one outgoing e-Fatura.
func EDMSample() *EDM {
	ubl := func(id string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" ` +
			`xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"><cbc:ID>` + id + `</cbc:ID></Invoice>`)
	}
	return &EDM{Invoices: []EDMInvoice{
		{UUID: "5f0c7a52-3a51-4c43-9c3b-6c1b0f0e0a01", ID: "EDM2026000000001", Direction: "IN",
			Sender: "1234567801", Supplier: "Tedarikçi A.Ş.", Receiver: "9000000017", Customer: "Atlas Ltd.",
			IssueDate: "2026-11-03", CDate: "2026-11-03T10:15:00", Amount: "1180.00", Status: "RECEIVE - SUCCEED", UBL: ubl("EDM2026000000001")},
		{UUID: "5f0c7a52-3a51-4c43-9c3b-6c1b0f0e0a02", ID: "ATL2026000000007", Direction: "OUT",
			Sender: "9000000017", Supplier: "Atlas Ltd.", Receiver: "1234567802", Customer: "Müşteri A.Ş.",
			IssueDate: "2026-11-04", CDate: "2026-11-04T09:00:00", Amount: "2360.00", Status: "SEND - SUCCEED", UBL: ubl("ATL2026000000007")},
	}}
}

// edmRequest holds every field of the requests the fake reads.
type edmRequest struct {
	XMLName  xml.Name
	Session  string `xml:"REQUEST_HEADER>SESSION_ID"`
	User     string `xml:"USER_NAME"`
	Password string `xml:"PASSWORD"`
	Key      struct {
		Limit     int    `xml:"LIMIT"`
		UUID      string `xml:"UUID"`
		Direction string `xml:"DIRECTION"`
		CRStart   string `xml:"CR_START_DATE"`
		CREnd     string `xml:"CR_END_DATE"`
		Offset    int    `xml:"OFFSET"`
	} `xml:"INVOICE_SEARCH_KEY"`
	HeaderOnly string `xml:"HEADER_ONLY"`
}

func edmDecode(body []byte) (edmRequest, error) {
	var env struct {
		Body struct {
			Req edmRequest `xml:",any"`
		} `xml:"Body"`
	}
	err := xml.Unmarshal(body, &env)
	return env.Body.Req, err
}

func edmFault(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault>`+
		`<faultcode>s:Client</faultcode><faultstring xml:lang="tr-TR">%s</faultstring><detail>`+
		`<RequestFault xmlns="http://tempuri.org/" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">`+
		`<INTL_TXN_ID xmlns="">0</INTL_TXN_ID><ERROR_CODE xmlns="">%s</ERROR_CODE><ERROR_SHORT_DES xmlns="">%s</ERROR_SHORT_DES>`+
		`</RequestFault></detail></s:Fault></s:Body></s:Envelope>`, html.EscapeString(msg), code, html.EscapeString(msg))
}

func edmOK(w http.ResponseWriter, inner string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` + inner + `</s:Body></s:Envelope>`))
}

func (f *EDM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests++
	var b bytes.Buffer
	_, _ = b.ReadFrom(r.Body)
	f.LastBody = b.String()
	if f.Down {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	req, err := edmDecode(b.Bytes())
	if err != nil {
		edmFault(w, "1000", "İstek okunamadı")
		return
	}
	switch req.XMLName.Local {
	case "LoginRequest":
		if req.User != EDMUsername || req.Password != EDMPassword {
			edmFault(w, "2001", "Kullanıcı adı veya şifre hatalı")
			return
		}
		f.Logins++
		edmOK(w, `<LoginResponse xmlns="http://tempuri.org/"><REQUEST_RETURN xmlns=""><INTL_TXN_ID>1</INTL_TXN_ID><RETURN_CODE>0</RETURN_CODE></REQUEST_RETURN>`+
			`<SESSION_ID xmlns="">`+edmSession+`</SESSION_ID></LoginResponse>`)
	case "LogoutRequest":
		f.Logouts++
		edmOK(w, `<LogoutResponse xmlns="http://tempuri.org/"><REQUEST_RETURN xmlns=""><INTL_TXN_ID>2</INTL_TXN_ID><RETURN_CODE>0</RETURN_CODE></REQUEST_RETURN></LogoutResponse>`)
	case "GetInvoiceRequest":
		if req.Session != edmSession {
			edmFault(w, "2005", "Aktif session bulunamadı")
			return
		}
		f.getInvoice(w, req)
	default:
		edmFault(w, "1001", "Bilinmeyen işlem")
	}
}

var edmZone = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*60*60)
}()

func edmTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", s, edmZone)
	return t, err == nil
}

func (f *EDM) getInvoice(w http.ResponseWriter, req edmRequest) {
	if req.Key.UUID != "" {
		f.Calls = append(f.Calls, "fetch "+req.Key.UUID)
	} else {
		f.Calls = append(f.Calls, fmt.Sprintf("list %s %s %d", req.Key.Direction, req.Key.CRStart, req.Key.Offset))
	}
	from, hasFrom := edmTime(req.Key.CRStart)
	to, hasTo := edmTime(req.Key.CREnd)
	var match []EDMInvoice
	for _, inv := range f.Invoices {
		if req.Key.UUID != "" && !strings.EqualFold(inv.UUID, req.Key.UUID) {
			continue
		}
		if req.Key.Direction != "" && inv.Direction != req.Key.Direction {
			continue
		}
		if c, ok := edmTime(inv.CDate); ok && (hasFrom && c.Before(from) || hasTo && c.After(to)) {
			continue
		}
		match = append(match, inv)
	}
	if req.Key.Offset > 0 {
		if req.Key.Offset >= len(match) {
			match = nil
		} else {
			match = match[req.Key.Offset:]
		}
	}
	if req.Key.Limit > 0 && len(match) > req.Key.Limit {
		match = match[:req.Key.Limit]
	}
	var sb strings.Builder
	sb.WriteString(`<GetInvoiceResponse xmlns="http://tempuri.org/">`)
	for i, inv := range match {
		fmt.Fprintf(&sb, `<INVOICE xmlns="" TRXID="%d" UUID="%s" ID="%s"><HEADER>`, i+1, inv.UUID, inv.ID)
		fmt.Fprintf(&sb, `<SENDER>%s</SENDER><RECEIVER>%s</RECEIVER><SUPPLIER>%s</SUPPLIER><CUSTOMER>%s</CUSTOMER>`,
			inv.Sender, inv.Receiver, html.EscapeString(inv.Supplier), html.EscapeString(inv.Customer))
		fmt.Fprintf(&sb, `<ISSUE_DATE>%s</ISSUE_DATE><PAYABLE_AMOUNT currencyID="TRY">%s</PAYABLE_AMOUNT><PROFILEID>TEMELFATURA</PROFILEID>`,
			inv.IssueDate, inv.Amount)
		fmt.Fprintf(&sb, `<STATUS>%s</STATUS><GIB_STATUS_CODE>1300</GIB_STATUS_CODE><CDATE>%s</CDATE>`, inv.Status, inv.CDate)
		fmt.Fprintf(&sb, `<INTERNETSALES>false</INTERNETSALES><EARCHIVE>%s</EARCHIVE><INVOICE_TYPE>SATIS</INVOICE_TYPE><DIRECTION>%s</DIRECTION>`,
			strconv.FormatBool(inv.EArchive), inv.Direction)
		sb.WriteString(`</HEADER>`)
		if req.HeaderOnly == "N" {
			sb.WriteString(`<CONTENT>` + base64.StdEncoding.EncodeToString(inv.UBL) + `</CONTENT>`)
		}
		sb.WriteString(`</INVOICE>`)
	}
	sb.WriteString(`</GetInvoiceResponse>`)
	edmOK(w, sb.String())
}
