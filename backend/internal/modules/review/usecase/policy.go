package usecase

import "github.com/shopspring/decimal"

// Default policy when neither the office nor the engagement set one.
var (
	DefaultAutoAccept = true
	DefaultThreshold  = decimal.RequireFromString("0.95")
)

// AnyKind is the review_settings kind that applies to every kind.
const AnyKind = "*"

// Policy is the resolved auto-accept rule for one kind.
type Policy struct {
	AutoAccept bool            `json:"auto_accept"`
	Threshold  decimal.Decimal `json:"threshold"`
}

// Accepts reports whether confidence clears the policy.
func (p Policy) Accepts(confidence decimal.Decimal) bool {
	return p.AutoAccept && confidence.GreaterThanOrEqual(p.Threshold)
}

// ResolvePolicy merges rows field by field, most specific first:
// engagement+kind, engagement+*, office+kind, office+*, default.
func ResolvePolicy(rows []SettingRow, engagementID int64, kind string) Policy {
	pick := func(eng int64, k string) *SettingRow {
		for i := range rows {
			if rows[i].EngagementID == eng && rows[i].Kind == k {
				return &rows[i]
			}
		}
		return nil
	}
	var chain []*SettingRow
	if engagementID > 0 {
		chain = append(chain, pick(engagementID, kind), pick(engagementID, AnyKind))
	}
	chain = append(chain, pick(0, kind), pick(0, AnyKind))
	p := Policy{AutoAccept: DefaultAutoAccept, Threshold: DefaultThreshold}
	autoSet, thrSet := false, false
	for _, r := range chain {
		if r == nil {
			continue
		}
		if !autoSet && r.AutoAccept != nil {
			p.AutoAccept, autoSet = *r.AutoAccept, true
		}
		if !thrSet && r.Threshold != nil {
			p.Threshold, thrSet = *r.Threshold, true
		}
	}
	return p
}
