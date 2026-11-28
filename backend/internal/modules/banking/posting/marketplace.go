package posting

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Marketplace payouts (banking.Marketplaces): an incoming movement whose
// description names a sales channel is proposed to that channel's cari
// sub-account (120.xx named after the channel or its company). Without one
// the bare main account is proposed with the "pazaryeri carisi tanımlı
// değil" warning, below the review threshold.

// marketplaceReasonPrefix starts every marketplace reason; List reads it
// back to tell the screen which note to translate.
const marketplaceReasonPrefix = "Açıklamada '"

var (
	marketplaceFound   = decimal.RequireFromString("0.80")
	marketplaceMissing = decimal.RequireFromString("0.40")
)

// cariPlan loads the active plan's accounts under main.
func cariPlan(ctx context.Context, q *db.Queries, sub Subject, main string) ([]engine.PlanAccount, error) {
	target, err := q.ActiveAccountPlanTarget(ctx, db.ActiveAccountPlanTargetParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID})
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPlanAccountsForMains(ctx, db.ListPlanAccountsForMainsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Target: target, Mains: []string{main},
	})
	if err != nil {
		return nil, err
	}
	out := make([]engine.PlanAccount, 0, len(rows))
	for _, r := range rows {
		out = append(out, engine.PlanAccount{Code: r.Code, Name: r.Name, IsLeaf: r.IsLeaf})
	}
	return out, nil
}

// MarketplaceAccount is the first leaf under main (in code order) whose
// name names the channel.
func MarketplaceAccount(mp banking.Marketplace, main string, plan []engine.PlanAccount) (engine.PlanAccount, bool) {
	for _, a := range plan {
		if a.IsLeaf && strings.HasPrefix(a.Code, main+".") && mp.NameMatches(a.Name) {
			return a, true
		}
	}
	return engine.PlanAccount{}, false
}

// marketplaceReason: "Açıklamada 'TRENDYOL' geçiyor → Trendyol carisi
// önerildi: 120.01.005 TRENDYOL".
func marketplaceReason(hit banking.MarketplaceHit, main string, acc *engine.PlanAccount) string {
	r := fmt.Sprintf("%s%s' geçiyor", marketplaceReasonPrefix, hit.Keyword)
	if hit.Temlik {
		r = fmt.Sprintf("%s%s' ve '%s' geçiyor (temlik)", marketplaceReasonPrefix, banking.TemlikPattern, hit.Keyword)
	}
	if acc == nil {
		return fmt.Sprintf("%s → %s; pazaryeri carisi tanımlı değil, %s altında %s alt hesabı açılmalı",
			r, hit.Marketplace.Name, main, hit.Marketplace.Name)
	}
	return fmt.Sprintf("%s → %s carisi önerildi: %s %s", r, hit.Marketplace.Name, acc.Code, acc.Name)
}

// MarketplaceSuggestion proposes the channel's cari under main, or main
// itself with the missing-account warning.
func MarketplaceSuggestion(hit banking.MarketplaceHit, main string, plan []engine.PlanAccount) Suggestion {
	if a, ok := MarketplaceAccount(hit.Marketplace, main, plan); ok {
		return Suggestion{AccountCode: a.Code, AccountName: a.Name, Confidence: marketplaceFound,
			Source: SourceCounterpartyAccount, Reason: marketplaceReason(hit, main, &a), Marketplace: hit.Marketplace.Key}
	}
	return Suggestion{AccountCode: main, AccountName: hit.Marketplace.Name, Confidence: marketplaceMissing,
		Source: SourceKindDefault, Reason: marketplaceReason(hit, main, nil), Marketplace: hit.Marketplace.Key}
}

// MarketplaceNote is the screen's translatable form of a marketplace
// suggestion.
type MarketplaceNote struct {
	Channel        string `json:"channel"`
	Name           string `json:"name"`
	Keyword        string `json:"keyword"`
	Temlik         bool   `json:"temlik"`
	AccountMissing bool   `json:"account_missing"`
}

// marketplaceNote rebuilds the note of a stored suggestion; nil when the
// suggestion did not come from the marketplace table.
func marketplaceNote(source, reason, description, account string) *MarketplaceNote {
	if (source != SourceCounterpartyAccount && source != SourceKindDefault) || !strings.HasPrefix(reason, marketplaceReasonPrefix) {
		return nil
	}
	hit, ok := banking.DetectMarketplace(description)
	if !ok {
		return nil
	}
	return &MarketplaceNote{Channel: hit.Marketplace.Key, Name: hit.Marketplace.Name, Keyword: hit.Keyword,
		Temlik: hit.Temlik, AccountMissing: !strings.Contains(account, ".")}
}
