package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/reviewlink"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoReceiptModel names the OCR model of the demo receipt's suggestions.
const DemoReceiptModel = "demo-ocr"

// demoReceiptFields are the fields of the demo fuel receipt the OCR read
// with low confidence: they wait in the office review queue (#318), so the
// review screens and the Maestro review-approve flow have something to
// approve. Values are what the receipt says.
var demoReceiptFields = []struct {
	field, value string
	confidence   float64
}{
	{extract.FieldTotal, "1250.00", 0.55},
	{extract.FieldTax, "208.33", 0.62},
	{extract.FieldReceiptNumber, "0042", 0.41},
}

// seedReview adds an OCR expense draft (a fuel receipt of October 2026) to
// the taxpayer and its low-confidence fields to the review queue of the
// engagement, through the review service as the receipt pipeline does.
func seedReview(ctx context.Context, pool *pgxpool.Pool, orgID, engID int64, taxpayerUUID string) error {
	var tpID, expenseID int64
	err := rls.System(ctx, pool, "demo-seed: receipt draft for the review queue", func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM taxpayers WHERE uuid = $1`, taxpayerUUID).Scan(&tpID); err != nil {
			return err
		}
		ocr := map[string]any{}
		for _, f := range demoReceiptFields {
			ocr[f.field] = map[string]any{"value": f.value, "confidence": f.confidence}
		}
		ocrJSON, _ := json.Marshal(ocr)
		date := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
		e, err := db.New(tx).InsertExpenseDraft(ctx, db.InsertExpenseDraftParams{
			TaxpayerID: tpID, SourceID: "demo:receipt-0042", ExpenseDate: pgtype.Date{Time: date, Valid: true},
			Period: pgtype.Text{String: "2026-10", Valid: true}, Amount: num("1250.00"), TaxRate: num("20"),
			TaxExclusiveAmount: num("1041.67"), TaxAmount: num("208.33"), ReceiptType: "fuel_receipt",
			VendorTaxNumber: pgtype.Text{String: "1234567890", Valid: true}, VendorName: pgtype.Text{String: "Atlas Akaryakıt", Valid: true},
			ReceiptNumber: pgtype.Text{String: "0042", Valid: true}, VehiclePlate: pgtype.Text{String: "34ABC123", Valid: true},
			OcrConfidence: num("0.55"), OcrFields: ocrJSON, RawPayload: []byte(`{}`),
		})
		expenseID = e.ID
		return err
	})
	if err != nil {
		return fmt.Errorf("receipt draft: %w", err)
	}

	q := db.New(pool)
	rev := review.New(reviewrepo.New(pool, q)).Register(reviewlink.Kind, reviewlink.Applier{})
	sub := review.Subject{OrganizationID: orgID, EngagementID: engID, TaxpayerID: tpID}
	for _, f := range demoReceiptFields {
		raw, _ := json.Marshal(f.value)
		target := expenseID
		if _, err := rev.Submit(ctx, sub, review.Input{
			Kind: reviewlink.Kind, TargetType: "expense", TargetID: &target, TargetField: f.field, Period: "2026-10",
			Value: raw, Confidence: decimal.NewFromFloat(f.confidence),
			Rationale:  fmt.Sprintf("Belge okuma bu alanı %%%.0f güvenle okudu; belgeyle karşılaştırın.", f.confidence*100),
			Provenance: review.Provenance{Origin: review.OriginModel, Model: DemoReceiptModel},
		}); err != nil {
			return fmt.Errorf("review %s: %w", f.field, err)
		}
	}
	return nil
}

func num(s string) pgtype.Numeric {
	d := decimal.RequireFromString(s)
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}
