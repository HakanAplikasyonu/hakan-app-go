package statement

// Luca "Banka Ekstreleri" CSV of one bank account and date range (issue
// #465). The layout is the unverified YAML template
// accountingexport/luca/templates/banka_ekstresi_*.yaml; delimiter, quote
// and amount layout can be chosen per file. Every file is recorded with the
// movements it holds, so a movement exported a second time is marked on
// the bank screen and in the file's history.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/luca"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// MaxExportDays bounds one bank statement file.
const MaxExportDays = 366

// LucaExportInput asks for a bank statement file.
type LucaExportInput struct {
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	DateFrom    string    `json:"date_from"`
	DateTo      string    `json:"date_to"`
	Template    string    `json:"template,omitempty"`
	luca.BankOptions
}

// LucaExportView is one file of the history.
type LucaExportView struct {
	UUID             uuid.UUID `json:"uuid"`
	CashBoxUUID      uuid.UUID `json:"cash_box_uuid"`
	CashBoxName      string    `json:"cash_box_name"`
	DateFrom         string    `json:"date_from"`
	DateTo           string    `json:"date_to"`
	TemplateVersion  string    `json:"template_version"`
	TemplateVerified bool      `json:"template_verified"`
	AmountLayout     string    `json:"amount_layout"`
	Delimiter        string    `json:"delimiter"`
	Quote            string    `json:"quote"`
	FileName         string    `json:"file_name"`
	SHA256           string    `json:"sha256"`
	Size             int32     `json:"size"`
	Lines            int32     `json:"line_count"`
	Repeated         int32     `json:"repeated_count"`
	CreatedAt        time.Time `json:"created_at"`
}

// GuideColumn is one row of the step-by-step mapping guide for Luca's
// ekstre tanımı.
type GuideColumn struct {
	Header       string `json:"header"`
	ProgramField string `json:"program_field"`
	DataType     string `json:"data_type"`
	Format       string `json:"format,omitempty"`
	Required     bool   `json:"required"`
}

// LucaGuide is the template the screen explains: columns per amount layout
// and the file container settings.
type LucaGuide struct {
	TemplateVersion  string                   `json:"template_version"`
	TemplateVerified bool                     `json:"template_verified"`
	Delimiter        string                   `json:"delimiter"`
	Quote            string                   `json:"quote"`
	Encoding         string                   `json:"encoding"`
	HeaderRow        bool                     `json:"header_row"`
	AmountLayout     string                   `json:"amount_layout"`
	IncomingSign     string                   `json:"incoming_sign"`
	Columns          map[string][]GuideColumn `json:"columns"`
	Notes            string                   `json:"notes"`
}

func (h *Handler) lucaGuide(w http.ResponseWriter, r *http.Request) {
	tpl, err := luca.BankStatementTemplate(r.URL.Query().Get("template"))
	if err != nil {
		response.BadRequest(w, r, "UNKNOWN_TEMPLATE", err.Error())
		return
	}
	g := LucaGuide{
		TemplateVersion: tpl.Version, TemplateVerified: tpl.Verified, Encoding: tpl.CSV.Encoding,
		Delimiter: tpl.CSV.Delimiter, Quote: tpl.CSV.Quote, HeaderRow: tpl.CSV.Header == nil || *tpl.CSV.Header,
		AmountLayout: tpl.Options["amount_layout"], IncomingSign: tpl.Options["incoming_sign"],
		Columns: map[string][]GuideColumn{}, Notes: tpl.Notes,
	}
	for _, layout := range []string{luca.AmountSigned, luca.AmountTwoColumns} {
		t, err := luca.ApplyBankOptions(tpl, luca.BankOptions{AmountLayout: layout})
		if err != nil {
			response.InternalErr(w, r, err, "bank statement template")
			return
		}
		for _, c := range t.Columns {
			g.Columns[layout] = append(g.Columns[layout], GuideColumn{Header: c.Header, ProgramField: c.ProgramField, DataType: c.DataType, Format: c.Format, Required: c.Required})
		}
	}
	response.JSON(w, r, http.StatusOK, g)
}

// Errors of BuildLucaStatement.
var (
	ErrExportRange    = errors.New("statement: date_from and date_to must be YYYY-MM-DD, in order, at most 366 days apart")
	ErrExportAccount  = errors.New("statement: bank account not found")
	ErrNoTransactions = errors.New("statement: the account has no movements in the date range")
)

// LucaStatement is a produced file with its history record.
type LucaStatement struct {
	Record   db.BankStatementExport
	Data     []byte
	Encoding string
	Repeated int
	Verified bool
}

// BuildLucaStatement writes the Luca bank statement CSV of one account and
// date range and records it with its movements. q runs in the caller's
// row level security scope.
func BuildLucaStatement(ctx context.Context, q *db.Queries, scope engagementctx.Scope, actorID int64, in LucaExportInput) (LucaStatement, error) {
	from, errFrom := time.Parse(time.DateOnly, in.DateFrom)
	to, errTo := time.Parse(time.DateOnly, in.DateTo)
	if errFrom != nil || errTo != nil || to.Before(from) || to.Sub(from) > MaxExportDays*24*time.Hour {
		return LucaStatement{}, ErrExportRange
	}
	box, err := q.GetCashBoxForExport(ctx, db.GetCashBoxForExportParams{TaxpayerID: scope.TaxpayerID, Uuid: in.CashBoxUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return LucaStatement{}, ErrExportAccount
	}
	if err != nil {
		return LucaStatement{}, err
	}
	base, err := luca.BankStatementTemplate(in.Template)
	if err != nil {
		return LucaStatement{}, err
	}
	tpl, err := luca.ApplyBankOptions(base, in.BankOptions)
	if err != nil {
		return LucaStatement{}, err
	}
	rows, err := q.ListBankTransactionsForExport(ctx, db.ListBankTransactionsForExportParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, CashBoxID: box.ID,
		DateFrom: pgtype.Date{Time: from, Valid: true}, DateTo: pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return LucaStatement{}, err
	}
	if len(rows) == 0 {
		return LucaStatement{}, ErrNoTransactions
	}
	lines := make([]luca.BankLine, len(rows))
	type item struct {
		ID       int64 `json:"id"`
		Previous int32 `json:"previous"`
	}
	items := make([]item, len(rows))
	repeated := 0
	for i, t := range rows {
		lines[i] = bankLine(t)
		items[i] = item{ID: t.ID, Previous: t.PreviousExports}
		if t.PreviousExports > 0 {
			repeated++
		}
	}
	res, err := luca.WriteBankStatement(tpl, lines)
	if err != nil {
		return LucaStatement{}, err
	}
	sum := sha256.Sum256(res.Data)
	name := fmt.Sprintf("luca_banka_%s_%s_%s.csv", fileSafe(box.Name, box.Uuid), from.Format("20060102"), to.Format("20060102"))
	var actor pgtype.Int8
	if actorID > 0 {
		actor = pgtype.Int8{Int64: actorID, Valid: true}
	}
	rec, err := q.InsertBankStatementExport(ctx, db.InsertBankStatementExportParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, CashBoxID: box.ID, Target: "luca",
		DateFrom: pgtype.Date{Time: from, Valid: true}, DateTo: pgtype.Date{Time: to, Valid: true},
		TemplateVersion: tpl.Version, TemplateVerified: tpl.Verified, AmountLayout: tpl.Options["amount_layout"],
		Delimiter: tpl.CSV.Delimiter, Quote: tpl.CSV.Quote, FileName: name, Sha256: hex.EncodeToString(sum[:]),
		Size: int32(len(res.Data)), LineCount: int32(len(rows)), RepeatedCount: int32(repeated), CreatedBy: actor, //nolint:gosec // bounded by the row limit
	})
	if err != nil {
		return LucaStatement{}, err
	}
	payload, _ := json.Marshal(items)
	if _, err := q.InsertBankStatementExportItems(ctx, db.InsertBankStatementExportItemsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, BankStatementExportID: rec.ID, TaxpayerID: scope.TaxpayerID, Items: payload,
	}); err != nil {
		return LucaStatement{}, err
	}
	return LucaStatement{Record: rec, Data: res.Data, Encoding: tpl.CSV.Encoding, Repeated: repeated, Verified: tpl.Verified}, nil
}

func (h *Handler) lucaExport(w http.ResponseWriter, r *http.Request) {
	var in LucaExportInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	var actor int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		actor = p.UserInternal
	}
	f, err := BuildLucaStatement(r.Context(), h.q, engagementctx.Must(r.Context()), actor, in)
	var fe *ae.FieldError
	switch {
	case err == nil:
	case errors.Is(err, ErrExportRange):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	case errors.Is(err, ErrExportAccount):
		response.NotFound(w, r, "bank account not found")
		return
	case errors.Is(err, ErrNoTransactions):
		response.Error(w, r, http.StatusUnprocessableEntity, "NO_TRANSACTIONS", "the account has no movements in the date range")
		return
	case errors.As(err, &fe):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "REQUIRED_FIELD_MISSING", "a required column is empty",
			[]response.Detail{{Field: fmt.Sprintf("row:%d/%s", fe.Row, fe.Column), Code: "required", Message: fe.Error()}})
		return
	case errors.Is(err, ae.ErrTemplate):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	default:
		response.InternalErr(w, r, err, "could not write the bank statement file")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset="+f.Encoding)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Record.FileName))
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Header().Set("X-Export-UUID", f.Record.Uuid.String())
	w.Header().Set("X-Repeated-Count", strconv.Itoa(f.Repeated))
	w.Header().Set("X-Template-Verified", strconv.FormatBool(f.Verified))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}

func (h *Handler) lucaExports(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	limit, offset := int32(50), int32(0)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	rows, err := h.q.ListBankStatementExports(r.Context(), db.ListBankStatementExportsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: limit, Off: offset})
	if err != nil {
		response.InternalErr(w, r, err, "could not list bank statement exports")
		return
	}
	out := make([]LucaExportView, len(rows))
	for i, e := range rows {
		out[i] = LucaExportView{
			UUID: e.Uuid, CashBoxUUID: e.CashBoxUuid, CashBoxName: e.CashBoxName,
			DateFrom: e.DateFrom.Time.Format(time.DateOnly), DateTo: e.DateTo.Time.Format(time.DateOnly),
			TemplateVersion: e.TemplateVersion, TemplateVerified: e.TemplateVerified, AmountLayout: e.AmountLayout,
			Delimiter: e.Delimiter, Quote: e.Quote, FileName: e.FileName, SHA256: e.Sha256, Size: e.Size,
			Lines: e.LineCount, Repeated: e.RepeatedCount, CreatedAt: e.CreatedAt.Time,
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out, "limit": limit, "offset": offset})
}

// bankLine maps a movement to a file row: Evrak No is the bank's receipt
// number or reference; Açıklama the description (the counterparty's name
// when it is empty); VKN / TCKN and IBAN the counterparty's.
func bankLine(t db.ListBankTransactionsForExportRow) luca.BankLine {
	desc := strings.TrimSpace(t.Description)
	if extra := strings.TrimSpace(t.DescriptionExtra.String); extra != "" && !strings.Contains(desc, extra) {
		desc = strings.TrimSpace(desc + " " + extra)
	}
	if desc == "" {
		desc = strings.TrimSpace(t.CounterpartyName.String)
	}
	taxID := t.CounterpartyTaxNumber.String
	if taxID == "" {
		taxID = t.CounterpartyIdentityNumber.String
	}
	docNo := strings.TrimSpace(t.ReceiptNumber.String)
	if docNo == "" {
		docNo = strings.TrimSpace(t.BankReference.String)
	}
	amt := decimalOf(t.Amount)
	return luca.BankLine{
		Date: t.TransactionDate.Time, DocNo: docNo, Description: desc, Incoming: t.DebitCredit == "A",
		Amount: amt, TaxID: taxID, IBAN: t.CounterpartyIban.String, Currency: t.Currency,
	}
}

// fileSafe is a short ASCII name for the file (the account name, else its
// uuid prefix).
func fileSafe(name string, id uuid.UUID) string {
	tr := strings.NewReplacer("ı", "i", "İ", "I", "ş", "s", "Ş", "S", "ğ", "g", "Ğ", "G", "ü", "u", "Ü", "U", "ö", "o", "Ö", "O", "ç", "c", "Ç", "C")
	var b strings.Builder
	for _, r := range tr.Replace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteByte('_')
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return id.String()[:8]
	}
	return s
}
