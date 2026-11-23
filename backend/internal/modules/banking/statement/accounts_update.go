package statement

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Account edit (#780): the accountant adds, edits and closes a taxpayer's
// bank accounts from the taxpayer screen. An account is never deleted (its
// movements and postings point at it); a closed account keeps importing
// statements that name its IBAN but drops out of the pickers.

// checkBank validates a picked bank id against the IBAN's bank and returns
// the bank to store ("" keeps the IBAN's or none).
func checkBank(bankID, iban string) (string, error) {
	bankID = strings.TrimSpace(bankID)
	if bankID == "" {
		return "", nil
	}
	if !KnownBank(bankID) {
		return "", &InvalidCashBoxError{"bank_id", "unknown bank"}
	}
	if iban != "" {
		if fromIBAN := BankFromIBAN(iban); fromIBAN != "" && fromIBAN != bankID {
			return "", &InvalidCashBoxError{"iban", "the IBAN belongs to another bank (" + fromIBAN + ")"}
		}
	}
	return bankID, nil
}

// CashBoxPatch is PATCH .../cash-boxes/{uuid}: only the sent fields change.
// An empty string clears an optional field (bank_branch_code,
// bank_account_number, iban when an account number remains); active false
// closes the account, true reopens it.
type CashBoxPatch struct {
	Name              *string `json:"name"`
	BankID            *string `json:"bank_id"`
	IBAN              *string `json:"iban"`
	Currency          *string `json:"currency"`
	BankAccountNumber *string `json:"bank_account_number"`
	BankBranchCode    *string `json:"bank_branch_code"`
	Active            *bool   `json:"active"`
}

// Apply merges the patch into the stored account.
func (in CashBoxPatch) Apply(b db.CashBox, now time.Time) (db.UpdateCashBoxParams, error) {
	p := db.UpdateCashBoxParams{
		TaxpayerID: b.TaxpayerID, Uuid: b.Uuid, Name: b.Name, BankID: b.BankID, Iban: b.Iban, Currency: b.Currency,
		BankAccountNumber: b.BankAccountNumber, BankBranchCode: b.BankBranchCode, DeactivatedAt: b.DeactivatedAt,
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 255 {
			return p, &InvalidCashBoxError{"name", "required, at most 255 characters"}
		}
		p.Name = name
	}
	linked := b.BankConnectionID.Valid
	if in.IBAN != nil {
		raw := strings.TrimSpace(*in.IBAN)
		iban := banking.NormalizeIBAN(raw)
		switch {
		case raw == "":
			p.Iban = pgtype.Text{}
		case iban == "":
			return p, &InvalidCashBoxError{"iban", "not a valid IBAN"}
		default:
			p.Iban = optText(iban)
		}
		// The bank web service owns its accounts' identity.
		if linked && p.Iban != b.Iban {
			return p, &InvalidCashBoxError{"iban", "the account is linked to the bank web service; its IBAN cannot change"}
		}
		if p.Iban.Valid {
			if bank := BankFromIBAN(p.Iban.String); bank != "" {
				p.BankID = optText(bank)
			}
		}
	}
	if in.BankID != nil {
		bank, err := checkBank(*in.BankID, p.Iban.String)
		if err != nil {
			return p, err
		}
		switch {
		case bank != "":
			p.BankID = optText(bank)
		case !p.Iban.Valid:
			p.BankID = pgtype.Text{}
		}
	}
	if in.Currency != nil {
		currency := banking.NormalizeCurrency(*in.Currency)
		if !currencyCode.MatchString(currency) {
			return p, &InvalidCashBoxError{"currency", "must be a 3-letter currency code"}
		}
		if linked && currency != b.Currency {
			return p, &InvalidCashBoxError{"currency", "the account is linked to the bank web service; its currency cannot change"}
		}
		p.Currency = currency
	}
	if in.BankAccountNumber != nil {
		p.BankAccountNumber = optText(*in.BankAccountNumber)
	}
	if in.BankBranchCode != nil {
		if len(strings.TrimSpace(*in.BankBranchCode)) > 32 {
			return p, &InvalidCashBoxError{"bank_branch_code", "at most 32 characters"}
		}
		p.BankBranchCode = optText(*in.BankBranchCode)
	}
	if b.Type == "bank_account" && !p.Iban.Valid && !p.BankAccountNumber.Valid && !p.BankID.Valid {
		return p, &InvalidCashBoxError{"iban", "a bank account needs a bank, an IBAN or an account number"}
	}
	if in.Active != nil {
		switch {
		case *in.Active:
			p.DeactivatedAt = pgtype.Timestamptz{}
		case !p.DeactivatedAt.Valid:
			p.DeactivatedAt = pgtype.Timestamptz{Time: now, Valid: true}
		}
	}
	return p, nil
}

// updateCashBox edits or closes an account (#144 rename, #780 full edit).
func (h *Handler) updateCashBox(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("cashBoxUuid"))
	if err != nil {
		response.NotFound(w, r, "cash box not found")
		return
	}
	var in CashBoxPatch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid request body")
		return
	}
	box, err := h.q.GetCashBoxByUUID(r.Context(), db.GetCashBoxByUUIDParams{TaxpayerID: scope.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "cash box not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not update cash box")
		return
	}
	p, err := in.Apply(box, time.Now())
	var ie *InvalidCashBoxError
	if errors.As(err, &ie) {
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "INVALID_CASH_BOX", ie.Message,
			[]response.Detail{{Field: ie.Field, Message: ie.Message}})
		return
	}
	box, err = h.q.UpdateCashBox(r.Context(), p)
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &pe) && pe.Code == "23505":
		response.Error(w, r, http.StatusConflict, "CASH_BOX_EXISTS", "the taxpayer already has an account with this IBAN")
		return
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "cash box not found")
		return
	case err != nil:
		response.InternalErr(w, r, err, "could not update cash box")
		return
	}
	response.JSON(w, r, http.StatusOK, viewOf(box))
}
