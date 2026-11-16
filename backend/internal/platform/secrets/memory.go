package secrets

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// MemoryStore is an in-process Store for tests.
type MemoryStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]envelope
	seq  map[uuid.UUID]int
	n    int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: map[uuid.UUID]envelope{}, seq: map[uuid.UUID]int{}}
}

func clone(e envelope) envelope {
	e.WrappedDEK = append([]byte(nil), e.WrappedDEK...)
	e.Ciphertext = append([]byte(nil), e.Ciphertext...)
	return e
}

func (m *MemoryStore) Insert(_ context.Context, e envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n++
	m.rows[e.ID], m.seq[e.ID] = clone(e), m.n
	return nil
}

func (m *MemoryStore) Get(_ context.Context, id uuid.UUID) (envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.rows[id]
	if !ok {
		return envelope{}, ErrNotFound
	}
	return clone(e), nil
}

func (m *MemoryStore) Replace(_ context.Context, e envelope) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.rows[e.ID]
	if !ok || cur.Purpose != e.Purpose {
		return false, nil
	}
	m.rows[e.ID] = clone(e)
	return true, nil
}

func (m *MemoryStore) Rewrap(_ context.Context, e envelope, oldKEK string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.rows[e.ID]
	if !ok || cur.KEKID != oldKEK {
		return false, nil
	}
	cur.KEKID, cur.WrappedDEK = e.KEKID, append([]byte(nil), e.WrappedDEK...)
	m.rows[e.ID] = cur
	return true, nil
}

func (m *MemoryStore) ListNotOnKEK(_ context.Context, active string, limit int32) ([]envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []envelope
	for _, e := range m.rows {
		if e.KEKID != active {
			out = append(out, clone(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.seq[out[i].ID] < m.seq[out[j].ID] })
	if intconv.Int32(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryStore) CountByKEK(context.Context) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int64{}
	for _, e := range m.rows {
		out[e.KEKID]++
	}
	return out, nil
}

func (m *MemoryStore) Delete(_ context.Context, id uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.rows[id]
	delete(m.rows, id)
	return ok, nil
}

// raw exposes stored rows to tests.
func (m *MemoryStore) raw() []envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]envelope, 0, len(m.rows))
	for _, e := range m.rows {
		out = append(out, clone(e))
	}
	return out
}
