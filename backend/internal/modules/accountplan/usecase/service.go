// Package usecase imports the office's target account plan (issue #30,
// docs/02 §2): the Luca hesap planı or a trial balance (mizan) export of one
// engagement. Journal lines are checked against it before export, so Luca's
// "hesap kodu bulunamadı" error is caught early as ACCOUNT_NOT_IN_TARGET_PLAN.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrInvalid     = errors.New("accountplan: invalid input")
	ErrEmpty       = errors.New("accountplan: the file has no accounts")
	ErrTooLarge    = errors.New("accountplan: file is too large")
	ErrUnsupported = errors.New("accountplan: unsupported file")
	ErrNotFound    = errors.New("accountplan: not found")
)

// Limits.
const (
	MaxUploadBytes = 10 << 20
	MaxAccounts    = 50_000
	MaxCheckCodes  = 5_000
	// DefaultTarget is the only target program today (D8).
	DefaultTarget = "luca"
)

// Subject is the engagement whose plan is used.
type Subject struct {
	OrganizationID int64
	EngagementID   int64
}

// Import is one stored import (a plan version).
type Import struct {
	ID           int64     `json:"-"`
	UUID         uuid.UUID `json:"uuid"`
	Target       string    `json:"target"`
	SourceFormat string    `json:"source_format"`
	SourceKind   string    `json:"source_kind"`
	FileName     string    `json:"file_name"`
	SHA256       string    `json:"sha256"`
	AccountCount int       `json:"account_count"`
	SkippedRows  int       `json:"skipped_rows"`
	CreatedAt    time.Time `json:"created_at"`
}

// ImportResult is what Import returns.
type ImportResult struct {
	Import     Import       `json:"import"`
	HeaderRow  int          `json:"header_row"`
	Duplicates int          `json:"duplicates"`
	Skipped    []SkippedRow `json:"skipped"`
}

// NewImport is an import to persist.
type NewImport struct {
	Target, Format, Kind, FileName, SHA256 string
	Skipped                                int
	UserID                                 int64
	Accounts                               []Account
}

// Page is a page of accounts.
type Page struct {
	Items []Account `json:"items"`
	Total int64     `json:"total"`
}

// Found is a code present in the plan.
type Found struct {
	Code   string
	IsLeaf bool
}

// Store persists plans.
type Store interface {
	// Replace stores imp as the engagement's active plan for imp.Target in
	// one transaction: the previous accounts are replaced, the import row
	// is kept as history.
	Replace(ctx context.Context, s Subject, imp NewImport) (Import, error)
	Active(ctx context.Context, s Subject, target string) (Import, error) // ErrNotFound when none
	Imports(ctx context.Context, s Subject, limit int32) ([]Import, error)
	Accounts(ctx context.Context, s Subject, target, q string, limit, offset int32) (Page, error)
	Find(ctx context.Context, s Subject, target string, codes []string) ([]Found, error)
	// Codes returns every code of the active plan (nil when none loaded).
	Codes(ctx context.Context, s Subject, target string) ([]string, error)
}

// Service runs imports and checks.
type Service struct{ store Store }

// New builds the service.
func New(store Store) *Service { return &Service{store: store} }

var targetRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func target(t string) (string, error) {
	if t == "" {
		return DefaultTarget, nil
	}
	if !targetRe.MatchString(t) {
		return "", fmt.Errorf("%w: target", ErrInvalid)
	}
	return t, nil
}

// Import parses data and makes it the engagement's active plan.
func (s *Service) Import(ctx context.Context, sub Subject, userID int64, tgt, fileName string, data []byte) (ImportResult, error) {
	tgt, err := target(tgt)
	if err != nil {
		return ImportResult{}, err
	}
	if len(data) == 0 {
		return ImportResult{}, ErrEmpty
	}
	if len(data) > MaxUploadBytes {
		return ImportResult{}, ErrTooLarge
	}
	p, err := Parse(fileName, data)
	if err != nil {
		return ImportResult{}, err
	}
	if len(p.Accounts) == 0 {
		return ImportResult{}, ErrEmpty
	}
	if len(p.Accounts) > MaxAccounts {
		return ImportResult{}, fmt.Errorf("%w: more than %d accounts", ErrTooLarge, MaxAccounts)
	}
	sum := sha256.Sum256(data)
	if len(fileName) > 255 {
		fileName = fileName[:255]
	}
	imp, err := s.store.Replace(ctx, sub, NewImport{
		Target: tgt, Format: p.Format, Kind: p.Kind, FileName: fileName,
		SHA256: hex.EncodeToString(sum[:]), Skipped: p.SkipCount, UserID: userID, Accounts: p.Accounts,
	})
	if err != nil {
		return ImportResult{}, err
	}
	skipped := p.Skipped
	if skipped == nil {
		skipped = []SkippedRow{}
	}
	return ImportResult{Import: imp, HeaderRow: p.HeaderRow, Duplicates: p.Duplicates, Skipped: skipped}, nil
}

// Imports lists the import history, newest first.
func (s *Service) Imports(ctx context.Context, sub Subject, limit int32) ([]Import, error) {
	return s.store.Imports(ctx, sub, limit)
}

// Accounts lists the active plan.
func (s *Service) Accounts(ctx context.Context, sub Subject, tgt, q string, limit, offset int32) (Page, error) {
	tgt, err := target(tgt)
	if err != nil {
		return Page{}, err
	}
	return s.store.Accounts(ctx, sub, tgt, strings.TrimSpace(q), limit, offset)
}

// CheckResult is the verdict on a set of codes.
type CheckResult struct {
	// PlanLoaded is false when the engagement has no plan for the target:
	// nothing can be checked, nothing is reported missing.
	PlanLoaded bool     `json:"plan_loaded"`
	Import     *Import  `json:"import,omitempty"`
	Missing    []string `json:"missing"`
	NonLeaf    []string `json:"non_leaf"`
	Invalid    []string `json:"invalid"`
}

// Check reports which codes are not in the active plan (missing), which are
// group accounts with sub-accounts (non_leaf) and which are not account
// codes at all (invalid, also treated as missing by the gate).
func (s *Service) Check(ctx context.Context, sub Subject, tgt string, codes []string) (CheckResult, error) {
	res := CheckResult{Missing: []string{}, NonLeaf: []string{}, Invalid: []string{}}
	tgt, err := target(tgt)
	if err != nil {
		return res, err
	}
	if len(codes) > MaxCheckCodes {
		return res, fmt.Errorf("%w: at most %d codes", ErrInvalid, MaxCheckCodes)
	}
	imp, err := s.store.Active(ctx, sub, tgt)
	if errors.Is(err, ErrNotFound) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.PlanLoaded, res.Import = true, &imp
	set := map[string]bool{}
	var valid []string
	for _, c := range codes {
		n := NormalizeCode(c)
		if set[n] {
			continue
		}
		set[n] = true
		if !ValidCode(n) {
			res.Invalid = append(res.Invalid, c)
			continue
		}
		valid = append(valid, n)
	}
	found, err := s.store.Find(ctx, sub, tgt, valid)
	if err != nil {
		return res, err
	}
	in := make(map[string]bool, len(found))
	for _, f := range found {
		in[f.Code] = true
		if !f.IsLeaf {
			res.NonLeaf = append(res.NonLeaf, f.Code)
		}
	}
	for _, c := range valid {
		if !in[c] {
			res.Missing = append(res.Missing, c)
		}
	}
	sort.Strings(res.Missing)
	sort.Strings(res.NonLeaf)
	return res, nil
}

// CodeSet returns the active plan of target as a set, nil when the
// engagement has no plan (the export then skips the account check).
func (s *Service) CodeSet(ctx context.Context, sub Subject, tgt string) (map[string]bool, error) {
	tgt, err := target(tgt)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Active(ctx, sub, tgt); errors.Is(err, ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	codes, err := s.store.Codes(ctx, sub, tgt)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(codes))
	for _, c := range codes {
		set[c] = true
	}
	return set, nil
}
