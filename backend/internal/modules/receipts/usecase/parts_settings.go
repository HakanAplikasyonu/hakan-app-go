package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// ErrPartsOff: the office turned the parçalı Z raporu setting off and a
// capture came with more than one photo.
var ErrPartsOff = errors.New("receipts: capturing a document in parts is turned off for this office")

// PartsSettings is the office's "parçalı Z raporu" setting.
type PartsSettings struct {
	Enabled   bool       `json:"enabled"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// PartsSettings returns the office's setting; without a row it is on.
func (s *Service) PartsSettings(ctx context.Context, orgID int64) (PartsSettings, error) {
	row, err := s.q.GetZReportPartsSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartsSettings{Enabled: true}, nil
	}
	if err != nil {
		return PartsSettings{}, fmt.Errorf("z report parts settings: %w", err)
	}
	return partsSettingsOf(row), nil
}

// SavePartsSettings stores the office's setting.
func (s *Service) SavePartsSettings(ctx context.Context, orgID, actorID int64, enabled bool) (PartsSettings, error) {
	var by pgtype.Int8
	if actorID > 0 {
		by = pgtype.Int8{Int64: actorID, Valid: true}
	}
	row, err := s.q.UpsertZReportPartsSettings(ctx, db.UpsertZReportPartsSettingsParams{
		OrganizationID: orgID, Enabled: enabled, UpdatedBy: by,
	})
	if err != nil {
		return PartsSettings{}, fmt.Errorf("save z report parts settings: %w", err)
	}
	return partsSettingsOf(row), nil
}

func partsSettingsOf(row db.ZReportPartsSetting) PartsSettings {
	out := PartsSettings{Enabled: row.Enabled}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		out.UpdatedAt = &t
	}
	return out
}
