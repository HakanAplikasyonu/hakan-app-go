// Package inbox is the office's "Gelen kutusu" (#780 b): everything that
// waits for the accountant in one period, in one list, beside the AI
// suggestion queue (review). Six kinds of work are gathered:
//
//   - invoice: an active invoice of the period with no journal entry and no
//     pending AI suggestion (nobody has coded it yet);
//   - bank_transaction: a bank movement that is not matched, not ignored and
//     has no approved counter account posting;
//   - expense: a masraf fişi awaiting approval;
//   - retail_sale: a Z raporu awaiting approval;
//   - foreign_invoice: a foreign service invoice the taxpayer sent (upload,
//     WhatsApp, e-mail) that is still a draft (KDV2, #780 A2);
//   - late_document: a document that arrived for a closed period and waits
//     for the office's decision (#780 C). It is shown whatever the period,
//     since by definition it belongs to an earlier, closed month.
//
// Every list is bounded: each kind is capped in SQL and the merged list is
// paged. Nothing is written here; each row deep-links to the screen where it
// is handled, and the two kinds with an existing approve endpoint (expense,
// retail_sale) say so, so the UI can offer a one-click "Onayla".
package inbox

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Item types.
const (
	TypeInvoice         = "invoice"
	TypeBankTransaction = "bank_transaction"
	TypeExpense         = "expense"
	TypeRetailSale      = "retail_sale"
	TypeForeignInvoice  = "foreign_invoice"
	TypeLateDocument    = "late_document"
)

// Types lists every type in display order.
var Types = []string{TypeInvoice, TypeBankTransaction, TypeExpense, TypeRetailSale, TypeForeignInvoice, TypeLateDocument}

// Paging bounds.
const (
	DefaultLimit = 50
	MaxLimit     = 200
	// MaxOffset keeps the per-type cap (limit + offset) bounded.
	MaxOffset = 2000
)

// ErrInvalid: a bad filter.
var ErrInvalid = errors.New("inbox: invalid filter")

// ValidType reports whether t is "" (every type) or a known type.
func ValidType(t string) bool {
	if t == "" {
		return true
	}
	for _, v := range Types {
		if v == t {
			return true
		}
	}
	return false
}

// tabs is where each type is handled on the engagement page.
var tabs = map[string]string{
	TypeInvoice:         "invoices",
	TypeBankTransaction: "bank",
	TypeExpense:         "expenses",
	TypeRetailSale:      "z-reports",
	TypeForeignInvoice:  "foreign-invoices",
	TypeLateDocument:    "late-documents",
}

// Approvable reports whether a type has a one-click approve endpoint.
func Approvable(t string) bool { return t == TypeExpense || t == TypeRetailSale }

// Target is where a row is handled: the engagement tab and the record.
type Target struct {
	Tab string    `json:"tab"`
	ID  uuid.UUID `json:"id"`
}

// Item is one waiting piece of work. Empty strings are null.
type Item struct {
	Type           string    `json:"type"`
	UUID           uuid.UUID `json:"uuid"`
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	Amount         *string   `json:"amount"`
	Currency       string    `json:"currency"`
	Date           *string   `json:"date"`
	Description    string    `json:"description"`
	Reference      *string   `json:"reference"`
	// Detail: invoice direction (incoming / outgoing), bank side (A / B),
	// expense receipt type, foreign invoice source, late document
	// "<document_type>:<period>".
	Detail     *string `json:"detail"`
	Target     Target  `json:"target"`
	CanApprove bool    `json:"can_approve"`
}

// Counts are the waiting items by type.
type Counts struct {
	Invoice         int64 `json:"invoice"`
	BankTransaction int64 `json:"bank_transaction"`
	Expense         int64 `json:"expense"`
	RetailSale      int64 `json:"retail_sale"`
	ForeignInvoice  int64 `json:"foreign_invoice"`
	LateDocument    int64 `json:"late_document"`
	Total           int64 `json:"total"`
}

// List is the answer.
type List struct {
	Period     string  `json:"period"`
	Items      []Item  `json:"items"`
	Counts     Counts  `json:"counts"`
	Limit      int32   `json:"limit"`
	Offset     int32   `json:"offset"`
	NextCursor *string `json:"next_cursor"`
}

// Filter narrows the inbox.
type Filter struct {
	OrganizationID int64
	AllEngagements bool
	UserID         int64
	Period         string
	Type           string
	Engagement     *uuid.UUID
	Limit, Offset  int32
}

// Queries is the subset of db.Querier used here.
type Queries interface {
	ListInboxItems(ctx context.Context, arg db.ListInboxItemsParams) ([]db.ListInboxItemsRow, error)
	CountInboxItems(ctx context.Context, arg db.CountInboxItemsParams) (db.CountInboxItemsRow, error)
}

// Service reads the inbox.
type Service struct {
	q   Queries
	now func() time.Time
}

// New builds the service.
func New(q Queries) *Service { return &Service{q: q, now: time.Now} }

// Istanbul is the month boundary of "the current period".
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

// Clamp normalizes paging: a missing or too large limit falls back to the
// default / maximum, a negative offset to 0, a too large one to MaxOffset.
func Clamp(limit, offset int32) (int32, int32) {
	switch {
	case limit <= 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	if offset > MaxOffset {
		offset = MaxOffset
	}
	return limit, offset
}

// List answers the page and the counts.
func (s *Service) List(ctx context.Context, f Filter) (List, error) {
	if !ValidType(f.Type) {
		return List{}, ErrInvalid
	}
	if f.Period == "" {
		f.Period = s.now().In(Istanbul).Format("2006-01")
	}
	f.Limit, f.Offset = Clamp(f.Limit, f.Offset)
	var eng pgtype.UUID
	if f.Engagement != nil {
		eng = pgtype.UUID{Bytes: *f.Engagement, Valid: true}
	}
	// One more row than the page tells whether a next page exists.
	rows, err := s.q.ListInboxItems(ctx, db.ListInboxItemsParams{
		OrganizationID: f.OrganizationID,
		EngagementUuid: eng,
		AllEngagements: f.AllEngagements,
		UserID:         f.UserID,
		ItemType:       f.Type,
		Period:         f.Period,
		Cap:            f.Limit + f.Offset + 1,
		Lim:            f.Limit + 1,
		Off:            f.Offset,
	})
	if err != nil {
		return List{}, err
	}
	c, err := s.q.CountInboxItems(ctx, db.CountInboxItemsParams{
		OrganizationID: f.OrganizationID,
		EngagementUuid: eng,
		AllEngagements: f.AllEngagements,
		UserID:         f.UserID,
		Period:         f.Period,
	})
	if err != nil {
		return List{}, err
	}
	out := List{Period: f.Period, Items: make([]Item, 0, len(rows)), Limit: f.Limit, Offset: f.Offset, Counts: toCounts(c)}
	if len(rows) > int(f.Limit) {
		rows = rows[:f.Limit]
		if next := f.Offset + f.Limit; next <= MaxOffset {
			v := strconv.Itoa(int(next))
			out.NextCursor = &v
		}
	}
	for _, r := range rows {
		out.Items = append(out.Items, toItem(r))
	}
	return out, nil
}

func toCounts(c db.CountInboxItemsRow) Counts {
	return Counts{
		Invoice:         c.Invoice,
		BankTransaction: c.BankTransaction,
		Expense:         c.Expense,
		RetailSale:      c.RetailSale,
		ForeignInvoice:  c.ForeignInvoice,
		LateDocument:    c.LateDocument,
		Total:           c.Invoice + c.BankTransaction + c.Expense + c.RetailSale + c.ForeignInvoice + c.LateDocument,
	}
}

func toItem(r db.ListInboxItemsRow) Item {
	return Item{
		Type:           r.ItemType,
		UUID:           r.ItemUuid,
		EngagementUUID: r.EngagementUuid,
		TaxpayerTitle:  r.TaxpayerTitle,
		Amount:         nullable(r.Amount),
		Currency:       r.Currency,
		Date:           nullable(r.ItemDate),
		Description:    r.Description,
		Reference:      nullable(r.Reference),
		Detail:         nullable(r.Detail),
		Target:         Target{Tab: tabs[r.ItemType], ID: r.ItemUuid},
		CanApprove:     Approvable(r.ItemType),
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
