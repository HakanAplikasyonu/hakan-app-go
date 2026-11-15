package engine

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
)

// MemoryStore is an in-memory Store with the same rules as the Postgres
// one. It backs the engine tests and runs the engine before the data_warning
// table exists.
type MemoryStore struct {
	mu   sync.Mutex
	rows []*StoredWarning
}

// StoredWarning is a MemoryStore row.
type StoredWarning struct {
	Warning
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ResolvedAt *time.Time
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Raise implements Store.
func (m *MemoryStore) Raise(_ context.Context, w Warning) (RaiseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last := m.latest(w.Key); last != nil {
		switch last.Status {
		case catalog.StatusOpen:
			last.Severity, last.Params, last.Producer, last.SeenAt = w.Severity, w.Params, w.Producer, w.SeenAt
			last.UpdatedAt = w.SeenAt
			return Refreshed, nil
		case catalog.StatusAcknowledged, catalog.StatusResolved:
			if bytes.Equal(last.Params, w.Params) {
				last.SeenAt = w.SeenAt
				return Suppressed, nil
			}
		}
	}
	m.rows = append(m.rows, &StoredWarning{Warning: w, CreatedAt: w.SeenAt, UpdatedAt: w.SeenAt})
	return Created, nil
}

// AutoResolve implements Store.
func (m *MemoryStore) AutoResolve(_ context.Context, s Subject, producer string, e Entity, codes, keep []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in := func(list []string, s string) bool {
		for _, v := range list {
			if v == s {
				return true
			}
		}
		return false
	}
	now := time.Now()
	n := 0
	for _, r := range m.rows {
		if r.Status != catalog.StatusOpen || r.Producer != producer || r.EngagementID != s.EngagementID ||
			r.EntityType != e.Type || !matches(r, e) || !in(codes, r.Code) || in(keep, r.Code) {
			continue
		}
		r.Status = catalog.StatusAutoResolved
		r.UpdatedAt, r.ResolvedAt = now, &now
		n++
	}
	return n, nil
}

// SetStatus changes a row's status (accountant action; tests).
func (m *MemoryStore) SetStatus(k Key, s catalog.Status) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last := m.latest(k); last != nil {
		last.Status = s
		return true
	}
	return false
}

// All returns a copy of every row.
func (m *MemoryStore) All() []StoredWarning {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]StoredWarning, len(m.rows))
	for i, r := range m.rows {
		out[i] = *r
	}
	return out
}

func (m *MemoryStore) latest(k Key) *StoredWarning {
	for i := len(m.rows) - 1; i >= 0; i-- {
		r := m.rows[i]
		if r.EngagementID == k.EngagementID && r.EntityType == k.EntityType && r.Code == k.Code &&
			r.Period == k.Period && sameID(r.EntityID, k.EntityID) {
			return r
		}
	}
	return nil
}

// matches reports whether row r belongs to entity e.
func matches(r *StoredWarning, e Entity) bool {
	return sameID(r.EntityID, e.ID) && r.Period == e.Period
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
