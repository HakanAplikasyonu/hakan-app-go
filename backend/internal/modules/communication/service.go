// Package communication is the office's outgoing message service (#780 A,
// İletişim merkezi): the office's own NetGSM account, the SMS and e-mail
// send service, and the send records (sms_messages / mail_messages) that
// prove what went to whom and when.
//
// Other modules send through Service.SendSMS / Service.SendMail (or the
// taxpayer helpers SMSToEngagement / MailToEngagement). Every send is
// recorded; in test mode (the office's switch, or NETGSM_SEND_ENABLED not
// "true" on the server) nothing leaves the system and the record says
// "deneme modu, gönderilmedi".
package communication

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication/netgsm"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// PasswordPurpose is the vault purpose of the NetGSM API password.
const PasswordPurpose = "communication.netgsm_password"

// Message statuses.
const (
	StatusPending   = "pending"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusUncertain = "uncertain"
	StatusDryRun    = "dry_run"
)

// DryRunNote is written on records that were not sent (test mode).
const DryRunNote = "Deneme modu, gönderilmedi."

// Encodings NetGSM accepts.
var Encodings = []string{"TR", "UTF-8", "UNICODE"}

var (
	// ErrNotConfigured: the office has no enabled NetGSM account.
	ErrNotConfigured = errors.New("communication: SMS is not set up for this office")
	// ErrInvalidSettings: a settings field is invalid.
	ErrInvalidSettings = errors.New("communication: invalid settings")
	// ErrNoVault: the credential vault is not configured on the server.
	ErrNoVault = errors.New("communication: credential vault not configured")
	// ErrNoContact: the taxpayer has no phone / e-mail on record.
	ErrNoContact = errors.New("communication: the taxpayer has no contact on record")
	// ErrNotFound: unknown engagement.
	ErrNotFound = errors.New("communication: not found")
)

// Service sends and records office messages.
type Service struct {
	pool   database.Pool
	q      *db.Queries
	vault  secrets.Vault
	client *netgsm.Client
	mailer mail.Sender
	// live: the server allows real NetGSM sends (NETGSM_SEND_ENABLED=true).
	live  bool
	limit *limiter
	now   func() time.Time
}

// Options configure the service.
type Options struct {
	// BaseURL of NetGSM ("" = the real API); tests point it at a fake.
	BaseURL string
	// Live allows real sends; false keeps every office in test mode.
	Live bool
	// Mailer sends e-mail; nil records e-mail as test mode.
	Mailer mail.Sender
	// PerMinute caps NetGSM requests per office (default 40).
	PerMinute int
}

// OptionsFromEnv reads NETGSM_BASE_URL and NETGSM_SEND_ENABLED.
func OptionsFromEnv(m mail.Sender) Options {
	return Options{
		BaseURL: strings.TrimSpace(os.Getenv("NETGSM_BASE_URL")),
		Live:    strings.EqualFold(strings.TrimSpace(os.Getenv("NETGSM_SEND_ENABLED")), "true"),
		Mailer:  m,
	}
}

// NewService builds the service. vault may be nil: saving a password then
// answers ErrNoVault.
func NewService(pool database.Pool, vault secrets.Vault, o Options) *Service {
	per := o.PerMinute
	if per <= 0 {
		per = 40
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, client: netgsm.New(o.BaseURL),
		mailer: o.Mailer, live: o.Live, limit: newLimiter(time.Minute / time.Duration(per)), now: time.Now}
}

// WithClient swaps the NetGSM client (tests).
func (s *Service) WithClient(c *netgsm.Client) *Service { s.client = c; return s }

// LastTest is the last connection test of the office.
type LastTest struct {
	At      time.Time `json:"at"`
	OK      bool      `json:"ok"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Headers []string  `json:"headers"`
}

// Settings is the office's NetGSM account as the screen sees it. The
// password never leaves the server: only PasswordSet.
type Settings struct {
	Enabled     bool       `json:"enabled"`
	TestMode    bool       `json:"test_mode"`
	LiveAllowed bool       `json:"live_allowed"`
	Usercode    string     `json:"usercode"`
	PasswordSet bool       `json:"password_set"`
	MsgHeader   string     `json:"msgheader"`
	Encoding    string     `json:"encoding"`
	AppName     string     `json:"appname"`
	LastTest    *LastTest  `json:"last_test"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func (s *Service) view(row *db.OfficeSmsSetting) Settings {
	if row == nil {
		return Settings{TestMode: true, LiveAllowed: s.live, Encoding: "TR"}
	}
	v := Settings{Enabled: row.Enabled, TestMode: row.TestMode, LiveAllowed: s.live, Usercode: row.Usercode,
		PasswordSet: row.PasswordRef.Valid && row.PasswordRef.String != "", MsgHeader: row.Msgheader,
		Encoding: row.Encoding, AppName: row.Appname}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		v.UpdatedAt = &t
	}
	if row.LastTestAt.Valid {
		lt := &LastTest{At: row.LastTestAt.Time, OK: row.LastTestOk.Bool, Code: row.LastTestCode, Headers: row.LastTestHeaders}
		if lt.Headers == nil {
			lt.Headers = []string{}
		}
		if lt.OK {
			lt.Message = "Bağlantı başarılı."
		} else {
			lt.Message = codeOrNetwork(row.LastTestCode)
		}
		v.LastTest = lt
	}
	return v
}

func codeOrNetwork(code string) string {
	if code == "network" {
		return "NetGSM'e ulaşılamadı. Sunucunun internet bağlantısını kontrol edin."
	}
	return netgsm.CodeMessage(code)
}

func (s *Service) load(ctx context.Context, org int64) (*db.OfficeSmsSetting, error) {
	row, err := s.q.GetOfficeSmsSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load sms settings: %w", err)
	}
	return &row, nil
}

// Get returns the office's settings (defaults when never saved).
func (s *Service) Get(ctx context.Context, org int64) (Settings, error) {
	row, err := s.load(ctx, org)
	if err != nil {
		return Settings{}, err
	}
	return s.view(row), nil
}

// SettingsInput is the body of PUT sms-settings. Password nil or "" keeps
// the stored one.
type SettingsInput struct {
	Enabled   bool    `json:"enabled"`
	TestMode  bool    `json:"test_mode"`
	Usercode  string  `json:"usercode"`
	Password  *string `json:"password"`
	MsgHeader string  `json:"msgheader"`
	Encoding  string  `json:"encoding"`
	AppName   string  `json:"appname"`
}

var (
	usercodePattern  = regexp.MustCompile(`^[0-9]{0,20}$`)
	msgheaderPattern = regexp.MustCompile(`^[\p{L}0-9 .\-]{3,11}$`)
)

// Save stores the settings; a new password goes to the vault.
func (s *Service) Save(ctx context.Context, org, actor int64, in SettingsInput) (Settings, error) {
	in.Usercode = strings.TrimSpace(in.Usercode)
	in.MsgHeader = strings.TrimSpace(in.MsgHeader)
	in.AppName = strings.TrimSpace(in.AppName)
	if in.Encoding == "" {
		in.Encoding = "TR"
	}
	switch {
	case !usercodePattern.MatchString(in.Usercode):
		return Settings{}, fmt.Errorf("%w: abone numarası yalnız rakamlardan oluşur", ErrInvalidSettings)
	case in.MsgHeader != "" && !msgheaderPattern.MatchString(in.MsgHeader):
		return Settings{}, fmt.Errorf("%w: gönderici adı 3 ile 11 karakter arasında olmalı", ErrInvalidSettings)
	case !validEncoding(in.Encoding):
		return Settings{}, fmt.Errorf("%w: kodlama TR, UTF-8 ya da UNICODE olmalı", ErrInvalidSettings)
	case len([]rune(in.AppName)) > 64:
		return Settings{}, fmt.Errorf("%w: uygulama adı en fazla 64 karakter", ErrInvalidSettings)
	}
	cur, err := s.load(ctx, org)
	if err != nil {
		return Settings{}, err
	}
	ref := pgtype.Text{}
	if cur != nil {
		ref = cur.PasswordRef
	}
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		if s.vault == nil {
			return Settings{}, ErrNoVault
		}
		plain := secrets.NewSecret([]byte(strings.TrimSpace(*in.Password)))
		if ref.Valid && ref.String != "" {
			if err := s.vault.Replace(ctx, PasswordPurpose, secrets.Ref(ref.String), plain); err != nil {
				return Settings{}, fmt.Errorf("store netgsm password: %w", err)
			}
		} else {
			r, err := s.vault.Put(ctx, PasswordPurpose, plain)
			if err != nil {
				return Settings{}, fmt.Errorf("store netgsm password: %w", err)
			}
			ref = pgtype.Text{String: string(r), Valid: true}
		}
	}
	if in.Enabled && (in.Usercode == "" || in.MsgHeader == "" || !ref.Valid || ref.String == "") {
		return Settings{}, fmt.Errorf("%w: açmak için abone numarası, şifre ve gönderici adı gerekli", ErrInvalidSettings)
	}
	row, err := s.q.UpsertOfficeSmsSettings(ctx, db.UpsertOfficeSmsSettingsParams{
		OrganizationID: org, Enabled: in.Enabled, TestMode: in.TestMode, Usercode: in.Usercode, PasswordRef: ref,
		Msgheader: in.MsgHeader, Encoding: in.Encoding, Appname: in.AppName, UpdatedBy: actorID(actor),
	})
	if err != nil {
		return Settings{}, fmt.Errorf("save sms settings: %w", err)
	}
	return s.view(&row), nil
}

func validEncoding(e string) bool {
	for _, x := range Encodings {
		if x == e {
			return true
		}
	}
	return false
}

func actorID(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func (s *Service) credentials(ctx context.Context, row *db.OfficeSmsSetting) (netgsm.Credentials, error) {
	if row == nil || row.Usercode == "" || !row.PasswordRef.Valid || row.PasswordRef.String == "" {
		return netgsm.Credentials{}, ErrNotConfigured
	}
	if s.vault == nil {
		return netgsm.Credentials{}, ErrNoVault
	}
	sec, err := s.vault.Get(ctx, PasswordPurpose, secrets.Ref(row.PasswordRef.String))
	if err != nil {
		return netgsm.Credentials{}, fmt.Errorf("read netgsm password: %w", err)
	}
	return netgsm.Credentials{Usercode: row.Usercode, Password: string(sec.Reveal())}, nil
}

// Test calls NetGSM's sender name list with the stored account and records
// the result. It only reads: no SMS is sent, also in test mode.
func (s *Service) Test(ctx context.Context, org int64) (Settings, error) {
	row, err := s.load(ctx, org)
	if err != nil {
		return Settings{}, err
	}
	cred, err := s.credentials(ctx, row)
	if err != nil {
		return Settings{}, err
	}
	headers, err := s.client.Headers(ctx, cred)
	code := "00"
	var apiErr *netgsm.APIError
	switch {
	case err == nil:
	case errors.As(err, &apiErr):
		code = apiErr.Code
	case errors.Is(err, context.Canceled):
		return Settings{}, err
	default:
		code = "network"
	}
	if headers == nil {
		headers = []string{}
	}
	upd, uerr := s.q.SetOfficeSmsTestResult(ctx, db.SetOfficeSmsTestResultParams{OrganizationID: org, Ok: pgtype.Bool{Bool: err == nil, Valid: true}, Code: code, Headers: headers})
	if uerr != nil {
		return Settings{}, fmt.Errorf("save sms test result: %w", uerr)
	}
	return s.view(&upd), nil
}

// SMSInput is one SMS to send. EngagementID ties it to a taxpayer of the
// office (nil for an office-only message such as a test).
type SMSInput struct {
	OrganizationID int64
	EngagementID   *int64
	Phone          string
	Text           string
	// SourceRule: the automation rule code ("kdv_tahakkuk", …) or a
	// channel tag ("manual", "test", "notification").
	SourceRule string
	// SourceRef: the source record (e.g. the tax return uuid).
	SourceRef string
	SentBy    int64
	BulkID    *uuid.UUID
}

// SendSMS normalizes, records and (unless in test mode) sends one SMS. A
// broken number is recorded as failed; campaign wording and an empty text
// are refused without a record. The returned record carries the outcome:
// sent, failed, uncertain or dry_run.
func (s *Service) SendSMS(ctx context.Context, in SMSInput) (db.SmsMessage, error) {
	text, err := NormalizeText(in.Text)
	if err != nil {
		return db.SmsMessage{}, err
	}
	if w := CampaignWords(text); len(w) > 0 {
		return db.SmsMessage{}, fmt.Errorf("%w: %s", ErrCampaignText, strings.Join(w, ", "))
	}
	row, err := s.load(ctx, in.OrganizationID)
	if err != nil {
		return db.SmsMessage{}, err
	}
	if row == nil || !row.Enabled {
		return db.SmsMessage{}, ErrNotConfigured
	}
	p := db.InsertSmsMessageParams{OrganizationID: in.OrganizationID, Body: text, Msgheader: row.Msgheader,
		Status: StatusPending, SourceRule: in.SourceRule, SourceRef: in.SourceRef, SentBy: actorID(in.SentBy)}
	if in.EngagementID != nil {
		p.EngagementID = pgtype.Int8{Int64: *in.EngagementID, Valid: true}
	}
	if in.BulkID != nil {
		p.BulkID = pgtype.UUID{Bytes: *in.BulkID, Valid: true}
	}
	phone, perr := NormalizePhone(in.Phone)
	switch {
	case perr != nil:
		p.Phone, p.Status, p.ErrorCode = strings.TrimSpace(in.Phone), StatusFailed, "phone"
		p.ErrorMessage = "Numara cep telefonu değil ya da hatalı."
	case row.TestMode || !s.live:
		p.Phone, p.Status, p.ErrorMessage = phone, StatusDryRun, DryRunNote
	default:
		p.Phone = phone
	}
	msg, err := s.q.InsertSmsMessage(ctx, p)
	if err != nil {
		return db.SmsMessage{}, fmt.Errorf("record sms: %w", err)
	}
	if msg.Status != StatusPending {
		return msg, nil
	}
	return s.deliver(ctx, row, msg)
}

// deliver sends a pending record and writes the outcome.
func (s *Service) deliver(ctx context.Context, row *db.OfficeSmsSetting, msg db.SmsMessage) (db.SmsMessage, error) {
	fin := db.FinishSmsMessageParams{OrganizationID: msg.OrganizationID, ID: msg.ID, Status: StatusFailed}
	cred, err := s.credentials(ctx, row)
	if err == nil {
		err = s.limit.wait(ctx, msg.OrganizationID)
	}
	var job string
	if err == nil {
		job, err = s.client.Send(ctx, cred, netgsm.SendRequest{Header: row.Msgheader, Encoding: row.Encoding,
			AppName: row.Appname, Items: []netgsm.SendItem{{Phone: msg.Phone, Text: msg.Body}}})
	}
	var apiErr *netgsm.APIError
	switch {
	case err == nil:
		fin.Status, fin.ProviderJobID = StatusSent, job
		fin.SentAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	case errors.Is(err, netgsm.ErrUncertain):
		fin.Status, fin.ErrorCode = StatusUncertain, "uncertain"
		fin.ErrorMessage = "Cevap beklerken bağlantı koptu; mesajın gidip gitmediği belirsiz. Tekrar denenmedi."
		fin.SentAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	case errors.As(err, &apiErr):
		fin.ErrorCode, fin.ErrorMessage = apiErr.Code, apiErr.Message()
	case errors.Is(err, netgsm.ErrUnreachable):
		fin.ErrorCode, fin.ErrorMessage = "network", codeOrNetwork("network")
	case errors.Is(err, ErrNoVault), errors.Is(err, ErrNotConfigured):
		fin.ErrorCode, fin.ErrorMessage = "config", "NetGSM hesabı eksik ayarlanmış."
	default:
		fin.ErrorCode, fin.ErrorMessage = "error", "Gönderim sırasında hata oluştu."
	}
	out, ferr := s.q.FinishSmsMessage(context.WithoutCancel(ctx), fin)
	if ferr != nil {
		return msg, fmt.Errorf("finish sms: %w", ferr)
	}
	return out, nil
}

// SMSToEngagement sends to the contact phone of the engagement's taxpayer.
func (s *Service) SMSToEngagement(ctx context.Context, org int64, engagement uuid.UUID, in SMSInput) (db.SmsMessage, error) {
	c, err := s.q.GetEngagementContactForMessage(ctx, db.GetEngagementContactForMessageParams{OrganizationID: org, EngagementUuid: engagement})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SmsMessage{}, ErrNotFound
	} else if err != nil {
		return db.SmsMessage{}, fmt.Errorf("engagement contact: %w", err)
	}
	if strings.TrimSpace(c.ContactPhone) == "" {
		return db.SmsMessage{}, ErrNoContact
	}
	in.OrganizationID, in.EngagementID, in.Phone = org, &c.EngagementID, c.ContactPhone
	return s.SendSMS(ctx, in)
}

// MailInput is one e-mail to send.
type MailInput struct {
	OrganizationID int64
	EngagementID   *int64
	To             string
	Subject        string
	Body           string
	SourceRule     string
	SourceRef      string
	SentBy         int64
	BulkID         *uuid.UUID
}

// SendMail records and (unless in test mode) sends one e-mail. The office's
// test mode switch covers e-mail too; without a NetGSM row the office is in
// test mode.
func (s *Service) SendMail(ctx context.Context, in MailInput) (db.MailMessage, error) {
	in.To, in.Subject, in.Body = strings.TrimSpace(in.To), strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	if in.To == "" || !strings.Contains(in.To, "@") || len(in.To) > 254 {
		return db.MailMessage{}, fmt.Errorf("%w: geçerli bir e-posta adresi gerekli", ErrInvalidSettings)
	}
	if in.Subject == "" || in.Body == "" {
		return db.MailMessage{}, ErrEmptyText
	}
	row, err := s.load(ctx, in.OrganizationID)
	if err != nil {
		return db.MailMessage{}, err
	}
	p := db.InsertMailMessageParams{OrganizationID: in.OrganizationID, ToEmail: in.To, Subject: in.Subject, Body: in.Body,
		Status: StatusPending, SourceRule: in.SourceRule, SourceRef: in.SourceRef, SentBy: actorID(in.SentBy)}
	if in.EngagementID != nil {
		p.EngagementID = pgtype.Int8{Int64: *in.EngagementID, Valid: true}
	}
	if in.BulkID != nil {
		p.BulkID = pgtype.UUID{Bytes: *in.BulkID, Valid: true}
	}
	if row == nil || row.TestMode || s.mailer == nil {
		p.Status, p.ErrorMessage = StatusDryRun, DryRunNote
	}
	msg, err := s.q.InsertMailMessage(ctx, p)
	if err != nil {
		return db.MailMessage{}, fmt.Errorf("record mail: %w", err)
	}
	if msg.Status != StatusPending {
		return msg, nil
	}
	fin := db.FinishMailMessageParams{OrganizationID: msg.OrganizationID, ID: msg.ID, Status: StatusSent,
		SentAt: pgtype.Timestamptz{Time: s.now(), Valid: true}}
	if err := s.mailer.Send(ctx, mail.Message{To: []string{msg.ToEmail}, Subject: msg.Subject, Body: msg.Body}); err != nil {
		fin.Status, fin.ErrorMessage, fin.SentAt = StatusFailed, "E-posta gönderilemedi.", pgtype.Timestamptz{}
	}
	out, err := s.q.FinishMailMessage(context.WithoutCancel(ctx), fin)
	if err != nil {
		return msg, fmt.Errorf("finish mail: %w", err)
	}
	return out, nil
}

// MailToEngagement sends to the contact e-mail of the engagement's taxpayer.
func (s *Service) MailToEngagement(ctx context.Context, org int64, engagement uuid.UUID, in MailInput) (db.MailMessage, error) {
	c, err := s.q.GetEngagementContactForMessage(ctx, db.GetEngagementContactForMessageParams{OrganizationID: org, EngagementUuid: engagement})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.MailMessage{}, ErrNotFound
	} else if err != nil {
		return db.MailMessage{}, fmt.Errorf("engagement contact: %w", err)
	}
	if strings.TrimSpace(c.ContactEmail) == "" {
		return db.MailMessage{}, ErrNoContact
	}
	in.OrganizationID, in.EngagementID, in.To = org, &c.EngagementID, c.ContactEmail
	return s.SendMail(ctx, in)
}

// SMSRecord is an SMS record for the screen.
type SMSRecord struct {
	UUID           uuid.UUID  `json:"uuid"`
	Phone          string     `json:"phone"`
	Body           string     `json:"body"`
	MsgHeader      string     `json:"msgheader"`
	Status         string     `json:"status"`
	ProviderJobID  string     `json:"provider_job_id"`
	ErrorCode      string     `json:"error_code"`
	ErrorMessage   string     `json:"error_message"`
	SourceRule     string     `json:"source_rule"`
	TaxpayerTitle  string     `json:"taxpayer_title"`
	EngagementUUID string     `json:"engagement_uuid"`
	CreatedAt      time.Time  `json:"created_at"`
	SentAt         *time.Time `json:"sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// RecordOf maps a stored SMS to the screen record.
func RecordOf(m db.SmsMessage) SMSRecord {
	return SMSRecord{UUID: m.Uuid, Phone: m.Phone, Body: m.Body, MsgHeader: m.Msgheader, Status: m.Status,
		ProviderJobID: m.ProviderJobID, ErrorCode: m.ErrorCode, ErrorMessage: m.ErrorMessage, SourceRule: m.SourceRule,
		CreatedAt: m.CreatedAt.Time, SentAt: tsPtr(m.SentAt), DeliveredAt: tsPtr(m.DeliveredAt)}
}

// Recent lists the office's latest SMS records.
func (s *Service) Recent(ctx context.Context, org int64, limit int32) ([]SMSRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListOfficeSmsMessages(ctx, db.ListOfficeSmsMessagesParams{OrganizationID: org, Lim: limit})
	if err != nil {
		return nil, fmt.Errorf("list sms: %w", err)
	}
	out := make([]SMSRecord, 0, len(rows))
	for _, r := range rows {
		rec := RecordOf(db.SmsMessage{Uuid: r.Uuid, Phone: r.Phone, Body: r.Body, Msgheader: r.Msgheader, Status: r.Status,
			ProviderJobID: r.ProviderJobID, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, SourceRule: r.SourceRule,
			CreatedAt: r.CreatedAt, SentAt: r.SentAt, DeliveredAt: r.DeliveredAt})
		rec.TaxpayerTitle, rec.EngagementUUID = r.TaxpayerTitle, r.EngagementUuid
		out = append(out, rec)
	}
	return out, nil
}

// limiter spaces NetGSM requests per office (40 per minute = one per 1.5 s).
type limiter struct {
	mu    sync.Mutex
	every time.Duration
	next  map[int64]time.Time
}

func newLimiter(every time.Duration) *limiter {
	return &limiter{every: every, next: map[int64]time.Time{}}
}

func (l *limiter) wait(ctx context.Context, org int64) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next[org]
	if at.Before(now) {
		at = now
	}
	l.next[org] = at.Add(l.every)
	l.mu.Unlock()
	if d := time.Until(at); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

// NewFromConfig builds the service with its own view of the credential
// vault (SECRETS_KEYRING, active id, APP_ENCRYPTION_KEY) and the NetGSM
// options from the environment. Without a usable keyring the vault is nil:
// settings without a password still save, sending answers ErrNoVault.
func NewFromConfig(pool database.Pool, keyring, activeID, appKey string, m mail.Sender, log *slog.Logger) *Service {
	var vault secrets.Vault
	if kr, err := secrets.ParseKeyring(keyring, activeID, appKey); err != nil {
		if log != nil {
			log.Warn("communication: credential vault disabled", "error", err)
		}
	} else {
		// Secrets are platform records: the vault never joins a request's
		// row level security transaction.
		vault = secrets.New(kr, secrets.NewPGStore(db.New(pool)))
	}
	return NewService(pool, vault, OptionsFromEnv(m))
}
