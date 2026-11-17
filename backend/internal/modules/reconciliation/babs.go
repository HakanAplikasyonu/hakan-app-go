package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ErrNoBABSHeader: the file has no row with VKN and amount columns.
var ErrNoBABSHeader = errors.New("reconciliation: no BA/BS header row (VKN/TCKN and tutar) in the file")

// MaxBABSRows caps one import.
const MaxBABSRows = 5000

// Declaration is the API view of a BA / BS form line.
type Declaration struct {
	UUID          uuid.UUID `json:"uuid"`
	Period        string    `json:"period"`
	FormType      string    `json:"form_type"`
	TaxID         string    `json:"counterparty_tax_id"`
	Name          string    `json:"counterparty_name"`
	DocumentCount int       `json:"document_count"`
	Amount        string    `json:"amount"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
}

func declarationView(d db.BabsDeclaration) Declaration {
	return Declaration{UUID: d.Uuid, Period: d.Period, FormType: d.FormType, TaxID: d.CounterpartyTaxID, Name: d.CounterpartyName,
		DocumentCount: int(d.DocumentCount), Amount: dec(d.Amount).StringFixed(2), Source: d.Source, CreatedAt: d.CreatedAt.Time}
}

// DeclarationInput is one form line.
type DeclarationInput struct {
	Period        string `json:"period"`
	FormType      string `json:"form_type"`
	TaxID         string `json:"counterparty_tax_id"`
	Name          string `json:"counterparty_name"`
	DocumentCount int    `json:"document_count"`
	Amount        string `json:"amount"`
}

func validTaxID(s string) bool {
	if len(s) != 10 && len(s) != 11 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normForm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "form ")
	switch s {
	case "ba", "b a", "form-ba":
		return "ba"
	case "bs", "b s", "form-bs":
		return "bs"
	}
	return ""
}

// Declarations lists the period's form lines (form "" = both).
func (s *Service) Declarations(ctx context.Context, sc engagementctx.Scope, period, form string) ([]Declaration, error) {
	if !ValidPeriod(period) {
		return nil, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if form != "" && normForm(form) == "" {
		return nil, fmt.Errorf("%w: form must be ba or bs", ErrInvalid)
	}
	rows, err := s.q.ListBabsDeclarations(ctx, db.ListBabsDeclarationsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		Period: period, FormType: optText(normForm(form))})
	if err != nil {
		return nil, err
	}
	out := make([]Declaration, 0, len(rows))
	for _, r := range rows {
		out = append(out, declarationView(r))
	}
	return out, nil
}

func (s *Service) upsertDeclaration(ctx context.Context, q *db.Queries, sc engagementctx.Scope, userID int64, in DeclarationInput, source string) (Declaration, error) {
	in.Period = strings.TrimSpace(in.Period)
	if !ValidPeriod(in.Period) {
		return Declaration{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	form := normForm(in.FormType)
	if form == "" {
		return Declaration{}, fmt.Errorf("%w: form_type must be ba or bs", ErrInvalid)
	}
	in.TaxID = strings.TrimSpace(in.TaxID)
	if !validTaxID(in.TaxID) {
		return Declaration{}, fmt.Errorf("%w: counterparty_tax_id must be a 10 digit VKN or 11 digit TCKN", ErrInvalid)
	}
	amt, err := decimal.NewFromString(strings.TrimSpace(in.Amount))
	if err != nil || amt.IsNegative() || amt.GreaterThan(decimal.New(1, 15)) {
		return Declaration{}, fmt.Errorf("%w: amount", ErrInvalid)
	}
	if in.DocumentCount < 0 || in.DocumentCount > 1_000_000 {
		return Declaration{}, fmt.Errorf("%w: document_count", ErrInvalid)
	}
	name := strings.Join(strings.Fields(in.Name), " ")
	if len([]rune(name)) > 300 {
		name = string([]rune(name)[:300])
	}
	row, err := q.UpsertBabsDeclaration(ctx, db.UpsertBabsDeclarationParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Period: in.Period, FormType: form,
		CounterpartyTaxID: in.TaxID, CounterpartyName: name, DocumentCount: int32(in.DocumentCount), //nolint:gosec // bounded above
		Amount: num(amt.Round(2)), Source: source, CreatedBy: optID(userID),
	})
	if err != nil {
		return Declaration{}, err
	}
	return declarationView(row), nil
}

// SaveDeclaration adds or replaces one form line (same period, form and
// counterparty).
func (s *Service) SaveDeclaration(ctx context.Context, sc engagementctx.Scope, userID int64, in DeclarationInput) (Declaration, error) {
	return s.upsertDeclaration(ctx, s.q, sc, userID, in, "manual")
}

// DeleteDeclaration removes a form line.
func (s *Service) DeleteDeclaration(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) error {
	n, err := s.q.DeleteBabsDeclaration(ctx, db.DeleteBabsDeclarationParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ImportResult is the outcome of a BA / BS file import.
type ImportResult struct {
	Imported int           `json:"imported"`
	Errors   []ImportError `json:"errors"`
}

// ImportError is one refused row.
type ImportError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

var babsHeaders = map[string][]string{
	"tax_id": {"vkn", "tckn", "vkn/tckn", "vkn tckn", "vergi no", "vergi kimlik no", "vergi numarası", "vergi/tc kimlik no", "tc kimlik no"},
	"name":   {"unvan", "ünvan", "adı soyadı/unvanı", "adı soyadı unvanı", "unvanı", "ad soyad unvan", "firma", "karşı taraf"},
	"count":  {"belge sayısı", "belge adedi", "adet", "fatura sayısı"},
	"amount": {"tutar", "mal ve hizmet bedeli", "toplam tutar", "kdv hariç tutar", "tutarı", "matrah"},
	"form":   {"form", "form türü", "tür", "ba/bs"},
	"period": {"dönem", "ay"},
}

// ImportDeclarations reads a BA / BS listing (Excel / CSV: VKN/TCKN,
// unvan, belge sayısı, tutar; optional form and dönem columns) and stores
// its lines. form and period apply to rows that do not name their own.
func (s *Service) ImportDeclarations(ctx context.Context, sc engagementctx.Scope, userID int64, period, form, name string, data []byte) (ImportResult, error) {
	if period != "" && !ValidPeriod(period) {
		return ImportResult{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if form != "" && normForm(form) == "" {
		return ImportResult{}, fmt.Errorf("%w: form must be ba or bs", ErrInvalid)
	}
	_, sheets, err := parse.Read(name, data)
	if err != nil {
		return ImportResult{}, err
	}
	want := map[string]string{}
	for k, hs := range babsHeaders {
		for _, h := range hs {
			want[parse.Normalize(h)] = k
		}
	}
	for _, sh := range sheets {
		for hi, hdr := range sh.Rows {
			idx := map[string]int{}
			for j, h := range hdr {
				if k, ok := want[parse.Normalize(h)]; ok {
					if _, dup := idx[k]; !dup {
						idx[k] = j
					}
				}
			}
			if _, ok := idx["tax_id"]; !ok {
				continue
			}
			if _, ok := idx["amount"]; !ok {
				continue
			}
			return s.importRows(ctx, sc, userID, period, form, sh.Rows, hi, idx)
		}
	}
	return ImportResult{}, ErrNoBABSHeader
}

func (s *Service) importRows(ctx context.Context, sc engagementctx.Scope, userID int64, period, form string, rows [][]string, hi int, idx map[string]int) (ImportResult, error) {
	cell := func(row []string, k string) string {
		j, ok := idx[k]
		if !ok || j >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[j])
	}
	var amounts []string
	for _, r := range rows[hi+1:] {
		amounts = append(amounts, cell(r, "amount"))
	}
	style := parse.DetectNumberStyle(amounts)
	if style == parse.StyleAuto {
		// BA / BS amounts are whole liras written the Turkish way: "5.000"
		// is five thousand, not five.
		style = parse.StyleTR
	}
	res := ImportResult{Errors: []ImportError{}}
	err := database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for ri := hi + 1; ri < len(rows); ri++ {
			row := rows[ri]
			if parse.IsTotalRow(row) || strings.Join(row, "") == "" {
				continue
			}
			if ri-hi > MaxBABSRows {
				return fmt.Errorf("%w: more than %d rows", ErrInvalid, MaxBABSRows)
			}
			line := ri + 1
			vkn, tckn, ok := parse.TaxID(cell(row, "tax_id"))
			if !ok {
				res.Errors = append(res.Errors, ImportError{Line: line, Message: "VKN/TCKN okunamadı"})
				continue
			}
			taxID := vkn
			if taxID == "" {
				taxID = tckn
			}
			amt, neg, ok := parse.ParseAmount(cell(row, "amount"), style)
			if !ok || neg {
				res.Errors = append(res.Errors, ImportError{Line: line, Message: "tutar okunamadı"})
				continue
			}
			count := 0
			if c := cell(row, "count"); c != "" {
				n, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(c, ".", ""), ",", ""))
				if err != nil || n < 0 {
					res.Errors = append(res.Errors, ImportError{Line: line, Message: "belge sayısı okunamadı"})
					continue
				}
				count = n
			}
			f := form
			if c := cell(row, "form"); c != "" {
				f = c
			}
			p := period
			if c := cell(row, "period"); c != "" {
				p = normPeriod(c)
			}
			if _, err := s.upsertDeclaration(ctx, q, sc, userID, DeclarationInput{
				Period: p, FormType: f, TaxID: taxID, Name: cell(row, "name"), DocumentCount: count, Amount: amt.String(),
			}, "import"); err != nil {
				if errors.Is(err, ErrInvalid) {
					res.Errors = append(res.Errors, ImportError{Line: line, Message: strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")})
					continue
				}
				return err
			}
			res.Imported++
		}
		return nil
	})
	return res, err
}

// normPeriod reads "2026-10", "10/2026", "2026/10" or "Ekim 2026".
func normPeriod(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if ValidPeriod(s) {
		return s
	}
	for _, sep := range []string{"/", ".", "-", " "} {
		a, b, ok := strings.Cut(s, sep)
		if !ok {
			continue
		}
		if _, err := strconv.Atoi(a); err == nil && len(a) == 4 {
			a, b = b, a
		}
		if m, ok := trMonths[a]; ok {
			a = m
		}
		if len(a) == 1 {
			a = "0" + a
		}
		if p := b + "-" + a; ValidPeriod(p) {
			return p
		}
	}
	return s
}

var trMonths = map[string]string{"ocak": "01", "şubat": "02", "mart": "03", "nisan": "04", "mayıs": "05", "haziran": "06",
	"temmuz": "07", "ağustos": "08", "eylül": "09", "ekim": "10", "kasım": "11", "aralık": "12"}
