package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
)

// Inspected is a message read back with the standard library only
// (net/mail, mime/multipart): what a receiving client would see. Tests and
// the template preview use it to check the MIME tree Compose produced.
type Inspected struct {
	Header netmail.Header
	// Subject is the decoded (RFC 2047) subject.
	Subject string
	// Tree is the MIME structure, e.g.
	// "multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]".
	Tree string
	// Text and HTML are the decoded bodies (CRLF turned into LF).
	Text, HTML string
	// Attachments by file name.
	Attachments map[string][]byte
}

// Inspect parses a raw RFC 5322 message.
func Inspect(raw []byte) (Inspected, error) {
	m, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Inspected{}, err
	}
	in := Inspected{Header: m.Header, Attachments: map[string][]byte{}}
	if in.Subject, err = new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); err != nil {
		return Inspected{}, fmt.Errorf("subject: %w", err)
	}
	in.Tree, err = in.walk(m.Header.Get("Content-Type"), m.Header.Get("Content-Transfer-Encoding"), "", m.Body)
	return in, err
}

func (in *Inspected) walk(ctype, cte, filename string, body io.Reader) (string, error) {
	mt, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		return "", fmt.Errorf("content type %q: %w", ctype, err)
	}
	if strings.HasPrefix(mt, "multipart/") {
		r := multipart.NewReader(body, params["boundary"])
		var kids []string
		for {
			part, err := r.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("%s: %w", mt, err)
			}
			// NextPart decodes quoted-printable itself and drops the header.
			kid, err := in.walk(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.FileName(), part)
			if err != nil {
				return "", err
			}
			kids = append(kids, kid)
		}
		return mt + "[" + strings.Join(kids, ",") + "]", nil
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(cte) {
	case "quoted-printable":
		if data, err = io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err != nil {
			return "", fmt.Errorf("quoted-printable: %w", err)
		}
	case "base64":
		if data, err = base64.StdEncoding.DecodeString(strings.NewReplacer("\r", "", "\n", "").Replace(string(data))); err != nil {
			return "", fmt.Errorf("base64: %w", err)
		}
	}
	if cs := params["charset"]; cs != "" && !strings.EqualFold(cs, "utf-8") {
		return "", fmt.Errorf("charset %q, want UTF-8", cs)
	}
	switch {
	case filename != "":
		in.Attachments[filename] = data
	case mt == "text/plain":
		in.Text = strings.ReplaceAll(string(data), "\r\n", "\n")
	case mt == "text/html":
		in.HTML = strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	return mt, nil
}
