// Package caritrigger is the new counterparty trigger of the desktop helper
// (docs/tasarim/masaustu-luca-sgk.md §0.1 Senaryo 1): when an invoice
// arrives and its counterparty's VKN / TCKN is not in the taxpayer's
// counterparty memory (counterparty_accounts), the counterparty joins the
// taxpayer's pending luca_cari_sync task (opened when there is none) — one
// row per VKN and main account. Only offices that turned the Luca helper on
// get the task; the agent runs it after the office's single confirmation or
// as the first step of a "Luca'ya aktar" run.
//
// The package only needs the generated queries, so invoice ingestion can
// call it without importing the helper module.
package caritrigger

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

var taxIDRe = regexp.MustCompile(`^[0-9]{10,11}$`)

// Counterparty is the invoice's other party.
type Counterparty struct {
	// Direction: "incoming" (a supplier, 320) or "outgoing" (a customer, 120).
	Direction      string
	TaxNumber      string
	IdentityNumber string
	Name           string
}

// MainCode is the counterparty's main account: 320 for a supplier, 120 for
// a customer; "" for an unknown direction.
func MainCode(direction string) string {
	switch direction {
	case "incoming":
		return "320"
	case "outgoing":
		return "120"
	}
	return ""
}

// TaxID is the VKN, else the TCKN; "" when neither is valid.
func TaxID(c Counterparty) string {
	for _, v := range []string{c.TaxNumber, c.IdentityNumber} {
		v = strings.TrimSpace(v)
		if taxIDRe.MatchString(v) {
			return v
		}
	}
	return ""
}

// Enqueue adds the counterparty of a new invoice of taxpayerID to the
// pending cari task. It runs in a savepoint of tx and is a side effect: a
// refusal (no office scope, a constraint) is rolled back alone and logged,
// the invoice is saved anyway.
func Enqueue(ctx context.Context, tx pgx.Tx, taxpayerID int64, c Counterparty) {
	main, tax := MainCode(c.Direction), TaxID(c)
	if main == "" || tax == "" || tx == nil {
		return
	}
	name := strings.TrimSpace(c.Name)
	if utf8.RuneCountInString(name) > 300 {
		name = string([]rune(name)[:300])
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		slog.WarnContext(ctx, "side_effect_failed", "name", "luca_cari_trigger", "error", err)
		return
	}
	if _, err := db.New(sp).EnqueueCariForInvoice(ctx, db.EnqueueCariForInvoiceParams{
		TaxID: tax, MainCode: main, Title: name, TaxpayerID: taxpayerID,
	}); err != nil {
		_ = sp.Rollback(ctx)
		slog.WarnContext(ctx, "side_effect_failed", "name", "luca_cari_trigger", "error", err)
		return
	}
	if err := sp.Commit(ctx); err != nil {
		slog.WarnContext(ctx, "side_effect_failed", "name", "luca_cari_trigger", "error", err)
	}
}
