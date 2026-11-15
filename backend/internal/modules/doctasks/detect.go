package doctasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// Candidate is a bank transaction as the control sees it.
type Candidate struct {
	ID               int64
	Period           string
	DebitCredit      string // A giriş, B çıkış
	KindDetail       string
	MatchStatus      string
	IsCancelled      bool
	HasDocument      bool
	Amount           string
	Currency         string
	TransactionDate  string
	CounterpartyName string
}

// Finding returns the warning the transaction deserves now, or nil:
//
//   - outgoing supplier_payment without a document → BNK_UNDOCUMENTED_PAYMENT
//   - incoming customer_collection without a document → BNK_UNINVOICED_COLLECTION
//
// "Document" is a matched record in the banking module (match_status
// matched; ignored means the accountant dismissed it) or a file the mükellef
// uploaded for it (bank_transaction_documents). Cancelled movements raise
// nothing.
func Finding(c Candidate) *engine.Finding {
	if c.IsCancelled || c.HasDocument || c.MatchStatus == "matched" || c.MatchStatus == "ignored" {
		return nil
	}
	var code string
	switch {
	case c.DebitCredit == "B" && c.KindDetail == "supplier_payment":
		code = CodeUndocumentedPayment
	case c.DebitCredit == "A" && c.KindDetail == "customer_collection":
		code = CodeUninvoicedCollection
	default:
		return nil
	}
	params := map[string]any{
		"amount":           c.Amount,
		"currency":         c.Currency,
		"transaction_date": c.TransactionDate,
	}
	if c.CounterpartyName != "" {
		params["counterparty_name"] = c.CounterpartyName
	}
	return &engine.Finding{Code: code, Params: params}
}

// DetectReport is what a detection run did.
type DetectReport struct {
	Checked      int `json:"checked"`
	Created      int `json:"created"`
	Refreshed    int `json:"refreshed"`
	Suppressed   int `json:"suppressed"`
	Disabled     int `json:"disabled"`
	AutoResolved int `json:"auto_resolved"`
	// TasksClosed counts open tasks whose condition went away another way.
	TasksClosed int `json:"tasks_closed"`
}

func (r *DetectReport) add(x engine.Report) {
	r.Created += x.Created
	r.Refreshed += x.Refreshed
	r.Suppressed += x.Suppressed
	r.Disabled += x.Disabled
	r.AutoResolved += x.AutoResolved
}

// Detect runs the control for one engagement and period.
func (s *Service) Detect(ctx context.Context, sub engine.Subject, period string) (DetectReport, error) {
	var rep DetectReport
	if !ValidPeriod(period) {
		return rep, ErrBadPeriod
	}
	rows, err := s.q.ListMissingDocumentCandidates(ctx, db.ListMissingDocumentCandidatesParams{
		TaxpayerID: sub.TaxpayerID, Period: period,
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Producer: Producer,
	})
	if err != nil {
		return rep, fmt.Errorf("doctasks: candidates: %w", err)
	}
	var resolved []int64
	for _, r := range rows {
		c := Candidate{
			ID: r.ID, Period: r.Period, DebitCredit: r.DebitCredit, KindDetail: r.KindDetail,
			MatchStatus: r.MatchStatus, IsCancelled: r.IsCancelled, HasDocument: r.HasDocument,
			Amount: numericString(r.Amount), Currency: r.Currency,
			TransactionDate: dateString(r.TransactionDate), CounterpartyName: r.CounterpartyName.String,
		}
		f, err := s.apply(ctx, sub, c)
		if err != nil {
			return rep, err
		}
		rep.Checked++
		rep.add(f.report)
		if !f.found {
			resolved = append(resolved, c.ID)
		}
	}
	n, err := s.closeResolved(ctx, sub, resolved)
	rep.TasksClosed = n
	return rep, err
}

// DetectTransaction re-runs the control for one transaction (after an
// upload). A transaction of another taxpayer is ErrNotFound.
func (s *Service) DetectTransaction(ctx context.Context, sub engine.Subject, bankTransactionID int64) (DetectReport, error) {
	var rep DetectReport
	r, err := s.q.GetMissingDocumentCandidate(ctx, db.GetMissingDocumentCandidateParams{TaxpayerID: sub.TaxpayerID, ID: bankTransactionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return rep, ErrNotFound
	}
	if err != nil {
		return rep, err
	}
	c := Candidate{
		ID: r.ID, Period: r.Period, DebitCredit: r.DebitCredit, KindDetail: r.KindDetail,
		MatchStatus: r.MatchStatus, IsCancelled: r.IsCancelled, HasDocument: r.HasDocument,
		Amount: numericString(r.Amount), Currency: r.Currency,
		TransactionDate: dateString(r.TransactionDate), CounterpartyName: r.CounterpartyName.String,
	}
	f, err := s.apply(ctx, sub, c)
	if err != nil {
		return rep, err
	}
	rep.Checked = 1
	rep.add(f.report)
	if !f.found {
		n, err := s.closeResolved(ctx, sub, []int64{c.ID})
		rep.TasksClosed = n
		return rep, err
	}
	return rep, nil
}

type applied struct {
	report engine.Report
	found  bool
}

func (s *Service) apply(ctx context.Context, sub engine.Subject, c Candidate) (applied, error) {
	var findings []engine.Finding
	f := Finding(c)
	if f != nil {
		findings = append(findings, *f)
	}
	id := c.ID
	rep, err := s.engine.Apply(ctx, sub, Producer,
		engine.Entity{Type: catalog.EntityBankTransaction, ID: &id, Period: c.Period}, OwnedCodes, findings)
	if err != nil {
		return applied{}, fmt.Errorf("doctasks: apply bank transaction %d: %w", c.ID, err)
	}
	return applied{report: rep, found: f != nil}, nil
}

func (s *Service) closeResolved(ctx context.Context, sub engine.Subject, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := s.q.CloseResolvedDocumentTasks(ctx, db.CloseResolvedDocumentTasksParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, BankTransactionIds: ids,
	})
	if err != nil {
		return 0, fmt.Errorf("doctasks: close resolved tasks: %w", err)
	}
	return int(n), nil
}

func numericString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil || n.NaN {
		return ""
	}
	d := decimal.NewFromBigInt(n.Int, n.Exp)
	if d.Equal(d.Round(2)) {
		return d.StringFixed(2)
	}
	return d.String()
}

func dateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}
