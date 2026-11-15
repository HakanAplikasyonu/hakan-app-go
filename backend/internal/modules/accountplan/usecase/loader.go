package usecase

import (
	"context"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// PlanLoader adapts the service to journals.PlanLoader (#26): the export
// batch gets the engagement's active Luca plan as TargetPlan, so the driver
// raises ACCOUNT_NOT_IN_TARGET_PLAN (blocking per BlockOnUnknownAccount).
// Drivers without a target plan (excel_formula) get nil: no check.
//
//	journals.New(pool, store).WithPlanLoader(accountplanSvc.PlanLoader())
func (s *Service) PlanLoader() func(ctx context.Context, scope engagementctx.Scope, driver string) (accountingexport.AccountPlan, error) {
	return func(ctx context.Context, scope engagementctx.Scope, driver string) (accountingexport.AccountPlan, error) {
		if driver != DefaultTarget {
			return nil, nil
		}
		set, err := s.CodeSet(ctx, Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}, driver)
		if err != nil || set == nil {
			return nil, err
		}
		return accountingexport.AccountPlan(set), nil
	}
}
