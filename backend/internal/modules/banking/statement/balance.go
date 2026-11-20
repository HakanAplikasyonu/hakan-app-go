package statement

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// BalanceCheck is one account's statement balance check for a period
// (#570): opening + in − out must equal the closing balance the bank
// reported. A non-zero difference means missing or duplicate movements.
type BalanceCheck struct {
	CashBoxUUID     uuid.UUID `json:"cash_box_uuid"`
	CashBoxName     string    `json:"cash_box_name"`
	IBAN            *string   `json:"iban"`
	Currency        string    `json:"currency"`
	Movements       int64     `json:"movements"`
	FirstDate       string    `json:"first_date"`
	LastDate        string    `json:"last_date"`
	InTotal         string    `json:"in_total"`
	OutTotal        string    `json:"out_total"`
	Opening         *string   `json:"opening"`
	ExpectedClosing *string   `json:"expected_closing"`
	Closing         *string   `json:"closing"`
	Difference      *string   `json:"difference"`
	// Balanced: the statement carries balances and they agree.
	Balanced bool `json:"balanced"`
	// WithoutBalance: movements without a bank balance; the check is
	// partial (Excel or PDF statements often lack the balance column).
	WithoutBalance int64 `json:"without_balance"`
}

// balanceCheck derives the expected closing balance and the difference.
func balanceCheck(r db.BankBalanceCheckRow) BalanceCheck {
	in, out := decimalOf(r.InTotal), decimalOf(r.OutTotal)
	c := BalanceCheck{
		CashBoxUUID: r.CashBoxUuid, CashBoxName: r.CashBoxName, IBAN: textPtr(r.Iban), Currency: r.Currency,
		Movements: r.Movements, WithoutBalance: r.WithoutBalance,
		InTotal: in.StringFixed(2), OutTotal: out.StringFixed(2),
	}
	if r.FirstDate.Valid {
		c.FirstDate = r.FirstDate.Time.Format(time.DateOnly)
	}
	if r.LastDate.Valid {
		c.LastDate = r.LastDate.Time.Format(time.DateOnly)
	}
	str := func(d decimal.Decimal) *string { s := d.StringFixed(2); return &s }
	if r.Opening.Valid {
		opening := decimalOf(r.Opening)
		expected := opening.Add(in).Sub(out)
		c.Opening, c.ExpectedClosing = str(opening), str(expected)
		if r.Closing.Valid {
			closing := decimalOf(r.Closing)
			diff := closing.Sub(expected)
			c.Closing, c.Difference = str(closing), str(diff)
			c.Balanced = diff.Round(2).IsZero()
		}
	} else if r.Closing.Valid {
		c.Closing = str(decimalOf(r.Closing))
	}
	return c
}

// balanceChecks: GET .../bank-balance-check?period=YYYY-MM.
func (h *Handler) balanceChecks(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	period := r.URL.Query().Get("period")
	if !periodRe.MatchString(period) {
		response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
		return
	}
	rows, err := h.q.BankBalanceCheck(r.Context(), db.BankBalanceCheckParams{TaxpayerID: scope.TaxpayerID, Period: period})
	if err != nil {
		response.InternalErr(w, r, err, "could not check bank balances")
		return
	}
	items := make([]BalanceCheck, len(rows))
	for i, row := range rows {
		items[i] = balanceCheck(row)
	}
	rows2, err := h.q.ListBankBalanceSuspects(r.Context(), db.ListBankBalanceSuspectsParams{TaxpayerID: scope.TaxpayerID, Period: period})
	if err != nil {
		response.InternalErr(w, r, err, "could not check bank balances")
		return
	}
	suspects := make([]BalanceSuspect, len(rows2))
	for i, row := range rows2 {
		suspects[i] = BalanceSuspect{
			UUID: row.Uuid, CashBoxUUID: row.CashBoxUuid, Date: row.TransactionDate.Time.Format(time.DateOnly),
			DebitCredit: row.DebitCredit, Amount: decimalOf(row.Amount).StringFixed(2), Currency: row.Currency,
			Description: row.Description, Flagged: row.Flagged, Excluded: row.ExcludedAt.Valid,
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"period": period, "items": items, "suspects": suspects})
}

// BalanceSuspect is a movement to look at when the balance does not agree
// (#710): one the import flagged as a possible duplicate, one with a twin
// (same account, day, direction and amount), or one the office took out.
type BalanceSuspect struct {
	UUID        uuid.UUID `json:"uuid"`
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	Date        string    `json:"transaction_date"`
	DebitCredit string    `json:"debit_credit"`
	Amount      string    `json:"amount"`
	Currency    string    `json:"currency"`
	Description string    `json:"description"`
	// Flagged: the import marked it as a possible duplicate.
	Flagged bool `json:"possible_duplicate"`
	// Excluded: the office took it out of the account; it can be put back.
	Excluded bool `json:"excluded"`
}
