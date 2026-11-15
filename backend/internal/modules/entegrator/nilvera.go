package entegrator

import (
	// Nilvera REST client (#59); registers "nilvera". Read calls only reach
	// it through Client: e-Fatura gelen/giden liste (GET /einvoice/Purchase,
	// /einvoice/Sale), e-Arşiv liste (GET /earchive/Invoices), UBL indirme
	// and status. Listings never send IsTransfer (OnlyNew is false), and
	// Operation/Transferred (MarkTaken), SendAnswer and Cancel are not
	// reachable (30 §5). Credentials: Credentials.APIKey (Bearer).
	_ "github.com/furkanmeclis/go-ubltr/provider/nilvera"
)
