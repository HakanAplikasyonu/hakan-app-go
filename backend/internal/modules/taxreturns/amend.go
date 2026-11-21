package taxreturns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Düzeltme beyannamesi (#575). Ofis kararı: verilmiş dönemde satış
// (fatura, Z raporu) değişirse düzeltme beyannamesi verilir.
//
//   - Verilmiş beyanın güncel verisi verildiği halinden farklıysa önizleme
//     "düzeltme gerekebilir" der (needed) ve farkları gösterir.
//   - POST .../amend verilmiş hali sürüm olarak arşivler, beyanı taslağa
//     döndürür (sürüm + 1). Taslak / hazırlanmış düzeltme beyannamesi
//     önceki sürümle farkını gösterir; akış (hazırla → verildi) aynıdır.

// AmendmentFigure is one key figure of the return, previous vs current.
type AmendmentFigure struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Previous   Money  `json:"previous"`
	Current    Money  `json:"current"`
	Difference Money  `json:"difference"`
}

// AmendmentVersion is an archived (filed) version.
type AmendmentVersion struct {
	Version       int32     `json:"version"`
	FiledOn       string    `json:"filed_on"`
	AccrualNumber *string   `json:"accrual_number"`
	PayableAmount *Money    `json:"payable_amount"`
	CarryOut      *Money    `json:"carry_out"`
	PaidOn        *string   `json:"paid_on"`
	AmendedAt     time.Time `json:"amended_at"`
}

// Amendment is the version state of a return: the current version, the
// archived ones, and the key figures against the previous one (or, for a
// filed return, the live data against what was filed).
type Amendment struct {
	Version int32 `json:"version"`
	// Needed: filed, and the live data now differs from what was filed.
	Needed bool `json:"needed"`
	// Previous: the archived version compared against; nil when the live
	// data is compared with the filed return (needed) or there is none.
	Previous    *AmendmentVersion  `json:"previous"`
	Differences []AmendmentFigure  `json:"differences"`
	Versions    []AmendmentVersion `json:"versions"`
}

type figure struct {
	key, label string
	value      decimal.Decimal
}

// figures are the return's key figures compared between versions.
func figures(kind string, b body) []figure {
	switch {
	case kind == KindKDV1 && b.KDV1 != nil:
		t := b.KDV1.Totals
		return []figure{
			{"base_total", "Matrah toplamı", t.BaseTotal.Dec()},
			{"calculated_vat", "Hesaplanan KDV", t.CalculatedVAT.Dec()},
			{"total_vat", "Toplam KDV", t.TotalVAT.Dec()},
			{"total_deductions", "İndirimler toplamı", t.TotalDeductions.Dec()},
			{"payable", "Ödenecek KDV", t.Payable.Dec()},
			{"carry_out", "Sonraki döneme devreden KDV", t.CarryOut.Dec()},
		}
	case kind == KindMUHSGK && b.MUHSGK != nil:
		t := b.MUHSGK.Totals
		return []figure{
			{"income_tax", "Gelir vergisi kesintisi", t.IncomeTax.Dec()},
			{"stamp_tax", "Damga vergisi", t.StampTax.Dec()},
			{"tax_payable", "Ödenecek vergi", t.TaxPayable.Dec()},
			{"premium_payable", "Ödenecek SGK primi", t.PremiumPayable.Dec()},
		}
	case kind == KindBABS && b.BABS != nil:
		return []figure{
			{"ba_amount", "Form Ba tutarı", b.BABS.BA.Amount.Dec()},
			{"ba_count", "Form Ba bildirilen kişi", decimal.NewFromInt(int64(len(b.BABS.BA.Rows)))},
			{"bs_amount", "Form Bs tutarı", b.BABS.BS.Amount.Dec()},
			{"bs_count", "Form Bs bildirilen kişi", decimal.NewFromInt(int64(len(b.BABS.BS.Rows)))},
		}
	}
	return nil
}

// compareFigures lists the key figures of prev and cur; changed reports a
// difference over half a kuruş.
func compareFigures(kind string, prev, cur body) (out []AmendmentFigure, changed bool) {
	p, c := figures(kind, prev), figures(kind, cur)
	out = []AmendmentFigure{}
	for i := range c {
		var pv decimal.Decimal
		if i < len(p) {
			pv = p[i].value
		}
		d := c[i].value.Sub(pv)
		if !M(d).Dec().IsZero() {
			changed = true
		}
		out = append(out, AmendmentFigure{Key: c[i].key, Label: c[i].label, Previous: M(pv), Current: M(c[i].value), Difference: M(d)})
	}
	return out, changed
}

// amendment fills the version state; nil for a first version that is not
// filed yet.
func (s *Service) amendment(ctx context.Context, sc engagementctx.Scope, kind, period string, rec *db.TaxReturn, cur body) (*Amendment, error) {
	if rec == nil || (rec.Version <= 1 && rec.Status != StatusFiled) {
		return nil, nil
	}
	rows, err := s.q.ListTaxReturnVersions(ctx, db.ListTaxReturnVersionsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxReturnID: rec.ID})
	if err != nil {
		return nil, fmt.Errorf("tax return versions: %w", err)
	}
	a := &Amendment{Version: rec.Version, Differences: []AmendmentFigure{}, Versions: []AmendmentVersion{}}
	for _, v := range rows {
		a.Versions = append(a.Versions, AmendmentVersion{Version: v.Version, FiledOn: dateStr(v.FiledOn), AccrualNumber: textPtr(v.AccrualNumber),
			PayableAmount: moneyPtr(v.PayableAmount), CarryOut: moneyPtr(v.CarryOut), PaidOn: datePtr(v.PaidOn), AmendedAt: v.AmendedAt.Time})
	}
	if rec.Status == StatusFiled {
		// Verildikten sonra veri değişti mi: güncel hesap ile verilmiş hal.
		live, _, err := s.compute(ctx, sc, kind, period, rec.Adjustments, acksOf(rec.Acknowledged))
		if err != nil {
			return nil, err
		}
		a.Differences, a.Needed = compareFigures(kind, cur, live)
		if a.Needed || len(rows) == 0 {
			return a, nil
		}
		// Verilmiş düzeltme, veri değişmedi: önceki sürümle fark.
	}
	if len(rows) == 0 {
		return a, nil
	}
	var prev body
	if err := json.Unmarshal(rows[0].Snapshot, &prev); err != nil {
		return nil, fmt.Errorf("tax return version snapshot: %w", err)
	}
	a.Previous = &a.Versions[0]
	a.Differences, _ = compareFigures(kind, prev, cur)
	return a, nil
}

// Amend reopens a filed return as a correction (düzeltme beyannamesi): the
// filed version is archived and the return is a draft again, version + 1.
func (s *Service) Amend(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period string) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	_, err := s.q.AmendTaxReturn(ctx, db.AmendTaxReturnParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		AmendedBy: actor(actorID), Kind: kind, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotFiled
	} else if err != nil {
		return Preview{}, fmt.Errorf("amend tax return: %w", err)
	}
	return s.Preview(ctx, sc, kind, period)
}
