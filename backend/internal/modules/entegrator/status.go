package entegrator

import (
	"strconv"
	"strings"
	"sync"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Normalized statuses (invoices.status_normalized, 01 §4.4).
const (
	StatusDraft     = "draft"
	StatusActive    = "active"
	StatusCancelled = "cancelled"
)

// Status warnings kept on entegrator_documents.warnings (30 §8).
const (
	WarnPendingResponse = "PENDING_RESPONSE"
	WarnRejected        = "REJECTED"
	WarnReturned        = "RETURNED"
	WarnDeliveryError   = "DELIVERY_ERROR"
	WarnStatusUnknown   = "STATUS_UNKNOWN"
)

// Normalized is a status_normalized value plus its warnings.
type Normalized struct {
	Status   string
	Warnings []string
}

// StatusTable lets an integrator module refine the common mapping with its
// own codes (the per-integrator table in 32–36). It returns ok=false to fall
// back to the common mapping.
type StatusTable func(provider.DocumentInfo) (Normalized, bool)

var (
	tablesMu sync.RWMutex
	tables   = map[string]StatusTable{}
)

// RegisterStatusTable installs the table for an integrator name.
func RegisterStatusTable(name string, t StatusTable) {
	tablesMu.Lock()
	defer tablesMu.Unlock()
	tables[name] = t
}

// RawStatus is the stored status_raw: normalized go-ubltr status, GİB code
// and the integrator's own code, so a later table change can re-derive it.
func RawStatus(d provider.DocumentInfo) string {
	parts := []string{string(d.Status)}
	if d.GIBCode != 0 {
		parts = append(parts, "gib="+strconv.Itoa(d.GIBCode))
	}
	if d.ProviderCode != "" {
		parts = append(parts, "code="+d.ProviderCode)
	}
	return strings.Join(parts, ";")
}

// NormalizeStatus maps a listed document to status_normalized (30 §8).
func NormalizeStatus(name string, d provider.DocumentInfo) Normalized {
	tablesMu.RLock()
	t := tables[name]
	tablesMu.RUnlock()
	if t != nil {
		if n, ok := t(d); ok {
			return n
		}
	}
	commercial := strings.EqualFold(d.Profile, "TICARIFATURA")
	switch d.Status {
	case provider.StatusAccepted:
		return Normalized{Status: StatusActive}
	case provider.StatusSent, provider.StatusDelivered, provider.StatusReceived:
		if commercial {
			return Normalized{Status: StatusActive, Warnings: []string{WarnPendingResponse}}
		}
		return Normalized{Status: StatusActive}
	case provider.StatusRejected:
		return Normalized{Status: StatusCancelled, Warnings: []string{WarnRejected}}
	case provider.StatusCancelled:
		return Normalized{Status: StatusCancelled}
	case provider.StatusDraft, provider.StatusQueued, provider.StatusProcessing:
		return Normalized{Status: StatusDraft}
	case provider.StatusFailed:
		return Normalized{Status: StatusDraft, Warnings: []string{WarnDeliveryError}}
	}
	return Normalized{Status: StatusActive, Warnings: []string{WarnStatusUnknown}}
}
