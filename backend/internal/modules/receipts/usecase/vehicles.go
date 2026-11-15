package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ErrConflict: the plate is already on the list.
var ErrConflict = errors.New("receipts: plate already registered")

// Vehicle is one entry of the taxpayer's vehicle list (T8).
type Vehicle struct {
	UUID        uuid.UUID `json:"uuid"`
	Plate       string    `json:"plate"`
	Description string    `json:"description"`
	ValidFrom   *string   `json:"valid_from"`
	ValidTo     *string   `json:"valid_to"`
}

// VehicleInput adds a vehicle.
type VehicleInput struct {
	Plate       string  `json:"plate"`
	Description string  `json:"description"`
	ValidFrom   *string `json:"valid_from"`
	ValidTo     *string `json:"valid_to"`
}

// Vehicles lists the taxpayer's vehicles.
func (s *Service) Vehicles(ctx context.Context, sc engagementctx.Scope) ([]Vehicle, error) {
	rows, err := s.q.ListTaxpayerVehicles(ctx, sc.TaxpayerID)
	if err != nil {
		return nil, err
	}
	out := make([]Vehicle, 0, len(rows))
	for _, r := range rows {
		out = append(out, vehicle(r))
	}
	return out, nil
}

// AddVehicle registers a plate.
func (s *Service) AddVehicle(ctx context.Context, sc engagementctx.Scope, userID int64, in VehicleInput) (Vehicle, error) {
	plate, ok := extract.NormalizePlate(in.Plate)
	if !ok {
		return Vehicle{}, fmt.Errorf("%w: plate %q is not a Turkish plate", ErrInvalid, in.Plate)
	}
	from, err := optDay(in.ValidFrom)
	if err != nil {
		return Vehicle{}, err
	}
	to, err := optDay(in.ValidTo)
	if err != nil {
		return Vehicle{}, err
	}
	if from.Valid && to.Valid && to.Time.Before(from.Time) {
		return Vehicle{}, fmt.Errorf("%w: valid_to before valid_from", ErrInvalid)
	}
	row, err := s.q.InsertTaxpayerVehicle(ctx, db.InsertTaxpayerVehicleParams{
		TaxpayerID: sc.TaxpayerID, Plate: plate, Description: strings.TrimSpace(in.Description),
		ValidFrom: from, ValidTo: to, CreatedBy: optID(userID),
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return Vehicle{}, ErrConflict
	}
	if err != nil {
		return Vehicle{}, err
	}
	return vehicle(row), nil
}

// DeleteVehicle removes a plate.
func (s *Service) DeleteVehicle(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) error {
	n, err := s.q.DeleteTaxpayerVehicle(ctx, db.DeleteTaxpayerVehicleParams{TaxpayerID: sc.TaxpayerID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func vehicle(r db.TaxpayerVehicle) Vehicle {
	v := Vehicle{UUID: r.Uuid, Plate: r.Plate, Description: r.Description}
	if r.ValidFrom.Valid {
		s := r.ValidFrom.Time.Format("2006-01-02")
		v.ValidFrom = &s
	}
	if r.ValidTo.Valid {
		s := r.ValidTo.Time.Format("2006-01-02")
		v.ValidTo = &s
	}
	return v
}

func optDay(s *string) (pgtype.Date, error) {
	if s == nil || *s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("%w: dates must be YYYY-MM-DD", ErrInvalid)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}
