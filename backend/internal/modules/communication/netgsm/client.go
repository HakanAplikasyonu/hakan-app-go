// Package netgsm is a minimal client for the NetGSM SMS REST v2 API
// (#780 A): sender names (msgheader) for the connection test and the send
// call. Credentials are the office's API sub-user (Basic auth).
//
// Retry rule: a request that never reached NetGSM (DNS, TCP, TLS failure,
// nothing written) is retried; a connection lost after the request was
// written is NOT retried and is reported as ErrUncertain, because a retry
// could send every recipient a second SMS.
package netgsm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// DefaultBaseURL is the NetGSM API origin.
const DefaultBaseURL = "https://api.netgsm.com.tr"

// MaxRecipients is the most recipients one send request carries.
const MaxRecipients = 1000

// Credentials of the office's API sub-user.
type Credentials struct {
	Usercode string
	Password string
}

// Client calls NetGSM.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Retries is how many times a request that never reached NetGSM is
	// retried (default 2).
	Retries int
	// Wait is the pause between retries (default 1s).
	Wait time.Duration
}

// New builds a client; baseURL "" means DefaultBaseURL.
func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}, Retries: 2, Wait: time.Second}
}

var (
	// ErrUncertain: the request was written but the answer was lost.
	ErrUncertain = errors.New("netgsm: connection lost while waiting for the answer")
	// ErrUnreachable: NetGSM could not be reached (after retries).
	ErrUnreachable = errors.New("netgsm: service unreachable")
)

// APIError is a NetGSM error code answer.
type APIError struct {
	Code        string
	Description string
}

func (e *APIError) Error() string { return "netgsm: code " + e.Code + ": " + e.Description }

// Message is a Turkish explanation of the code for the screen.
func (e *APIError) Message() string { return CodeMessage(e.Code) }

// CodeMessage explains a NetGSM answer code in Turkish.
func CodeMessage(code string) string {
	switch code {
	case "00", "01", "02":
		return "Gönderim kabul edildi."
	case "20":
		return "Mesaj metni hatalı ya da izin verilen uzunluğu aşıyor."
	case "30":
		return "Kullanıcı adı veya şifre hatalı, API erişimi kapalı ya da sunucu IP adresi izinli değil."
	case "40":
		return "Gönderici adı (başlık) hesabınızda tanımlı değil."
	case "50":
		return "İYS kontrollü gönderim bu hesapta yapılamıyor."
	case "51":
		return "İYS marka bilgisi hesabınızda bulunamadı."
	case "60":
		return "Hesapta bu işlem için yetki yok."
	case "70":
		return "İstek parametreleri hatalı ya da eksik."
	case "80":
		return "Gönderim sınırı aşıldı."
	case "85":
		return "Aynı numaraya kısa sürede çok fazla mesaj gönderildi."
	case "100", "101":
		return "NetGSM tarafında sistem hatası."
	}
	if code == "" {
		return "NetGSM beklenmeyen bir cevap verdi."
	}
	return "NetGSM hata kodu " + code + "."
}

// Success reports a send-accepted code.
func Success(code string) bool { return code == "00" || code == "01" || code == "02" }

type answer struct {
	Code        string          `json:"code"`
	Description string          `json:"description"`
	JobID       string          `json:"jobid"`
	Headers     json.RawMessage `json:"msgheaders"`
	// Raw is the whole answer body (the report call reads its job list).
	Raw []byte `json:"-"`
}

// do sends one request with the retry rule of the package doc.
func (c *Client) do(ctx context.Context, cred Credentials, method, path string, body []byte) (answer, error) {
	retries := c.Retries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return answer{}, ctx.Err()
			case <-time.After(c.Wait):
			}
		}
		a, written, err := c.once(ctx, cred, method, path, body)
		if err == nil {
			return a, nil
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return a, err
		}
		if written {
			return answer{}, fmt.Errorf("%w: %v", ErrUncertain, err)
		}
		if ctx.Err() != nil {
			return answer{}, ctx.Err()
		}
		lastErr = err
	}
	return answer{}, fmt.Errorf("%w: %v", ErrUnreachable, lastErr)
}

func (c *Client) once(ctx context.Context, cred Credentials, method, path string, body []byte) (answer, bool, error) {
	written := false
	trace := &httptrace.ClientTrace{WroteRequest: func(i httptrace.WroteRequestInfo) {
		if i.Err == nil {
			written = true
		}
	}}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, c.BaseURL+path, rd)
	if err != nil {
		return answer{}, false, err
	}
	req.SetBasicAuth(cred.Usercode, cred.Password)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return answer{}, written, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return answer{}, true, err
	}
	var a answer
	if err := json.Unmarshal(raw, &a); err != nil || a.Code == "" {
		// A plain-text error answer ("30", "30 ...") is still a code.
		code := strings.Fields(strings.TrimSpace(string(raw)))
		a = answer{}
		if len(code) > 0 && len(code[0]) <= 3 {
			a.Code = code[0]
		}
		if resp.StatusCode == http.StatusUnauthorized && a.Code == "" {
			a.Code = "30"
		}
		if a.Code == "" && resp.StatusCode == http.StatusOK && json.Valid(raw) {
			// A JSON answer without a code (a bare report list): the
			// caller decides; Headers and Send treat it as an error.
			return answer{Raw: raw}, true, nil
		}
		if a.Code == "" {
			return answer{}, true, &APIError{Code: "", Description: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		}
	}
	a.Raw = raw
	return a, true, nil
}

// Headers lists the approved sender names of the account (connection test).
func (c *Client) Headers(ctx context.Context, cred Credentials) ([]string, error) {
	a, err := c.do(ctx, cred, http.MethodGet, "/sms/rest/v2/msgheader", nil)
	if err != nil {
		return nil, err
	}
	if a.Code != "00" {
		return nil, &APIError{Code: a.Code, Description: a.Description}
	}
	return parseHeaders(a.Headers), nil
}

// parseHeaders accepts ["A","B"] or [{"msgheader":"A"}] shapes.
func parseHeaders(raw json.RawMessage) []string {
	out := []string{}
	if len(raw) == 0 {
		return out
	}
	var plain []string
	if json.Unmarshal(raw, &plain) == nil {
		for _, h := range plain {
			if h = strings.TrimSpace(h); h != "" {
				out = append(out, h)
			}
		}
		return out
	}
	var objs []map[string]any
	if json.Unmarshal(raw, &objs) == nil {
		for _, o := range objs {
			for _, k := range []string{"msgheader", "header", "name"} {
				if s, ok := o[k].(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
					break
				}
			}
		}
	}
	return out
}

// SendItem is one recipient of a send request.
type SendItem struct {
	Phone string // 10 digits, 5XXXXXXXXX
	Text  string
}

// SendRequest is one send call (at most MaxRecipients items).
type SendRequest struct {
	Header   string
	Encoding string
	AppName  string
	Items    []SendItem
}

type sendBody struct {
	MsgHeader string        `json:"msgheader"`
	Messages  []sendMessage `json:"messages"`
	Encoding  string        `json:"encoding,omitempty"`
	IYSFilter string        `json:"iysfilter"`
	AppName   string        `json:"appname,omitempty"`
}

type sendMessage struct {
	Msg string `json:"msg"`
	No  string `json:"no"`
}

// Send submits the messages; the answer is NetGSM's job id. iysfilter is
// "0": informational messages, not commercial.
func (c *Client) Send(ctx context.Context, cred Credentials, in SendRequest) (string, error) {
	if len(in.Items) == 0 || len(in.Items) > MaxRecipients {
		return "", fmt.Errorf("netgsm: 1..%d recipients per request", MaxRecipients)
	}
	b := sendBody{MsgHeader: in.Header, Encoding: in.Encoding, IYSFilter: "0", AppName: in.AppName}
	for _, it := range in.Items {
		b.Messages = append(b.Messages, sendMessage{Msg: it.Text, No: it.Phone})
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	a, err := c.do(ctx, cred, http.MethodPost, "/sms/rest/v2/send", raw)
	if err != nil {
		return "", err
	}
	if !Success(a.Code) {
		return "", &APIError{Code: a.Code, Description: a.Description}
	}
	return a.JobID, nil
}
