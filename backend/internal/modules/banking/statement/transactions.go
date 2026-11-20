package statement

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Banka hareketleri listesi (arayüz 6/9, #329): ofis panelindeki "Banka"
// sekmesi. Yalnız okuma; eşleştirme ve içe aktarma kendi uçlarında.

const (
	txDefaultLimit = 50
	txMaxLimit     = 200
)

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

var matchStatuses = map[string]bool{"unmatched": true, "suggested": true, "matched": true, "ignored": true}

// TransactionView is one bank transaction of the list.
type TransactionView struct {
	UUID              uuid.UUID `json:"uuid"`
	Date              string    `json:"transaction_date"`
	Direction         string    `json:"direction"`
	Amount            string    `json:"amount"`
	Currency          string    `json:"currency"`
	Description       string    `json:"description"`
	CounterpartyName  *string   `json:"counterparty_name"`
	Kind              string    `json:"kind"`
	KindDetail        string    `json:"kind_detail"`
	MatchStatus       string    `json:"match_status"`
	Period            string    `json:"period"`
	PossibleDuplicate bool      `json:"possible_duplicate"`
	CashBoxUUID       uuid.UUID `json:"cash_box_uuid"`
	CashBoxName       string    `json:"cash_box_name"`
	// LucaExports: how many Luca bank statement files held the movement
	// (#465); more than one means it was exported again.
	LucaExports      int32      `json:"luca_exports"`
	LucaLastExportAt *time.Time `json:"luca_last_exported_at,omitempty"`
	// BookedAccountCode: the trial-balance account the movement is booked
	// to (an approved posting, #565); empty when it has none yet.
	BookedAccountCode string `json:"booked_account_code,omitempty"`
	BookedAccountName string `json:"booked_account_name,omitempty"`
}

// TransactionSummary counts the filtered set (match status not applied).
type TransactionSummary struct {
	Total     int64  `json:"total"`
	InTotal   string `json:"in_total"`
	OutTotal  string `json:"out_total"`
	Unmatched int64  `json:"unmatched"`
	Suggested int64  `json:"suggested"`
	Matched   int64  `json:"matched"`
}

func decimalString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil || n.NaN {
		return "0.00"
	}
	return decimal.NewFromBigInt(n.Int, n.Exp).StringFixed(2)
}

func decimalOf(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil || n.NaN {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

// direction: A (alacak) para girişi, B (borç) para çıkışı.
func direction(dc string) string {
	if dc == "A" {
		return "in"
	}
	return "out"
}

// txFilter is the list / export filter.
type txFilter struct {
	period, status pgtype.Text
	box            pgtype.UUID
}

func parseTxFilter(w http.ResponseWriter, r *http.Request) (txFilter, bool) {
	q := r.URL.Query()
	var f txFilter
	if v := q.Get("period"); v != "" {
		if !periodRe.MatchString(v) {
			response.BadRequest(w, r, response.CodeValidationError, "period must be YYYY-MM")
			return f, false
		}
		f.period = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("match_status"); v != "" && v != "all" {
		if !matchStatuses[v] {
			response.BadRequest(w, r, response.CodeValidationError, "match_status must be unmatched, suggested, matched, ignored or all")
			return f, false
		}
		f.status = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("cash_box_uuid"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "cash_box_uuid is not a uuid")
			return f, false
		}
		f.box = pgtype.UUID{Bytes: id, Valid: true}
	}
	return f, true
}

// listTransactions: GET .../bank-transactions?period=&match_status=&cash_box_uuid=&limit=&offset=.
func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	q := r.URL.Query()
	f, ok := parseTxFilter(w, r)
	if !ok {
		return
	}
	period, status, box := f.period, f.status, f.box
	limit, offset := int32(txDefaultLimit), int32(0)
	if v := q.Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > txMaxLimit {
			response.BadRequest(w, r, response.CodeValidationError, "limit must be 1..200")
			return
		}
		limit = int32(n)
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 || n > 1_000_000 {
			response.BadRequest(w, r, response.CodeValidationError, "offset must be 0..1000000")
			return
		}
		offset = int32(n)
	}

	rows, err := h.q.ListBankTransactionsPage(r.Context(), db.ListBankTransactionsPageParams{
		TaxpayerID: scope.TaxpayerID, Period: period, MatchStatus: status, CashBoxUuid: box, Lim: limit, Off: offset,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not list bank transactions")
		return
	}
	sum, err := h.q.SummarizeBankTransactions(r.Context(), db.SummarizeBankTransactionsParams{
		TaxpayerID: scope.TaxpayerID, Period: period, CashBoxUuid: box,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not summarize bank transactions")
		return
	}
	items := make([]TransactionView, len(rows))
	ids := make([]uuid.UUID, len(rows))
	for i, t := range rows {
		ids[i] = t.Uuid
	}
	marks, err := h.q.BankTransactionExportMarks(r.Context(), db.BankTransactionExportMarksParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, Uuids: ids,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not load export marks")
		return
	}
	marked := make(map[uuid.UUID]db.BankTransactionExportMarksRow, len(marks))
	for _, m := range marks {
		marked[m.BankTransactionUuid] = m
	}
	booked, err := h.q.BankTransactionBookedAccounts(r.Context(), db.BankTransactionBookedAccountsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, Uuids: ids,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not load booked accounts")
		return
	}
	accounts := make(map[uuid.UUID]db.BankTransactionBookedAccountsRow, len(booked))
	for _, b := range booked {
		accounts[b.BankTransactionUuid] = b
	}
	for i, t := range rows {
		items[i] = TransactionView{
			UUID: t.Uuid, Date: t.TransactionDate.Time.Format(time.DateOnly), Direction: direction(t.DebitCredit),
			Amount: decimalString(t.Amount), Currency: t.Currency, Description: t.Description,
			CounterpartyName: textPtr(t.CounterpartyName), Kind: t.Kind, KindDetail: t.KindDetail,
			MatchStatus: t.MatchStatus, Period: t.Period, PossibleDuplicate: t.PossibleDuplicate,
			CashBoxUUID: t.CashBoxUuid, CashBoxName: t.CashBoxName,
		}
		if b, ok := accounts[t.Uuid]; ok {
			items[i].BookedAccountCode, items[i].BookedAccountName = b.AccountCode, b.AccountName
		}
		if m, ok := marked[t.Uuid]; ok {
			items[i].LucaExports = m.Exports
			if m.LastExportedAt.Valid {
				at := m.LastExportedAt.Time
				items[i].LucaLastExportAt = &at
			}
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items": items, "limit": limit, "offset": offset,
		"summary": TransactionSummary{
			Total: sum.Total, InTotal: decimalString(sum.InTotal), OutTotal: decimalString(sum.OutTotal),
			Unmatched: sum.Unmatched, Suggested: sum.Suggested, Matched: sum.Matched,
		},
	})
}
