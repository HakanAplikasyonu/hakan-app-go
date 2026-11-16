package webservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// VaultPurpose tags bank credentials in the vault.
const VaultPurpose = "bank_connection"

// SyncThrottle: "şimdi çek" at most once per 5 minutes per connection (20 §7.2).
const SyncThrottle = 5 * time.Minute

var (
	ErrNotFound       = errors.New("webservice: connection not found")
	ErrUnknownBank    = errors.New("webservice: bank has no web-service adapter")
	ErrBadState       = errors.New("webservice: action not allowed in this state")
	ErrBadCredentials = errors.New("webservice: credentials incomplete")
	ErrThrottled      = errors.New("webservice: sync requested too often")
	ErrBadRange       = errors.New("webservice: invalid date range")
	ErrNoVault        = errors.New("webservice: credential vault is not configured")
)

// Options are the deployment's fixed values for the instruction (20 §4–5).
type Options struct {
	EgressIPs   []string
	TechContact string
}

// Service drives the connection flow (20 §5.1):
//
//	draft → form_ready → submitted_to_bank → credentials_entered → verifying → active | failed
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	vault secrets.Vault
	bank  *banking.Service
	reg   Registry
	opt   Options
	now   func() time.Time
}

// NewService builds the service. vault may be nil (credentials then refused).
func NewService(pool *pgxpool.Pool, vault secrets.Vault, bank *banking.Service, reg Registry, opt Options) *Service {
	return &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, bank: bank, reg: reg, opt: opt, now: time.Now}
}

// Meta is what the connection keeps in source_connections.meta.
type Meta struct {
	BankID          string     `json:"bank_id"`
	Environment     string     `json:"environment"`
	IBANs           []string   `json:"ibans,omitempty"`
	FormAt          *time.Time `json:"form_at,omitempty"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncRequest *time.Time `json:"last_sync_request_at,omitempty"`
}

// Connection is the API view.
type Connection struct {
	UUID           uuid.UUID         `json:"uuid"`
	BankID         string            `json:"bank_id"`
	BankName       string            `json:"bank_name"`
	Environment    string            `json:"environment"`
	SetupState     string            `json:"setup_state"`
	Status         string            `json:"status"`
	IBANs          []string          `json:"ibans"`
	HasCredentials bool              `json:"has_credentials"`
	Error          *ErrorView        `json:"error,omitempty"`
	Fields         []CredentialField `json:"credential_fields"`
	SubmittedAt    *time.Time        `json:"submitted_at,omitempty"`
	VerifiedAt     *time.Time        `json:"verified_at,omitempty"`
	LastSyncAt     *time.Time        `json:"last_sync_at,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

// ErrorView is last_error: the reason, the bank's code and the Turkish text.
type ErrorView struct {
	Reason  Reason    `json:"reason"`
	Code    string    `json:"code,omitempty"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// BankInfo lists a bank with an adapter.
type BankInfo struct {
	BankID string            `json:"bank_id"`
	Name   string            `json:"name"`
	Fields []CredentialField `json:"credential_fields"`
	Limits struct {
		MaxRangeDays   int `json:"max_range_days"`
		MaxHistoryDays int `json:"max_history_days"`
	} `json:"limits"`
}

// Banks lists the banks that can be connected.
func (s *Service) Banks() []BankInfo {
	ids := make([]string, 0, len(s.reg))
	for id := range s.reg {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]BankInfo, 0, len(ids))
	for _, id := range ids {
		a, _ := s.reg.Get(id, EnvProduction)
		b := BankInfo{BankID: id, Name: a.BankName(), Fields: a.CredentialFields()}
		b.Limits.MaxRangeDays, b.Limits.MaxHistoryDays = a.Limits().MaxRangeDays, a.Limits().MaxHistoryDays
		out = append(out, b)
	}
	return out
}

// Create opens a draft connection for bankID with the accounts to list on
// the instruction.
func (s *Service) Create(ctx context.Context, taxpayerID int64, bankID, environment string, ibans []string) (Connection, error) {
	if environment == "" {
		environment = EnvProduction
	}
	if environment != EnvProduction && environment != EnvTest {
		return Connection{}, fmt.Errorf("%w: environment", ErrBadState)
	}
	a, ok := s.reg.Get(bankID, environment)
	if !ok {
		return Connection{}, ErrUnknownBank
	}
	clean, err := cleanIBANs(ibans)
	if err != nil {
		return Connection{}, err
	}
	meta, _ := json.Marshal(Meta{BankID: bankID, Environment: environment, IBANs: clean})
	row, err := s.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: a.SourceSystem(), SetupState: "draft", Meta: meta,
	})
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

// ErrInvalidIBAN: an account on the instruction is not a TR IBAN.
var ErrInvalidIBAN = errors.New("webservice: invalid IBAN")

func cleanIBANs(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		raw := strings.TrimSpace(v)
		if raw == "" {
			continue
		}
		v = banking.NormalizeIBAN(raw)
		if v == "" {
			return nil, fmt.Errorf("%w: %s", ErrInvalidIBAN, raw)
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

// List returns the taxpayer's bank connections.
func (s *Service) List(ctx context.Context, taxpayerID int64) ([]Connection, error) {
	rows, err := s.q.ListBankConnections(ctx, taxpayerID)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r))
	}
	return out, nil
}

// Get returns one connection.
func (s *Service) Get(ctx context.Context, taxpayerID int64, id uuid.UUID) (Connection, error) {
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

func (s *Service) load(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, error) {
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !isBank(row.SourceSystem)) {
		return db.SourceConnection{}, ErrNotFound
	}
	return row, err
}

func isBank(sys string) bool { return strings.HasPrefix(sys, "bank_") && sys != "bank_file" }

func meta(row db.SourceConnection) Meta {
	var m Meta
	_ = json.Unmarshal(row.Meta, &m)
	if m.Environment == "" {
		m.Environment = EnvProduction
	}
	return m
}

func (s *Service) view(row db.SourceConnection) Connection {
	m := meta(row)
	c := Connection{
		UUID: row.Uuid, BankID: m.BankID, Environment: m.Environment, SetupState: row.SetupState, Status: row.Status,
		IBANs: m.IBANs, HasCredentials: row.CredentialsRef.Valid, SubmittedAt: m.SubmittedAt, VerifiedAt: m.VerifiedAt,
		LastSyncAt: m.LastSyncAt, CreatedAt: row.CreatedAt.Time,
	}
	if c.IBANs == nil {
		c.IBANs = []string{}
	}
	if a, ok := s.reg.Get(m.BankID, m.Environment); ok {
		c.BankName, c.Fields = a.BankName(), a.CredentialFields()
	}
	if len(row.LastError) > 0 {
		var e ErrorView
		if json.Unmarshal(row.LastError, &e) == nil && e.Reason != "" {
			c.Error = &e
		}
	}
	return c
}

func (s *Service) update(ctx context.Context, row db.SourceConnection, state, status string, lastErr *ErrorView, patch map[string]any) (db.SourceConnection, error) {
	var le []byte
	if lastErr != nil {
		le, _ = json.Marshal(lastErr)
	}
	if patch == nil {
		patch = map[string]any{}
	}
	p, _ := json.Marshal(patch)
	return s.q.UpdateBankConnectionSetup(ctx, db.UpdateBankConnectionSetupParams{
		SetupState: state, Status: status, LastError: le, MetaPatch: p, TaxpayerID: row.TaxpayerID, ID: row.ID,
	})
}

// Instruction renders the pre-filled instruction PDF; a draft becomes
// form_ready (20 §5.1).
func (s *Service) Instruction(ctx context.Context, taxpayerID int64, id uuid.UUID) ([]byte, string, error) {
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return nil, "", err
	}
	m := meta(row)
	a, ok := s.reg.Get(m.BankID, m.Environment)
	if !ok {
		return nil, "", ErrUnknownBank
	}
	tp, err := s.q.GetTaxpayerByID(ctx, taxpayerID)
	if err != nil {
		return nil, "", err
	}
	ibans := make([]string, 0, len(m.IBANs))
	boxes, err := s.q.ListCashBoxes(ctx, taxpayerID)
	if err != nil {
		return nil, "", err
	}
	cur := map[string]string{}
	for _, b := range boxes {
		if b.Iban.Valid {
			cur[b.Iban.String] = b.Currency
		}
	}
	for _, v := range m.IBANs {
		c := cur[v]
		if c == "" {
			c = "TRY"
		}
		ibans = append(ibans, formatIBAN(v)+"  /  "+c)
	}
	now := s.now()
	pdf, err := InstructionPDF(InstructionData{
		BankName: a.BankName(), Title: tp.Title, TaxID: tp.TaxID, TaxOffice: tp.TaxOffice,
		IBANs: ibans, EgressIPs: s.opt.EgressIPs, TechContact: s.opt.TechContact, Date: now.In(istanbul),
	})
	if err != nil {
		return nil, "", err
	}
	if row.SetupState == "draft" || row.SetupState == "failed" && !row.CredentialsRef.Valid {
		if _, err := s.update(ctx, row, "form_ready", row.Status, nil, map[string]any{"form_at": now}); err != nil {
			return nil, "", err
		}
	}
	name := fmt.Sprintf("%s-talimat-%s.pdf", m.BankID, tp.TaxID)
	return pdf, name, nil
}

func formatIBAN(v string) string {
	var b strings.Builder
	for i, r := range v {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// MarkSubmitted records that the taxpayer handed the instruction to the branch.
func (s *Service) MarkSubmitted(ctx context.Context, taxpayerID int64, id uuid.UUID) (Connection, error) {
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	if row.SetupState != "draft" && row.SetupState != "form_ready" && row.SetupState != "submitted_to_bank" {
		return Connection{}, ErrBadState
	}
	row, err = s.update(ctx, row, "submitted_to_bank", row.Status, nil, map[string]any{"submitted_at": s.now()})
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

// SetCredentials stores the bank's service user in the vault and verifies it
// with a one-day query: success links the accounts to cash boxes and
// activates the connection; a bank refusal leaves it failed with the reason.
// The returned Connection describes either outcome.
func (s *Service) SetCredentials(ctx context.Context, taxpayerID int64, id uuid.UUID, values map[string]string) (Connection, error) {
	if s.vault == nil {
		return Connection{}, ErrNoVault
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	m := meta(row)
	a, ok := s.reg.Get(m.BankID, m.Environment)
	if !ok {
		return Connection{}, ErrUnknownBank
	}
	creds := Credentials{}
	for _, f := range a.CredentialFields() {
		v := strings.TrimSpace(values[f.Key])
		if v == "" {
			return Connection{}, fmt.Errorf("%w: %s", ErrBadCredentials, f.Key)
		}
		creds[f.Key] = v
	}
	plain, _ := json.Marshal(creds)
	secret := secrets.NewSecret(plain)
	ref := secrets.Ref(row.CredentialsRef.String)
	if row.CredentialsRef.Valid {
		if err := s.vault.Replace(ctx, VaultPurpose, ref, secret); err != nil {
			return Connection{}, err
		}
	} else {
		if ref, err = s.vault.Put(ctx, VaultPurpose, secret); err != nil {
			return Connection{}, err
		}
		if _, err := s.q.SetSourceConnectionCredentialsRef(ctx, db.SetSourceConnectionCredentialsRefParams{
			CredentialsRef: pgtype.Text{String: string(ref), Valid: true}, SetupState: "credentials_entered",
			TaxpayerID: taxpayerID, ID: row.ID,
		}); err != nil {
			return Connection{}, err
		}
	}
	if row, err = s.update(ctx, row, "verifying", row.Status, nil, nil); err != nil {
		return Connection{}, err
	}
	return s.verify(ctx, row, a, creds)
}

// ErrTaxIDMismatch: the bank's accounts belong to another VKN (22 §1).
const reasonTaxIDMismatch Reason = "tax_id_mismatch"

func (s *Service) verify(ctx context.Context, row db.SourceConnection, a Adapter, creds Credentials) (Connection, error) {
	now := s.now().In(istanbul)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
	res, err := a.Fetch(ctx, creds, Window{Start: day, End: now})
	if err != nil {
		return s.fail(ctx, row, AsError(err))
	}
	tp, err := s.q.GetTaxpayerByID(ctx, row.TaxpayerID)
	if err != nil {
		return Connection{}, err
	}
	for _, acc := range res.Accounts {
		if acc.OwnerTaxID != "" && acc.OwnerTaxID != tp.TaxID {
			return s.fail(ctx, row, &Error{Reason: reasonTaxIDMismatch, Message: "hesap sahibi VKN " + acc.OwnerTaxID})
		}
	}
	ibans, err := s.linkAccounts(ctx, row, a, res.Accounts)
	if err != nil {
		return Connection{}, err
	}
	row, err = s.update(ctx, row, "active", "active", nil, map[string]any{"verified_at": s.now(), "ibans": ibans})
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

func (s *Service) fail(ctx context.Context, row db.SourceConnection, be *Error) (Connection, error) {
	status := "pending"
	switch be.Reason {
	case ReasonBadCredentials, ReasonIPRejected, ReasonUnauthorized, ReasonInstructionMissing:
		status = "auth_failed"
	case ReasonRateLimited:
		status = "rate_limited"
	}
	msg := be.Reason.Message()
	if be.Reason == reasonTaxIDMismatch {
		msg = "Bankadaki hesaplar başka bir vergi numarasına ait. Talimatı mükellefin kendi hesapları için verin."
	}
	state := "failed"
	if row.SetupState == "active" {
		state = "active" // a sync failure keeps the setup, only the status changes
	}
	row, err := s.update(ctx, row, state, status, &ErrorView{Reason: be.Reason, Code: be.Code, Message: msg, At: s.now()}, nil)
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

// linkAccounts maps the bank's accounts to cash boxes by IBAN (20 §5.3):
// an existing box is linked, a missing one is created.
func (s *Service) linkAccounts(ctx context.Context, row db.SourceConnection, a Adapter, accs []Account) ([]string, error) {
	var ibans []string
	for _, acc := range accs {
		if banking.NormalizeIBAN(acc.IBAN) == "" {
			continue
		}
		ibans = append(ibans, acc.IBAN)
		box, err := s.q.FindCashBoxByIBAN(ctx, db.FindCashBoxByIBANParams{TaxpayerID: row.TaxpayerID, Iban: pgtype.Text{String: acc.IBAN, Valid: true}})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			name := acc.Nickname
			if name == "" {
				name = a.BankName() + " " + acc.Currency
			}
			_, err = s.q.InsertCashBox(ctx, db.InsertCashBoxParams{
				TaxpayerID: row.TaxpayerID, Type: "bank_account", Name: name,
				BankID: text(a.BankID()), Iban: text(acc.IBAN), Currency: acc.Currency,
				BankAccountNumber: text(acc.Number), BankBranchCode: text(acc.BranchCode),
				BankAccountKind: text(acc.Kind), BankConnectionID: pgtype.Int8{Int64: row.ID, Valid: true},
			})
			if err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		default:
			if _, err := s.q.LinkCashBoxToBankConnection(ctx, db.LinkCashBoxToBankConnectionParams{
				BankConnectionID: pgtype.Int8{Int64: row.ID, Valid: true}, BankID: text(a.BankID()),
				BankAccountNumber: text(acc.Number), BankBranchCode: text(acc.BranchCode),
				TaxpayerID: row.TaxpayerID, ID: box.ID,
			}); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(ibans)
	return ibans, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// SyncResult reports one "şimdi çek".
type SyncResult struct {
	Windows            int        `json:"windows"`
	Accounts           int        `json:"accounts"`
	Inserted           int        `json:"inserted"`
	Merged             int        `json:"merged"`
	Skipped            int        `json:"skipped"`
	PossibleDuplicates int        `json:"possible_duplicates"`
	Connection         Connection `json:"connection"`
}

// Sync reads [from, to] (dates, Europe/Istanbul) and writes the movements
// through banking.Service (dedup and classification, 21 §5). Production runs
// this from the fixed-egress worker on the 20 §7.2 schedule; the endpoint is
// the accountant's "şimdi çek".
func (s *Service) Sync(ctx context.Context, taxpayerID int64, id uuid.UUID, from, to time.Time) (SyncResult, error) {
	if s.vault == nil {
		return SyncResult{}, ErrNoVault
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return SyncResult{}, err
	}
	if row.SetupState != "active" || !row.CredentialsRef.Valid {
		return SyncResult{}, ErrBadState
	}
	m := meta(row)
	now := s.now()
	if m.LastSyncRequest != nil && now.Sub(*m.LastSyncRequest) < SyncThrottle {
		return SyncResult{}, ErrThrottled
	}
	a, ok := s.reg.Get(m.BankID, m.Environment)
	if !ok {
		return SyncResult{}, ErrUnknownBank
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, istanbul)
	end := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 0, 0, istanbul)
	if nowTR := now.In(istanbul); end.After(nowTR) {
		end = nowTR
	}
	if end.Before(start) {
		return SyncResult{}, ErrBadRange
	}
	if h := a.Limits().MaxHistoryDays; h > 0 && start.Before(now.In(istanbul).AddDate(0, 0, -h)) {
		return SyncResult{}, fmt.Errorf("%w: %s keeps %d days", ErrBadRange, a.BankName(), h)
	}
	if _, err := s.update(ctx, row, row.SetupState, row.Status, nil, map[string]any{"last_sync_request_at": now}); err != nil {
		return SyncResult{}, err
	}
	sec, err := s.vault.Get(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String))
	if err != nil {
		return SyncResult{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(sec.Reveal(), &creds); err != nil {
		return SyncResult{}, err
	}

	out := SyncResult{}
	seen := map[string]bool{}
	for _, w := range Windows(start, end, a.Limits().MaxRangeDays) {
		res, err := a.Fetch(ctx, creds, w)
		if err != nil {
			c, ferr := s.fail(ctx, row, AsError(err))
			if ferr != nil {
				return out, ferr
			}
			out.Connection = c
			return out, AsError(err)
		}
		out.Windows++
		for _, acc := range res.Accounts {
			if acc.IBAN == "" {
				continue
			}
			if !seen[acc.IBAN] {
				seen[acc.IBAN] = true
				out.Accounts++
			}
			if len(acc.Movements) == 0 {
				continue
			}
			box, err := s.q.FindCashBoxByIBAN(ctx, db.FindCashBoxByIBANParams{TaxpayerID: taxpayerID, Iban: pgtype.Text{String: acc.IBAN, Valid: true}})
			if errors.Is(err, pgx.ErrNoRows) {
				if _, err := s.linkAccounts(ctx, row, a, []Account{acc}); err != nil {
					return out, err
				}
				box, err = s.q.FindCashBoxByIBAN(ctx, db.FindCashBoxByIBANParams{TaxpayerID: taxpayerID, Iban: pgtype.Text{String: acc.IBAN, Valid: true}})
			}
			if err != nil {
				return out, err
			}
			r, err := s.bank.Ingest(ctx, taxpayerID, box.ID, nil, acc.Movements)
			if err != nil {
				return out, err
			}
			out.Inserted += r.Inserted
			out.Merged += r.Merged
			out.Skipped += r.Skipped
			out.PossibleDuplicates += r.PossibleDuplicates
		}
	}
	p, _ := json.Marshal(map[string]any{"last_sync_at": s.now()})
	if err := s.q.MarkBankConnectionSynced(ctx, db.MarkBankConnectionSyncedParams{MetaPatch: p, TaxpayerID: taxpayerID, ID: row.ID}); err != nil {
		return out, err
	}
	c, err := s.Get(ctx, taxpayerID, id)
	out.Connection = c
	return out, err
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()
