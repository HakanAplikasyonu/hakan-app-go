package journals

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Taslak fiş engelleri (#566). Engelli fiş onaylanmaz; kontrol ekranı
// her fişin engelini ve düzeltileceği yeri gösterir.
const (
	IssueUnbalanced     = "unbalanced"
	IssueUnknownAccount = "unknown_account"
	IssueParentAccount  = "parent_account"
	IssuePeriodLocked   = "period_locked"
	// Uyarılar (#669, ofis kararı): onay engellenmez ama açıkça sorulur.
	IssueMissingDocument   = "missing_document"
	IssuePossibleDuplicate = "possible_duplicate"
)

// confirmable keeps the warnings the approver must accept explicitly.
func confirmable(issues []DraftIssue) []DraftIssue {
	var out []DraftIssue
	for _, is := range issues {
		if is.Code == IssueMissingDocument || is.Code == IssuePossibleDuplicate {
			out = append(out, is)
		}
	}
	return out
}

// DraftNeedsConfirmationError: the draft has warnings (missing document,
// possible duplicate); the approval needs the explicit confirmation.
type DraftNeedsConfirmationError struct{ Issues []DraftIssue }

func (e *DraftNeedsConfirmationError) Error() string {
	return fmt.Sprintf("journals: draft has %d warning(s) to confirm", len(e.Issues))
}

// Confirmation is the approver's answer to the draft's warnings.
type Confirmation struct {
	// Confirmed: "eksik belge / mükerrer olabilir; yine de onaylıyorum".
	Confirmed bool
	// Note: optional reason, kept on the entry.
	Note string
}

// DraftIssue is a problem the control screen shows on a draft. Blocking
// ones (unbalanced, locked period) stop the approval; account code problems
// are warnings fixed here with "Hesabı düzelt" (the export's own check
// stops a code that is not in the program's plan).
type DraftIssue struct {
	Code     string `json:"code"`
	Blocking bool   `json:"blocking"`
	// LineNo and Account: the line with the wrong account code.
	LineNo  int32  `json:"line_no,omitempty"`
	Account string `json:"account,omitempty"`
	// Difference: debit minus credit of an unbalanced draft.
	Difference *decimal.Decimal `json:"difference,omitempty"`
}

// blocking keeps the issues that stop an approval.
func blocking(issues []DraftIssue) []DraftIssue {
	var out []DraftIssue
	for _, is := range issues {
		if is.Blocking {
			out = append(out, is)
		}
	}
	return out
}

// DraftBlockedError: the draft has blocking issues; approval is refused.
type DraftBlockedError struct{ Issues []DraftIssue }

func (e *DraftBlockedError) Error() string {
	return fmt.Sprintf("journals: draft has %d blocking issue(s)", len(e.Issues))
}

// draftIssues returns the blocking issues of the given drafts by id.
func (s *Service) draftIssues(ctx context.Context, scope engagementctx.Scope, ids []int64) (map[int64][]DraftIssue, error) {
	out := map[int64][]DraftIssue{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.JournalDraftIssues(ctx, db.JournalDraftIssuesParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, DraftIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("draft issues: %w", err)
	}
	for _, r := range rows {
		is := DraftIssue{Code: r.Code, LineNo: r.LineNo, Account: r.AccountCode,
			Blocking: r.Code == IssueUnbalanced || r.Code == IssuePeriodLocked}
		if r.Code == IssueUnbalanced {
			d := repository.Dec(r.Difference)
			is.Difference = &d
		}
		out[r.DraftID] = append(out[r.DraftID], is)
	}
	return out, nil
}

var accountCodeRe = regexp.MustCompile(`^[0-9]{3}(\.[0-9A-Za-z]+)*$`)

// SetLineAccount changes one line's account code of a draft (#566): the
// control screen's "Hesabı düzelt". Approved or exported entries are not
// changed (reverse them instead).
func (s *Service) SetLineAccount(ctx context.Context, scope engagementctx.Scope, id uuid.UUID, lineNo int32, account string) error {
	account = strings.TrimSpace(account)
	if !accountCodeRe.MatchString(account) {
		return fmt.Errorf("%w: account must be a dotted account code (e.g. 770.01)", ErrInvalid)
	}
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Status != "draft" {
		return fmt.Errorf("%w: status is %s", ErrInvalid, row.Status)
	}
	n, err := s.q.SetJournalDraftLineAccount(ctx, db.SetJournalDraftLineAccountParams{
		AccountCode: account, OrganizationID: scope.OrganizationID, EngagementID: scope.ID, DraftID: row.ID, LineNo: lineNo,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
