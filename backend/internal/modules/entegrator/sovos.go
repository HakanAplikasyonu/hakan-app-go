package entegrator

import (
	"math"

	// Sovos (Foriba) e-Fatura SOAP client (#59); registers "sovos". It
	// lists the e-Fatura inbox and outbox (getUBLList, one day per call,
	// Page.Next is the next day) and downloads UBL (getUBL ZIP). e-Arşiv
	// can be downloaded but not listed (not_listable). Sovos has no "taken"
	// flag; send, response and cancel are not reachable through Client
	// (30 §5). Credentials: Username / Password; Options postbox_alias
	// (gelen), sender_alias (giden), earchive_endpoint.
	_ "github.com/furkanmeclis/go-ubltr/provider/sovos"
)

func init() {
	// getUBLList returns a whole day; go-ubltr cuts the page to ListQuery.
	// Limit without a next page for the rest, so the limit is left open.
	RegisterListProfile("sovos", ListProfile{PageSize: math.MaxInt32})
}
