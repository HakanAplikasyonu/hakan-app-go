package entegrator

import (
	"github.com/furkanmeclis/go-ubltr/provider"
	// İzibiz SOAP client (36); registers "izibiz" in the provider registry.
	// Only its read methods are reachable through Client (30 §5).
	_ "github.com/furkanmeclis/go-ubltr/provider/izibiz"
)

// WarnStatusUnverified: İzibiz returned a status the go-ubltr table does not
// know (36 §7: the vendor publishes no complete list).
const WarnStatusUnverified = "STATUS_UNVERIFIED"

// izibizPageSize is the GetInvoice LIMIT for header-only listings (36 §5.1:
// 1000 for lists; the upper bound is confirmed in the test environment).
const izibizPageSize = 1000

func init() {
	// GetInvoice has no paging: LIMIT is only an upper bound (36 §5.3).
	RegisterListProfile("izibiz", ListProfile{PageSize: izibizPageSize, NoCursor: true})
	RegisterStatusTable("izibiz", func(d provider.DocumentInfo) (Normalized, bool) {
		if d.Status == provider.StatusUnknown || d.Status == "" {
			return Normalized{Status: StatusActive, Warnings: []string{WarnStatusUnverified}}, true
		}
		return Normalized{}, false // go-ubltr's STATUS_CODE table (izibiz/status.go)
	})
}
