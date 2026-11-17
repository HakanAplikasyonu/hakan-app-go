package statement

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/target"
)

// ExcelTarget writes bank movement rows of the common Excel import (#395)
// through the statement importer: the same duplicate checks, running
// balance control, BNK_GAP warning and classification as a statement file.
func (im *Importer) ExcelTarget() target.Target { return excelTarget{im: im} }

type excelTarget struct{ im *Importer }

// excelColumns is the canonical sheet: field -> a header the tabular
// reader knows (headerNames).
var excelColumns = []target.Column{
	{Field: "date", Header: "Tarih"},
	{Field: "value_date", Header: "Valör Tarihi"},
	{Field: "time", Header: "Saat"},
	{Field: "description", Header: "Açıklama"},
	{Field: "amount", Header: "Tutar"},
	{Field: "debit", Header: "Borç"},
	{Field: "credit", Header: "Alacak"},
	{Field: "balance", Header: "Bakiye"},
	{Field: "receipt_number", Header: "Dekont No"},
	{Field: "reference", Header: "Referans No"},
	{Field: "channel", Header: "Kanal"},
	{Field: "counterparty", Header: "Karşı Taraf"},
	{Field: "counterparty_iban", Header: "Karşı Taraf IBAN"},
	{Field: "counterparty_tax_id", Header: "Karşı Taraf VKN"},
}

var fieldOf = map[column]string{
	colDate: "date", colValueDate: "value_date", colTime: "time", colDescription: "description",
	colAmount: "amount", colDebit: "debit", colCredit: "credit", colBalance: "balance",
	colReceipt: "receipt_number", colReference: "reference", colChannel: "channel",
	colCounterparty: "counterparty", colCounterpartyIBAN: "counterparty_iban", colCounterpartyTaxID: "counterparty_tax_id",
}

// accountHeaders name the statement's own account column.
var accountHeaders = []string{"IBAN", "HESAP IBAN", "HESAP", "HESAP NO", "HESAP NUMARASI", "ACCOUNT", "ACCOUNT IBAN"}

// HeaderField maps a statement header to a bank_transactions field.
func HeaderField(h string) string {
	n := banking.NormalizeDescription(h)
	if n == "" {
		return ""
	}
	for _, a := range accountHeaders {
		if n == a {
			return "account_iban"
		}
	}
	for c, names := range headerNames {
		for _, name := range names {
			if n == name {
				return fieldOf[c]
			}
		}
	}
	return ""
}

// Heuristic maps the header names banks use.
func (excelTarget) Heuristic(l parse.Layout) mapping.Mapping {
	return target.MatchHeaders(mapping.BankTransactions, l, HeaderField)
}

// Commit groups the rows by account and imports each group as one
// statement. Rows without an account go to the taxpayer's only bank
// account; with more than one the account column must be mapped.
func (t excelTarget) Commit(ctx context.Context, in target.Input) (target.Result, error) {
	groups := map[string][]target.Row{}
	for _, r := range in.Rows {
		iban := banking.NormalizeIBAN(r.Values["account_iban"])
		if iban == "" && strings.TrimSpace(r.Values["account_iban"]) != "" {
			return target.Result{}, target.Rejectf("satır %d: hesap IBAN'ı okunamadı (%s)", r.RowNo, r.Values["account_iban"])
		}
		groups[iban] = append(groups[iban], r)
	}
	var fallback int64
	if len(groups[""]) > 0 {
		boxes, err := database.QueriesFor(t.im.pool).ListCashBoxes(ctx, in.Scope.TaxpayerID)
		if err != nil {
			return target.Result{}, err
		}
		var banks []int64
		for _, b := range boxes {
			if b.Type == "bank_account" {
				banks = append(banks, b.ID)
			}
		}
		if len(banks) != 1 {
			return target.Result{}, target.Rejectf("dosyada hesap bilgisi yok ve mükellefin %d banka hesabı var; hesap IBAN kolonunu eşleyin", len(banks))
		}
		fallback = banks[0]
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := target.Result{Errors: []target.RowError{}}
	accounts := 0
	for _, iban := range keys {
		var pre [][]string
		req := Request{
			TaxpayerID: in.Scope.TaxpayerID, OrganizationID: in.Scope.OrganizationID, EngagementID: in.Scope.ID,
			FileName: target.CSVName(in.FileName),
		}
		if iban != "" {
			pre = [][]string{{"Hesap IBAN", iban}}
		} else {
			req.CashBoxID = fallback
		}
		data, lines, err := target.CSV(pre, excelColumns, groups[iban])
		if err != nil {
			return out, err
		}
		req.Data = data
		rep, err := t.im.Import(ctx, req)
		var unknown *UnknownAccountError
		switch {
		case errors.As(err, &unknown), errors.Is(err, ErrAccountRequired):
			return out, target.Rejectf("%s hesabı mükellefin banka hesaplarında yok; önce hesabı ekleyin", iban)
		case errors.As(err, new(parseError)):
			return out, target.Rejectf("%v", err)
		case err != nil:
			return out, err
		}
		for _, a := range rep.Accounts {
			accounts++
			out.Added += a.Inserted
			out.Duplicates += a.Merged + a.Skipped
			for _, e := range a.Errors {
				out.Errors = append(out.Errors, target.RowError{RowNo: lines[e.Row], Reason: e.Reason})
			}
		}
	}
	out.Note = fmt.Sprintf("%d hesaba %d hareket yazıldı", accounts, out.Added)
	return out, nil
}
