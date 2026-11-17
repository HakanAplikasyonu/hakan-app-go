package reconciliation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	platformmail "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Letter errors.
var (
	ErrBadRecipient    = errors.New("reconciliation: recipient is not a valid email address")
	ErrNoRecipient     = errors.New("reconciliation: no recipient email")
	ErrLettersDisabled = errors.New("reconciliation: letters are not configured")
	ErrLetterExpired   = errors.New("reconciliation: the response link has expired")
	ErrLetterAnswered  = errors.New("reconciliation: the letter is already answered")
)

// LetterTTL is how long the response link works.
const LetterTTL = 60 * 24 * time.Hour

// MaxLetterAttempts: a letter whose email fails this often is failed.
const MaxLetterAttempts = 5

// LetterConfig enables the letters: Secret keys the response link token
// (the application encryption key), FrontendURL is where the link opens.
type LetterConfig struct {
	Secret      string
	FrontendURL string
}

// WithLetters enables the reconciliation letters.
func (s *Service) WithLetters(c LetterConfig) *Service {
	s.letters = &c
	return s
}

// LetterToken is the response link token of a letter: an HMAC of its id,
// so the worker can rebuild the link and the database keeps only a digest.
func LetterToken(secret string, letter uuid.UUID) string {
	m := hmac.New(sha256.New, []byte("reconciliation-letter:"+secret))
	m.Write(letter[:])
	return hex.EncodeToString(m.Sum(nil))
}

// TokenDigest is what is stored of a token.
func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LetterLink is the counterparty's response page.
func LetterLink(frontend, token string) string {
	return strings.TrimRight(frontend, "/") + "/mutabakat/" + token
}

// LetterSnapshot is what the letter says, frozen when it is created.
type LetterSnapshot struct {
	Office            string `json:"office"`
	TaxpayerTitle     string `json:"taxpayer_title"`
	TaxpayerTaxID     string `json:"taxpayer_tax_id"`
	CounterpartyName  string `json:"counterparty_name"`
	CounterpartyTaxID string `json:"counterparty_tax_id"`
	AccountCode       string `json:"account_code"`
	AsOfDate          string `json:"as_of_date"`
	Balance           string `json:"balance"`
	Currency          string `json:"currency"`
	Period            string `json:"period"`
	Note              string `json:"note"`
}

// Letter is the API view of a reconciliation letter.
type Letter struct {
	UUID            uuid.UUID      `json:"uuid"`
	TaxID           string         `json:"counterparty_tax_id"`
	Name            string         `json:"counterparty_name"`
	RecipientEmail  *string        `json:"recipient_email"`
	AccountCode     *string        `json:"account_code"`
	AsOfDate        string         `json:"as_of_date"`
	Balance         string         `json:"balance"`
	Currency        string         `json:"currency"`
	Status          string         `json:"status"`
	Attempts        int            `json:"attempts"`
	LastError       *string        `json:"last_error"`
	SentAt          *time.Time     `json:"sent_at"`
	Response        *string        `json:"response"`
	ResponseBalance *string        `json:"response_balance"`
	ResponseNote    *string        `json:"response_note"`
	ResponderName   *string        `json:"responder_name"`
	ResponseChannel *string        `json:"response_channel"`
	RespondedAt     *time.Time     `json:"responded_at"`
	ExpiresAt       *time.Time     `json:"expires_at"`
	Snapshot        LetterSnapshot `json:"snapshot"`
	CreatedAt       time.Time      `json:"created_at"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func letterView(l db.ReconciliationLetter) Letter {
	v := Letter{
		UUID: l.Uuid, TaxID: l.CounterpartyTaxID, Name: l.CounterpartyName, RecipientEmail: textPtr(l.RecipientEmail),
		AccountCode: textPtr(l.AccountCode), AsOfDate: day(l.AsOfDate), Balance: dec(l.Balance).StringFixed(2), Currency: l.Currency,
		Status: l.Status, Attempts: int(l.Attempts), LastError: textPtr(l.LastError), SentAt: tsPtr(l.SentAt),
		Response: textPtr(l.Response), ResponseBalance: numStr(l.ResponseBalance), ResponseNote: textPtr(l.ResponseNote),
		ResponderName: textPtr(l.ResponderName), ResponseChannel: textPtr(l.ResponseChannel), RespondedAt: tsPtr(l.RespondedAt),
		ExpiresAt: tsPtr(l.TokenExpiresAt), CreatedAt: l.CreatedAt.Time,
	}
	_ = json.Unmarshal(l.Snapshot, &v.Snapshot)
	return v
}

// ValidEmail returns the bare address of s.
func ValidEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 || !strings.Contains(s[strings.LastIndex(s, "@")+1:], ".") {
		return "", ErrBadRecipient
	}
	return a.Address, nil
}

// LetterInput creates a letter for a counterparty of a run.
type LetterInput struct {
	// CounterpartyUUID is the counterparty reconciliation row.
	CounterpartyUUID uuid.UUID `json:"counterparty_uuid"`
	Email            *string   `json:"email"`
	// AsOfDate defaults to the run's period end.
	AsOfDate *string `json:"as_of_date"`
	// Balance overrides the letter's balance (default: the trial balance's,
	// else the portal's).
	Balance *string `json:"balance"`
	Note    *string `json:"note"`
	// Send queues the email at once (needs an email).
	Send bool `json:"send"`
}

// CreateLetter freezes a letter for a counterparty and queues it when asked.
func (s *Service) CreateLetter(ctx context.Context, sc engagementctx.Scope, userID int64, in LetterInput) (Letter, error) {
	if s.letters == nil {
		return Letter{}, ErrLettersDisabled
	}
	c, err := s.counterpartyByUUID(ctx, sc, in.CounterpartyUUID)
	if err != nil {
		return Letter{}, err
	}
	_, end := periodBounds(c.RunPeriod, c.RunPeriod)
	asOf := end
	if in.AsOfDate != nil && strings.TrimSpace(*in.AsOfDate) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*in.AsOfDate))
		if err != nil {
			return Letter{}, fmt.Errorf("%w: as_of_date must be YYYY-MM-DD", ErrInvalid)
		}
		asOf = t
	}
	balance := dec(c.SourceBalance)
	if c.LedgerBalance.Valid {
		balance = dec(c.LedgerBalance)
	}
	if in.Balance != nil && strings.TrimSpace(*in.Balance) != "" {
		b, err := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(*in.Balance, ",", ".")))
		if err != nil {
			return Letter{}, fmt.Errorf("%w: balance", ErrInvalid)
		}
		balance = b.Round(2)
	}
	note, err := cleanNote(in.Note)
	if err != nil {
		return Letter{}, err
	}
	var email *string
	if in.Email != nil && strings.TrimSpace(*in.Email) != "" {
		e, err := ValidEmail(*in.Email)
		if err != nil {
			return Letter{}, err
		}
		email = &e
	}
	if in.Send && email == nil {
		return Letter{}, ErrNoRecipient
	}
	tp, err := s.q.GetTaxpayerByID(ctx, sc.TaxpayerID)
	if err != nil {
		return Letter{}, err
	}
	office := ""
	if o, err := s.q.GetOrganizationByID(ctx, sc.OrganizationID); err == nil {
		office = o.Name
	}
	snap := LetterSnapshot{
		Office: office, TaxpayerTitle: tp.Title, TaxpayerTaxID: tp.TaxID, CounterpartyName: c.CounterpartyName,
		CounterpartyTaxID: c.CounterpartyTaxID, AccountCode: c.AccountCode.String, AsOfDate: asOf.Format("2006-01-02"),
		Balance: balance.StringFixed(2), Currency: "TRY", Period: c.RunPeriod,
	}
	if note != nil {
		snap.Note = *note
	}
	body, _ := json.Marshal(snap)
	status := "draft"
	var next pgtype.Timestamptz
	if in.Send {
		status, next = "queued", pgtype.Timestamptz{Time: s.now(), Valid: true}
	}
	var out db.ReconciliationLetter
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		l, err := q.InsertReconciliationLetter(ctx, db.InsertReconciliationLetterParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
			CounterpartyTaxID: c.CounterpartyTaxID, CounterpartyName: c.CounterpartyName, RecipientEmail: ptrText(email),
			AccountCode: c.AccountCode, AsOfDate: pgtype.Date{Time: asOf, Valid: true}, Balance: num(balance),
			Snapshot: body, Status: status, NextAttemptAt: next, CreatedBy: optID(userID),
		})
		if err != nil {
			return err
		}
		expires := pgtype.Timestamptz{Time: s.now().Add(LetterTTL), Valid: true}
		digest := TokenDigest(LetterToken(s.letters.Secret, l.Uuid))
		if err := q.SetReconciliationLetterToken(ctx, db.SetReconciliationLetterTokenParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: l.ID,
			TokenDigest: pgtype.Text{String: digest, Valid: true}, TokenExpiresAt: expires,
		}); err != nil {
			return err
		}
		l.TokenDigest, l.TokenExpiresAt = pgtype.Text{String: digest, Valid: true}, expires
		out = l
		return nil
	})
	if err != nil {
		return Letter{}, err
	}
	return letterView(out), nil
}

// Letters lists the engagement's letters, newest first (taxID "" = all).
func (s *Service) Letters(ctx context.Context, sc engagementctx.Scope, taxID string) ([]Letter, error) {
	if taxID != "" && !validTaxID(taxID) {
		return nil, fmt.Errorf("%w: counterparty_tax_id", ErrInvalid)
	}
	rows, err := s.q.ListReconciliationLetters(ctx, db.ListReconciliationLettersParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, CounterpartyTaxID: optText(taxID), Lim: 200,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Letter, 0, len(rows))
	for _, r := range rows {
		out = append(out, letterView(r))
	}
	return out, nil
}

func (s *Service) letter(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.ReconciliationLetter, error) {
	l, err := s.q.GetReconciliationLetter(ctx, db.GetReconciliationLetterParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// LetterPDF renders a letter.
func (s *Service) LetterPDF(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) ([]byte, string, error) {
	l, err := s.letter(ctx, sc, id)
	if err != nil {
		return nil, "", err
	}
	v := letterView(l)
	link := ""
	if s.letters != nil {
		link = LetterLink(s.letters.FrontendURL, LetterToken(s.letters.Secret, l.Uuid))
	}
	body, err := RenderLetterPDF(v.Snapshot, link)
	return body, LetterFileName(v.Snapshot), err
}

// SendLetter queues a letter's email (again), optionally to another address.
func (s *Service) SendLetter(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, email *string) (Letter, error) {
	if s.letters == nil {
		return Letter{}, ErrLettersDisabled
	}
	l, err := s.letter(ctx, sc, id)
	if err != nil {
		return Letter{}, err
	}
	if l.Status == "responded" {
		return Letter{}, ErrLetterAnswered
	}
	to := l.RecipientEmail.String
	if email != nil && strings.TrimSpace(*email) != "" {
		to = *email
	}
	if strings.TrimSpace(to) == "" {
		return Letter{}, ErrNoRecipient
	}
	addr, err := ValidEmail(to)
	if err != nil {
		return Letter{}, err
	}
	row, err := s.q.QueueReconciliationLetter(ctx, db.QueueReconciliationLetterParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: l.ID, RecipientEmail: pgtype.Text{String: addr, Valid: true},
	})
	if err != nil {
		return Letter{}, err
	}
	return letterView(row), nil
}

// ResponseInput is the counterparty's answer.
type ResponseInput struct {
	Response      string  `json:"response"`
	Balance       *string `json:"balance"`
	Note          *string `json:"note"`
	ResponderName *string `json:"responder_name"`
}

func (in ResponseInput) clean() (ResponseInput, pgtype.Numeric, error) {
	if in.Response != "agreed" && in.Response != "disputed" {
		return in, pgtype.Numeric{}, fmt.Errorf("%w: response must be agreed or disputed", ErrInvalid)
	}
	var bal pgtype.Numeric
	if in.Balance != nil && strings.TrimSpace(*in.Balance) != "" {
		b, err := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(*in.Balance, ",", ".")))
		if err != nil || b.Abs().GreaterThan(decimal.New(1, 15)) {
			return in, bal, fmt.Errorf("%w: balance", ErrInvalid)
		}
		bal = num(b.Round(2))
	}
	if in.Response == "disputed" && !bal.Valid && (in.Note == nil || strings.TrimSpace(*in.Note) == "") {
		return in, bal, fmt.Errorf("%w: a dispute needs the balance in your books or a note", ErrInvalid)
	}
	n, err := cleanNote(in.Note)
	if err != nil {
		return in, bal, err
	}
	in.Note = n
	if in.ResponderName != nil {
		name := strings.Join(strings.Fields(*in.ResponderName), " ")
		if len([]rune(name)) > 200 {
			return in, bal, fmt.Errorf("%w: responder_name is too long", ErrInvalid)
		}
		if name == "" {
			in.ResponderName = nil
		} else {
			in.ResponderName = &name
		}
	}
	return in, bal, nil
}

func (s *Service) respond(ctx context.Context, q *db.Queries, orgID, engID, letterID int64, taxID string, in ResponseInput, bal pgtype.Numeric, channel string) (db.ReconciliationLetter, error) {
	row, err := q.RespondReconciliationLetter(ctx, db.RespondReconciliationLetterParams{
		OrganizationID: orgID, EngagementID: engID, ID: letterID, Response: pgtype.Text{String: in.Response, Valid: true},
		ResponseBalance: bal, ResponseNote: ptrText(in.Note), ResponderName: ptrText(in.ResponderName), ResponseChannel: pgtype.Text{String: channel, Valid: true},
	})
	if err != nil {
		return row, err
	}
	if in.Response == "agreed" {
		if err := q.ResolveLatestCounterpartyReconciliation(ctx, db.ResolveLatestCounterpartyReconciliationParams{
			OrganizationID: orgID, EngagementID: engID, CounterpartyTaxID: taxID,
			Note: "Karşı taraf mutabakat mektubunu onayladı (" + row.RespondedAt.Time.Format("02.01.2006") + ").",
		}); err != nil {
			return row, err
		}
	}
	return row, nil
}

// RecordResponse stores an answer the accountant received another way
// (phone, signed paper).
func (s *Service) RecordResponse(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, in ResponseInput) (Letter, error) {
	in, bal, err := in.clean()
	if err != nil {
		return Letter{}, err
	}
	l, err := s.letter(ctx, sc, id)
	if err != nil {
		return Letter{}, err
	}
	var out db.ReconciliationLetter
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.respond(ctx, s.q.WithTx(tx), sc.OrganizationID, sc.ID, l.ID, l.CounterpartyTaxID, in, bal, "manual")
		return err
	})
	if err != nil {
		return Letter{}, err
	}
	return letterView(out), nil
}

// PublicLetter is what the counterparty sees behind the link.
type PublicLetter struct {
	Office            string     `json:"office"`
	TaxpayerTitle     string     `json:"taxpayer_title"`
	TaxpayerTaxID     string     `json:"taxpayer_tax_id"`
	CounterpartyName  string     `json:"counterparty_name"`
	CounterpartyTaxID string     `json:"counterparty_tax_id"`
	AsOfDate          string     `json:"as_of_date"`
	Balance           string     `json:"balance"`
	Currency          string     `json:"currency"`
	Note              string     `json:"note"`
	Response          *string    `json:"response"`
	ResponseBalance   *string    `json:"response_balance"`
	RespondedAt       *time.Time `json:"responded_at"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

func publicView(v Letter) PublicLetter {
	return PublicLetter{
		Office: v.Snapshot.Office, TaxpayerTitle: v.Snapshot.TaxpayerTitle, TaxpayerTaxID: v.Snapshot.TaxpayerTaxID,
		CounterpartyName: v.Snapshot.CounterpartyName, CounterpartyTaxID: v.Snapshot.CounterpartyTaxID, AsOfDate: v.AsOfDate,
		Balance: v.Balance, Currency: v.Currency, Note: v.Snapshot.Note, Response: v.Response, ResponseBalance: v.ResponseBalance,
		RespondedAt: v.RespondedAt, ExpiresAt: v.ExpiresAt,
	}
}

func validToken(t string) bool {
	if len(t) != 64 {
		return false
	}
	for _, r := range t {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) || unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// byToken opens a letter by its response link (rls.System: the caller is
// not signed in; the token's digest is unique).
func (s *Service) byToken(ctx context.Context, token string, fn func(ctx context.Context, q *db.Queries, l db.GetReconciliationLetterByTokenRow) error) error {
	if !validToken(token) {
		return ErrNotFound
	}
	return rls.System(ctx, s.pool, "reconciliation: letter response link", func(ctx context.Context, tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		l, err := q.GetReconciliationLetterByToken(ctx, pgtype.Text{String: TokenDigest(token), Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if l.TokenExpiresAt.Valid && s.now().After(l.TokenExpiresAt.Time) {
			return ErrLetterExpired
		}
		return fn(ctx, q, l)
	})
}

func letterFromToken(l db.GetReconciliationLetterByTokenRow) db.ReconciliationLetter {
	return db.ReconciliationLetter{
		ID: l.ID, Uuid: l.Uuid, OrganizationID: l.OrganizationID, EngagementID: l.EngagementID, TaxpayerID: l.TaxpayerID,
		CounterpartyTaxID: l.CounterpartyTaxID, CounterpartyName: l.CounterpartyName, RecipientEmail: l.RecipientEmail,
		AccountCode: l.AccountCode, AsOfDate: l.AsOfDate, Balance: l.Balance, Currency: l.Currency, Snapshot: l.Snapshot,
		Status: l.Status, TokenDigest: l.TokenDigest, TokenExpiresAt: l.TokenExpiresAt, Attempts: l.Attempts,
		NextAttemptAt: l.NextAttemptAt, LastError: l.LastError, SentAt: l.SentAt, Response: l.Response,
		ResponseBalance: l.ResponseBalance, ResponseNote: l.ResponseNote, ResponderName: l.ResponderName,
		ResponseChannel: l.ResponseChannel, RespondedAt: l.RespondedAt, CreatedBy: l.CreatedBy, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

// PublicLetterByToken returns the letter behind a response link.
func (s *Service) PublicLetterByToken(ctx context.Context, token string) (PublicLetter, error) {
	var out PublicLetter
	err := s.byToken(ctx, token, func(_ context.Context, _ *db.Queries, l db.GetReconciliationLetterByTokenRow) error {
		out = publicView(letterView(letterFromToken(l)))
		return nil
	})
	return out, err
}

// PublicRespond stores the counterparty's answer through the link. An
// answered letter cannot be answered again.
func (s *Service) PublicRespond(ctx context.Context, token string, in ResponseInput) (PublicLetter, error) {
	in, bal, err := in.clean()
	if err != nil {
		return PublicLetter{}, err
	}
	var out PublicLetter
	err = s.byToken(ctx, token, func(ctx context.Context, q *db.Queries, l db.GetReconciliationLetterByTokenRow) error {
		if l.Response.Valid {
			return ErrLetterAnswered
		}
		row, err := s.respond(ctx, q, l.OrganizationID, l.EngagementID, l.ID, l.CounterpartyTaxID, in, bal, "link")
		if err != nil {
			return err
		}
		out = publicView(letterView(row))
		return nil
	})
	return out, err
}

// ---- PDF and email ----

// LetterFileName is the PDF's name.
func LetterFileName(s LetterSnapshot) string {
	return "mutabakat-" + s.AsOfDate + "-" + fileSlug(s.CounterpartyName) + ".pdf"
}

func fileSlug(s string) string {
	repl := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "İ", "i", "I", "i", "Ö", "o", "Ş", "s", "Ü", "u")
	s = strings.ToLower(repl.Replace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}
	return "cari"
}

func trDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02.01.2006")
}

// BalanceSentence is how the letter states the balance.
func BalanceSentence(s LetterSnapshot) string {
	b, _ := decimal.NewFromString(s.Balance)
	side := "borç"
	if b.IsNegative() {
		side = "alacak"
	}
	if b.IsZero() {
		return fmt.Sprintf("%s tarihi itibarıyla firmamız kayıtlarında cari hesabınız bakiye vermemektedir.", trDate(s.AsOfDate))
	}
	return fmt.Sprintf("%s tarihi itibarıyla firmamız kayıtlarında cari hesabınız %s %s bakiyesi vermektedir.",
		trDate(s.AsOfDate), Money(b.Abs()), side)
}

// RenderLetterPDF prints the letter on A4: the balance, the response link
// and a reply section to sign and send back.
func RenderLetterPDF(s LetterSnapshot, link string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.SetMargins(22, 20, 22)
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetTitle("Cari hesap mutabakat mektubu", true)
	pdf.SetCreator("HAKAN mutabakat", true)
	font := func(style string, size float64) { pdf.SetFont(ioengine.PDFFontFamily, style, size) }
	pdf.AddPage()
	font("B", 11)
	pdf.MultiCell(0, 6, s.TaxpayerTitle, "", "L", false)
	font("", 9)
	pdf.CellFormat(0, 5, "VKN/TCKN: "+s.TaxpayerTaxID, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Tarih: "+time.Now().Format("02.01.2006"), "", 1, "R", false, 0, "")
	pdf.Ln(6)
	font("B", 13)
	pdf.CellFormat(0, 8, "CARİ HESAP MUTABAKAT MEKTUBU", "", 1, "C", false, 0, "")
	pdf.Ln(4)
	font("", 11)
	pdf.MultiCell(0, 6, "Sayın "+s.CounterpartyName+" (VKN/TCKN: "+s.CounterpartyTaxID+"),", "", "L", false)
	pdf.Ln(3)
	pdf.MultiCell(0, 6, BalanceSentence(s)+" Kayıtlarınızla mutabık olup olmadığınızı aşağıdaki bölümü doldurarak ya da bağlantıdaki sayfadan en geç 15 gün içinde bildirmenizi rica ederiz. Mutabık değilseniz kendi kayıtlarınızdaki bakiyeyi ve hesap ekstrenizi göndermeniz farkın bulunmasını kolaylaştırır.", "", "J", false)
	if strings.TrimSpace(s.Note) != "" {
		pdf.Ln(2)
		font("I", 10)
		pdf.MultiCell(0, 5.5, "Not: "+s.Note, "", "L", false)
		font("", 11)
	}
	if link != "" {
		pdf.Ln(3)
		font("", 10)
		pdf.MultiCell(0, 5.5, "Yanıt bağlantısı: "+link, "", "L", false)
	}
	pdf.Ln(6)
	font("", 11)
	pdf.MultiCell(0, 6, "Saygılarımızla,\n"+s.TaxpayerTitle, "", "L", false)
	if s.Office != "" {
		font("", 9)
		pdf.MultiCell(0, 5, "Mali müşavir: "+s.Office, "", "L", false)
	}
	pdf.Ln(8)
	pdf.SetDrawColor(120, 120, 120)
	pdf.Line(22, pdf.GetY(), 188, pdf.GetY())
	pdf.Ln(4)
	font("B", 11)
	pdf.CellFormat(0, 6, "YANIT", "", 1, "L", false, 0, "")
	font("", 10)
	pdf.MultiCell(0, 6, "[  ] "+trDate(s.AsOfDate)+" tarihi itibarıyla bildirilen bakiye ile mutabıkız.\n[  ] Mutabık değiliz. Kayıtlarımızdaki bakiye: ........................................ TL (borç / alacak)", "", "L", false)
	pdf.Ln(4)
	pdf.CellFormat(90, 6, "Firma kaşesi / yetkili imza:", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 6, "Tarih: ....../....../..........", "", 1, "L", false, 0, "")
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LetterEmail is the letter's email (without recipient and attachment).
func LetterEmail(s LetterSnapshot, link string) platformmail.Message {
	var b strings.Builder
	fmt.Fprintf(&b, "Sayın %s,\n\n", s.CounterpartyName)
	b.WriteString(BalanceSentence(s) + "\n\n")
	b.WriteString("Mutabakat mektubu ektedir (PDF). Mutabık olup olmadığınızı aşağıdaki bağlantıdan tek tıkla bildirebilirsiniz:\n")
	b.WriteString(link + "\n\n")
	if strings.TrimSpace(s.Note) != "" {
		b.WriteString("Not: " + s.Note + "\n\n")
	}
	fmt.Fprintf(&b, "Saygılarımızla,\n%s\n", s.TaxpayerTitle)
	if s.Office != "" {
		fmt.Fprintf(&b, "(Mali müşavir: %s)\n", s.Office)
	}
	b.WriteString("\nBu e-posta HAKAN üzerinden gönderildi.\n")
	return platformmail.Message{Subject: fmt.Sprintf("%s — %s tarihli cari hesap mutabakatı", s.TaxpayerTitle, trDate(s.AsOfDate)), Body: b.String()}
}

// LetterSender sends queued letters (worker sweep, every office: run it
// inside rls.System).
type LetterSender struct {
	Q      *db.Queries
	Mail   platformmail.Sender
	Config LetterConfig
	Log    *slog.Logger
	Now    func() time.Time
}

// SendDue sends the letters whose turn has come.
func (s *LetterSender) SendDue(ctx context.Context) (int, error) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	rows, err := s.Q.ClaimReconciliationLetters(ctx, 20)
	if err != nil {
		return 0, fmt.Errorf("reconciliation: claim letters: %w", err)
	}
	sent := 0
	for _, l := range rows {
		err := s.send(ctx, l)
		if err == nil {
			if err := s.Q.MarkReconciliationLetterSent(ctx, l.ID); err != nil {
				return sent, err
			}
			sent++
			continue
		}
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		log.Warn("reconciliation_letter_failed", "letter", l.Uuid.String(), "attempt", l.Attempts, "error", msg)
		if l.Attempts >= MaxLetterAttempts {
			if err := s.Q.MarkReconciliationLetterFailed(ctx, db.MarkReconciliationLetterFailedParams{ID: l.ID, LastError: pgtype.Text{String: msg, Valid: true}}); err != nil {
				return sent, err
			}
			continue
		}
		if err := s.Q.MarkReconciliationLetterRetry(ctx, db.MarkReconciliationLetterRetryParams{ID: l.ID,
			LastError: pgtype.Text{String: msg, Valid: true}, NextAttemptAt: pgtype.Timestamptz{Time: now().Add(time.Duration(l.Attempts) * 5 * time.Minute), Valid: true}}); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

func (s *LetterSender) send(ctx context.Context, l db.ReconciliationLetter) error {
	if s.Mail == nil {
		return errors.New("mail sender is not configured")
	}
	if !l.RecipientEmail.Valid {
		return ErrNoRecipient
	}
	v := letterView(l)
	link := LetterLink(s.Config.FrontendURL, LetterToken(s.Config.Secret, l.Uuid))
	body, err := RenderLetterPDF(v.Snapshot, link)
	if err != nil {
		return fmt.Errorf("pdf: %w", err)
	}
	msg := LetterEmail(v.Snapshot, link)
	msg.To = []string{l.RecipientEmail.String}
	msg.Attachments = []platformmail.Attachment{{Name: LetterFileName(v.Snapshot), ContentType: "application/pdf", Data: body}}
	return s.Mail.Send(ctx, msg)
}
