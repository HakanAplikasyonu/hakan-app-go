package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
)

// Customer types a cari card can have (customers.type).
const (
	CustomerTypeCustomer = "customer"
	CustomerTypeSupplier = "supplier"
	CustomerTypeBoth     = "both"
)

// SaveCustomer writes one cari card from a list (Excel import, #669) with
// the invoice rule (`01` §4.3): one card per VKN, or per TCKN without VKN,
// within the taxpayer. An existing card keeps its content; a customer that
// is now also a supplier (or the reverse) becomes "both". created reports a
// new card.
func SaveCustomer(ctx context.Context, tx pgx.Tx, taxpayerID int64, cp model.Party, typ, sourceSystem string) (created bool, err error) {
	if cp.TaxNumber == "" && cp.IdentityNumber == "" {
		return false, errors.New("repository: customer needs a VKN or TCKN")
	}
	q := db.New(tx)
	find := func() (db.Customer, error) {
		if cp.TaxNumber != "" {
			return q.FindCustomerByTaxNumber(ctx, db.FindCustomerByTaxNumberParams{TaxpayerID: taxpayerID, TaxNumber: Text(cp.TaxNumber)})
		}
		return q.FindCustomerByIdentityNumber(ctx, db.FindCustomerByIdentityNumberParams{TaxpayerID: taxpayerID, IdentityNumber: Text(cp.IdentityNumber)})
	}
	sourceID := cp.TaxNumber
	if sourceID == "" {
		sourceID = cp.IdentityNumber
	}
	c, err := find()
	if errors.Is(err, pgx.ErrNoRows) {
		inv := model.Invoice{Counterparty: cp, SourceSystem: sourceSystem, CaptureMethod: "excel_import"}
		c, err = insertCustomer(ctx, tx, taxpayerID, inv, typ, sourceID)
		created = err == nil
		if isUniqueViolation(err) {
			c, err = find()
		}
	}
	if err != nil {
		return false, err
	}
	if !created && c.Type != typ && c.Type != CustomerTypeBoth && c.Type != "employee" {
		if err := q.SetCustomerType(ctx, db.SetCustomerTypeParams{TaxpayerID: taxpayerID, ID: c.ID, Type: CustomerTypeBoth}); err != nil {
			return false, err
		}
	}
	raw, err := sealCounterparty(cp)
	if err != nil {
		return false, err
	}
	if err := q.UpsertCustomerSourceLink(ctx, db.UpsertCustomerSourceLinkParams{
		TaxpayerID: taxpayerID, CustomerID: c.ID, SourceSystem: sourceSystem, SourceID: sourceID, RawPayload: raw,
	}); err != nil {
		return false, err
	}
	return created, nil
}
