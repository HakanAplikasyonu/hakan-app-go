package usecase

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory Store with the same rules as Postgres
// (one pending suggestion per target field, transactional rollback). It
// backs the unit tests.
type MemoryStore struct {
	mu        sync.Mutex
	rows      []Suggestion
	decisions map[int64][]Decision
	settings  []SettingRow
	nextID    int64
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{decisions: map[int64][]Decision{}} }

type memTx struct{ m *MemoryStore }

// InTx implements Store; a failing fn leaves the store unchanged.
func (m *MemoryStore) InTx(_ context.Context, fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := append([]Suggestion(nil), m.rows...)
	dec := map[int64][]Decision{}
	for k, v := range m.decisions {
		dec[k] = append([]Decision(nil), v...)
	}
	next := m.nextID
	if err := fn(memTx{m}); err != nil {
		m.rows, m.decisions, m.nextID = rows, dec, next
		return err
	}
	return nil
}

func (t memTx) SupersedePending(_ context.Context, s Subject, in Input) ([]Superseded, error) {
	var out []Superseded
	for i := range t.m.rows {
		r := &t.m.rows[i]
		if r.Status == StatusPending && r.EngagementID == s.EngagementID && sameTarget(r.Input, in) {
			r.Status = StatusSuperseded
			out = append(out, Superseded{ID: r.ID, Confidence: r.Input.Confidence})
		}
	}
	return out, nil
}

func (t memTx) Insert(_ context.Context, s Subject, in Input) (Suggestion, error) {
	t.m.nextID++
	now := time.Now()
	sug := Suggestion{ID: t.m.nextID, Subject: s, UUID: uuid.New(), Input: in, Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	t.m.rows = append(t.m.rows, sug)
	return sug, nil
}

func (t memTx) GetForUpdate(_ context.Context, s Subject, id uuid.UUID) (Suggestion, error) {
	for _, r := range t.m.rows {
		if r.UUID == id && r.OrganizationID == s.OrganizationID && r.EngagementID == s.EngagementID {
			return r, nil
		}
	}
	return Suggestion{}, ErrNotFound
}

func (t memTx) UpdateDecision(_ context.Context, sug Suggestion) (Suggestion, error) {
	for i := range t.m.rows {
		if t.m.rows[i].ID == sug.ID {
			now := time.Now()
			sug.DecidedAt, sug.UpdatedAt = &now, now
			t.m.rows[i] = sug
			return sug, nil
		}
	}
	return Suggestion{}, ErrNotFound
}

func (t memTx) InsertDecision(_ context.Context, _ Subject, id int64, d Decision) error {
	d.CreatedAt = time.Now()
	t.m.decisions[id] = append(t.m.decisions[id], d)
	return nil
}

func (t memTx) Handle() any { return nil }

// Get implements Store.
func (m *MemoryStore) Get(_ context.Context, s Subject, id uuid.UUID) (Suggestion, []Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.UUID == id && r.OrganizationID == s.OrganizationID && r.EngagementID == s.EngagementID {
			return r, append([]Decision(nil), m.decisions[r.ID]...), nil
		}
	}
	return Suggestion{}, nil, ErrNotFound
}

// List implements Store (no assignment filtering: tests pass AllEngagements).
func (m *MemoryStore) List(_ context.Context, f Filter) ([]ListRow, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ListRow
	for _, r := range m.rows {
		if r.OrganizationID != f.OrganizationID ||
			(f.EngagementID > 0 && r.EngagementID != f.EngagementID) ||
			(f.Status != "" && string(r.Status) != f.Status) ||
			(f.Kind != "" && r.Input.Kind != f.Kind) ||
			(f.RuleCandidate != nil && r.RuleCandidate != *f.RuleCandidate) {
			continue
		}
		out = append(out, ListRow{Suggestion: r})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Input.Confidence.LessThan(out[j].Input.Confidence)
	})
	return out, int64(len(out)), nil
}

// Settings implements Store.
func (m *MemoryStore) Settings(_ context.Context, _, engagementID int64) ([]SettingRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []SettingRow
	for _, r := range m.settings {
		if r.EngagementID == 0 || r.EngagementID == engagementID {
			out = append(out, r)
		}
	}
	return out, nil
}

// UpsertSetting implements Store.
func (m *MemoryStore) UpsertSetting(_ context.Context, _, engagementID int64, row SettingRow, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.EngagementID = engagementID
	for i, r := range m.settings {
		if r.EngagementID == engagementID && r.Kind == row.Kind {
			m.settings[i] = row
			return nil
		}
	}
	m.settings = append(m.settings, row)
	return nil
}

// DeleteSetting implements Store.
func (m *MemoryStore) DeleteSetting(_ context.Context, _, engagementID int64, kind string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.settings {
		if r.EngagementID == engagementID && r.Kind == kind {
			m.settings = append(m.settings[:i], m.settings[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func sameTarget(a, b Input) bool {
	return a.Kind == b.Kind && a.TargetType == b.TargetType && a.TargetField == b.TargetField &&
		((a.TargetID == nil && b.TargetID == nil) || (a.TargetID != nil && b.TargetID != nil && *a.TargetID == *b.TargetID))
}
