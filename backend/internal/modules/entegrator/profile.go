package entegrator

import "sync"

// ListProfile describes how an integrator's listing pages (36 §5.3, 37 §5.3).
type ListProfile struct {
	// PageSize is the LIMIT sent with each listing (0: Config.PageSize).
	PageSize int
	// NoCursor: the listing has no paging; a page holding PageSize items
	// (or more) means the window may hold more documents, so the window is
	// split in two, down to Config.MinWindow. A full one-day window is
	// recorded as LIMIT_REACHED.
	NoCursor bool
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
