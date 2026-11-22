package docuploads

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Document types the office picks when uploading.
const (
	TypeInvoice       = "invoice"
	TypeZReport       = "z_report"
	TypeExpense       = "expense"
	TypeBankStatement = "bank_statement"
	TypeOther         = "other"
)

// Types are the accepted document types.
var Types = []string{TypeInvoice, TypeZReport, TypeExpense, TypeBankStatement, TypeOther}

// Where the period came from.
const (
	SourceDocumentDate = "document_date"
	SourceOCR          = "ocr"
	SourceCurrentMonth = "current_month"
	SourceManual       = "manual"
)

// Choices for a locked month: the same three as a change request for a
// locked period (reject, correction of the closed period, apply to another
// month).
const (
	ChoiceReject        = "reject"
	ChoiceCorrection    = "correction"
	ChoiceApplySelected = "apply_selected"
)

// Upload statuses.
const (
	StatusProcessed = "processed"
	StatusDuplicate = "duplicate"
	StatusFailed    = "failed"
	StatusRejected  = "rejected"
)

// Errors.
var (
	ErrInvalid = errors.New("docuploads: invalid input")
	// ErrPeriodLocked: the chosen month is locked and no choice was given.
	ErrPeriodLocked = errors.New("docuploads: the chosen month is locked")
	// ErrTargetLocked: "apply to the selected month" named a locked month.
	ErrTargetLocked = errors.New("docuploads: the selected month is locked too")
	// ErrNeedsImage: Z reports and expense receipts are read from a photo.
	ErrNeedsImage = errors.New("docuploads: Z reports and expense receipts need a photo")
	// ErrUnsupported: the file type does not fit the document type.
	ErrUnsupported = errors.New("docuploads: file type is not accepted for this document type")
	// ErrExists: the same file is already in the archive.
	ErrExists = errors.New("docuploads: the same file was uploaded before")
	// ErrDisabled: the office turned typed uploads off.
	ErrDisabled = errors.New("docuploads: the office turned typed uploads off")
)

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// istanbul is the month boundary (Türkiye).
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*3600)
}()

// SuggestPeriod is the month offered for an upload: the document date's
// month when one is known, else the current month in Türkiye.
func SuggestPeriod(docDate *time.Time, now time.Time) (period, source string) {
	if docDate != nil {
		return docDate.Format("2006-01"), SourceDocumentDate
	}
	return now.In(istanbul).Format("2006-01"), SourceCurrentMonth
}

// Plan is where an upload lands after the locked-month decision.
type Plan struct {
	// Period is the month the record is written to.
	Period string
	// LockChoice / LockedPeriod: set when the requested month was locked.
	LockChoice   string
	LockedPeriod string
	// Reject: the user chose not to take the document in.
	Reject bool
}

// Decide applies the locked-month choice. requested is the month the user
// picked; locked reports whether it is locked. For ChoiceApplySelected,
// target is the open month the document goes to and targetLocked whether
// that month is locked as well. A choice sent for an open month is ignored.
func Decide(requested string, locked bool, choice, target string, targetLocked bool) (Plan, error) {
	if !ValidPeriod(requested) {
		return Plan{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if !locked {
		return Plan{Period: requested}, nil
	}
	switch choice {
	case "":
		return Plan{}, ErrPeriodLocked
	case ChoiceReject:
		return Plan{Period: requested, LockChoice: choice, LockedPeriod: requested, Reject: true}, nil
	case ChoiceCorrection:
		return Plan{Period: requested, LockChoice: choice, LockedPeriod: requested}, nil
	case ChoiceApplySelected:
		if !ValidPeriod(target) {
			return Plan{}, fmt.Errorf("%w: target_period must be YYYY-MM", ErrInvalid)
		}
		if target == requested || targetLocked {
			return Plan{}, ErrTargetLocked
		}
		return Plan{Period: target, LockChoice: choice, LockedPeriod: requested}, nil
	default:
		return Plan{}, fmt.Errorf("%w: lock_choice must be reject, correction or apply_selected", ErrInvalid)
	}
}
