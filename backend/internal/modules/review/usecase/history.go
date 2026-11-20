package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CodeUse is one account code a counterparty's lines were booked to (#669 L).
type CodeUse struct {
	AccountCode string    `json:"account_code"`
	Uses        int64     `json:"uses"`
	LastUsedAt  time.Time `json:"last_used_at"`
}

// HistoryStore is implemented by stores that can aggregate decided
// suggestions per counterparty. Kept apart from Store so test fakes need
// not implement it.
type HistoryStore interface {
	CounterpartyHistory(ctx context.Context, organizationID, engagementID int64, counterpartyKey string, limit int32) ([]CodeUse, error)
}

// CounterpartyHistory: the engagement's line account codes for one
// counterparty (provenance input counterparty_key), most used first, over all
// decisions, not only the queue's first page.
func (s *Service) CounterpartyHistory(ctx context.Context, sub Subject, counterpartyKey string, limit int32) ([]CodeUse, error) {
	key := strings.TrimSpace(counterpartyKey)
	if key == "" {
		return nil, fmt.Errorf("%w: counterparty is required", ErrInvalid)
	}
	hs, ok := s.store.(HistoryStore)
	if !ok {
		return []CodeUse{}, nil
	}
	return hs.CounterpartyHistory(ctx, sub.OrganizationID, sub.EngagementID, key, limit)
}
