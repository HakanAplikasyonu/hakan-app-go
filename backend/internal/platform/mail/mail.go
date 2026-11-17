package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
)

// defaultTimeout bounds a send when none is configured.
const defaultTimeout = 10 * time.Second

// Message is an outbound email.
type Message struct {
	To      []string
	Subject string
	// Body is the text/plain part. It is written to be read as is (not
	// derived from HTML): every message carries it.
	Body string
	// HTML is the optional text/html alternative (see package mailtpl).
	// When set, the message is multipart/alternative.
	HTML string
	// ReplyTo overrides the configured MAIL_REPLY_TO (e.g. the office that
	// sent a letter).
	ReplyTo string
	// ListUnsubscribe is the one-click unsubscribe URL of a notification
	// email (RFC 2369 / RFC 8058). Empty for transactional mail: sign-in
	// codes, invitations, letters.
	ListUnsubscribe string
	// Attachments are sent as multipart/mixed parts after the readable body
	// (issue #386: the taxpayer report PDF).
	Attachments []Attachment
}

// Attachment is one file attached to a message.
type Attachment struct {
	// Name is the file name the recipient sees (UTF-8 allowed).
	Name        string
	ContentType string
	Data        []byte
}

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// SMTPSender sends mail via SMTP (MailHog in development).
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	replyTo  string
	timeout  time.Duration
	// tlsMode is one of auto|starttls|implicit|none (see config.SMTPConfig).
	tlsMode  string
	insecure bool
}

// NewSMTPSender builds a sender from config.
func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if mode == "" {
		mode = "auto"
	}
	return &SMTPSender{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
		fromName: cfg.FromName,
		replyTo:  cfg.ReplyTo,
		timeout:  timeout,
		tlsMode:  mode,
		insecure: cfg.TLSInsecureSkipVerify,
	}
}

// Send composes msg (see Compose) and delivers it.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if s == nil {
		return fmt.Errorf("mail: sender is nil")
	}
	payload, err := Compose(msg, s.options())
	if err != nil {
		return err
	}
	if err := s.deliver(ctx, s.envelopeFrom(), msg.To, payload); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

func (s *SMTPSender) envelopeFrom() string {
	if s.from == "" {
		return "noreply@localhost"
	}
	return s.from
}

func (s *SMTPSender) options() ComposeOptions {
	return ComposeOptions{From: s.envelopeFrom(), FromName: s.fromName, ReplyTo: s.replyTo}
}

// deliver opens an SMTP connection (with timeout and the configured transport
// security) and sends one message. net/smtp's SendMail offers neither a
// timeout nor implicit TLS, so the client flow is driven explicitly here.
func (s *SMTPSender) deliver(ctx context.Context, from string, to []string, payload []byte) error {
	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	// Overall deadline: the smaller of the configured timeout and any deadline
	// already on the context.
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := &net.Dialer{Timeout: timeout}
	// Bound dial + handshake by the same deadline and make them cancellable.
	dctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Resolve transport: implicit TLS wraps the socket immediately (port 465);
	// opportunistic/required STARTTLS upgrades a plaintext connection.
	mode := s.tlsMode
	implicit := mode == "implicit" || (mode == "auto" && s.port == 465)
	useSTARTTLS := mode == "starttls" || (mode == "auto" && !implicit)
	requireSTARTTLS := mode == "starttls"

	tlsConfig := &tls.Config{
		ServerName:         s.host,
		InsecureSkipVerify: s.insecure, //nolint:gosec // guarded: config refuses this outside development
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	var err error
	if implicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(dctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(dctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	if useSTARTTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if requireSTARTTLS {
			return fmt.Errorf("server at %s does not advertise STARTTLS", addr)
		}
		// In "auto" mode a server that does not advertise STARTTLS is spoken to
		// in plaintext (opportunistic). An on-path attacker could strip the
		// advertisement, so production should set SMTP_TLS=starttls or implicit
		// rather than rely on auto.
	}

	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt to: %w", err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := wc.Write(payload); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	// The server has accepted the message at this point; a failed QUIT (e.g.
	// the server drops the connection) must not report the send as failed, or
	// the queue would retry and deliver a duplicate.
	_ = c.Quit()
	return nil
}

func stripHeaderBreaks(v string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(v)
}

// NoopSender discards mail (tests).
type NoopSender struct{}

// Send implements Sender.
func (NoopSender) Send(context.Context, Message) error { return nil }

// Recorder keeps every message in memory instead of sending it: tests and
// the template preview use it so nothing reaches a real mailbox.
type Recorder struct {
	mu   sync.Mutex
	sent []Message
}

// Send implements Sender.
func (r *Recorder) Send(_ context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return fmt.Errorf("mail: recipient required")
	}
	if _, err := Compose(msg, ComposeOptions{}); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, msg)
	return nil
}

// Sent returns a copy of the recorded messages, oldest first.
func (r *Recorder) Sent() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Message(nil), r.sent...)
}

var (
	_ Sender = (*SMTPSender)(nil)
	_ Sender = NoopSender{}
	_ Sender = (*Recorder)(nil)
)
