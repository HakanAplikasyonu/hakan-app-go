package entegrator

import (
	"time"

	// Turkcell e-Şirket REST client (#59); registers "turkcell". It lists
	// only the e-Fatura inbox (GET /v1/inboxinvoice/list); outgoing
	// e-Fatura and e-Arşiv can be downloaded but not listed (recorded as
	// not_listable). Listings never send isNew (OnlyNew is false);
	// updateisnew (MarkTaken), invoiceresponse and cancel are not reachable
	// through Client (30 §5). Credentials: Credentials.APIKey (x-api-key).
	_ "github.com/furkanmeclis/go-ubltr/provider/turkcell"
)

// turkcellListGap: the inbox list endpoint is limited to 6 calls per 15
// minutes (go-ubltr turkcell ListInbox).
const turkcellListGap = 150 * time.Second

func init() {
	RegisterListProfile("turkcell", ListProfile{PageSize: 500, ListGap: turkcellListGap})
}
