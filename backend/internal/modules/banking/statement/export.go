package statement

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Banka hareketleri Excel dışa aktarımı (#395): listenin süzgecine uyan
// hareketler, ekstre kolonlarıyla. Başlıklar ekstre okuyucusunun ve ortak
// Excel içe aktarmanın tanıdığı adlardır: dosya düzenlenip geri yüklenince
// hareketler hesap IBAN'ına göre yazılır, mevcutlar mükerrer olmaz.

// exportLimit bounds one export (a busy year of one account).
const exportLimit = 20_000

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

var exportColumns = []sheet.Column{
	{Key: "account_name", Header: "Hesap Adı", Width: 22},
	{Key: "account_iban", Header: "Hesap IBAN", Width: 30},
	{Key: "date", Header: "Tarih", Width: 12},
	{Key: "value_date", Header: "Valör Tarihi", Width: 12},
	{Key: "description", Header: "Açıklama", Width: 40},
	{Key: "counterparty", Header: "Karşı Taraf", Width: 26},
	{Key: "counterparty_iban", Header: "Karşı Taraf IBAN", Width: 30},
	{Key: "counterparty_tax_id", Header: "Karşı Taraf VKN", Width: 14},
	{Key: "debit", Header: "Borç", Width: 14, Money: true},
	{Key: "credit", Header: "Alacak", Width: 14, Money: true},
	{Key: "balance", Header: "Bakiye", Width: 14, Money: true},
	{Key: "currency", Header: "Para Birimi", Width: 8},
	{Key: "receipt_number", Header: "Dekont No", Width: 14},
	{Key: "reference", Header: "Referans No", Width: 16},
	{Key: "channel", Header: "Kanal", Width: 12},
	{Key: "kind", Header: "Hareket Türü", Width: 16},
	{Key: "match_status", Header: "Eşleşme", Width: 12},
}

var matchLabels = map[string]string{
	"unmatched": "Eşleşmedi", "suggested": "Öneri var", "matched": "Eşleşti", "ignored": "Yok sayıldı",
}

func optStr(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func trDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("02.01.2006")
}

// ExportRows turns the export query rows into sheet rows.
func ExportRows(rows []db.ExportBankTransactionsRow) []sheet.Row {
	out := make([]sheet.Row, 0, len(rows))
	for _, t := range rows {
		r := sheet.Row{
			"account_name": t.CashBoxName, "account_iban": optStr(t.CashBoxIban),
			"date": trDate(t.TransactionDate), "value_date": trDate(t.ValueDate),
			"description": t.Description, "counterparty": optStr(t.CounterpartyName),
			"counterparty_iban": optStr(t.CounterpartyIban), "counterparty_tax_id": t.CounterpartyTaxID,
			"currency": t.Currency, "receipt_number": optStr(t.ReceiptNumber), "reference": optStr(t.BankReference),
			"channel": optStr(t.Channel), "kind": t.Kind, "match_status": matchLabels[t.MatchStatus],
		}
		if direction(t.DebitCredit) == "in" {
			r["credit"] = decimalString(t.Amount)
		} else {
			r["debit"] = decimalString(t.Amount)
		}
		if t.BalanceAfter.Valid {
			r["balance"] = decimalString(t.BalanceAfter)
		}
		out = append(out, r)
	}
	return out
}

// exportTransactions: GET .../bank-transactions/export?period=&match_status=&cash_box_uuid= (.xlsx).
func (h *Handler) exportTransactions(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	f, ok := parseTxFilter(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ExportBankTransactions(r.Context(), db.ExportBankTransactionsParams{
		TaxpayerID: scope.TaxpayerID, Period: f.period, MatchStatus: f.status, CashBoxUuid: f.box, Lim: exportLimit,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not export bank transactions")
		return
	}
	data, err := sheet.Write("Banka hareketleri", exportColumns, ExportRows(rows))
	if err != nil {
		response.InternalErr(w, r, err, "could not write the workbook")
		return
	}
	suffix := f.period.String
	if suffix == "" {
		suffix = time.Now().Format(time.DateOnly)
	}
	w.Header().Set("Content-Type", xlsxType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="banka-hareketleri-%s.xlsx"`, suffix))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
