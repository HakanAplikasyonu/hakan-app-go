// Package posting books bank movements into the trial balance (issue
// #383): the counterparty alias memory (who a payer is in the mizan), the
// account code suggestion for every movement, its review and the journal
// drafts of approved movements.
package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Errors.
var (
	ErrInvalid  = errors.New("posting: invalid input")
	ErrNotFound = errors.New("posting: not found")
)

// Alias match types.
const (
	MatchIBAN  = "iban"
	MatchTaxID = "tax_id"
	MatchName  = "name"
)

// Alias statuses and origins.
const (
	StatusLearned = "learned"
	StatusRule    = "rule"

	OriginDecision  = "decision"
	OriginManual    = "manual"
	OriginCandidate = "candidate"
)

// Rule candidate defaults (account_rule_settings, #333): an office without a
// row asks after 3 decisions, counting corrections and manual acceptances.
const (
	defaultMinCount       = 3
	decisionsCorrections  = "corrections_only"
	decisionsWithAccepts  = "corrections_and_accepts"
	defaultDecisionsCount = decisionsWithAccepts
)

// Subject is the engagement the alias memory belongs to.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
	TaxpayerID     int64
}

// Counterparty is what a movement says about the other side.
type Counterparty struct {
	Name      string
	IBAN      string
	TaxID     string
	Direction banking.Direction
}

// Keys are the normalized lookup keys of a counterparty.
type Keys struct {
	IBAN, TaxID, Name string
}

// KeysOf normalizes a counterparty's keys.
func KeysOf(c Counterparty) Keys {
	vkn, tckn := banking.SplitTaxID(c.TaxID)
	return Keys{IBAN: banking.NormalizeIBAN(c.IBAN), TaxID: vkn + tckn, Name: banking.NormalizeName(c.Name)}
}

func (k Keys) empty() bool { return k.IBAN == "" && k.TaxID == "" && k.Name == "" }

// Alias is the API view of an alias.
type Alias struct {
	UUID          uuid.UUID  `json:"uuid"`
	MatchType     string     `json:"match_type"`
	MatchValue    string     `json:"match_value"`
	DisplayName   string     `json:"display_name"`
	Direction     *string    `json:"direction"`
	AccountCode   string     `json:"account_code"`
	AccountName   string     `json:"account_name"`
	Status        string     `json:"status"`
	Origin        string     `json:"origin"`
	Accepts       int32      `json:"accepts"`
	Corrections   int32      `json:"corrections"`
	Hits          int32      `json:"hits"`
	LastDecidedAt *time.Time `json:"last_decided_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func aliasView(r db.BankCounterpartyAlias) Alias {
	a := Alias{UUID: r.Uuid, MatchType: r.MatchType, MatchValue: r.MatchValue, DisplayName: r.DisplayName,
		AccountCode: r.AccountCode, AccountName: r.AccountName, Status: r.Status, Origin: r.Origin,
		Accepts: r.Accepts, Corrections: r.Corrections, Hits: r.Hits, UpdatedAt: r.UpdatedAt.Time}
	if r.DebitCredit.Valid {
		d := r.DebitCredit.String
		a.Direction = &d
	}
	if r.LastDecidedAt.Valid {
		t := r.LastDecidedAt.Time
		a.LastDecidedAt = &t
	}
	return a
}

// Match is the alias that names a movement's account.
type Match struct {
	Alias      db.BankCounterpartyAlias
	Confidence decimal.Decimal
	Reason     string
}

var matchLabel = map[string]string{MatchIBAN: "IBAN", MatchTaxID: "VKN/TCKN", MatchName: "ad"}

// Resolve finds the alias of a counterparty: a rule before a learned alias,
// then IBAN before VKN / TCKN before the name, a directional alias before a
// two-way one. A rule (defined or
// promoted) is surer than a learned alias; repeated decisions add a little.
func Resolve(ctx context.Context, q *db.Queries, sub Subject, c Counterparty) (*Match, error) {
	k := KeysOf(c)
	if k.empty() {
		return nil, nil
	}
	rows, err := q.FindBankCounterpartyAliases(ctx, db.FindBankCounterpartyAliasesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, DebitCredit: string(c.Direction),
		Iban: orNone(k.IBAN), TaxID: orNone(k.TaxID), Name: orNone(k.Name),
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	a := rows[0]
	conf := decimal.RequireFromString("0.90")
	if a.MatchType == MatchName {
		conf = decimal.RequireFromString("0.85")
	}
	if a.Accepts+a.Corrections >= 2 {
		conf = conf.Add(decimal.RequireFromString("0.03"))
	}
	if a.Status == StatusRule {
		conf = decimal.RequireFromString("0.97")
	}
	who := a.DisplayName
	if who == "" {
		who = a.MatchValue
	}
	reason := fmt.Sprintf("Takma ad (%s): %s → %s", matchLabel[a.MatchType], who, a.AccountCode)
	if a.AccountName != "" {
		reason += " " + a.AccountName
	}
	return &Match{Alias: a, Confidence: conf, Reason: reason}, nil
}

// orNone: an absent key is "", which never matches (match_value has a
// non-empty CHECK).
func orNone(s string) string { return s }

// Decision is what the accountant did with a suggestion.
type Decision struct {
	Correction   bool // corrected rather than accepted
	AccountCode  string
	AccountName  string
	SuggestionID int64
	UserID       int64
	At           time.Time
}

// Learn remembers the decided account under every key of the counterparty
// (direction-specific: one firm can be customer and supplier). The same code
// again counts the decision; another code replaces the alias and starts the
// count anew (a correction against a rule demotes it to learned). With a
// VKN / TCKN the cari hafızası (counterparty_accounts, #319) is written too,
// so the next invoice of the same firm finds the same cari.
func Learn(ctx context.Context, q *db.Queries, sub Subject, c Counterparty, d Decision) error {
	if !engine.ValidCode(d.AccountCode) {
		return fmt.Errorf("%w: account code %q", ErrInvalid, d.AccountCode)
	}
	k := KeysOf(c)
	dir := pgtype.Text{String: string(c.Direction), Valid: c.Direction != ""}
	at := pgtype.Timestamptz{Time: d.At, Valid: !d.At.IsZero()}
	if !at.Valid {
		at = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	sugID := pgtype.Int8{Int64: d.SuggestionID, Valid: d.SuggestionID > 0}
	user := pgtype.Int8{Int64: d.UserID, Valid: d.UserID > 0}
	display := strings.TrimSpace(c.Name)
	for _, key := range []struct{ typ, val string }{{MatchIBAN, k.IBAN}, {MatchTaxID, k.TaxID}, {MatchName, k.Name}} {
		if key.val == "" {
			continue
		}
		cur, err := q.GetBankCounterpartyAliasByKey(ctx, db.GetBankCounterpartyAliasByKeyParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, MatchType: key.typ, MatchValue: key.val, DebitCredit: dir,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			acc, cor := int32(1), int32(0)
			if d.Correction {
				acc, cor = 0, 1
			}
			if _, err := q.InsertBankCounterpartyAlias(ctx, db.InsertBankCounterpartyAliasParams{
				OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
				MatchType: key.typ, MatchValue: key.val, DisplayName: display, DebitCredit: dir,
				AccountCode: d.AccountCode, AccountName: d.AccountName, Status: StatusLearned, Origin: OriginDecision,
				Accepts: acc, Corrections: cor, SuggestionID: sugID, DecidedBy: user, LastDecidedAt: at,
			}); err != nil {
				return fmt.Errorf("learn alias: %w", err)
			}
			continue
		case err != nil:
			return err
		}
		p := db.UpdateBankCounterpartyAliasParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: cur.ID,
			DisplayName: firstNonEmpty(display, cur.DisplayName), AccountCode: cur.AccountCode,
			AccountName: firstNonEmpty(d.AccountName, cur.AccountName), Status: cur.Status, Origin: cur.Origin,
			Accepts: cur.Accepts, Corrections: cur.Corrections, SuggestionID: sugID, DecidedBy: user, LastDecidedAt: at,
		}
		if cur.AccountCode != d.AccountCode {
			p.AccountCode, p.AccountName = d.AccountCode, d.AccountName
			p.Status, p.Origin, p.Accepts, p.Corrections = StatusLearned, OriginDecision, 0, 0
		}
		if d.Correction {
			p.Corrections++
		} else {
			p.Accepts++
		}
		if _, err := q.UpdateBankCounterpartyAlias(ctx, p); err != nil {
			return fmt.Errorf("learn alias: %w", err)
		}
	}
	if k.TaxID != "" && cariMain[d.AccountCode[:3]] {
		if err := q.UpsertCounterpartyAccount(ctx, db.UpsertCounterpartyAccountParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
			CounterpartyTaxID: k.TaxID, MainCode: d.AccountCode[:3], AccountCode: d.AccountCode,
			AccountName: firstNonEmpty(d.AccountName, display), SuggestionID: sugID, DecidedBy: user,
		}); err != nil {
			return fmt.Errorf("remember counterparty account: %w", err)
		}
	}
	return nil
}

// cariMain: main accounts that hold one sub-account per counterparty.
var cariMain = map[string]bool{
	"120": true, "121": true, "126": true, "131": true, "136": true, "159": true,
	"320": true, "321": true, "326": true, "329": true, "331": true, "335": true, "336": true, "340": true,
}

// Unlearn takes back what a reverted decision taught: the decision count
// drops, an alias only that decision made is deleted, and the cari hafızası
// written by it is forgotten.
func Unlearn(ctx context.Context, q *db.Queries, sub Subject, suggestionID int64) error {
	if suggestionID <= 0 {
		return nil
	}
	rows, err := q.ListBankCounterpartyAliasesBySuggestion(ctx, db.ListBankCounterpartyAliasesBySuggestionParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, SuggestionID: pgtype.Int8{Int64: suggestionID, Valid: true},
	})
	if err != nil {
		return err
	}
	for _, a := range rows {
		if a.Origin == OriginDecision && a.Accepts+a.Corrections <= 1 {
			if _, err := q.DeleteBankCounterpartyAlias(ctx, db.DeleteBankCounterpartyAliasParams{
				OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: a.Uuid,
			}); err != nil {
				return err
			}
			continue
		}
		p := db.UpdateBankCounterpartyAliasParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: a.ID,
			DisplayName: a.DisplayName, AccountCode: a.AccountCode, AccountName: a.AccountName, Status: a.Status,
			Origin: a.Origin, Accepts: a.Accepts, Corrections: a.Corrections, DecidedBy: a.DecidedBy, LastDecidedAt: a.LastDecidedAt,
		}
		if p.Corrections > 0 {
			p.Corrections--
		} else if p.Accepts > 0 {
			p.Accepts--
		}
		if _, err := q.UpdateBankCounterpartyAlias(ctx, p); err != nil {
			return err
		}
	}
	_, err = q.DeleteCounterpartyAccountBySuggestion(ctx, db.DeleteCounterpartyAccountBySuggestionParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, SuggestionID: pgtype.Int8{Int64: suggestionID, Valid: true},
	})
	return err
}

// Service is the alias memory API.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) q() *db.Queries { return database.QueriesFor(s.pool) }

// AliasPageMax is the largest alias page (issue #453); the list grows with
// every counterparty the office teaches.
const AliasPageMax = 500

// Aliases lists the engagement's first AliasPageMax aliases (status "" = all).
func (s *Service) Aliases(ctx context.Context, sub Subject, status string) ([]Alias, error) {
	out, _, err := s.AliasesPage(ctx, sub, status, AliasPageMax, 0)
	return out, err
}

// AliasesPage is one page of the engagement's aliases, newest change
// first, and their total.
func (s *Service) AliasesPage(ctx context.Context, sub Subject, status string, limit, offset int32) ([]Alias, int64, error) {
	if limit < 1 || limit > AliasPageMax {
		limit = AliasPageMax
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.q().ListBankCounterpartyAliasesPage(ctx, db.ListBankCounterpartyAliasesPageParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Status: pgtype.Text{String: status, Valid: status != ""},
		Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Alias, len(rows))
	var total int64
	for i, r := range rows {
		out[i] = aliasView(r.BankCounterpartyAlias)
		total = r.TotalCount
	}
	return out, total, nil
}

// AliasInput defines an alias by hand.
type AliasInput struct {
	MatchType   string  `json:"match_type"`
	MatchValue  string  `json:"match_value"`
	DisplayName string  `json:"display_name"`
	Direction   *string `json:"direction"`
	AccountCode string  `json:"account_code"`
	AccountName string  `json:"account_name"`
}

func normalizeKey(typ, v string) (string, error) {
	switch typ {
	case MatchIBAN:
		if n := banking.NormalizeIBAN(v); n != "" {
			return n, nil
		}
		return "", fmt.Errorf("%w: match_value is not a valid IBAN", ErrInvalid)
	case MatchTaxID:
		vkn, tckn := banking.SplitTaxID(v)
		if vkn+tckn != "" {
			return vkn + tckn, nil
		}
		return "", fmt.Errorf("%w: match_value must be a 10-digit VKN or an 11-digit TCKN", ErrInvalid)
	case MatchName:
		if n := banking.NormalizeName(v); n != "" {
			return n, nil
		}
		return "", fmt.Errorf("%w: match_value is empty", ErrInvalid)
	}
	return "", fmt.Errorf("%w: match_type must be iban, tax_id or name", ErrInvalid)
}

// CreateAlias defines (or redefines) an alias as a rule.
func (s *Service) CreateAlias(ctx context.Context, sub Subject, in AliasInput, userID int64) (Alias, error) {
	val, err := normalizeKey(in.MatchType, in.MatchValue)
	if err != nil {
		return Alias{}, err
	}
	code := strings.TrimSpace(in.AccountCode)
	if !engine.ValidCode(code) {
		return Alias{}, fmt.Errorf("%w: account_code %q", ErrInvalid, in.AccountCode)
	}
	dir := pgtype.Text{}
	if in.Direction != nil && *in.Direction != "" {
		if *in.Direction != string(banking.DirIn) && *in.Direction != string(banking.DirOut) {
			return Alias{}, fmt.Errorf("%w: direction must be A or B", ErrInvalid)
		}
		dir = pgtype.Text{String: *in.Direction, Valid: true}
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = strings.TrimSpace(in.MatchValue)
	}
	user := pgtype.Int8{Int64: userID, Valid: userID > 0}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	q := s.q()
	cur, err := q.GetBankCounterpartyAliasByKey(ctx, db.GetBankCounterpartyAliasByKeyParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, MatchType: in.MatchType, MatchValue: val, DebitCredit: dir,
	})
	if err == nil {
		row, err := q.UpdateBankCounterpartyAlias(ctx, db.UpdateBankCounterpartyAliasParams{
			OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: cur.ID, DisplayName: display,
			AccountCode: code, AccountName: strings.TrimSpace(in.AccountName), Status: StatusRule, Origin: OriginManual,
			Accepts: cur.Accepts, Corrections: cur.Corrections, DecidedBy: user, LastDecidedAt: now,
		})
		return aliasView(row), err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Alias{}, err
	}
	row, err := q.InsertBankCounterpartyAlias(ctx, db.InsertBankCounterpartyAliasParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
		MatchType: in.MatchType, MatchValue: val, DisplayName: display, DebitCredit: dir,
		AccountCode: code, AccountName: strings.TrimSpace(in.AccountName), Status: StatusRule, Origin: OriginManual,
		DecidedBy: user, LastDecidedAt: now,
	})
	if err != nil {
		return Alias{}, err
	}
	return aliasView(row), nil
}

// AliasPatch changes an alias's account or status.
type AliasPatch struct {
	AccountCode *string `json:"account_code"`
	AccountName *string `json:"account_name"`
	Status      *string `json:"status"`
}

// UpdateAlias edits an alias. Setting status rule promotes it by hand.
func (s *Service) UpdateAlias(ctx context.Context, sub Subject, id uuid.UUID, in AliasPatch, userID int64) (Alias, error) {
	q := s.q()
	cur, err := q.GetBankCounterpartyAlias(ctx, db.GetBankCounterpartyAliasParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Alias{}, ErrNotFound
	} else if err != nil {
		return Alias{}, err
	}
	p := db.UpdateBankCounterpartyAliasParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: cur.ID, DisplayName: cur.DisplayName,
		AccountCode: cur.AccountCode, AccountName: cur.AccountName, Status: cur.Status, Origin: cur.Origin,
		Accepts: cur.Accepts, Corrections: cur.Corrections, SuggestionID: cur.SuggestionID,
		DecidedBy: pgtype.Int8{Int64: userID, Valid: userID > 0}, LastDecidedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	if in.AccountCode != nil {
		code := strings.TrimSpace(*in.AccountCode)
		if !engine.ValidCode(code) {
			return Alias{}, fmt.Errorf("%w: account_code %q", ErrInvalid, *in.AccountCode)
		}
		if code != p.AccountCode {
			p.AccountCode, p.Origin = code, OriginManual
		}
	}
	if in.AccountName != nil {
		p.AccountName = strings.TrimSpace(*in.AccountName)
	}
	if in.Status != nil {
		if *in.Status != StatusLearned && *in.Status != StatusRule {
			return Alias{}, fmt.Errorf("%w: status must be learned or rule", ErrInvalid)
		}
		p.Status = *in.Status
	}
	row, err := q.UpdateBankCounterpartyAlias(ctx, p)
	if err != nil {
		return Alias{}, err
	}
	return aliasView(row), nil
}

// DeleteAlias forgets an alias.
func (s *Service) DeleteAlias(ctx context.Context, sub Subject, id uuid.UUID) error {
	n, err := s.q().DeleteBankCounterpartyAlias(ctx, db.DeleteBankCounterpartyAliasParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Candidate is a learned alias decided often enough to ask "make it a rule?".
type Candidate struct {
	Alias
	Decisions int32  `json:"decisions"`
	Question  string `json:"question"`
}

// Candidates lists learned aliases with enough counted decisions. The
// office's rule candidate setting (account_rule_settings, #333) gives the
// count and whether manual acceptances count; automatic ones never do.
func (s *Service) Candidates(ctx context.Context, sub Subject) ([]Candidate, error) {
	q := s.q()
	minCount, decisions := int32(defaultMinCount), defaultDecisionsCount
	set, err := q.GetAccountRuleSettings(ctx, sub.OrganizationID)
	switch {
	case err == nil:
		minCount, decisions = set.RuleCandidateMinCount, set.RuleCandidateDecisions
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	rows, err := q.ListBankCounterpartyAliases(ctx, db.ListBankCounterpartyAliasesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Status: pgtype.Text{String: StatusLearned, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, r := range rows {
		n := r.Corrections
		if decisions != decisionsCorrections {
			n += r.Accepts
		}
		if n < minCount {
			continue
		}
		who := firstNonEmpty(r.DisplayName, r.MatchValue)
		verb := "ödemeleri"
		if r.DebitCredit.String == string(banking.DirIn) {
			verb = "tahsilatları"
		}
		out = append(out, Candidate{Alias: aliasView(r), Decisions: n,
			Question: fmt.Sprintf("%q %s %d kez %s hesabına işlendi. Kural yapılsın mı?", who, verb, n, r.AccountCode)})
	}
	return out, nil
}

// Promote turns a learned alias into a rule.
func (s *Service) Promote(ctx context.Context, sub Subject, id uuid.UUID, userID int64) (Alias, error) {
	q := s.q()
	cur, err := q.GetBankCounterpartyAlias(ctx, db.GetBankCounterpartyAliasParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Alias{}, ErrNotFound
	} else if err != nil {
		return Alias{}, err
	}
	row, err := q.UpdateBankCounterpartyAlias(ctx, db.UpdateBankCounterpartyAliasParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: cur.ID, DisplayName: cur.DisplayName,
		AccountCode: cur.AccountCode, AccountName: cur.AccountName, Status: StatusRule, Origin: OriginCandidate,
		Accepts: cur.Accepts, Corrections: cur.Corrections, SuggestionID: cur.SuggestionID,
		DecidedBy: pgtype.Int8{Int64: userID, Valid: userID > 0}, LastDecidedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return Alias{}, err
	}
	return aliasView(row), nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
