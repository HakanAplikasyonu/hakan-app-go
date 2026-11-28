package entegrator

import (
	"sync"
	"time"
)

// ListProfile describes how an integrator's listing pages (36 §5.3, 37 §5.3).
type ListProfile struct {
	// PageSize is the LIMIT sent with each listing (0: Config.PageSize).
	PageSize int
	// NoCursor: the listing has no paging; a page holding PageSize items
	// (or more) means the window may hold more documents, so the window is
	// split in two, down to Config.MinWindow. A full one-day window is
	// recorded as LIMIT_REACHED.
	NoCursor bool
	// EndDateOnly: the integrator cuts the end bound to its date (00:00),
	// so the end is sent as the start of the day after the window's last
	// instant (Uyumsoft CreateEndDate, seen in the test system). The extra
	// hours overlap the next window; indexing is idempotent.
	EndDateOnly bool
	// SecondBounds: the integrator takes the bounds as whole seconds (the
	// value is sent without a fraction: EDM CR_END_DATE, İşNet
	// MaxInvoiceCreationDate) while it keeps creation times with their
	// fraction. The window's last instant (…23:59:59.999999999) would go
	// out as …23:59:59 and a document created in that last second would be
	// listed by neither window, so the end is rounded up to the whole
	// second (the window's exclusive end itself). The shared instant with
	// the next window is harmless: indexing is idempotent.
	SecondBounds bool
	// ListGap is the least time between two listing calls of a connection,
	// for integrators that rate-limit the list endpoint itself (Turkcell:
	// 6 calls / 15 minutes). Config.RequestGap still applies to every call.
	ListGap time.Duration
}

var (
	profilesMu sync.RWMutex
	profiles   = map[string]ListProfile{}
)

// RegisterListProfile installs the listing profile of an integrator name.
func RegisterListProfile(name string, p ListProfile) {
	profilesMu.Lock()
	defer profilesMu.Unlock()
	profiles[name] = p
}

func listProfile(name string) ListProfile {
	profilesMu.RLock()
	defer profilesMu.RUnlock()
	return profiles[name]
}
