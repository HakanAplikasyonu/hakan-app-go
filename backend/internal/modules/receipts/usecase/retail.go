package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ZContinuityProducer is the data_warning producer of the Z continuity check.
const ZContinuityProducer = "control:z_continuity"

// VatLine is one VAT rate of a Z report.
type VatLine struct {
	Rate decimal.Decimal `json:"tax_rate"`
	Base decimal.Decimal `json:"tax_base"`
	Tax  decimal.Decimal `json:"tax_amount"`
}

// RetailSaleView is one Z report in the office panel.
type RetailSaleView struct {
	UUID              uuid.UUID        `json:"uuid"`
	Date              *string          `json:"sale_date"`
	Period            *string          `json:"period"`
	ZNumber           *string          `json:"z_report_number"`
	OKCSerial         *string          `json:"okc_serial_number"`
	Subtotal          *decimal.Decimal `json:"subtotal_amount"`
	TaxAmount         *decimal.Decimal `json:"tax_amount"`
	Total             *decimal.Decimal `json:"total_amount"`
	Currency          string           `json:"currency"`
	Cash              *decimal.Decimal `json:"cash_amount"`
	Card              *decimal.Decimal `json:"card_amount"`
	OtherPayment      *decimal.Decimal `json:"other_payment_amount"`
	ReceiptCount      *int32           `json:"receipt_count"`
	VatLines          []VatLine        `json:"vat_lines"`
	SourceSystem      string           `json:"source_system"`
	CaptureMethod     string           `json:"capture_method"`
	ApprovalStatus    string           `json:"approval_status"`
	OCRConfidence     *decimal.Decimal `json:"ocr_confidence"`
	HasAttachment     bool             `json:"has_attachment"`
	Warnings          []string         `json:"warnings"`
	Journal           *JournalRef      `json:"journal"`
	ApprovedAt        *time.Time       `json:"approved_at"`
	CreatedAt         time.Time        `json:"created_at"`
	BreakdownMismatch bool             `json:"breakdown_mismatch"`
}

// RetailFilter narrows the Z list.
type RetailFilter struct {
	Period string
	Status string
	OKC    string // serial, or "-" for none
	Limit  int32
	Offset int32
}

// Device is one ÖKC of the taxpayer.
type Device struct {
	Serial   string  `json:"okc_serial_number"`
	Reports  int64   `json:"reports"`
	LastDate *string `json:"last_date"`
}

// RetailPage is one page of Z reports with totals over the filter.
type RetailPage struct {
	Items     []RetailSaleView `json:"items"`
	Total     int64            `json:"total"`
	Pending   int64            `json:"pending"`
	Amount    decimal.Decimal  `json:"total_amount"`
	TaxAmount decimal.Decimal  `json:"tax_amount"`
	Cash      decimal.Decimal  `json:"cash_amount"`
	Card      decimal.Decimal  `json:"card_amount"`
	VatRates  []VatLine        `json:"vat_rates"`
	Devices   []Device         `json:"devices"`
	Limit     int32            `json:"limit"`
	Offset    int32            `json:"offset"`
}

// RetailSales lists the taxpayer's Z reports by ÖKC and day.
func (s *Service) RetailSales(ctx context.Context, sc engagementctx.Scope, f RetailFilter) (RetailPage, error) {
	if !statuses[f.Status] {
		return RetailPage{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	period, status, okc := text(f.Period), text(f.Status), text(strings.TrimSpace(f.OKC))
	rows, err := s.q.ListEngagementRetailSales(ctx, db.ListEngagementRetailSalesParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
		Period: period, Status: status, Okc: okc, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return RetailPage{}, err
	}
	sum, err := s.q.SummarizeEngagementRetailSales(ctx, db.SummarizeEngagementRetailSalesParams{TaxpayerID: sc.TaxpayerID, Period: period, Status: status, Okc: okc})
	if err != nil {
		return RetailPage{}, err
	}
	rates, err := s.q.SummarizeRetailSaleVatRates(ctx, db.SummarizeRetailSaleVatRatesParams{TaxpayerID: sc.TaxpayerID, Period: period, Status: status, Okc: okc})
	if err != nil {
		return RetailPage{}, err
	}
	devs, err := s.q.ListRetailSaleDevices(ctx, sc.TaxpayerID)
	if err != nil {
		return RetailPage{}, err
	}
	out := RetailPage{Items: make([]RetailSaleView, 0, len(rows)), Total: sum.Total, Pending: sum.Pending,
		Amount: dec(sum.TotalAmount), TaxAmount: dec(sum.TaxAmount), Cash: dec(sum.CashAmount), Card: dec(sum.CardAmount),
		VatRates: []VatLine{}, Devices: []Device{}, Limit: f.Limit, Offset: f.Offset}
	for _, r := range rates {
		out.VatRates = append(out.VatRates, VatLine{Rate: dec(r.TaxRate), Base: dec(r.TaxBase), Tax: dec(r.TaxAmount)})
	}
	for _, d := range devs {
		dv := Device{Serial: d.OkcSerialNumber, Reports: d.Reports}
		if d.LastDate.Valid {
			t := d.LastDate.Time.Format(time.DateOnly)
			dv.LastDate = &t
		}
		out.Devices = append(out.Devices, dv)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	lines, err := s.vatLines(ctx, sc.TaxpayerID, ids)
	if err != nil {
		return RetailPage{}, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, retailView(r, lines[r.ID]))
	}
	return out, nil
}

func (s *Service) vatLines(ctx context.Context, taxpayerID int64, ids []int64) (map[int64][]VatLine, error) {
	out := map[int64][]VatLine{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListRetailSaleVatLines(ctx, db.ListRetailSaleVatLinesParams{TaxpayerID: taxpayerID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.RetailSaleID] = append(out[r.RetailSaleID], VatLine{Rate: dec(r.TaxRate), Base: dec(r.TaxBase), Tax: dec(r.TaxAmount)})
	}
	return out, nil
}

func retailView(r db.ListEngagementRetailSalesRow, lines []VatLine) RetailSaleView {
	v := RetailSaleView{
		UUID: r.Uuid, Period: str(r.Period), ZNumber: str(r.ZReportNumber), OKCSerial: str(r.OkcSerialNumber),
		Subtotal: decPtr(r.SubtotalAmount), TaxAmount: decPtr(r.TaxAmount), Total: decPtr(r.TotalAmount), Currency: r.Currency,
		Cash: decPtr(r.CashAmount), Card: decPtr(r.CardAmount), OtherPayment: decPtr(r.OtherPaymentAmount),
		VatLines: lines, SourceSystem: r.SourceSystem, CaptureMethod: r.CaptureMethod, ApprovalStatus: r.ApprovalStatus,
		OCRConfidence: decPtr(r.OcrConfidence), HasAttachment: r.AttachmentID.Valid, Warnings: r.WarningCodes, CreatedAt: r.CreatedAt.Time,
	}
	if v.VatLines == nil {
		v.VatLines = []VatLine{}
	}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	if r.ReceiptCount.Valid {
		n := r.ReceiptCount.Int32
		v.ReceiptCount = &n
	}
	if r.SaleDate.Valid {
		d := r.SaleDate.Time.Format(time.DateOnly)
		v.Date = &d
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		v.ApprovedAt = &t
	}
	if r.JournalUuid.Valid {
		v.Journal = &JournalRef{UUID: r.JournalUuid.Bytes, EntryNo: r.JournalEntryNo.Int32, Status: r.JournalStatus.String}
	}
	if len(lines) > 0 && v.Total != nil {
		var sum decimal.Decimal
		for _, l := range lines {
			sum = sum.Add(l.Base).Add(l.Tax)
		}
		v.BreakdownMismatch = sum.Sub(*v.Total).Abs().GreaterThan(decimal.RequireFromString("0.10"))
	}
	return v
}

// RetailSale returns one Z report.
func (s *Service) RetailSale(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (RetailSaleView, error) {
	r, err := s.q.GetRetailSaleByUUID(ctx, db.GetRetailSaleByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RetailSaleView{}, ErrNotFound
	}
	if err != nil {
		return RetailSaleView{}, err
	}
	return s.retailByID(ctx, sc, r)
}

func (s *Service) retailByID(ctx context.Context, sc engagementctx.Scope, r db.RetailSale) (RetailSaleView, error) {
	row := db.ListEngagementRetailSalesRow{
		ID: r.ID, Uuid: r.Uuid, SourceSystem: r.SourceSystem, CaptureMethod: r.CaptureMethod, SaleDate: r.SaleDate, Period: r.Period,
		ZReportNumber: r.ZReportNumber, OkcSerialNumber: r.OkcSerialNumber, SubtotalAmount: r.SubtotalAmount, TaxAmount: r.TaxAmount,
		TotalAmount: r.TotalAmount, Currency: r.Currency, CashAmount: r.CashAmount, CardAmount: r.CardAmount,
		OtherPaymentAmount: r.OtherPaymentAmount, ReceiptCount: r.ReceiptCount, ApprovalStatus: r.ApprovalStatus,
		OcrConfidence: r.OcrConfidence, AttachmentID: r.AttachmentID, ApprovedAt: r.ApprovedAt, CreatedAt: r.CreatedAt,
	}
	if p, err := s.q.GetReceiptPosting(ctx, db.GetReceiptPostingParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TargetType: TargetRetailSale, TargetID: r.ID}); err == nil && p.JournalDraftID.Valid {
		if j, err := s.q.GetJournalDraftByID(ctx, db.GetJournalDraftByIDParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ID: p.JournalDraftID.Int64}); err == nil {
			row.JournalUuid = pgtype.UUID{Bytes: j.Uuid, Valid: true}
			row.JournalEntryNo = pgtype.Int4{Int32: j.EntryNo, Valid: true}
			row.JournalStatus = pgtype.Text{String: j.Status, Valid: true}
		}
	}
	codes, err := s.q.OpenWarningCodesForEntity(ctx, db.OpenWarningCodesForEntityParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, EntityType: catalog.EntityRetailSale, EntityID: pgtype.Int8{Int64: r.ID, Valid: true},
	})
	if err != nil {
		return RetailSaleView{}, err
	}
	row.WarningCodes = codes
	lines, err := s.vatLines(ctx, sc.TaxpayerID, []int64{r.ID})
	if err != nil {
		return RetailSaleView{}, err
	}
	return retailView(row, lines[r.ID]), nil
}

// RetailInput corrects a pending Z report (nil = unchanged). VatLines, when
// given, replace the breakdown.
type RetailInput struct {
	Date         *string          `json:"sale_date"`
	ZNumber      *string          `json:"z_report_number"`
	OKCSerial    *string          `json:"okc_serial_number"`
	Total        *decimal.Decimal `json:"total_amount"`
	TaxAmount    *decimal.Decimal `json:"tax_amount"`
	Cash         *decimal.Decimal `json:"cash_amount"`
	Card         *decimal.Decimal `json:"card_amount"`
	OtherPayment *decimal.Decimal `json:"other_payment_amount"`
	ReceiptCount *int32           `json:"receipt_count"`
	VatLines     *[]VatLine       `json:"vat_lines"`
}

// UpdateRetailSale corrects a pending Z report and re-runs the continuity
// check of its ÖKC.
func (s *Service) UpdateRetailSale(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, in RetailInput) (RetailSaleView, error) {
	r, err := s.q.GetRetailSaleByUUID(ctx, db.GetRetailSaleByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RetailSaleView{}, ErrNotFound
	}
	if err != nil {
		return RetailSaleView{}, err
	}
	if r.ApprovalStatus != "pending" {
		return RetailSaleView{}, ErrState
	}
	p := db.SetRetailSaleBreakdownParams{TaxpayerID: sc.TaxpayerID, ID: r.ID}
	if in.Date != nil {
		d, err := time.Parse(time.DateOnly, *in.Date)
		if err != nil {
			return RetailSaleView{}, fmt.Errorf("%w: sale_date must be YYYY-MM-DD", ErrInvalid)
		}
		p.SaleDate = pgtype.Date{Time: d, Valid: true}
	}
	if in.ZNumber != nil {
		p.ZReportNumber = text(strings.TrimSpace(*in.ZNumber))
	}
	if in.OKCSerial != nil {
		p.OkcSerialNumber = text(strings.TrimSpace(*in.OKCSerial))
	}
	for _, n := range []struct {
		v   *decimal.Decimal
		dst *pgtype.Numeric
	}{{in.Total, &p.TotalAmount}, {in.TaxAmount, &p.TaxAmount}, {in.Cash, &p.CashAmount}, {in.Card, &p.CardAmount}, {in.OtherPayment, &p.OtherPaymentAmount}} {
		if n.v == nil {
			continue
		}
		if n.v.IsNegative() {
			return RetailSaleView{}, fmt.Errorf("%w: amounts must not be negative", ErrInvalid)
		}
		*n.dst = toNumeric(*n.v)
	}
	if in.ReceiptCount != nil {
		if *in.ReceiptCount < 0 {
			return RetailSaleView{}, fmt.Errorf("%w: receipt_count", ErrInvalid)
		}
		p.ReceiptCount = pgtype.Int4{Int32: *in.ReceiptCount, Valid: true}
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return RetailSaleView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	row, err := q.SetRetailSaleBreakdown(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return RetailSaleView{}, ErrState
	}
	if err != nil {
		return RetailSaleView{}, err
	}
	if in.VatLines != nil {
		if err := replaceVatLines(ctx, q, sc.TaxpayerID, r.ID, *in.VatLines); err != nil {
			return RetailSaleView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RetailSaleView{}, err
	}
	for _, okc := range []string{r.OkcSerialNumber.String, row.OkcSerialNumber.String} {
		s.CheckZContinuity(ctx, sc, okc)
	}
	return s.retailByID(ctx, sc, row)
}

func replaceVatLines(ctx context.Context, q *db.Queries, taxpayerID, saleID int64, lines []VatLine) error {
	seen := map[string]bool{}
	rows := make([]map[string]string, 0, len(lines))
	for _, l := range lines {
		if l.Rate.IsNegative() || l.Rate.GreaterThan(decimal.NewFromInt(100)) || l.Base.IsNegative() || l.Tax.IsNegative() {
			return fmt.Errorf("%w: VAT line amounts and rate", ErrInvalid)
		}
		if seen[l.Rate.String()] {
			return fmt.Errorf("%w: VAT rate %s twice", ErrInvalid, l.Rate)
		}
		seen[l.Rate.String()] = true
		if l.Base.IsZero() && l.Tax.IsZero() {
			continue
		}
		rows = append(rows, map[string]string{"tax_rate": l.Rate.String(), "tax_base": l.Base.String(), "tax_amount": l.Tax.String()})
	}
	if err := q.DeleteRetailSaleVatLines(ctx, db.DeleteRetailSaleVatLinesParams{TaxpayerID: taxpayerID, RetailSaleID: saleID}); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	raw, _ := json.Marshal(rows)
	_, err := q.InsertRetailSaleVatLines(ctx, db.InsertRetailSaleVatLinesParams{TaxpayerID: taxpayerID, RetailSaleID: saleID, Rows: raw})
	return err
}

// storeZBreakdown writes what the AI read of a Z report's breakdown (in the
// capture's transaction).
func storeZBreakdown(ctx context.Context, q *db.Queries, taxpayerID, saleID int64, res extract.Result) error {
	p := db.SetRetailSaleBreakdownParams{TaxpayerID: taxpayerID, ID: saleID}
	if v, ok := res.Decimal(extract.FieldCashAmount); ok {
		p.CashAmount = toNumeric(v)
	}
	if v, ok := res.Decimal(extract.FieldCardAmount); ok {
		p.CardAmount = toNumeric(v)
	}
	if v, ok := res.Decimal(extract.FieldReceiptCount); ok && v.IsInteger() && v.IntPart() >= 0 && v.IntPart() < 1<<31 {
		p.ReceiptCount = pgtype.Int4{Int32: int32(v.IntPart()), Valid: true} //nolint:gosec // bounded above
	}
	if _, err := q.SetRetailSaleBreakdown(ctx, p); err != nil {
		return err
	}
	var lines []VatLine
	for _, z := range res.ZVatLines() {
		tax := z.Tax
		if tax.IsNegative() || tax.GreaterThan(z.Total) {
			tax = decimal.Zero
		}
		lines = append(lines, VatLine{Rate: z.Rate, Base: z.Total.Sub(tax), Tax: tax})
	}
	if len(lines) == 0 {
		return nil
	}
	return replaceVatLines(ctx, q, taxpayerID, saleID, lines)
}

// CheckZContinuity runs the Z number / day continuity check on every Z
// report of one ÖKC and records Z_NUMBER_GAP, Z_NUMBER_DUPLICATE and
// Z_DAY_GAP. It only adds information: failures are logged, never returned.
// It returns how many reports have a finding.
func (s *Service) CheckZContinuity(ctx context.Context, sc engagementctx.Scope, okc string) int {
	okc = strings.TrimSpace(okc)
	if okc == "" || s.warnings == nil {
		return 0
	}
	n := 0
	_ = database.BestEffort(ctx, "receipt_z_continuity", func(ctx context.Context) error {
		rows, err := s.q.ListRetailSalesForContinuity(ctx, db.ListRetailSalesForContinuityParams{TaxpayerID: sc.TaxpayerID, OkcSerialNumber: text(okc)})
		if err != nil {
			return err
		}
		days := make([]posting.ZDay, 0, len(rows))
		for _, r := range rows {
			days = append(days, posting.ZDay{ID: r.ID, Date: r.SaleDate.Time, Number: r.ZReportNumber.String})
		}
		found := posting.Continuity(days, posting.MaxGapDays)
		subj := engine.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID}
		for _, r := range rows {
			id := r.ID
			var findings []engine.Finding
			for _, f := range found[r.ID] {
				f.Params["okc_serial_number"] = okc
				findings = append(findings, engine.Finding{Code: f.Code, Params: f.Params})
			}
			if len(findings) > 0 {
				n++
			}
			if _, err := s.warnings.Apply(ctx, subj, ZContinuityProducer,
				engine.Entity{Type: catalog.EntityRetailSale, ID: &id, Period: r.Period.String}, posting.ZCodes, findings); err != nil {
				return err
			}
		}
		return nil
	})
	return n
}

// RunZContinuity checks every ÖKC of the taxpayer; it returns the number of
// reports with a finding.
func (s *Service) RunZContinuity(ctx context.Context, sc engagementctx.Scope) (int, error) {
	if s.warnings == nil {
		return 0, ErrNoWarnings
	}
	devs, err := s.q.ListRetailSaleDevices(ctx, sc.TaxpayerID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range devs {
		n += s.CheckZContinuity(ctx, sc, d.OkcSerialNumber)
	}
	return n, nil
}

// ErrNoWarnings: the warning engine is not wired.
var ErrNoWarnings = errors.New("receipts: warnings are not available")

// DecideRetailSale approves or rejects a pending Z report. Approval creates
// the sales journal draft (100 / 108 – 600 – 391 by VAT rate).
func (s *Service) DecideRetailSale(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, approve bool) (RetailSaleView, error) {
	r, err := s.q.GetRetailSaleByUUID(ctx, db.GetRetailSaleByUUIDParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RetailSaleView{}, ErrNotFound
	}
	if err != nil {
		return RetailSaleView{}, err
	}
	if r.ApprovalStatus != "pending" {
		return RetailSaleView{}, ErrState
	}
	if approve && (!r.SaleDate.Valid || !dec(r.TotalAmount).IsPositive()) {
		return RetailSaleView{}, ErrIncomplete
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return RetailSaleView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tctx := dbtx.WithTx(ctx, tx)
	q := s.q.WithTx(tx)
	status := map[bool]string{true: "approved", false: "rejected"}[approve]
	row, err := q.SetRetailSaleApproval(tctx, db.SetRetailSaleApprovalParams{ApprovalStatus: status, ApprovedBy: optID(userID), TaxpayerID: sc.TaxpayerID, ID: r.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return RetailSaleView{}, ErrState
	}
	if err != nil {
		return RetailSaleView{}, err
	}
	if err := q.SetCaptureStatusForRetailSale(tctx, db.SetCaptureStatusForRetailSaleParams{
		Status: map[bool]string{true: StatusApproved, false: StatusRejected}[approve], OrganizationID: sc.OrganizationID,
		EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, RetailSaleID: pgtype.Int8{Int64: r.ID, Valid: true},
	}); err != nil {
		return RetailSaleView{}, err
	}
	if approve {
		if err := s.postRetailSale(tctx, q, sc, row, userID); err != nil {
			return RetailSaleView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RetailSaleView{}, err
	}
	if !approve {
		// A rejected report leaves the continuity sequence.
		s.CheckZContinuity(ctx, sc, r.OkcSerialNumber.String)
	}
	return s.retailByID(ctx, sc, row)
}

// postRetailSale creates the sales journal draft of an approved Z report.
func (s *Service) postRetailSale(ctx context.Context, q *db.Queries, sc engagementctx.Scope, r db.RetailSale, userID int64) error {
	if _, err := q.UpsertReceiptPosting(ctx, db.UpsertReceiptPostingParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, TargetType: TargetRetailSale, TargetID: r.ID,
		AccountCode: text("600"), AccountSource: text(posting.SourceCategory), DecidedBy: optID(userID),
	}); err != nil {
		return err
	}
	if s.journals == nil || !r.SaleDate.Valid || !dec(r.TotalAmount).IsPositive() {
		return nil
	}
	lines, err := s.vatLines(ctx, sc.TaxpayerID, []int64{r.ID})
	if err != nil {
		return err
	}
	plan, err := s.plan(ctx, sc, "100", "108", "600", "391")
	if err != nil {
		return err
	}
	in := posting.ZInput{SourceID: "retail_sale:" + r.Uuid.String(), Date: r.SaleDate.Time, ZNumber: r.ZReportNumber.String,
		OKC: r.OkcSerialNumber.String, Total: dec(r.TotalAmount), Tax: dec(r.TaxAmount), Cash: decPtr(r.CashAmount),
		Card: decPtr(r.CardAmount), Plan: plan}
	for _, l := range lines[r.ID] {
		in.Lines = append(in.Lines, posting.ZVatLine{Rate: l.Rate, Base: l.Base, Tax: l.Tax})
	}
	entry, err := posting.ZEntry(in)
	if errors.Is(err, posting.ErrZBreakdown) {
		return fmt.Errorf("%w: the VAT breakdown does not add up to the Z total", ErrIncomplete)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIncomplete, err)
	}
	return s.writeJournal(ctx, q, sc, userID, TargetRetailSale, r.ID, entry, map[string]any{
		"source": "receipt", "target_type": TargetRetailSale, "retail_sale_uuid": r.Uuid.String(),
		"z_report_number": r.ZReportNumber.String, "okc_serial_number": r.OkcSerialNumber.String,
	})
}
