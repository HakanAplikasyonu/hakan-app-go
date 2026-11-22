package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Mükelleften yabancı fatura (#780 A2): Emre Çelik panelden bu ayın Meta
// reklam faturasının fotoğrafını göndermiş; AI satıcı, ülke, tutar ve
// dövizi önermiş ("önerildi"), kur girilmemiş, ofis inceleyip onaylayacak.
// Fotoğraf her çalıştırmada aynı baytlar: aynı dosya aynı taslağı açar,
// tekrar çalıştırmada çoğalmaz.

// demoForeignPrefill stands in for the AI reading of the demo photo.
type demoForeignPrefill struct{ date string }

func (p demoForeignPrefill) ReadForeignInvoice(context.Context, extract.Request) (extract.ForeignInvoice, error) {
	return extract.ForeignInvoice{Vendor: "Meta Platforms Ireland Ltd.", Country: "IE", InvoiceNo: "FBADS-" + p.date[:7],
		InvoiceDate: p.date, Amount: "450.00", Currency: "USD", ServiceKind: foreigninvoices.KindAdvertising, Model: "demo"}, nil
}

// seedForeignInvoiceUpload adds Çelik's uploaded draft through the real
// intake (file in the attachments store, draft with source and suggestion).
func seedForeignInvoiceUpload(ctx context.Context, pool *pgxpool.Pool, orgID int64, now time.Time) error {
	var taxID string
	for _, tp := range demo.OperatingTaxpayers {
		if tp.Key == demo.CelikKey {
			taxID = tp.TaxID
		}
	}
	t := now.In(foreigninvoices.Istanbul)
	date := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	return rls.System(ctx, pool, "demo-seed: foreign invoice upload", func(ctx context.Context, tx pgx.Tx) error {
		var engID, taxpayerID int64
		err := tx.QueryRow(ctx, `SELECT e.id, e.taxpayer_id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND t.tax_id = $2 AND e.status = 'active' ORDER BY e.id DESC LIMIT 1`,
			orgID, taxID).Scan(&engID, &taxpayerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("celik engagement: %w", err)
		}
		q := db.New(database.Ambient{Pool: pool})
		in := foreigninvoices.NewIntake(foreigninvoices.New(q), q, payrollStore{up: seedStore}, demoForeignPrefill{date: date}, nil)
		_, err = in.Submit(ctx, foreigninvoices.Subject{OrganizationID: orgID, EngagementID: engID}, taxpayerID, foreigninvoices.IntakeInput{
			Source: foreigninvoices.SourceTaxpayerUpload, FileName: "meta-reklam-faturasi.png", Data: blurryPhoto(91),
			Note: "Bu ayın Instagram reklam faturası",
		})
		if errors.Is(err, foreigninvoices.ErrDisabled) {
			return nil
		}
		return err
	})
}
