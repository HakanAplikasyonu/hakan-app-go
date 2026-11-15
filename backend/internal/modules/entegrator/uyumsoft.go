package entegrator

import (
	"github.com/furkanmeclis/go-ubltr/provider"
	// Uyumsoft Integration SOAP client (33); registers "uyumsoft" (and the
	// muhasebeizleme preset, not used here). Listing goes through
	// GetInboxInvoiceList / GetOutboxInvoiceList with OnlyNewestInvoices
	// false; GetInboxInvoices(Data) with SetTaken and SetInvoicesTaken are
	// not reachable through Client (33 §6, 30 §5).
	_ "github.com/furkanmeclis/go-ubltr/provider/uyumsoft"
)

// WarnStatusUnmapped: a Uyumsoft status the table does not know (33 §9.1).
const WarnStatusUnmapped = "STATUS_UNMAPPED"

func init() {
	// CreateEndDate is compared by date only (33 §7.2; test system,
	// 2026-11-15: a document at 19:25 is missing from a window ending the
	// same day at 19:26 and present when the end is the next midnight).
	RegisterListProfile("uyumsoft", ListProfile{EndDateOnly: true})
	RegisterStatusTable("uyumsoft", func(d provider.DocumentInfo) (Normalized, bool) {
		switch d.ProviderCode {
		case "Return": // İade edildi (33 §9.1 ⚠️): müşavir karar verir
			return Normalized{Status: StatusCancelled, Warnings: []string{WarnReturned}}, true
		case "NotPrepared", "NotSend": // kodları dokümanda yok ⚠️
			return Normalized{Status: StatusDraft, Warnings: []string{WarnStatusUnmapped}}, true
		}
		if d.Status == provider.StatusUnknown || d.Status == "" {
			return Normalized{Status: StatusActive, Warnings: []string{WarnStatusUnmapped}}, true
		}
		return Normalized{}, false
	})
}
