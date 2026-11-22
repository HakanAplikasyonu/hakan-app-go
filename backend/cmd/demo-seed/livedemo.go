package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// liveDemoPhotos are Atlas Gıda's photos "just sent from the phone" (#780
// canlı demo): two expense receipts and a Z report read by the AI, waiting
// for the office. The market receipt has a %1 / %10 / %20 breakdown and its
// vendor name was read with low confidence ("kontrol et"); the fuel receipt
// is from a vendor with an approved October expense (vendor memory
// suggestion); the Z report's card total was read with low confidence.
// November, so October (locked by the smoke test) is untouched.
var liveDemoPhotos = []struct {
	name, kind, answer string
}{
	{"fis-market-kasim.png", extract.KindExpense, `{"kind":"expense_receipt","kind_confidence":0.96,"fields":{
"date":{"value":"18.11.2026","confidence":0.95},"vendor_tax_number":{"value":"9000000068","confidence":0.93},
"vendor_name":{"value":"Yıldız Market Gıda Ltd. Şti.","confidence":0.66},"receipt_number":{"value":"0457","confidence":0.9},
"total_amount":{"value":"671,00","confidence":0.95},"tax_amount":{"value":"71,00","confidence":0.92},
"payment_method":{"value":"KREDİ KARTI","confidence":0.9},
"vat_1_total":{"value":"101,00","confidence":0.9},"vat_1_tax":{"value":"1,00","confidence":0.9},
"vat_10_total":{"value":"330,00","confidence":0.9},"vat_10_tax":{"value":"30,00","confidence":0.9},
"vat_20_total":{"value":"240,00","confidence":0.9},"vat_20_tax":{"value":"40,00","confidence":0.9}}}`},
	{"fis-akaryakit-kasim.png", extract.KindFuel, `{"kind":"fuel_receipt","kind_confidence":0.97,"fields":{
"date":{"value":"19.11.2026","confidence":0.95},"vendor_tax_number":{"value":"9000000033","confidence":0.94},
"vendor_name":{"value":"Demirtepe Petrol Ltd. Şti.","confidence":0.92},"receipt_number":{"value":"0061","confidence":0.9},
"total_amount":{"value":"1.800,00","confidence":0.95},"tax_amount":{"value":"300,00","confidence":0.93},
"tax_rate":{"value":"20","confidence":0.9},"vehicle_plate":{"value":"34ABC123","confidence":0.9},
"payment_method":{"value":"KREDİ KARTI","confidence":0.9},
"vat_20_total":{"value":"1.800,00","confidence":0.9},"vat_20_tax":{"value":"300,00","confidence":0.9}}}`},
	{"z-raporu-kasim.png", extract.KindZReport, `{"kind":"z_report","kind_confidence":0.97,"fields":{
"date":{"value":"19.11.2026","confidence":0.95},"z_report_number":{"value":"0012","confidence":0.94},
"okc_serial_number":{"value":"JH20099999","confidence":0.92},
"total_amount":{"value":"10.500,00","confidence":0.95},"tax_amount":{"value":"1.635,00","confidence":0.93},
"cash_amount":{"value":"4.000,00","confidence":0.92},"card_amount":{"value":"6.500,00","confidence":0.55},
"receipt_count":{"value":"63","confidence":0.9},
"vat_1_total":{"value":"202,00","confidence":0.9},"vat_1_tax":{"value":"2,00","confidence":0.9},
"vat_10_total":{"value":"1.100,00","confidence":0.9},"vat_10_tax":{"value":"100,00","confidence":0.9},
"vat_20_total":{"value":"9.198,00","confidence":0.9},"vat_20_tax":{"value":"1.533,00","confidence":0.9}}}`},
}

// fixedReader answers with one stored AI answer (the photo was "read").
type fixedReader struct{ answer string }

func (f fixedReader) Read(context.Context, extract.Request) (extract.Result, error) {
	r, err := extract.Parse(f.answer)
	r.Model = "demo"
	return r, err
}

// seedLiveDemo adds the live demo photos through the capture pipeline (the
// same draft, VAT lines and review flags as a real upload). Idempotent: a
// photo whose capture exists is skipped; nothing is deleted.
func seedLiveDemo(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.System(ctx, pool, "demo-seed: live demo photos (#780)", func(ctx context.Context, tx pgx.Tx) error {
		q := db.New(pool)
		warn := warningsengine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
		for i, p := range liveDemoPhotos {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM receipt_captures c JOIN attachments a ON a.id = c.attachment_id
				WHERE c.organization_id = $1 AND c.engagement_id = $2 AND c.taxpayer_id = $3 AND a.file_name = $4)`,
				scope.OrganizationID, scope.ID, scope.TaxpayerID, p.name).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			svc := receipts.New(pool, seedStore, fixedReader{answer: p.answer}, nil, warn, nil)
			if _, err := svc.Capture(ctx, scope, actor, p.name, blurryPhoto(uint8(40+i)), p.kind); err != nil {
				return fmt.Errorf("live demo photo %s: %w", p.name, err)
			}
		}
		return nil
	})
}
