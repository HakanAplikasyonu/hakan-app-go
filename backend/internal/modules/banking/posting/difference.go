package posting

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Reasons why a bank account's statement balance differs from its trial
// balance (102.xx) balance (#780 C). Each is signed like the difference:
// positive when the statement is higher than the ledger.
const (
	// ReasonOpening: the balance before the first imported movement of the
	// year differs from the ledger's opening (açılış fişi and lines dated
	// before that movement).
	ReasonOpening = "opening"
	// ReasonUnbooked: movements on the statement that are in no journal
	// entry yet.
	ReasonUnbooked = "unbooked"
	// ReasonOtherAccount: movements booked to another bank account's 102
	// code, and other accounts' movements booked to this one.
	ReasonOtherAccount = "other_account"
	// ReasonManualLines: lines on this 102 code that come from no bank
	// movement (manual entries).
	ReasonManualLines = "manual_lines"
	// ReasonGaps: days missing between two statements (the next opening
	// balance does not continue the previous closing balance).
	ReasonGaps = "gaps"
	// ReasonDuplicates: movements the bank did not count (a possible
	// duplicate or a cancelled one) that are booked anyway.
	ReasonDuplicates = "duplicates"
	// ReasonUnexplained: what remains after the reasons above.
	ReasonUnexplained = "unexplained"
)

// DifferenceReasons is every reason in display order.
var DifferenceReasons = []string{ReasonOpening, ReasonUnbooked, ReasonOtherAccount, ReasonManualLines,
	ReasonGaps, ReasonDuplicates, ReasonUnexplained}

// CorrectionSourcePrefix marks a correction entry's bank line
// (journal_draft_lines.source_id = "bankfark:<reason>", source_kind manual):
// it is counted as the correction of that reason, not as a manual line.
const CorrectionSourcePrefix = "bankfark:"

// maxRecords caps the records listed under one reason.
const maxRecords = 50

// diffBox is a bank account and its 102 code.
type diffBox struct {
	ID   int64
	UUID uuid.UUID
	Name string
	IBAN string
	Code string
}

// diffTx is a TRY bank movement of the year up to the period.
type diffTx struct {
	ID          int64
	UUID        uuid.UUID
	Box         int64
	Date        time.Time
	Period      string
	Signed      decimal.Decimal // in +, out −
	Before      *decimal.Decimal
	After       *decimal.Decimal
	Cancelled   bool
	Flagged     bool
	Partner     int64
	Description string
}

// diffLine is a journal line on a 102 code (drafts included).
type diffLine struct {
	DraftUUID   uuid.UUID
	EntryNo     int32
	Period      string
	EntryType   string
	Date        time.Time
	LineNo      int32
	Code        string
	Description string
	Signed      decimal.Decimal // debit − credit
	SourceKind  string
	SourceID    string
}

// DifferenceRecord is one record behind a reason, linked from the screen.
type DifferenceRecord struct {
	// Kind: bank_transaction | journal_line | gap.
	Kind            string     `json:"kind"`
	Date            string     `json:"date"`
	Period          string     `json:"period"`
	Amount          string     `json:"amount"`
	Description     string     `json:"description"`
	TransactionUUID *uuid.UUID `json:"transaction_uuid,omitempty"`
	DraftUUID       *uuid.UUID `json:"draft_uuid,omitempty"`
	EntryNo         int32      `json:"entry_no,omitempty"`
	AccountCode     string     `json:"account_code,omitempty"`
	// ToDate: a gap's last day (Date is its first).
	ToDate string `json:"to_date,omitempty"`
}

// DifferenceResolution is the office's decision on a reason: explained,
// or a correction draft booked. Who and when are kept.
type DifferenceResolution struct {
	UUID           uuid.UUID  `json:"uuid"`
	Action         string     `json:"action"`
	Amount         *string    `json:"amount"`
	CounterAccount *string    `json:"counter_account"`
	Explanation    string     `json:"explanation"`
	DraftUUID      *uuid.UUID `json:"journal_draft_uuid"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

// DifferenceReason is one reason's share of the difference.
type DifferenceReason struct {
	Reason string `json:"reason"`
	Amount string `json:"amount"`
	// Corrected: the correction entries booked for this reason (signed
	// like Amount).
	Corrected string `json:"corrected"`
	// Open: what is still to look at: Amount − Corrected, zero once
	// explained.
	Open        string                 `json:"open"`
	Explained   *DifferenceResolution  `json:"explained"`
	Corrections []DifferenceResolution `json:"corrections"`
	Records     []DifferenceRecord     `json:"records"`
	// More: records left out of the list (over the cap).
	More int `json:"more"`
}

// DifferenceAccount is one bank account's statement vs ledger comparison.
type DifferenceAccount struct {
	CashBoxUUID uuid.UUID `json:"cash_box_uuid"`
	Name        string    `json:"name"`
	IBAN        string    `json:"iban"`
	AccountCode string    `json:"account_code"`
	// SharedCode: another bank account has the same 102 code; the ledger
	// cannot tell them apart.
	SharedCode bool `json:"shared_code"`
	// StatementBalance: the bank's balance at the end of the period (last
	// movement with a balance, later movements added); null when no
	// statement carries a balance.
	StatementBalance *string            `json:"statement_balance"`
	StatementDate    string             `json:"statement_date"`
	LedgerBalance    string             `json:"ledger_balance"`
	Difference       *string            `json:"difference"`
	Balanced         bool               `json:"balanced"`
	OpenTotal        string             `json:"open_total"`
	Reasons          []DifferenceReason `json:"reasons"`
}

// normalizeLedgerCode turns "102 01" / "102-01" into "102.01".
func normalizeLedgerCode(code string) string {
	f := strings.FieldsFunc(strings.TrimSpace(code), func(r rune) bool { return r == ' ' || r == '.' || r == '-' })
	return strings.Join(f, ".")
}

func codeUnder(code, parent string) bool {
	return code == parent || strings.HasPrefix(code, parent+".")
}

// lineOwners: the boxes whose 102 code is the most specific one the line's
// code falls under.
func lineOwners(code string, boxes []diffBox) []int64 {
	best := -1
	var out []int64
	for _, b := range boxes {
		if !codeUnder(code, b.Code) {
			continue
		}
		switch n := len(b.Code); {
		case n > best:
			best, out = n, []int64{b.ID}
		case n == best:
			out = append(out, b.ID)
		}
	}
	return out
}

type reasonAcc struct {
	amount  decimal.Decimal
	records []DifferenceRecord
	more    int
}

func (a *reasonAcc) add(d decimal.Decimal, rec DifferenceRecord) {
	a.amount = a.amount.Add(d)
	if len(a.records) < maxRecords {
		a.records = append(a.records, rec)
	} else {
		a.more++
	}
}

func txRecord(t diffTx, amount decimal.Decimal, code string) DifferenceRecord {
	id := t.UUID
	return DifferenceRecord{Kind: "bank_transaction", Date: t.Date.Format(time.DateOnly), Period: t.Period,
		Amount: amount.StringFixed(2), Description: t.Description, TransactionUUID: &id, AccountCode: code}
}

func lineRecord(l diffLine, amount decimal.Decimal) DifferenceRecord {
	id := l.DraftUUID
	return DifferenceRecord{Kind: "journal_line", Date: l.Date.Format(time.DateOnly), Period: l.Period,
		Amount: amount.StringFixed(2), Description: l.Description, DraftUUID: &id, EntryNo: l.EntryNo, AccountCode: l.Code}
}

func contains(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// computeDifference compares one bank account's statement balance at the
// end of the period with its 102 balance (January to the period) and
// breaks the difference down by reason. txs are every bank account's
// movements of the range (ordered by box, date, bank order), lines every
// 102 line of the range; resolutions are this account's decisions.
//
// With S the statement and L the ledger:
//
//	S − L = (opening − ledger opening) + unbooked + gaps − booked duplicates
//	        + (movements booked elsewhere − others' movements booked here)
//	        − manual lines − corrections + unexplained
//
// so every reason is signed like the difference, and unexplained is what
// the records do not account for.
func computeDifference(box diffBox, boxes []diffBox, txs []diffTx, lines []diffLine, resolutions map[string][]DifferenceResolution) DifferenceAccount {
	out := DifferenceAccount{CashBoxUUID: box.UUID, Name: box.Name, IBAN: box.IBAN, AccountCode: box.Code, Reasons: []DifferenceReason{}}
	for _, b := range boxes {
		if b.ID != box.ID && b.Code == box.Code {
			out.SharedCode = true
		}
	}
	byID := make(map[int64]diffTx, len(txs))
	partner := map[int64]int64{}
	for _, t := range txs {
		byID[t.ID] = t
		if t.Partner > 0 {
			partner[t.ID] = t.Partner
			if _, ok := partner[t.Partner]; !ok {
				partner[t.Partner] = t.ID
			}
		}
	}

	// Statement: the chain of this account's counted movements.
	var own []diffTx
	for _, t := range txs {
		if t.Box == box.ID {
			own = append(own, t)
		}
	}
	acc := map[string]*reasonAcc{}
	for _, r := range DifferenceReasons {
		acc[r] = &reasonAcc{}
	}
	var opening, prevAfter *decimal.Decimal
	var firstDate, prevDate, lastDate time.Time
	pre, pending := decimal.Zero, decimal.Zero
	for _, t := range own {
		if t.Cancelled || t.Flagged {
			continue
		}
		if firstDate.IsZero() {
			firstDate = t.Date
		}
		lastDate = t.Date
		if t.After == nil {
			if prevAfter == nil {
				pre = pre.Add(t.Signed)
			} else {
				pending = pending.Add(t.Signed)
			}
			continue
		}
		before := t.After.Sub(t.Signed)
		if t.Before != nil {
			before = *t.Before
		}
		if prevAfter == nil {
			o := before.Sub(pre)
			opening = &o
		} else if jump := before.Sub(prevAfter.Add(pending)); !jump.Round(2).IsZero() {
			acc[ReasonGaps].add(jump, DifferenceRecord{Kind: "gap", Date: prevDate.Format(time.DateOnly),
				ToDate: t.Date.Format(time.DateOnly), Period: t.Period, Amount: jump.StringFixed(2)})
		}
		after := *t.After
		prevAfter, prevDate, pending = &after, t.Date, decimal.Zero
	}

	// Ledger: this account's lines, classified.
	ledger, ledgerOpening := decimal.Zero, decimal.Zero
	onCode := map[int64]decimal.Decimal{}
	corrected := map[string]decimal.Decimal{}
	elsewhere := map[int64]decimal.Decimal{}
	elsewhereCode := map[int64]string{}
	for _, l := range lines {
		owners := lineOwners(l.Code, boxes)
		mine := contains(owners, box.ID)
		if !mine {
			// An own movement booked to another 102 code (not the transfer
			// partner's account, which is the expected counter line).
			if l.SourceKind != DraftSourceKind {
				continue
			}
			id, err := strconv.ParseInt(l.SourceID, 10, 64)
			if err != nil {
				continue
			}
			t, ok := byID[id]
			if !ok || t.Box != box.ID {
				continue
			}
			if p, ok := byID[partner[id]]; ok && contains(owners, p.Box) {
				continue
			}
			elsewhere[id] = elsewhere[id].Add(l.Signed)
			elsewhereCode[id] = l.Code
			continue
		}
		ledger = ledger.Add(l.Signed)
		switch {
		case l.SourceKind == "manual" && strings.HasPrefix(l.SourceID, CorrectionSourcePrefix):
			r := strings.TrimPrefix(l.SourceID, CorrectionSourcePrefix)
			if _, ok := acc[r]; !ok {
				r = ReasonUnexplained
			}
			corrected[r] = corrected[r].Add(l.Signed)
		case l.EntryType == "acilis" || (!firstDate.IsZero() && l.Date.Before(firstDate)):
			ledgerOpening = ledgerOpening.Add(l.Signed)
		case l.SourceKind == DraftSourceKind:
			id, err := strconv.ParseInt(l.SourceID, 10, 64)
			t, ok := byID[id]
			switch {
			case err != nil || !ok:
				acc[ReasonManualLines].add(l.Signed.Neg(), lineRecord(l, l.Signed.Neg()))
			case t.Box == box.ID:
				onCode[t.ID] = onCode[t.ID].Add(l.Signed)
			default:
				if p, ok := byID[partner[t.ID]]; ok && p.Box == box.ID {
					onCode[p.ID] = onCode[p.ID].Add(l.Signed)
				} else {
					acc[ReasonOtherAccount].add(l.Signed.Neg(), lineRecord(l, l.Signed.Neg()))
				}
			}
		default:
			acc[ReasonManualLines].add(l.Signed.Neg(), lineRecord(l, l.Signed.Neg()))
		}
	}
	out.LedgerBalance = ledger.StringFixed(2)

	// Movements: booked, booked elsewhere, not booked, booked duplicates.
	for _, t := range own {
		booked := onCode[t.ID]
		if t.Cancelled || t.Flagged {
			if !booked.Round(2).IsZero() {
				acc[ReasonDuplicates].add(booked.Neg(), txRecord(t, booked.Neg(), box.Code))
			}
			continue
		}
		residual := t.Signed.Sub(booked)
		if residual.Round(2).IsZero() {
			continue
		}
		switch {
		case !elsewhere[t.ID].IsZero():
			acc[ReasonOtherAccount].add(residual, txRecord(t, residual, elsewhereCode[t.ID]))
		case booked.IsZero():
			acc[ReasonUnbooked].add(residual, txRecord(t, residual, ""))
		}
	}

	if opening == nil {
		return out // no statement balance: nothing to compare
	}
	closing := prevAfter.Add(pending)
	diff := closing.Sub(ledger)
	cs, ds := closing.StringFixed(2), diff.StringFixed(2)
	out.StatementBalance, out.Difference = &cs, &ds
	out.StatementDate = lastDate.Format(time.DateOnly)
	out.Balanced = diff.Round(2).IsZero()
	if o := opening.Sub(ledgerOpening); !o.Round(2).IsZero() {
		acc[ReasonOpening].amount = o
	}
	explained := diff
	for _, c := range corrected {
		explained = explained.Add(c)
	}
	for _, r := range DifferenceReasons[:len(DifferenceReasons)-1] {
		explained = explained.Sub(acc[r].amount)
	}
	acc[ReasonUnexplained].amount = explained

	open := decimal.Zero
	for _, r := range DifferenceReasons {
		a := acc[r]
		c := corrected[r]
		res := resolutions[r]
		if a.amount.Round(2).IsZero() && c.Round(2).IsZero() && len(res) == 0 {
			continue
		}
		reason := DifferenceReason{Reason: r, Amount: a.amount.StringFixed(2), Corrected: c.StringFixed(2),
			Records: a.records, More: a.more, Corrections: []DifferenceResolution{}}
		if reason.Records == nil {
			reason.Records = []DifferenceRecord{}
		}
		for i := range res {
			if res[i].Action == ActionExplained {
				reason.Explained = &res[i]
			} else {
				reason.Corrections = append(reason.Corrections, res[i])
			}
		}
		left := a.amount.Sub(c)
		if reason.Explained != nil {
			left = decimal.Zero
		}
		reason.Open = left.StringFixed(2)
		open = open.Add(left)
		out.Reasons = append(out.Reasons, reason)
	}
	sort.SliceStable(out.Reasons, func(i, j int) bool {
		return reasonOrder(out.Reasons[i].Reason) < reasonOrder(out.Reasons[j].Reason)
	})
	out.OpenTotal = open.StringFixed(2)
	return out
}

func reasonOrder(r string) int {
	for i, x := range DifferenceReasons {
		if x == r {
			return i
		}
	}
	return len(DifferenceReasons)
}

// Resolution actions.
const (
	ActionExplained = "explained"
	ActionCorrected = "corrected"
)

// ValidReason reports whether r is a difference reason.
func ValidReason(r string) bool { return reasonOrder(r) < len(DifferenceReasons) }
