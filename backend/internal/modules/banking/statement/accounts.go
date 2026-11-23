package statement

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Cash boxes (#144): the taxpayer's bank accounts and cash registers. The
// statement file channel (#19) only imports into an account the taxpayer
// already has; before this, only a bank web service connection (#49) could
// create one, so a fresh taxpayer's MT940 / Excel / PDF was always refused
// with UNKNOWN_BANK_ACCOUNT.

// CashBoxView is the API shape of a cash box.
type CashBoxView struct {
	UUID              uuid.UUID `json:"uuid"`
	Type              string    `json:"type"`
	Name              string    `json:"name"`
	BankID            *string   `json:"bank_id"`
	IBAN              *string   `json:"iban"`
	Currency          string    `json:"currency"`
	BankAccountNumber *string   `json:"bank_account_number"`
	BankBranchCode    *string   `json:"bank_branch_code"`
	WebService        bool      `json:"web_service"`
	// Active: false once the accountant closed the account (#780). Its
	// movements stay; a statement naming its IBAN still imports.
	Active bool `json:"active"`
}

func viewOf(b db.CashBox) CashBoxView {
	return CashBoxView{
		UUID: b.Uuid, Type: b.Type, Name: b.Name, BankID: textPtr(b.BankID), IBAN: textPtr(b.Iban),
		Currency: b.Currency, BankAccountNumber: textPtr(b.BankAccountNumber), BankBranchCode: textPtr(b.BankBranchCode),
		WebService: b.BankConnectionID.Valid, Active: !b.DeactivatedAt.Valid,
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

// CashBoxInput is POST .../cash-boxes.
type CashBoxInput struct {
	Type string `json:"type"`
	Name string `json:"name"`
	// BankID: the bank picked from the list (#780); with an IBAN it must be
	// the IBAN's bank, empty takes the IBAN's.
	BankID            string `json:"bank_id"`
	IBAN              string `json:"iban"`
	Currency          string `json:"currency"`
	BankAccountNumber string `json:"bank_account_number"`
	BankBranchCode    string `json:"bank_branch_code"`
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

var cashBoxTypes = map[string]bool{"cash": true, "bank_account": true, "credit_card": true, "valor": true}

// InvalidCashBoxError carries the field the input failed on.
type InvalidCashBoxError struct{ Field, Message string }

func (e *InvalidCashBoxError) Error() string { return e.Field + ": " + e.Message }

// Params validates in and builds the insert for taxpayerID.
func (in CashBoxInput) Params(taxpayerID int64) (db.InsertCashBoxParams, error) {
	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = "bank_account"
	}
	if !cashBoxTypes[typ] {
		return db.InsertCashBoxParams{}, &InvalidCashBoxError{"type", "must be cash, bank_account, credit_card or valor"}
	}
	name := strings.TrimSpace(in.Name)
	if len(name) > 255 {
		return db.InsertCashBoxParams{}, &InvalidCashBoxError{"name", "at most 255 characters"}
	}
	currency := "TRY"
	if strings.TrimSpace(in.Currency) != "" {
		currency = banking.NormalizeCurrency(in.Currency)
		if !currencyCode.MatchString(currency) {
			return db.InsertCashBoxParams{}, &InvalidCashBoxError{"currency", "must be a 3-letter currency code"}
		}
	}
	p := db.InsertCashBoxParams{TaxpayerID: taxpayerID, Type: typ, Name: name, Currency: currency}
	if raw := strings.TrimSpace(in.IBAN); raw != "" {
		iban := banking.NormalizeIBAN(raw)
		if iban == "" {
			return db.InsertCashBoxParams{}, &InvalidCashBoxError{"iban", "not a valid IBAN"}
		}
		p.Iban = optText(iban)
		p.BankID = optText(BankFromIBAN(iban))
	}
	bank, err := checkBank(in.BankID, p.Iban.String)
	if err != nil {
		return db.InsertCashBoxParams{}, err
	}
	if bank != "" {
		p.BankID = optText(bank)
	}
	p.BankAccountNumber = optText(in.BankAccountNumber)
	p.BankBranchCode = optText(in.BankBranchCode)
	if typ == "bank_account" && !p.Iban.Valid && !p.BankAccountNumber.Valid {
		return db.InsertCashBoxParams{}, &InvalidCashBoxError{"iban", "a bank account needs an IBAN or an account number"}
	}
	if p.Name == "" {
		p.Name = defaultName(p)
	}
	return p, nil
}

func defaultName(p db.InsertCashBoxParams) string {
	switch {
	case p.Type == "cash":
		return "Kasa " + p.Currency
	case p.Iban.Valid:
		return "Banka " + p.Currency + " ···" + p.Iban.String[len(p.Iban.String)-4:]
	case p.BankAccountNumber.Valid:
		return "Banka " + p.Currency + " " + p.BankAccountNumber.String
	}
	return p.Type
}

func (h *Handler) listCashBoxes(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	rows, err := h.q.ListCashBoxes(r.Context(), scope.TaxpayerID)
	if err != nil {
		response.InternalErr(w, r, err, "could not list cash boxes")
		return
	}
	out := make([]CashBoxView, len(rows))
	for i, b := range rows {
		out[i] = viewOf(b)
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createCashBox(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	var in CashBoxInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	p, err := in.Params(scope.TaxpayerID)
	var ie *InvalidCashBoxError
	if errors.As(err, &ie) {
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "INVALID_CASH_BOX", ie.Message,
			[]response.Detail{{Field: ie.Field, Message: ie.Message}})
		return
	}
	box, err := h.q.InsertCashBox(r.Context(), p)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		response.Error(w, r, http.StatusConflict, "CASH_BOX_EXISTS", "the taxpayer already has an account with this IBAN")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not create cash box")
		return
	}
	response.JSON(w, r, http.StatusCreated, viewOf(box))
}
