package taxpayers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Bulk taxpayer import (issue #273, setup wizard): an office uploads its
// taxpayer list as Excel or CSV. The file is read with the excelimport
// reader, every row is validated and the row-by-row report is stored as a
// batch. Sending the batch opens one add-mükellef request per valid row
// (consent email, #7): the same flow, the same answer and the same padding
// as a single request, so the import reveals nothing about which tax ids
// are on the platform (D1b).

// MaxImportRows bounds one file.
const MaxImportRows = 1000

// importConcurrency is how many consent requests are opened at once; each
// is padded to MinRequestDuration.
const importConcurrency = 8

var (
	// ErrImportNoHeader: no row names both a tax id and a title column.
	ErrImportNoHeader = errors.New("taxpayers: no header row with a tax id and a title column")
	// ErrImportTooLarge: more than MaxImportRows data rows.
	ErrImportTooLarge = fmt.Errorf("taxpayers: more than %d rows", MaxImportRows)
	// ErrImportSent: the batch was already sent.
	ErrImportSent = errors.New("taxpayers: import already sent")
)

// Row error codes.
const (
	RowErrRequired  = "required"
	RowErrInvalid   = "invalid"
	RowErrDuplicate = "duplicate"
)

// RowError is one problem of one row.
type RowError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ImportRow is one data row of the uploaded list.
type ImportRow struct {
	// Row is the 1-based row number in the sheet.
	Row          int        `json:"row"`
	Kind         string     `json:"kind"`
	TaxID        string     `json:"tax_id"`
	Title        string     `json:"title"`
	TaxOffice    string     `json:"tax_office"`
	ContactEmail string     `json:"contact_email"`
	Errors       []RowError `json:"errors"`
}

// Valid reports a row without errors.
func (r ImportRow) Valid() bool { return len(r.Errors) == 0 }

// ImportOutcome is the result of sending one row.
type ImportOutcome struct {
	Row         int        `json:"row"`
	Status      string     `json:"status"` // requested | skipped | failed
	RequestUUID *uuid.UUID `json:"request_uuid,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ImportBatch is a stored import.
type ImportBatch struct {
	UUID      uuid.UUID       `json:"uuid"`
	FileName  string          `json:"file_name"`
	Total     int             `json:"total"`
	Valid     int             `json:"valid"`
	Invalid   int             `json:"invalid"`
	CreatedAt time.Time       `json:"created_at"`
	SentAt    *time.Time      `json:"sent_at"`
	Rows      []ImportRow     `json:"rows,omitempty"`
	Outcome   []ImportOutcome `json:"outcome,omitempty"`
}

// Column aliases, after parse.Normalize (lower case, Turkish letters folded,
// punctuation dropped).
var importAliases = map[string][]string{
	"tax_id": {"vkn", "tckn", "vkn tckn", "tckn vkn", "vergi no", "vergi numarasi", "vergi kimlik no",
		"vergi kimlik numarasi", "tc kimlik no", "tc no", "kimlik no", "vkn tc"},
	"title": {"unvan", "unvani", "ad soyad", "adi soyadi", "ad soyad unvan", "unvan ad soyad", "firma",
		"firma adi", "mukellef", "mukellef adi", "musteri", "musteri adi", "sirket", "sirket adi"},
	"tax_office":    {"vergi dairesi", "vd", "vergi dairesi adi"},
	"contact_email": {"e posta", "eposta", "email", "e mail", "mail", "iletisim e posta", "e posta adresi"},
	"kind":          {"tur", "tip", "mukellef turu", "mukellef tipi"},
}

// ParseTaxpayerList reads an Excel / CSV taxpayer list into validated rows.
// The header row is the first of the first 20 rows that names a tax id and
// a title column.
func ParseTaxpayerList(name string, data []byte) ([]ImportRow, error) {
	_, sheets, err := parse.Read(name, data)
	if err != nil {
		return nil, err
	}
	for _, s := range sheets {
		header, cols := findHeader(s.Rows)
		if header < 0 {
			continue
		}
		var rows []ImportRow
		seen := map[string]int{}
		for i := header + 1; i < len(s.Rows); i++ {
			cells := s.Rows[i]
			if blank(cells) || parse.IsTotalRow(cells) {
				continue
			}
			if len(rows) == MaxImportRows {
				return nil, ErrImportTooLarge
			}
			r := buildRow(i+1, cells, cols)
			if r.TaxID != "" && !hasField(r.Errors, "tax_id") {
				if first, dup := seen[r.TaxID]; dup {
					r.Errors = append(r.Errors, RowError{Field: "tax_id", Code: RowErrDuplicate,
						Message: fmt.Sprintf("aynı VKN/TCKN %d. satırda da var", first)})
				} else {
					seen[r.TaxID] = r.Row
				}
			}
			rows = append(rows, r)
		}
		return rows, nil
	}
	return nil, ErrImportNoHeader
}

func findHeader(rows [][]string) (int, map[string]int) {
	limit := min(len(rows), 20)
	for i := 0; i < limit; i++ {
		cols := map[string]int{}
		for j, cell := range rows[i] {
			n := parse.Normalize(cell)
			for field, aliases := range importAliases {
				if _, taken := cols[field]; taken {
					continue
				}
				for _, a := range aliases {
					if n == a {
						cols[field] = j
						break
					}
				}
			}
		}
		_, hasID := cols["tax_id"]
		_, hasTitle := cols["title"]
		if hasID && hasTitle {
			return i, cols
		}
	}
	return -1, nil
}

func buildRow(rowNo int, cells []string, cols map[string]int) ImportRow {
	get := func(field string) string {
		j, ok := cols[field]
		if !ok || j >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[j])
	}
	r := ImportRow{Row: rowNo, Errors: []RowError{}}
	rawID := strings.Map(func(c rune) rune {
		if c >= '0' && c <= '9' {
			return c
		}
		return -1
	}, get("tax_id"))
	r.TaxID = rawID
	r.Title = get("title")
	r.TaxOffice = get("tax_office")
	r.ContactEmail = strings.ToLower(get("contact_email"))
	r.Kind = kindOf(get("kind"), rawID)

	if rawID == "" {
		r.Errors = append(r.Errors, RowError{Field: "tax_id", Code: RowErrRequired, Message: "VKN/TCKN boş"})
	}
	if r.Title == "" {
		r.Errors = append(r.Errors, RowError{Field: "title", Code: RowErrRequired, Message: "unvan boş"})
	}
	if rawID != "" && r.Title != "" {
		v, _, err := Validate(CreateInput{Kind: r.Kind, TaxID: rawID, Title: r.Title, TaxOffice: r.TaxOffice})
		if err != nil {
			r.Errors = append(r.Errors, RowError{Field: "tax_id", Code: RowErrInvalid, Message: validationMessage(err)})
		} else {
			r.TaxID, r.Title, r.TaxOffice = v.TaxID, v.Title, v.TaxOffice
		}
	}
	if r.ContactEmail == "" {
		r.Errors = append(r.Errors, RowError{Field: "contact_email", Code: RowErrRequired,
			Message: "onay e-postası için iletişim e-postası gerekli"})
	} else if e, err := normalizeEmail(r.ContactEmail); err != nil {
		r.Errors = append(r.Errors, RowError{Field: "contact_email", Code: RowErrInvalid, Message: "e-posta geçersiz"})
	} else {
		r.ContactEmail = e
	}
	return r
}

// kindOf reads the kind column, else infers it: 11 digits are a TCKN.
func kindOf(raw, taxID string) string {
	switch parse.Normalize(raw) {
	case "gercek", "gercek kisi", "sahis", "individual":
		return KindIndividual
	case "tuzel", "tuzel kisi", "sirket", "legal":
		return KindLegal
	}
	if len(taxID) == 11 {
		return KindIndividual
	}
	return KindLegal
}

// validationMessage is the Turkish text an office sees for a rejected row:
// Validate's known causes are translated; anything else loses its Go error
// prefixes ("taxpayers: invalid request: …"), never the raw chain (#397).
func validationMessage(err error) string {
	switch {
	case errors.Is(err, taxid.ErrChecksum):
		return "VKN/TCKN geçersiz: sağlama hanesi tutmuyor"
	case errors.Is(err, taxid.ErrFormat):
		return "VKN 10, TCKN 11 haneli olmalı"
	case errors.Is(err, errLegalNeedsVKN):
		return "tüzel kişi için 10 haneli VKN gerekli"
	case errors.Is(err, errTitle):
		return fmt.Sprintf("unvan boş ya da %d karakterden uzun", maxTitleLen)
	case errors.Is(err, errTaxOfficeLen):
		return fmt.Sprintf("vergi dairesi en çok %d karakter olabilir", maxTaxOfficeLen)
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}

func hasField(errs []RowError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func blank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// Importer stores and sends taxpayer import batches.
type Importer struct {
	pool database.Pool
	flow *Onboarding
}

// NewImporter builds the importer on the add-mükellef flow.
func NewImporter(pool database.Pool, flow *Onboarding) *Importer {
	return &Importer{pool: pool, flow: flow}
}

// Upload parses the file and stores the validated rows as a batch.
func (im *Importer) Upload(ctx context.Context, orgID, userID int64, fileName string, data []byte) (ImportBatch, error) {
	rows, err := ParseTaxpayerList(fileName, data)
	if err != nil {
		return ImportBatch{}, err
	}
	valid := 0
	for _, r := range rows {
		if r.Valid() {
			valid++
		}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return ImportBatch{}, err
	}
	b, err := database.QueriesFor(im.pool).CreateTaxpayerImportBatch(ctx, db.CreateTaxpayerImportBatchParams{
		OrganizationID: orgID, FileName: cleanFileName(fileName), Rows: raw,
		Total: int32(len(rows)), Valid: int32(valid), Invalid: int32(len(rows) - valid), //nolint:gosec // bounded by MaxImportRows
		CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	})
	if err != nil {
		return ImportBatch{}, err
	}
	return batchView(b)
}

// Get returns one batch with its rows and outcome.
func (im *Importer) Get(ctx context.Context, orgID int64, id uuid.UUID) (ImportBatch, error) {
	b, err := database.QueriesFor(im.pool).GetTaxpayerImportBatch(ctx, db.GetTaxpayerImportBatchParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportBatch{}, ErrNotFound
	}
	if err != nil {
		return ImportBatch{}, err
	}
	return batchView(b)
}

// List returns the office's recent batches (without rows).
func (im *Importer) List(ctx context.Context, orgID int64) ([]ImportBatch, error) {
	rows, err := database.QueriesFor(im.pool).ListTaxpayerImportBatches(ctx, db.ListTaxpayerImportBatchesParams{OrganizationID: orgID, LimitCount: 20})
	if err != nil {
		return nil, err
	}
	out := make([]ImportBatch, 0, len(rows))
	for _, r := range rows {
		v := ImportBatch{UUID: r.Uuid, FileName: r.FileName, Total: int(r.Total), Valid: int(r.Valid), Invalid: int(r.Invalid), CreatedAt: r.CreatedAt.Time}
		if r.SentAt.Valid {
			t := r.SentAt.Time
			v.SentAt = &t
		}
		out = append(out, v)
	}
	return out, nil
}

// Send opens an add-mükellef request for every valid row of the batch, once.
// Rows are sent concurrently; each request is the normal single request
// (padding, consent email). Failures are reported per row.
func (im *Importer) Send(ctx context.Context, orgID int64, orgName string, userID int64, id uuid.UUID) (ImportBatch, error) {
	var b db.TaxpayerImportBatch
	err := database.BeginFunc(ctx, im.pool, func(tx pgx.Tx) error {
		var err error
		b, err = db.New(tx).LockTaxpayerImportBatch(ctx, db.LockTaxpayerImportBatchParams{OrganizationID: orgID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if b.SentAt.Valid {
			return ErrImportSent
		}
		// Claim the batch before the slow part, so a second click waits and
		// then sees it sent.
		_, err = db.New(tx).MarkTaxpayerImportBatchSent(ctx, db.MarkTaxpayerImportBatchSentParams{
			OrganizationID: orgID, ID: b.ID, SentBy: pgtype.Int8{Int64: userID, Valid: userID > 0}, Outcome: []byte("[]"),
		})
		return err
	})
	if err != nil {
		return ImportBatch{}, err
	}
	var rows []ImportRow
	if err := json.Unmarshal(b.Rows, &rows); err != nil {
		return ImportBatch{}, err
	}
	out := make([]ImportOutcome, len(rows))
	// A transaction is not safe for concurrent use: every row gets its own,
	// under the request's row level security scope when there is one.
	scoped := rls.InScope(ctx)
	scope := rls.FromRequest(ctx)
	rowCtx := dbtx.WithTx(ctx, nil)
	request := func(in RequestInput) (OnboardingRequest, error) {
		if !scoped {
			return im.flow.Request(rowCtx, in)
		}
		var req OnboardingRequest
		err := rls.WithScope(rowCtx, im.pool, scope, func(ctx context.Context, _ pgx.Tx) error {
			var err error
			req, err = im.flow.Request(ctx, in)
			return err
		})
		return req, err
	}
	sem := make(chan struct{}, importConcurrency)
	var wg sync.WaitGroup
	for i, r := range rows {
		if !r.Valid() {
			out[i] = ImportOutcome{Row: r.Row, Status: "skipped"}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r ImportRow) {
			defer wg.Done()
			defer func() { <-sem }()
			req, err := request(RequestInput{
				OrgID: orgID, OrgName: orgName, UserID: userID,
				Kind: r.Kind, TaxID: r.TaxID, Title: r.Title, ContactEmail: r.ContactEmail,
			})
			if err != nil {
				out[i] = ImportOutcome{Row: r.Row, Status: "failed", Error: validationMessage(err)}
				return
			}
			u := req.UUID
			out[i] = ImportOutcome{Row: r.Row, Status: "requested", RequestUUID: &u}
		}(i, r)
	}
	wg.Wait()
	raw, err := json.Marshal(out)
	if err != nil {
		return ImportBatch{}, err
	}
	if _, err := database.QueriesFor(im.pool).MarkTaxpayerImportBatchSentOutcome(ctx, db.MarkTaxpayerImportBatchSentOutcomeParams{
		OrganizationID: orgID, ID: b.ID, Outcome: raw,
	}); err != nil {
		return ImportBatch{}, err
	}
	return im.Get(ctx, orgID, id)
}

func batchView(b db.TaxpayerImportBatch) (ImportBatch, error) {
	v := ImportBatch{UUID: b.Uuid, FileName: b.FileName, Total: int(b.Total), Valid: int(b.Valid), Invalid: int(b.Invalid), CreatedAt: b.CreatedAt.Time}
	if b.SentAt.Valid {
		t := b.SentAt.Time
		v.SentAt = &t
	}
	if err := json.Unmarshal(b.Rows, &v.Rows); err != nil {
		return ImportBatch{}, err
	}
	if len(b.Outcome) > 0 {
		if err := json.Unmarshal(b.Outcome, &v.Outcome); err != nil {
			return ImportBatch{}, err
		}
	}
	return v, nil
}

func cleanFileName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		name = "mukellefler"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
