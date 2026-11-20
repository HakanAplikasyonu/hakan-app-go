package office

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// CounterpartyTaxID checks an office-entered VKN (10 digits) or TCKN (11
// digits) with its check digits; it returns the VKN or the TCKN.
func CounterpartyTaxID(raw string) (vkn, tckn string, ok bool) {
	d := model.DigitsOnly(strings.TrimSpace(raw))
	if len(d) != len(strings.Join(strings.Fields(raw), "")) {
		return "", "", false // letters or other characters
	}
	switch {
	case len(d) == 10 && taxid.ValidVKN(d):
		return d, "", true
	case len(d) == 11 && taxid.ValidTCKN(d):
		return "", d, true
	}
	return "", "", false
}

// setCounterpartyTaxID completes the counterparty's VKN / TCKN when the
// source had none or an invalid one (#669, INVALID_TAX_ID): the cari is
// found or created and linked, the warning is resolved, who and when are
// kept. An invoice that has a number and a locked period are refused.
func (h *Handler) setCounterpartyTaxID(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	var body struct {
		TaxID string `json:"tax_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "body must be {\"tax_id\": \"…\"}")
		return
	}
	vkn, tckn, ok := CounterpartyTaxID(body.TaxID)
	if !ok {
		response.Error(w, r, http.StatusBadRequest, "INVALID_TAX_ID", "tax_id must be a valid VKN (10 digits) or TCKN (11 digits)")
		return
	}
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if inv.CustomerTaxNumber.Valid || inv.CustomerIdentityNumber.Valid {
		response.Error(w, r, http.StatusConflict, "TAX_ID_SET", "the invoice already has a counterparty tax id")
		return
	}
	if _, err := h.q.GetPeriodLock(r.Context(), db.GetPeriodLockParams{
		OrganizationID: s.OrganizationID, EngagementID: s.ID, Period: inv.Period,
	}); err == nil {
		response.Error(w, r, http.StatusConflict, "PERIOD_LOCKED", "the period is locked")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not check the period lock")
		return
	}
	c, err := h.customerFor(r.Context(), s.TaxpayerID, inv, vkn, tckn)
	if err != nil {
		response.InternalErr(w, r, err, "could not link the counterparty")
		return
	}
	n, err := h.q.SetMissingInvoiceCounterpartyTaxID(r.Context(), db.SetMissingInvoiceCounterpartyTaxIDParams{
		CustomerID: pgtype.Int8{Int64: c, Valid: true}, TaxNumber: optText(vkn), IdentityNumber: optText(tckn),
		SetBy: actorOf(r), TaxpayerID: s.TaxpayerID, ID: inv.ID,
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not set the tax id")
		return
	}
	if n == 0 {
		response.Error(w, r, http.StatusConflict, "TAX_ID_SET", "the invoice already has a counterparty tax id")
		return
	}
	if _, err := h.q.ResolveEntityDataWarnings(r.Context(), db.ResolveEntityDataWarningsParams{
		ResolvedBy: actorOf(r), Note: pgtype.Text{String: "VKN/TCKN ofiste girildi: " + vkn + tckn, Valid: true},
		OrganizationID: s.OrganizationID, EngagementID: s.ID,
		EntityType: model.EntityInvoice, EntityID: pgtype.Int8{Int64: inv.ID, Valid: true}, Code: model.WarnInvalidTaxID,
	}); err != nil {
		response.InternalErr(w, r, err, "could not resolve the warning")
		return
	}
	h.get(w, r)
}

// customerFor finds the taxpayer's cari with the number or creates it (one
// cari per VKN, or per TCKN without VKN: `01` §4.3).
func (h *Handler) customerFor(ctx context.Context, taxpayerID int64, inv db.Invoice, vkn, tckn string) (int64, error) {
	find := func() (db.Customer, error) {
		if vkn != "" {
			return h.q.FindCustomerByTaxNumber(ctx, db.FindCustomerByTaxNumberParams{TaxpayerID: taxpayerID, TaxNumber: optText(vkn)})
		}
		return h.q.FindCustomerByIdentityNumber(ctx, db.FindCustomerByIdentityNumberParams{TaxpayerID: taxpayerID, IdentityNumber: optText(tckn)})
	}
	c, err := find()
	if err == nil {
		return c.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	typ := "customer"
	if inv.Direction == string(model.DirectionIncoming) {
		typ = "supplier"
	}
	name := strings.TrimSpace(inv.CustomerName.String)
	if name == "" {
		name = vkn + tckn
	}
	c, err = h.q.InsertCustomer(ctx, db.InsertCustomerParams{
		TaxpayerID: taxpayerID, SourceSystem: "manual_upload", SourceID: vkn + tckn, CaptureMethod: "manual",
		Type: typ, Name: name, TaxNumber: optText(vkn), IdentityNumber: optText(tckn), RawPayload: []byte("{}"),
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" { // created concurrently
		c, err = find()
	}
	if err != nil {
		return 0, err
	}
	return c.ID, nil
}

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
