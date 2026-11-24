package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Posting sources (bank_transaction_postings.source).
const (
	SourceAlias               = "alias"
	SourceCounterpartyAccount = "counterparty_account"
	SourceInvoiceMatch        = "invoice_match"
	SourceKindDefault         = "kind_default"
	SourceManual              = "manual"
)

// DefaultBankAccount is the bank side of a movement whose cash box has no
// ledger account yet (bank_ledger_accounts).
const DefaultBankAccount = "102"

// kindDefault is the usual counter account of a classified movement
// (Tek Düzen Hesap Planı). Own transfers and FX trades have none: their
// counter account is the other bank account, which only the accountant
// knows; a bare "102" would book the bank against itself (102 borç / 102
// alacak in one entry). conf is the most the default may claim; the
// classification's own confidence lowers it.
type kindDefault struct {
	code, name string
	conf       float64
}

var kindDefaults = map[banking.Detail]kindDefault{
	banking.DetailCustomerCollection: {"120", "Alıcılar", 0.40},
	banking.DetailSupplierPayment:    {"320", "Satıcılar", 0.40},
	banking.DetailBankFee:            {"780", "Finansman giderleri", 0.75},
	banking.DetailCommission:         {"780", "Finansman giderleri", 0.75},
	banking.DetailBSMV:               {"780", "Finansman giderleri", 0.75},
	banking.DetailInterestExpense:    {"780", "Finansman giderleri", 0.75},
	banking.DetailInterestIncome:     {"642", "Faiz gelirleri", 0.75},
	banking.DetailWithholdingTax:     {"193", "Peşin ödenen vergiler ve fonlar", 0.70},
	banking.DetailLoanPrincipal:      {"300", "Banka kredileri", 0.70},
	banking.DetailLoanDisbursement:   {"300", "Banka kredileri", 0.70},
	banking.DetailTaxPayment:         {"360", "Ödenecek vergi ve fonlar", 0.70},
	banking.DetailSGKPayment:         {"361", "Ödenecek sosyal güvenlik kesintileri", 0.75},
	banking.DetailSalaryPayment:      {"335", "Personele borçlar", 0.75},
	banking.DetailRentPayment:        {"770", "Genel yönetim giderleri", 0.55},
	banking.DetailUtilityPayment:     {"770", "Genel yönetim giderleri", 0.55},
	banking.DetailPOSSettlement:      {"108", "Diğer hazır değerler", 0.60},
	banking.DetailPOSCommission:      {"780", "Finansman giderleri", 0.65},
	banking.DetailInsurancePayment:   {"770", "Genel yönetim giderleri", 0.55},
	banking.DetailVehicleToll:        {"770", "Genel yönetim giderleri", 0.55},
	banking.DetailCashDeposit:        {"100", "Kasa", 0.70},
	banking.DetailCashWithdrawal:     {"100", "Kasa", 0.70},
	banking.DetailPrivate:            {"331", "Ortaklara borçlar", 0.50},
}

// cardPayment is the counter account of a credit card payment (KK_ODEMESI).
var cardPayment = kindDefault{"309", "Diğer mali borçlar (kredi kartı)", 0.60}

// cariDetails: movements whose counter account is the counterparty's cari.
var cariDetails = map[banking.Detail]bool{
	banking.DetailCustomerCollection: true, banking.DetailSupplierPayment: true, banking.DetailUnknown: true,
	banking.DetailRentPayment: true, banking.DetailUtilityPayment: true,
}

// DefaultCounterAccount is the usual counter account of a movement type
// (kindDefaults), else the cari main account by direction (120 incoming,
// 320 outgoing). The Luca list export writes it for a movement without an
// account and marks the line "kontrol edin".
func DefaultCounterAccount(detail banking.Detail, dir banking.Direction) string {
	if d, ok := kindDefaults[detail]; ok {
		return d.code
	}
	if dir == banking.DirIn {
		return "120"
	}
	return "320"
}

// Movement is what the suggestion needs of a bank transaction.
type Movement struct {
	ID             int64
	Direction      banking.Direction
	Amount         decimal.Decimal
	Description    string
	Counterparty   Counterparty
	Kind           banking.Kind
	Detail         banking.Detail
	KindConfidence *decimal.Decimal
	// MatchedTaxID: VKN / TCKN of the invoice the movement is matched to.
	MatchedTaxID string
	MatchedName  string
}

// Suggestion is the proposed counter account of a movement.
type Suggestion struct {
	AccountCode string
	AccountName string
	Confidence  decimal.Decimal
	Source      string
	Reason      string
	AliasID     int64
}

// Suggest proposes the counter account: the alias memory first, then the
// cari hafızası by the matched invoice's or the movement's VKN / TCKN, then
// the usual account of the movement's type. nil: nothing to propose.
func Suggest(ctx context.Context, q *db.Queries, sub Subject, m Movement) (*Suggestion, error) {
	if a, err := Resolve(ctx, q, sub, m.Counterparty); err != nil {
		return nil, err
	} else if a != nil {
		return &Suggestion{AccountCode: a.Alias.AccountCode, AccountName: a.Alias.AccountName, Confidence: a.Confidence,
			Source: SourceAlias, Reason: a.Reason, AliasID: a.Alias.ID}, nil
	}
	if cariDetails[m.Detail] {
		main := "320"
		if m.Direction == banking.DirIn {
			main = "120"
		}
		for _, c := range []struct {
			taxID, source, conf, why string
		}{
			{m.MatchedTaxID, SourceInvoiceMatch, "0.88", "Eşleşen faturanın carisi"},
			{KeysOf(m.Counterparty).TaxID, SourceCounterpartyAccount, "0.85", "VKN/TCKN ile cari hafızası"},
		} {
			vkn, tckn := banking.SplitTaxID(c.taxID)
			if vkn+tckn == "" {
				continue
			}
			row, err := q.GetCounterpartyAccount(ctx, db.GetCounterpartyAccountParams{
				OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, CounterpartyTaxID: vkn + tckn, MainCode: main,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			reason := fmt.Sprintf("%s: %s → %s", c.why, firstNonEmpty(row.AccountName, vkn+tckn), row.AccountCode)
			return &Suggestion{AccountCode: row.AccountCode, AccountName: row.AccountName,
				Confidence: decimal.RequireFromString(c.conf), Source: c.source, Reason: reason}, nil
		}
	}
	d, ok := kindDefaults[m.Detail]
	if m.Kind == banking.KindKKOdemesi {
		// A credit card payment is classified as an own transfer, but its
		// counter account is the card debt, not another bank account.
		d, ok = cardPayment, true
	}
	if !ok {
		return nil, nil
	}
	conf := decimal.NewFromFloat(d.conf)
	if m.KindConfidence != nil {
		if m.KindConfidence.LessThan(conf) {
			conf = *m.KindConfidence
		}
	} else {
		conf = conf.Mul(decimal.RequireFromString("0.8"))
	}
	name := d.name
	if who := strings.TrimSpace(firstNonEmpty(m.Counterparty.Name, m.MatchedName)); who != "" && (d.code == "120" || d.code == "320") {
		name = who
	}
	label := detailLabel(m.Detail)
	if m.Kind == banking.KindKKOdemesi {
		label = "kredi kartı ödemesi"
	}
	reason := fmt.Sprintf("İşlem türü (%s): %s %s", label, d.code, d.name)
	if d.code == "120" || d.code == "320" {
		reason += "; carisi bulunamadı, alt hesap seçilmeli"
	}
	return &Suggestion{AccountCode: d.code, AccountName: name, Confidence: conf.Round(4), Source: SourceKindDefault, Reason: reason}, nil
}

var detailLabels = map[banking.Detail]string{
	banking.DetailCustomerCollection: "müşteri tahsilatı", banking.DetailSupplierPayment: "satıcı ödemesi",
	banking.DetailOwnTransfer: "hesaplar arası virman", banking.DetailBankFee: "banka masrafı",
	banking.DetailCommission: "komisyon", banking.DetailBSMV: "BSMV", banking.DetailInterestExpense: "faiz gideri",
	banking.DetailInterestIncome: "faiz geliri", banking.DetailWithholdingTax: "stopaj",
	banking.DetailLoanPrincipal: "kredi ödemesi", banking.DetailLoanDisbursement: "kredi kullanımı",
	banking.DetailTaxPayment: "vergi ödemesi", banking.DetailSGKPayment: "SGK ödemesi",
	banking.DetailSalaryPayment: "maaş ödemesi", banking.DetailRentPayment: "kira ödemesi",
	banking.DetailUtilityPayment: "fatura ödemesi", banking.DetailPOSSettlement: "POS tahsilatı",
	banking.DetailPOSCommission: "POS komisyonu", banking.DetailInsurancePayment: "sigorta ödemesi",
	banking.DetailVehicleToll: "HGS/OGS", banking.DetailFXTrade: "döviz alım satımı",
	banking.DetailCashDeposit: "nakit yatırma", banking.DetailCashWithdrawal: "nakit çekme",
	banking.DetailPrivate: "özel harcama",
}

func detailLabel(d banking.Detail) string {
	if l, ok := detailLabels[d]; ok {
		return l
	}
	return string(d)
}
