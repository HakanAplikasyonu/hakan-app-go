package entegrator

import (
	// İşNet InvoiceService SOAP client (32); registers "isnet". The module
	// implements only read calls (SearchAllInvoice, SearchArchiveInvoice,
	// GetTaxPayer): ArchiveInvoice, DeArchiveInvoice, UpdateInvoiceState and
	// SendInvoiceReply are not in it (30 §5). Listings send
	// IsArchiveIncluded=true. İşNet has no username: access is the fixed
	// egress IP plus the company VKN, which the opener always takes from the
	// connection's taxpayer record (32 §2). A call from an IP İşNet has not
	// allowlisted times out and comes back as ip-not-allowlisted
	// (KindPermission), which marks the connection auth_failed (37 §4).
	_ "github.com/furkanmeclis/go-ubltr/provider/isnet"
)

func init() {
	// Min/MaxInvoiceCreationDate go out as yyyy-MM-ddTHH:mm:ss (32 §3).
	RegisterListProfile("isnet", ListProfile{SecondBounds: true})
}
