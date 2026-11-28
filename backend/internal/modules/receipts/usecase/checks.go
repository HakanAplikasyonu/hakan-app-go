package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Fiş yapay zekâ kontrolleri (Oğuz'un geri bildirimi): warnings raised while
// a receipt is read. The taxpayer sees them in the mobile capture preview
// (View.Checks), the office as a badge on the expense (ExpenseView.Checks).
// Each is a data_warnings code, so the office turns it off in its warning
// settings like any other.
const (
	CodeBuyerMismatch      = "RECEIPT_BUYER_MISMATCH"
	CodePlateNotRegistered = "PLATE_NOT_REGISTERED"
	CodePlateUnreadable    = "PLATE_UNREADABLE"
	// BuyerProducer raises RECEIPT_BUYER_MISMATCH.
	BuyerProducer = "control:receipt_buyer"
)

// ReceiptCheckCodes are the codes shown as a badge on an expense.
var ReceiptCheckCodes = []string{CodeBuyerMismatch, CodePlateNotRegistered, CodePlateUnreadable}

// BuyerMatch compares the buyer printed on a receipt with the taxpayer:
// "match", "mismatch" or "" (no buyer read, nothing to compare). The tax
// number decides when it was read; otherwise the name, which mismatches
// only when it shares no word with the taxpayer's title (company suffixes
// such as "LTD", "ŞTİ", "A.Ş." do not count).
func BuyerMatch(buyerTaxID, buyerName, taxID, title string) string {
	if buyerTaxID != "" {
		if strings.TrimSpace(taxID) == "" {
			return ""
		}
		if buyerTaxID == strings.TrimSpace(taxID) {
			return "match"
		}
		return "mismatch"
	}
	a, b := nameWords(buyerName), nameWords(title)
	if len(a) == 0 || len(b) == 0 {
		return ""
	}
	for w := range a {
		if b[w] {
			return "match"
		}
	}
	return "mismatch"
}

var companyWords = map[string]bool{
	"ltd": true, "sti": true, "a": true, "s": true, "as": true, "san": true, "tic": true, "ve": true,
	"limited": true, "sirketi": true, "anonim": true, "ticaret": true, "sanayi": true, "ins": true,
	"insaat": true, "ith": true, "ihr": true, "paz": true, "hiz": true, "hizmetleri": true, "ltdsti": true,
}

// nameWords are the meaningful words of a name, folded (ı→i, ş→s, …).
func nameWords(s string) map[string]bool {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(s)
	s = strings.ToLower(s)
	s = strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c").Replace(s)
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) {
		if len(w) >= 2 && !companyWords[w] {
			out[w] = true
		}
	}
	return out
}

// applyCheck records one producer's findings on an expense and reports
// whether the finding stays on (false when the office turned the code off).
func (s *Service) applyCheck(ctx context.Context, sc engagementctx.Scope, expenseID int64, period, producer string, owned []string, findings []engine.Finding) bool {
	if s.warnings == nil {
		return len(findings) > 0
	}
	id := expenseID
	shown := len(findings) > 0
	_ = database.BestEffort(ctx, "receipt_check_"+strings.TrimPrefix(producer, "control:"), func(ctx context.Context) error {
		rep, err := s.warnings.Apply(ctx, engine.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID},
			producer, engine.Entity{Type: catalog.EntityExpense, ID: &id, Period: period}, owned, findings)
		if err == nil && rep.Disabled > 0 {
			shown = false
		}
		return err
	})
	return shown
}

// checkBuyer raises or resolves RECEIPT_BUYER_MISMATCH; true when raised
// (and not turned off).
func (s *Service) checkBuyer(ctx context.Context, sc engagementctx.Scope, expenseID int64, res extract.Result, period string) bool {
	buyerID, buyerName := res.Get(extract.FieldBuyerTaxNumber), res.Get(extract.FieldBuyerName)
	if buyerID == "" && buyerName == "" {
		return false
	}
	var tp db.Taxpayer
	if err := database.BestEffort(ctx, "receipt_buyer_taxpayer", func(ctx context.Context) error {
		var err error
		tp, err = s.q.GetTaxpayerByID(ctx, sc.TaxpayerID)
		return err
	}); err != nil {
		return false
	}
	var findings []engine.Finding
	if BuyerMatch(buyerID, buyerName, tp.TaxID, tp.Title) == "mismatch" {
		findings = []engine.Finding{{Code: CodeBuyerMismatch, Params: map[string]any{"buyer_tax_number": buyerID, "buyer_name": buyerName}}}
	}
	return s.applyCheck(ctx, sc, expenseID, period, BuyerProducer, []string{CodeBuyerMismatch}, findings)
}

// checkPlate raises or resolves PLATE_NOT_REGISTERED (05 §3.3, T8) and,
// on a fuel receipt whose plate could not be read, PLATE_UNREADABLE. It
// returns registered | not_registered | no_plate | unreadable | error and
// whether a warning is shown.
func (s *Service) checkPlate(ctx context.Context, sc engagementctx.Scope, expenseID int64, res extract.Result, date time.Time, hasDate bool, period string) (string, bool) {
	owned := []string{CodePlateNotRegistered, CodePlateUnreadable}
	plate := res.Get(extract.FieldPlate)
	if plate == "" {
		if res.Kind != extract.KindFuel {
			return "no_plate", false
		}
		f := []engine.Finding{{Code: CodePlateUnreadable, Params: map[string]any{"date": res.Get(extract.FieldDate)}}}
		return "unreadable", s.applyCheck(ctx, sc, expenseID, period, PlateProducer, owned, f)
	}
	on := pgtype.Date{}
	if hasDate {
		on = pgtype.Date{Time: date, Valid: true}
	}
	// The check and its warning only add information: side effects in
	// savepoints, so a failure never aborts the upload (#311).
	var ok bool
	if err := database.BestEffort(ctx, "receipt_plate_check", func(ctx context.Context) error {
		var err error
		ok, err = s.q.VehicleRegisteredOn(ctx, db.VehicleRegisteredOnParams{TaxpayerID: sc.TaxpayerID, Plate: plate, OnDate: on})
		return err
	}); err != nil {
		return "error", false
	}
	if ok {
		s.applyCheck(ctx, sc, expenseID, period, PlateProducer, owned, nil)
		return "registered", false
	}
	f := []engine.Finding{{Code: CodePlateNotRegistered, Params: map[string]any{"plate": plate, "date": res.Get(extract.FieldDate)}}}
	return "not_registered", s.applyCheck(ctx, sc, expenseID, period, PlateProducer, owned, f)
}

// expenseChecks loads the open receipt-check codes of the given expenses.
func (s *Service) expenseChecks(ctx context.Context, sc engagementctx.Scope, ids []int64) map[int64][]string {
	out := map[int64][]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.q.ListExpenseReceiptChecks(ctx, db.ListExpenseReceiptChecksParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Ids: ids, Codes: ReceiptCheckCodes,
	})
	if err != nil {
		s.log.Warn("receipt_checks_list_failed", "error", err)
		return out
	}
	for _, r := range rows {
		if r.ExpenseID.Valid {
			out[r.ExpenseID.Int64] = r.Codes
		}
	}
	return out
}
