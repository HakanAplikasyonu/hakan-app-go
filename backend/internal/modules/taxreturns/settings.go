package taxreturns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Settings is the office's tax return setting (#710, ofis kararı A.5).
type Settings struct {
	// FinalCheckRequired: "kontrol edildi" and "verildi" are marked only by
	// the responsible (tenant.tax_returns.approve). Off, whoever prepares
	// the return (tenant.tax_returns.write) marks them too. Default on.
	FinalCheckRequired bool       `json:"final_check_required"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}

// Settings returns the office's setting; an office without a row gets the
// default (final check on).
func (s *Service) Settings(ctx context.Context, orgID int64) (Settings, error) {
	row, err := s.q.GetTaxReturnSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{FinalCheckRequired: true}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("tax return settings: %w", err)
	}
	return settingsOf(row), nil
}

// SaveSettings stores the office's setting.
func (s *Service) SaveSettings(ctx context.Context, orgID, actorID int64, in Settings) (Settings, error) {
	row, err := s.q.UpsertTaxReturnSettings(ctx, db.UpsertTaxReturnSettingsParams{
		OrganizationID: orgID, FinalCheckRequired: in.FinalCheckRequired, UpdatedBy: actor(actorID),
	})
	if err != nil {
		return Settings{}, fmt.Errorf("save tax return settings: %w", err)
	}
	return settingsOf(row), nil
}

func settingsOf(row db.TaxReturnSetting) Settings {
	out := Settings{FinalCheckRequired: row.FinalCheckRequired}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		out.UpdatedAt = &t
	}
	return out
}
