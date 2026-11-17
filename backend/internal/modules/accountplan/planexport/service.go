// Package planexport produces the Luca "Hesap Planı Aktar" file (issue
// #464): cari sub account codes under a main account (120, 320, 159, 329 …)
// that do not clash with the engagement's Luca plan, a "new accounts" file
// with only what Luca does not have yet (compared with the last imported
// Luca hesap planı / mizan) and the full plan file. Every download is kept
// in the history. The file layout is an unverified YAML template
// (accountingexport/luca/templates/hesap_plani_*.yaml).
package planexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/luca"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Target is the only program with a plan file today.
const Target = "luca"

// File kinds.
const (
	KindNew  = "new"
	KindFull = "full"
)

// Main accounts used by Generate for customers and suppliers.
const (
	MainCustomers = "120"
	MainSuppliers = "320"
)

// Errors.
var (
	ErrInvalid       = errors.New("planexport: invalid input")
	ErrNotFound      = errors.New("planexport: not found")
	ErrNothingNew    = errors.New("planexport: Luca already has every account")
	ErrNoFreeCode    = luca.ErrNoFreeCode
	ErrMissingFields = ae.ErrMissingField
)

var taxIDRe = regexp.MustCompile(`^[0-9]{10,11}$`)

// DB is what the service needs.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Service allocates sub accounts and writes plan files.
type Service struct {
	db  DB
	q   *db.Queries
	now func() time.Time
}

// New builds the service. Statements join the request's row level
// security scope (database.Ambient).
func New(pool DB) *Service {
	amb := database.Ambient{Pool: pool}
	return &Service{db: amb, q: db.New(amb), now: time.Now}
}

// Subaccount is one cari sub account code.
type Subaccount struct {
	Code           string     `json:"code"`
	ParentCode     string     `json:"parent_code,omitempty"`
	MainCode       string     `json:"main_code"`
	Name           string     `json:"name"`
	CounterpartyID string     `json:"counterparty_key"`
	CustomerUUID   *uuid.UUID `json:"customer_uuid,omitempty"`
	// Source: generated (by the portal), memory (an earlier review
	// decision, counterparty_accounts).
	Source    string     `json:"source"`
	InPlan    bool       `json:"in_plan"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// AllocateInput asks for the sub account of one cari.
type AllocateInput struct {
	MainCode string `json:"main_code"`
	// CustomerUUID picks the cari card; or give TaxID and Name.
	CustomerUUID *uuid.UUID `json:"customer_uuid,omitempty"`
	TaxID        string     `json:"tax_id,omitempty"`
	Name         string     `json:"name,omitempty"`
}

// Allocate returns the cari's sub account under the main account, making
// a new code only when the cari has none: a generated code or a code from
// the cari memory is returned as is (created false). New codes are the
// next free code under the main account (luca.NextSubaccount), serialised
// per engagement and main account so two requests never get the same code.
func (s *Service) Allocate(ctx context.Context, scope engagementctx.Scope, actorID int64, in AllocateInput) (Subaccount, bool, error) {
	main := strings.TrimSpace(in.MainCode)
	if len(main) != 3 || strings.Trim(main, "0123456789") != "" {
		return Subaccount{}, false, fmt.Errorf("%w: main_code must be three digits", ErrInvalid)
	}
	name, taxID := strings.TrimSpace(in.Name), strings.TrimSpace(in.TaxID)
	var custUUID *uuid.UUID
	if in.CustomerUUID != nil {
		c, err := s.q.GetCustomerForSubaccount(ctx, db.GetCustomerForSubaccountParams{TaxpayerID: scope.TaxpayerID, Uuid: *in.CustomerUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Subaccount{}, false, ErrNotFound
		}
		if err != nil {
			return Subaccount{}, false, err
		}
		id := c.Uuid
		custUUID = &id
		if name == "" {
			name = strings.TrimSpace(c.Name)
		}
		if taxID == "" {
			taxID = firstText(c.TaxNumber, c.IdentityNumber)
		}
	}
	if name == "" {
		return Subaccount{}, false, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if taxID != "" && !taxIDRe.MatchString(taxID) {
		return Subaccount{}, false, fmt.Errorf("%w: tax_id must be a 10 digit VKN or 11 digit TCKN", ErrInvalid)
	}
	key := taxID
	if key == "" {
		if custUUID == nil {
			return Subaccount{}, false, fmt.Errorf("%w: give tax_id or customer_uuid", ErrInvalid)
		}
		key = "customer:" + custUUID.String()
	}

	tx, err := database.Begin(ctx, s.db)
	if err != nil {
		return Subaccount{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	lock := fmt.Sprintf("generated_subaccounts:%d:%s:%s", scope.ID, Target, main)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lock); err != nil {
		return Subaccount{}, false, err
	}
	if got, err := q.GetGeneratedSubaccount(ctx, db.GetGeneratedSubaccountParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target, MainCode: main, CounterpartyKey: key,
	}); err == nil {
		return generatedView(got), false, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Subaccount{}, false, err
	}
	memory, err := q.ListEngagementCounterpartyAccounts(ctx, db.ListEngagementCounterpartyAccountsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID})
	if err != nil {
		return Subaccount{}, false, err
	}
	for _, m := range memory {
		if taxID != "" && m.CounterpartyTaxID == taxID && m.MainCode == main {
			return Subaccount{Code: m.AccountCode, MainCode: main, Name: firstNonEmpty(m.AccountName, name), CounterpartyID: key,
				CustomerUUID: custUUID, Source: "memory"}, false, tx.Commit(ctx)
		}
	}
	planRows, err := q.ListPlanAccountsForExport(ctx, db.ListPlanAccountsForExportParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target})
	if err != nil {
		return Subaccount{}, false, err
	}
	generated, err := q.ListGeneratedSubaccounts(ctx, db.ListGeneratedSubaccountsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target})
	if err != nil {
		return Subaccount{}, false, err
	}
	plan := make([]string, 0, len(planRows))
	taken := map[string]bool{}
	for _, p := range planRows {
		plan = append(plan, p.Code)
		taken[p.Code] = true
	}
	for _, g := range generated {
		taken[g.Code] = true
	}
	for _, m := range memory {
		taken[m.AccountCode] = true
	}
	tpl, err := luca.AccountPlanTemplate("")
	if err != nil {
		return Subaccount{}, false, err
	}
	code, parent, err := luca.NextSubaccount(main, plan, taken, luca.DefaultSegments(tpl))
	if err != nil {
		return Subaccount{}, false, err
	}
	p := db.InsertGeneratedSubaccountParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, Target: Target,
		MainCode: main, Code: code, ParentCode: parent, CounterpartyKey: key, Name: name, CreatedBy: actor(actorID),
	}
	if custUUID != nil {
		p.CustomerUuid = pgtype.UUID{Bytes: *custUUID, Valid: true}
	}
	row, err := q.InsertGeneratedSubaccount(ctx, p)
	if err != nil {
		return Subaccount{}, false, err
	}
	if taxID != "" {
		if err := q.RememberGeneratedCounterpartyAccount(ctx, db.RememberGeneratedCounterpartyAccountParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
			CounterpartyTaxID: taxID, MainCode: main, AccountCode: code, AccountName: name, DecidedBy: actor(actorID),
		}); err != nil {
			return Subaccount{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Subaccount{}, false, err
	}
	return generatedView(row), true, nil
}

// GenerateInput asks for sub accounts of every cari card without one.
type GenerateInput struct {
	// MainCodes limits the run (default: 120 for customers, 320 for
	// suppliers; "both" cards get both).
	MainCodes []string `json:"main_codes,omitempty"`
}

// GenerateResult summarises a Generate run.
type GenerateResult struct {
	Created  []Subaccount `json:"created"`
	Existing int          `json:"existing"`
}

// Generate allocates a sub account for every customer (120) and supplier
// (320) card of the taxpayer that has none yet.
func (s *Service) Generate(ctx context.Context, scope engagementctx.Scope, actorID int64, in GenerateInput) (GenerateResult, error) {
	want := map[string]bool{MainCustomers: true, MainSuppliers: true}
	if len(in.MainCodes) > 0 {
		want = map[string]bool{}
		for _, m := range in.MainCodes {
			if m != MainCustomers && m != MainSuppliers {
				return GenerateResult{}, fmt.Errorf("%w: generate covers %s and %s; use a single allocation for other main accounts", ErrInvalid, MainCustomers, MainSuppliers)
			}
			want[m] = true
		}
	}
	cards, err := s.q.ListSubaccountCandidates(ctx, scope.TaxpayerID)
	if err != nil {
		return GenerateResult{}, err
	}
	out := GenerateResult{Created: []Subaccount{}}
	for _, c := range cards {
		var mains []string
		if (c.Type == "customer" || c.Type == "both") && want[MainCustomers] {
			mains = append(mains, MainCustomers)
		}
		if (c.Type == "supplier" || c.Type == "both") && want[MainSuppliers] {
			mains = append(mains, MainSuppliers)
		}
		for _, m := range mains {
			id := c.Uuid
			sub, created, err := s.Allocate(ctx, scope, actorID, AllocateInput{MainCode: m, CustomerUUID: &id})
			if err != nil {
				return out, fmt.Errorf("planexport: %s (%s): %w", c.Name, m, err)
			}
			if created {
				out.Created = append(out.Created, sub)
			} else {
				out.Existing++
			}
		}
	}
	return out, nil
}

// List returns the engagement's cari sub accounts: generated ones and the
// cari memory, with whether Luca's plan (last import) has them.
func (s *Service) List(ctx context.Context, scope engagementctx.Scope) ([]Subaccount, error) {
	plan, err := s.planSet(ctx, scope)
	if err != nil {
		return nil, err
	}
	subs, err := s.subaccounts(ctx, scope)
	if err != nil {
		return nil, err
	}
	for i := range subs {
		subs[i].InPlan = plan[subs[i].Code] != ""
	}
	return subs, nil
}

func (s *Service) subaccounts(ctx context.Context, scope engagementctx.Scope) ([]Subaccount, error) {
	generated, err := s.q.ListGeneratedSubaccounts(ctx, db.ListGeneratedSubaccountsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target})
	if err != nil {
		return nil, err
	}
	memory, err := s.q.ListEngagementCounterpartyAccounts(ctx, db.ListEngagementCounterpartyAccountsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID})
	if err != nil {
		return nil, err
	}
	out := make([]Subaccount, 0, len(generated)+len(memory))
	seen := map[string]bool{}
	for _, g := range generated {
		out = append(out, generatedView(g))
		seen[g.Code] = true
	}
	for _, m := range memory {
		if seen[m.AccountCode] {
			continue
		}
		seen[m.AccountCode] = true
		out = append(out, Subaccount{Code: m.AccountCode, MainCode: m.MainCode, Name: m.AccountName, CounterpartyID: m.CounterpartyTaxID, Source: "memory"})
	}
	return out, nil
}

// planSet is code → name of the active plan.
func (s *Service) planSet(ctx context.Context, scope engagementctx.Scope) (map[string]string, error) {
	rows, err := s.q.ListPlanAccountsForExport(ctx, db.ListPlanAccountsForExportParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		name := r.Name
		if name == "" {
			name = r.Code
		}
		out[r.Code] = name
	}
	return out, nil
}

// FileInput asks for a plan file.
type FileInput struct {
	Kind     string `json:"kind"`
	Template string `json:"template,omitempty"`
}

// File is a produced plan file with its history record.
type File struct {
	Export
	Data        []byte `json:"-"`
	ContentType string `json:"content_type"`
}

// Export is one download of the history.
type Export struct {
	UUID             uuid.UUID  `json:"uuid"`
	Kind             string     `json:"kind"`
	TemplateVersion  string     `json:"template_version"`
	TemplateVerified bool       `json:"template_verified"`
	FileName         string     `json:"file_name"`
	SHA256           string     `json:"sha256"`
	Size             int32      `json:"size"`
	Accounts         int32      `json:"account_count"`
	ComparedImport   *uuid.UUID `json:"compared_import_uuid,omitempty"`
	ComparedFileName string     `json:"compared_file_name,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// BuildFile writes the plan file and records the download. new: accounts
// Luca does not have yet (cari sub accounts and their missing parents,
// compared with the last imported Luca plan / trial balance); full: the
// plan plus those.
func (s *Service) BuildFile(ctx context.Context, scope engagementctx.Scope, actorID int64, in FileInput) (File, error) {
	if in.Kind != KindNew && in.Kind != KindFull {
		return File{}, fmt.Errorf("%w: kind must be new or full", ErrInvalid)
	}
	tpl, err := luca.AccountPlanTemplate(in.Template)
	if err != nil {
		return File{}, err
	}
	plan, err := s.planSet(ctx, scope)
	if err != nil {
		return File{}, err
	}
	subs, err := s.subaccounts(ctx, scope)
	if err != nil {
		return File{}, err
	}
	cards, err := s.cards(ctx, scope, subs)
	if err != nil {
		return File{}, err
	}
	rows := map[string]luca.PlanAccount{}
	if in.Kind == KindFull {
		for code, name := range plan {
			rows[code] = luca.PlanAccount{Code: code, Name: name}
		}
	}
	for _, sub := range subs {
		a := luca.PlanAccount{Code: sub.Code, Name: sub.Name}
		if c, ok := cards[sub.CounterpartyID]; ok {
			a = c.account(sub.Code, sub.Name)
		}
		_, inPlan := plan[sub.Code]
		if in.Kind == KindFull || !inPlan {
			rows[sub.Code] = a
		}
		// A parent Luca does not have goes in too, named after the plan's
		// main account.
		if sub.ParentCode != "" && sub.ParentCode != sub.MainCode {
			if _, ok := plan[sub.ParentCode]; !ok {
				if _, ok := rows[sub.ParentCode]; !ok {
					rows[sub.ParentCode] = luca.PlanAccount{Code: sub.ParentCode, Name: firstNonEmpty(plan[sub.MainCode], sub.MainCode) + " " + strings.TrimPrefix(sub.ParentCode, sub.MainCode+".")}
				}
			}
		}
	}
	if in.Kind == KindNew && len(rows) == 0 {
		return File{}, ErrNothingNew
	}
	accounts := make([]luca.PlanAccount, 0, len(rows))
	for _, a := range rows {
		accounts = append(accounts, a)
	}
	res, err := luca.WriteAccountPlan(tpl, accounts)
	if err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(res.Data)
	name := fmt.Sprintf("luca_hesap_plani_%s_%s.csv", map[string]string{KindNew: "yeni", KindFull: "tum"}[in.Kind], s.now().Format("2006-01-02"))
	var compared pgtype.Int8
	var comparedUUID *uuid.UUID
	var comparedName string
	if imp, err := s.q.LatestAccountPlanImport(ctx, db.LatestAccountPlanImportParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target}); err == nil {
		compared = pgtype.Int8{Int64: imp.ID, Valid: true}
		id := imp.Uuid
		comparedUUID, comparedName = &id, imp.FileName
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return File{}, err
	}
	rec, err := s.q.InsertAccountPlanExport(ctx, db.InsertAccountPlanExportParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Target: Target, Kind: in.Kind,
		TemplateVersion: tpl.Version, TemplateVerified: tpl.Verified, ComparedImportID: compared, FileName: name,
		Sha256: hex.EncodeToString(sum[:]), Size: int32(len(res.Data)), AccountCount: int32(res.Rows), //nolint:gosec // bounded by the plan size
		CreatedBy: actor(actorID),
	})
	if err != nil {
		return File{}, err
	}
	return File{
		Export: Export{
			UUID: rec.Uuid, Kind: rec.Kind, TemplateVersion: rec.TemplateVersion, TemplateVerified: rec.TemplateVerified,
			FileName: rec.FileName, SHA256: rec.Sha256, Size: rec.Size, Accounts: rec.AccountCount,
			ComparedImport: comparedUUID, ComparedFileName: comparedName, CreatedAt: rec.CreatedAt.Time,
		},
		Data: res.Data, ContentType: luca.AccountPlanContentType + "; charset=" + tpl.CSV.Encoding,
	}, nil
}

// Exports lists the download history, newest first.
func (s *Service) Exports(ctx context.Context, scope engagementctx.Scope, limit, offset int32) ([]Export, error) {
	rows, err := s.q.ListAccountPlanExports(ctx, db.ListAccountPlanExportsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: limit, Off: offset})
	if err != nil {
		return nil, err
	}
	out := make([]Export, 0, len(rows))
	for _, r := range rows {
		e := Export{
			UUID: r.Uuid, Kind: r.Kind, TemplateVersion: r.TemplateVersion, TemplateVerified: r.TemplateVerified,
			FileName: r.FileName, SHA256: r.Sha256, Size: r.Size, Accounts: r.AccountCount, CreatedAt: r.CreatedAt.Time,
			ComparedFileName: r.ComparedFileName.String,
		}
		if r.ComparedImportUuid.Valid {
			id := uuid.UUID(r.ComparedImportUuid.Bytes)
			e.ComparedImport = &id
		}
		out = append(out, e)
	}
	return out, nil
}

// card is the cari card behind a sub account (tax fields for the file).
type card struct {
	TaxNo, TCKN, TaxOffice, Address, Email string
}

func (c card) account(code, name string) luca.PlanAccount {
	return luca.PlanAccount{Code: code, Name: name, TaxNo: c.TaxNo, TCKN: c.TCKN, TaxOffice: c.TaxOffice, Address: c.Address, Email: c.Email}
}

// cards finds the cari card of every sub account by VKN / TCKN or by the
// card's uuid ("customer:<uuid>" keys).
func (s *Service) cards(ctx context.Context, scope engagementctx.Scope, subs []Subaccount) (map[string]card, error) {
	var taxIDs []string
	out := map[string]card{}
	for _, sub := range subs {
		switch {
		case taxIDRe.MatchString(sub.CounterpartyID):
			taxIDs = append(taxIDs, sub.CounterpartyID)
		case sub.CustomerUUID != nil:
			c, err := s.q.GetCustomerForSubaccount(ctx, db.GetCustomerForSubaccountParams{TaxpayerID: scope.TaxpayerID, Uuid: *sub.CustomerUUID})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out[sub.CounterpartyID] = newCard(c.TaxNumber, c.IdentityNumber, c.TaxOffice, c.Address, c.District, c.City, c.Email)
		}
	}
	if len(taxIDs) == 0 {
		return out, nil
	}
	rows, err := s.q.ListCustomersByTaxIDs(ctx, db.ListCustomersByTaxIDsParams{TaxpayerID: scope.TaxpayerID, TaxIds: taxIDs})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		c := newCard(r.TaxNumber, r.IdentityNumber, r.TaxOffice, r.Address, r.District, r.City, r.Email)
		if c.TaxNo != "" {
			out[c.TaxNo] = c
		}
		if c.TCKN != "" {
			out[c.TCKN] = c
		}
	}
	return out, nil
}

func newCard(taxNo, tckn, office, address, district, city, email pgtype.Text) card {
	addr := []string{}
	for _, p := range []pgtype.Text{address, district, city} {
		if v := strings.TrimSpace(p.String); v != "" {
			addr = append(addr, v)
		}
	}
	return card{TaxNo: taxNo.String, TCKN: tckn.String, TaxOffice: strings.TrimSpace(office.String), Address: strings.Join(addr, " "), Email: strings.TrimSpace(email.String)}
}

func generatedView(g db.GeneratedSubaccount) Subaccount {
	v := Subaccount{Code: g.Code, ParentCode: g.ParentCode, MainCode: g.MainCode, Name: g.Name, CounterpartyID: g.CounterpartyKey, Source: "generated"}
	if g.CustomerUuid.Valid {
		id := uuid.UUID(g.CustomerUuid.Bytes)
		v.CustomerUUID = &id
	}
	if g.CreatedAt.Valid {
		t := g.CreatedAt.Time
		v.CreatedAt = &t
	}
	return v
}

func firstText(vals ...pgtype.Text) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v.String); v.Valid && s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func actor(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
