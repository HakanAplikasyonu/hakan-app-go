package edm

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// Wire types of the EDM Bilişim EFaturaEDM service (WCF, SOAP 1.1,
// document/literal). Shapes follow the WSDL served by the test system
// (test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc?singleWsdl,
// checked 2026-11-22): request and response roots are in the
// http://tempuri.org/ namespace (elementFormDefault="qualified"), their
// children are form="unqualified". Only the elements this read-only
// module uses are declared; order follows the XSD sequences.

const nsTempuri = "http://tempuri.org/"

// SOAP actions as published in the WSDL binding.
const (
	actionLogin      = "LoginRequest"
	actionLogout     = "LogoutRequest"
	actionGetInvoice = "GetInvoiceRequest"
)

// requestHeader is REQUEST_HEADERType (subset, XSD order).
type requestHeader struct {
	SessionID       string `xml:"SESSION_ID"`
	ClientTxnID     string `xml:"CLIENT_TXN_ID,omitempty"`
	ActionDate      string `xml:"ACTION_DATE,omitempty"`
	ApplicationName string `xml:"APPLICATION_NAME,omitempty"`
	ChannelName     string `xml:"CHANNEL_NAME,omitempty"`
	Compressed      string `xml:"COMPRESSED,omitempty"`
}

type loginRequest struct {
	XMLName  xml.Name      `xml:"tem:LoginRequest"`
	NS       string        `xml:"xmlns:tem,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	UserName string        `xml:"USER_NAME"`
	Password string        `xml:"PASSWORD"`
}

type loginResponse struct {
	SessionID string `xml:"SESSION_ID"`
}

type logoutRequest struct {
	XMLName xml.Name      `xml:"tem:LogoutRequest"`
	NS      string        `xml:"xmlns:tem,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
}

// searchKey is GetInvoiceRequest/INVOICE_SEARCH_KEY (subset, XSD order).
type searchKey struct {
	Limit        int    `xml:"LIMIT,omitempty"`
	UUID         string `xml:"UUID,omitempty"`
	ReadIncluded string `xml:"READ_INCLUDED,omitempty"`
	Direction    string `xml:"DIRECTION,omitempty"`
	CRStartDate  string `xml:"CR_START_DATE,omitempty"`
	CREndDate    string `xml:"CR_END_DATE,omitempty"`
	Offset       *int   `xml:"OFFSET,omitempty"`
}

type getInvoiceRequest struct {
	XMLName     xml.Name      `xml:"tem:GetInvoiceRequest"`
	NS          string        `xml:"xmlns:tem,attr"`
	Header      requestHeader `xml:"REQUEST_HEADER"`
	SearchKey   searchKey     `xml:"INVOICE_SEARCH_KEY"`
	HeaderOnly  string        `xml:"HEADER_ONLY"`
	ContentType string        `xml:"INVOICE_CONTENT_TYPE"`
}

type amount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}

// invoiceHeader is INVOICE/HEADER (subset).
type invoiceHeader struct {
	Sender        string `xml:"SENDER"`
	Receiver      string `xml:"RECEIVER"`
	Supplier      string `xml:"SUPPLIER"`
	Customer      string `xml:"CUSTOMER"`
	IssueDate     string `xml:"ISSUE_DATE"`
	Payable       amount `xml:"PAYABLE_AMOUNT"`
	ProfileID     string `xml:"PROFILEID"`
	Status        string `xml:"STATUS"`
	StatusDesc    string `xml:"STATUS_DESCRIPTION"`
	GIBStatusCode string `xml:"GIB_STATUS_CODE"`
	ResponseCode  string `xml:"RESPONSE_CODE"`
	CDate         string `xml:"CDATE"`
	EArchive      string `xml:"EARCHIVE"`
	InvoiceType   string `xml:"INVOICE_TYPE"`
	Direction     string `xml:"DIRECTION"`
}

type invoice struct {
	ID      string        `xml:"ID,attr"`
	UUID    string        `xml:"UUID,attr"`
	Header  invoiceHeader `xml:"HEADER"`
	Content string        `xml:"CONTENT"`
}

// getInvoiceResponse is GetInvoiceResponse (ArrayOfINVOICE).
type getInvoiceResponse struct {
	Invoices []invoice `xml:"INVOICE"`
}

// requestError is REQUEST_ERRORType, carried in a RequestFault detail.
type requestError struct {
	Code  string
	Short string
	Long  string
}

func (e *requestError) message() string {
	s := strings.TrimSpace(e.Short)
	if l := strings.TrimSpace(e.Long); l != "" && l != s {
		if s == "" {
			return l
		}
		s += " (" + l + ")"
	}
	return s
}

// parseFault reads ERROR_CODE / ERROR_SHORT_DES / ERROR_LONG_DES from a
// SOAP fault detail (RequestFault). It returns nil when the detail holds
// no REQUEST_ERRORType fields.
func parseFault(detail string) *requestError {
	if strings.TrimSpace(detail) == "" {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader([]byte("<d>" + detail + "</d>")))
	dec.Strict = false
	var out requestError
	found := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) || err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var dst *string
		switch se.Name.Local {
		case "ERROR_CODE":
			dst = &out.Code
		case "ERROR_SHORT_DES":
			dst = &out.Short
		case "ERROR_LONG_DES":
			dst = &out.Long
		default:
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err != nil {
			break
		}
		if *dst == "" {
			*dst = strings.TrimSpace(v)
			found = true
		}
	}
	if !found {
		return nil
	}
	return &out
}
