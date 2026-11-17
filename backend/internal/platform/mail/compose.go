package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// ComposeOptions are the envelope values Compose cannot take from the
// message: the sender identity and, for golden tests, fixed values for the
// parts that are random or time based in production.
type ComposeOptions struct {
	// From is the sender address (MAIL_FROM); FromName its display name.
	From     string
	FromName string
	// ReplyTo is the default Reply-To (MAIL_REPLY_TO); Message.ReplyTo wins.
	ReplyTo string
	// Now stamps the Date header; zero = time.Now().
	Now time.Time
	// MessageID overrides the generated Message-ID (without angle brackets).
	MessageID string
	// Boundary returns the boundary for the n-th multipart (0, 1, ...);
	// nil = random boundaries.
	Boundary func(n int) string
}

// ErrInvalidHeader is returned when a header value would break the message
// (CR/LF in an address, a non-absolute unsubscribe URL).
var ErrInvalidHeader = errors.New("mail: invalid header value")

// Compose renders msg as an RFC 5322 message.
//
// MIME tree:
//
//	text only:          text/plain
//	text + HTML:        multipart/alternative { text/plain, text/html }
//	with attachments:   multipart/mixed { <one of the above>, attachment... }
//
// The plain-text part always comes first inside alternative so clients that
// prefer the last part they can show pick the HTML. Text parts are
// quoted-printable: safe on 7-bit relays and against the 998-octet line
// limit regardless of how long an HTML line is.
func Compose(msg Message, opts ComposeOptions) ([]byte, error) {
	if len(msg.To) == 0 {
		return nil, fmt.Errorf("mail: recipient required")
	}
	from := strings.TrimSpace(opts.From)
	if from == "" {
		from = "noreply@localhost"
	}
	for _, v := range append([]string{from, opts.FromName, msg.ReplyTo, opts.ReplyTo, msg.ListUnsubscribe}, msg.To...) {
		if strings.ContainsAny(v, "\r\n") {
			return nil, ErrInvalidHeader
		}
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	id := opts.MessageID
	if id == "" {
		var err error
		if id, err = newMessageID(from); err != nil {
			return nil, err
		}
	}

	h := headerWriter{}
	h.add("From", (&netmail.Address{Name: opts.FromName, Address: from}).String())
	h.add("To", strings.Join(msg.To, ", "))
	if rt := firstNonEmpty(msg.ReplyTo, opts.ReplyTo); rt != "" {
		addr, err := netmail.ParseAddress(rt)
		if err != nil {
			return nil, ErrInvalidHeader
		}
		h.add("Reply-To", addr.String())
	}
	h.add("Subject", mime.QEncoding.Encode("utf-8", stripHeaderBreaks(msg.Subject)))
	h.add("Date", now.Format(time.RFC1123Z))
	h.add("Message-ID", "<"+id+">")
	h.add("MIME-Version", "1.0")
	if u := strings.TrimSpace(msg.ListUnsubscribe); u != "" {
		parsed, err := url.Parse(u)
		if err != nil || !parsed.IsAbs() || parsed.Host == "" {
			return nil, ErrInvalidHeader
		}
		h.add("List-Unsubscribe", "<"+u+">")
		// RFC 8058 one-click: only over HTTPS; mail providers ignore it
		// otherwise and some flag the mismatch.
		if parsed.Scheme == "https" {
			h.add("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
		}
	}
	c := composer{boundary: opts.Boundary}
	var body bytes.Buffer
	var top textproto.MIMEHeader
	var err error
	if len(msg.Attachments) == 0 {
		top, err = c.content(&body, msg)
	} else {
		top, err = c.mixed(&body, msg)
	}
	if err != nil {
		return nil, fmt.Errorf("mail: %w", err)
	}
	for _, k := range []string{"Content-Type", "Content-Transfer-Encoding"} {
		if v := top.Get(k); v != "" {
			h.add(k, v)
		}
	}
	var out bytes.Buffer
	out.WriteString(h.String())
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

type headerWriter struct{ b strings.Builder }

func (h *headerWriter) add(k, v string) {
	h.b.WriteString(k)
	h.b.WriteString(": ")
	h.b.WriteString(v)
	h.b.WriteString("\r\n")
}

func (h *headerWriter) String() string { return h.b.String() }

type composer struct {
	boundary func(int) string
	n        int
}

func (c *composer) newWriter(w io.Writer) (*multipart.Writer, error) {
	mw := multipart.NewWriter(w)
	if c.boundary != nil {
		if err := mw.SetBoundary(c.boundary(c.n)); err != nil {
			return nil, err
		}
	}
	c.n++
	return mw, nil
}

// content writes the readable body (text, or text + HTML alternative) and
// returns the headers that describe it.
func (c *composer) content(w io.Writer, msg Message) (textproto.MIMEHeader, error) {
	if strings.TrimSpace(msg.HTML) == "" {
		if err := writeQP(w, msg.Body); err != nil {
			return nil, err
		}
		return textPartHeader("text/plain"), nil
	}
	mw, err := c.newWriter(w)
	if err != nil {
		return nil, err
	}
	for _, p := range []struct{ ct, body string }{{"text/plain", msg.Body}, {"text/html", msg.HTML}} {
		pw, err := mw.CreatePart(textPartHeader(p.ct))
		if err != nil {
			return nil, err
		}
		if err := writeQP(pw, p.body); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return textproto.MIMEHeader{"Content-Type": {`multipart/alternative; boundary="` + mw.Boundary() + `"`}}, nil
}

// mixed writes multipart/mixed: the readable body first, then every
// attachment base64-encoded in 76-character lines.
func (c *composer) mixed(w io.Writer, msg Message) (textproto.MIMEHeader, error) {
	mw, err := c.newWriter(w)
	if err != nil {
		return nil, err
	}
	var inner bytes.Buffer
	innerHeader, err := c.content(&inner, msg)
	if err != nil {
		return nil, err
	}
	pw, err := mw.CreatePart(innerHeader)
	if err != nil {
		return nil, err
	}
	if _, err := pw.Write(inner.Bytes()); err != nil {
		return nil, err
	}
	for _, a := range msg.Attachments {
		if err := writeAttachment(mw, a); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return textproto.MIMEHeader{"Content-Type": {`multipart/mixed; boundary="` + mw.Boundary() + `"`}}, nil
}

func textPartHeader(ct string) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Type":              {ct + "; charset=UTF-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	}
}

func writeQP(w io.Writer, s string) error {
	// Normalise to CRLF so the encoder emits canonical line breaks.
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(s)); err != nil {
		return err
	}
	return qp.Close()
}

func writeAttachment(mw *multipart.Writer, a Attachment) error {
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	name := stripHeaderBreaks(a.Name)
	if name == "" {
		name = "attachment"
	}
	disp := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disp == "" {
		disp = `attachment; filename="attachment"`
	}
	ctype := mime.FormatMediaType(ct, map[string]string{"name": name})
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	part, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {ctype},
		"Content-Disposition":       {disp},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return err
	}
	enc := base64.StdEncoding.EncodeToString(a.Data)
	for len(enc) > 76 {
		if _, err := io.WriteString(part, enc[:76]+"\r\n"); err != nil {
			return err
		}
		enc = enc[76:]
	}
	_, err = io.WriteString(part, enc+"\r\n")
	return err
}

// newMessageID is <random@domain-of-from>: unique, and on the sender's own
// domain so it aligns with SPF/DKIM (a localhost or random domain is a spam
// signal).
func newMessageID(from string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mail: message id: %w", err)
	}
	domain := "localhost"
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i < len(from)-1 {
		domain = strings.Trim(from[i+1:], "<> ")
	}
	return hex.EncodeToString(b[:]) + "@" + domain, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
