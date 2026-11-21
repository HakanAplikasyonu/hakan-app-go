package taxreturns

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Faturasız kira stopajı (#575): şahıstan kiralanan işyerinin kirası
// bankadan net ödenir. Ofis banka çıkışını kira ödemesi işaretler; stopaj
// ödendiği ayın muhtasarına "Kira ödemeleri" satırı olarak girer:
//
//	brüt = net / (1 − oran), stopaj = brüt − net (oran varsayılan %20,
//	GVK 94/5-a; muhtasar ödeme türü 041).
//
// Açıklamasında kira geçen, faturayla eşleşmemiş ve işaretlenmemiş çıkışlar
// kontrol listesinde uyarıdır: faturalı kirada stopaj faturadan gelir.

// WithholdingRent is the MUHSGK row of rent paid from the bank without an
// invoice.
const WithholdingRent = "rent"

// DefaultRentRate is the GVK 94/5-a rate for rent paid to individuals.
var DefaultRentRate = decimal.NewFromInt(20)

// ErrInvalidRent: the movement is missing, not an outgoing TRY payment of
// the period's taxpayer, cancelled, or the rate is out of range.
var ErrInvalidRent = errors.New("taxreturns: invalid rent payment")

// MUHSGKRentPayment is a bank movement that is (or may be) rent paid
// without an invoice.
type MUHSGKRentPayment struct {
	UUID             uuid.UUID `json:"uuid"`
	TransactionDate  string    `json:"transaction_date"`
	Description      string    `json:"description"`
	CounterpartyName string    `json:"counterparty_name"`
	Net              Money     `json:"net"`
	Marked           bool      `json:"marked"`
	Rate             string    `json:"rate"`
	Gross            Money     `json:"gross"`
	Tax              Money     `json:"tax"`
}

// RentInput is the body of PUT .../rent-withholdings/{transactionUuid}.
type RentInput struct {
	Rate *string `json:"rate"`
}

// rentGross: brüt = net / (1 − oran/100), kuruşa yuvarlanır.
func rentGross(net, rate decimal.Decimal) (gross, tax decimal.Decimal) {
	gross = net.Div(decimal.NewFromInt(1).Sub(rate.Div(decimal.NewFromInt(100)))).Round(2)
	return gross, gross.Sub(net)
}

// applyRent adds the period's rent paid without an invoice to the MUHSGK
// preview: a "rent" row per rate, the tax in the totals, the movements, and
// a warning for rent-looking movements not marked.
func (s *Service) applyRent(ctx context.Context, sc engagementctx.Scope, period string, m *MUHSGK) ([]CheckItem, error) {
	rows, err := s.q.ListRentWithholdingRows(ctx, db.ListRentWithholdingRowsParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Period: period,
	})
	if err != nil {
		return nil, fmt.Errorf("rent withholdings: %w", err)
	}
	m.RentPayments = []MUHSGKRentPayment{}
	type agg struct {
		count      int64
		gross, tax decimal.Decimal
	}
	byRate := map[string]*agg{}
	var rates []string
	var total decimal.Decimal
	var unmarked int64
	for _, r := range rows {
		p := MUHSGKRentPayment{UUID: r.Uuid, TransactionDate: dateStr(r.TransactionDate), Description: r.Description,
			CounterpartyName: r.CounterpartyName, Net: M(dec(r.Amount)), Marked: r.Marked}
		if r.Marked {
			rate := dec(r.Rate)
			p.Rate, p.Gross, p.Tax = rateString(rate), M(dec(r.GrossAmount)), M(dec(r.TaxAmount))
			k := rateString(rate)
			if byRate[k] == nil {
				byRate[k] = &agg{}
				rates = append(rates, k)
			}
			a := byRate[k]
			a.count++
			a.gross, a.tax = a.gross.Add(dec(r.GrossAmount)), a.tax.Add(dec(r.TaxAmount))
			total = total.Add(dec(r.TaxAmount))
		} else {
			unmarked++
		}
		m.RentPayments = append(m.RentPayments, p)
	}
	for _, k := range rates {
		a := byRate[k]
		m.Withholdings = append(m.Withholdings, MUHSGKWithholding{Kind: WithholdingRent, Rate: k, Count: a.count, Base: M(a.gross), Tax: M(a.tax)})
	}
	var items []CheckItem
	if total.IsPositive() {
		it := m.Totals.IncomeTax.Dec().Add(total)
		m.Totals.IncomeTax = M(it)
		m.Totals.TaxPayable = M(it.Add(m.Totals.StampTax.Dec()))
		items = append(items, itemAmount("RENT_WITHHOLDING", SeverityInfo,
			"Bankadan faturasız ödenen kiranın stopajı (brüt üzerinden) muhtasara eklendi.", int64(len(rows))-unmarked, total))
	}
	if unmarked > 0 {
		items = append(items, item("RENT_UNMARKED", SeverityWarning,
			fmt.Sprintf("%d banka çıkışı kira ödemesine benziyor; şahsa faturasız ödendiyse kira stopajı olarak ekleyin.", unmarked), unmarked))
	}
	return items, nil
}

// SetRent marks (or with rate changes) a bank movement of the period as
// rent paid without an invoice. Only while the MUHSGK return is a draft.
func (s *Service) SetRent(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string, txn uuid.UUID, in RentInput) (Preview, error) {
	b, err := s.rentTarget(ctx, sc, kind, period, txn)
	if err != nil {
		return Preview{}, err
	}
	if b.DebitCredit != "B" || b.Currency != "TRY" || b.IsCancelled {
		return Preview{}, fmt.Errorf("%w: the movement must be an outgoing TRY payment", ErrInvalidRent)
	}
	rate := DefaultRentRate
	if in.Rate != nil && strings.TrimSpace(*in.Rate) != "" {
		r, err := decimal.NewFromString(strings.TrimSpace(*in.Rate))
		if err != nil || !r.IsPositive() || r.GreaterThanOrEqual(decimal.NewFromInt(100)) {
			return Preview{}, fmt.Errorf("%w: rate must be between 0 and 100", ErrInvalidRent)
		}
		rate = r.Round(2)
	}
	net := dec(b.Amount).Round(2)
	gross, tax := rentGross(net, rate)
	if err := s.q.UpsertRentWithholding(ctx, db.UpsertRentWithholdingParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, BankTransactionID: b.ID, Period: period,
		Rate: num(rate), NetAmount: num(net), GrossAmount: num(gross), TaxAmount: num(tax), CreatedBy: actor(actorID),
	}); err != nil {
		return Preview{}, fmt.Errorf("set rent withholding: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

// DeleteRent removes the rent mark of a movement (draft MUHSGK only).
func (s *Service) DeleteRent(ctx context.Context, sc engagementctx.Scope, kind, period string, txn uuid.UUID) (Preview, error) {
	b, err := s.rentTarget(ctx, sc, kind, period, txn)
	if err != nil {
		return Preview{}, err
	}
	if _, err := s.q.DeleteRentWithholding(ctx, db.DeleteRentWithholdingParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, BankTransactionID: b.ID,
	}); err != nil {
		return Preview{}, fmt.Errorf("delete rent withholding: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}

// rentTarget checks the return (MUHSGK, draft) and finds the movement of
// the period.
func (s *Service) rentTarget(ctx context.Context, sc engagementctx.Scope, kind, period string, txn uuid.UUID) (db.GetBankTransactionForRentRow, error) {
	var none db.GetBankTransactionForRentRow
	if err := check(kind, period); err != nil {
		return none, err
	}
	if kind != KindMUHSGK {
		return none, fmt.Errorf("%w: rent withholding belongs to the MUHSGK return", ErrInvalidInput)
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return none, err
	}
	if rec != nil && rec.Status != StatusDraft {
		return none, ErrNotDraft
	}
	b, err := s.q.GetBankTransactionForRent(ctx, db.GetBankTransactionForRentParams{TaxpayerID: sc.TaxpayerID, Uuid: txn})
	if errors.Is(err, pgx.ErrNoRows) {
		return none, ErrNotFound
	} else if err != nil {
		return none, fmt.Errorf("rent movement: %w", err)
	}
	if b.Period != period {
		return none, fmt.Errorf("%w: the movement is not in the period", ErrInvalidRent)
	}
	return b, nil
}
