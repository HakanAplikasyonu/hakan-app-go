package entegrator

import (
	// NES REST client (#59); registers "nes". Read calls only reach it
	// through Client: e-Fatura gelen/giden liste (GET /einvoice/v1/incoming/
	// invoices, /outgoing/invoices), e-Arşiv liste (GET /earchive/v1/invoices,
	// iptaller dahil), UBL indirme and status. Listings never send
	// erpTransferred (OnlyNew is false); Transferred (MarkTaken),
	// documentAnswer and cancel calls are not reachable (30 §5).
	// Credentials: Credentials.APIKey (Bearer).
	_ "github.com/furkanmeclis/go-ubltr/provider/nes"
)
