package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/posting"
)

// CategoryInput creates or replaces a category in a scope (office template
// or taxpayer). The same code in the scope is overwritten.
type CategoryInput struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	AccountCode    string   `json:"account_code"`
	VATAccountCode string   `json:"vat_account_code"`
	Deductible     string   `json:"deductible"`
	ReceiptTypes   []string `json:"receipt_types"`
	Keywords       []string `json:"keywords"`
	Active         *bool    `json:"is_active"`
}

var (
	categoryCodeRe = regexp.MustCompile(`^[a-z0-9_]{1,48}$`)
	receiptTypes   = map[string]bool{"okc_receipt": true, "fuel_receipt": true, "expense_voucher": true, "smm": true,
		"bank_fee": true, "salary": true, "tax_payment": true, "other": true}
)

// categories returns the effective categories of an engagement (0: the
// office template over the platform defaults).
func (s *Service) categories(ctx context.Context, orgID, engagementID int64) ([]posting.Category, error) {
	rows, err := s.q.ListExpenseCategories(ctx, db.ListExpenseCategoriesParams{OrganizationID: orgID, EngagementID: optID(engagementID)})
	if err != nil {
		return nil, err
	}
	var office, own []posting.Category
	for _, r := range rows {
		c := categoryView(r)
		if r.EngagementID.Valid {
			own = append(own, c)
		} else {
			office = append(office, c)
		}
	}
	return posting.Merge(posting.DefaultCategories, office, own), nil
}

func categoryView(r db.ExpenseCategory) posting.Category {
	id := r.Uuid
	scope := posting.ScopeOffice
	if r.EngagementID.Valid {
		scope = posting.ScopeTaxpayer
	}
	return posting.Category{UUID: &id, Code: r.Code, Name: r.Name, AccountCode: r.AccountCode, VATAccountCode: r.VatAccountCode,
		Deductible: r.Deductible, ReceiptTypes: nonNilStrings(r.ReceiptTypes), Keywords: nonNilStrings(r.Keywords),
		Active: r.IsActive, Scope: scope}
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Categories lists the effective categories (engagementID 0: office template).
func (s *Service) Categories(ctx context.Context, orgID, engagementID int64) ([]posting.Category, error) {
	return s.categories(ctx, orgID, engagementID)
}

// SaveCategory writes a category in the office template (engagementID 0)
// or for one taxpayer.
func (s *Service) SaveCategory(ctx context.Context, orgID, engagementID, userID int64, in CategoryInput) (posting.Category, error) {
	in.Code = strings.TrimSpace(strings.ToLower(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.AccountCode = strings.TrimSpace(in.AccountCode)
	in.VATAccountCode = strings.TrimSpace(in.VATAccountCode)
	if in.VATAccountCode == "" {
		in.VATAccountCode = "191"
	}
	if in.Deductible == "" {
		in.Deductible = "yes"
	}
	switch {
	case !categoryCodeRe.MatchString(in.Code):
		return posting.Category{}, fmt.Errorf("%w: code must be 1-48 of a-z, 0-9, _", ErrInvalid)
	case in.Name == "" || len(in.Name) > 120:
		return posting.Category{}, fmt.Errorf("%w: name is required (max 120)", ErrInvalid)
	case !accountRe.MatchString(in.AccountCode) || !accountRe.MatchString(in.VATAccountCode):
		return posting.Category{}, fmt.Errorf("%w: account codes must look like 770 or 770.01.001", ErrInvalid)
	case in.Deductible != "yes" && in.Deductible != "no":
		return posting.Category{}, fmt.Errorf("%w: deductible is yes or no", ErrInvalid)
	}
	for _, t := range in.ReceiptTypes {
		if !receiptTypes[t] {
			return posting.Category{}, fmt.Errorf("%w: receipt type %q", ErrInvalid, t)
		}
	}
	kw := make([]string, 0, len(in.Keywords))
	for _, k := range in.Keywords {
		if k = strings.TrimSpace(k); k != "" && len(kw) < 30 {
			kw = append(kw, k)
		}
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	r, err := s.q.UpsertExpenseCategory(ctx, db.UpsertExpenseCategoryParams{
		OrganizationID: orgID, EngagementID: optID(engagementID), Code: in.Code, Name: in.Name,
		AccountCode: in.AccountCode, VatAccountCode: in.VATAccountCode, Deductible: in.Deductible,
		ReceiptTypes: nonNilStrings(in.ReceiptTypes), Keywords: kw, IsActive: active, CreatedBy: optID(userID),
	})
	if err != nil {
		return posting.Category{}, err
	}
	return categoryView(r), nil
}

// DeleteCategory removes an office or taxpayer category row; the layer
// below (office template / platform default) shows again. engagementID 0
// may delete only template rows, otherwise only that taxpayer's rows.
func (s *Service) DeleteCategory(ctx context.Context, orgID, engagementID int64, id uuid.UUID) error {
	r, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if r.EngagementID.Valid != (engagementID > 0) || (engagementID > 0 && r.EngagementID.Int64 != engagementID) {
		return ErrNotFound
	}
	_, err = s.q.DeleteExpenseCategory(ctx, db.DeleteExpenseCategoryParams{OrganizationID: orgID, Uuid: id})
	return err
}
