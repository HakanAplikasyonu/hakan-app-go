package orgcache

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type fakeEngagementResolver struct {
	mu       sync.Mutex
	calls    int
	assigned map[int64]bool
	err      error
}

func (f *fakeEngagementResolver) ResolveEngagement(_ context.Context, orgID int64, id uuid.UUID, userID int64) (engagementctx.Access, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return engagementctx.Access{}, f.err
	}
	return engagementctx.Access{ID: 1, UUID: id, OrganizationID: orgID, Status: engagementctx.StatusActive, Assigned: f.assigned[userID]}, nil
}

func (f *fakeEngagementResolver) setAssigned(userID int64, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assigned[userID] = v
}

func newEngagementCache(t *testing.T, inner *fakeEngagementResolver) (*EngagementCache, *CachedResolver) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	org := New(&fakeResolver{}, rdb, 0, "test")
	return NewEngagementCache(inner, org), org
}

func TestEngagementCacheHitsAndInvalidates(t *testing.T) {
	ctx := context.Background()
	inner := &fakeEngagementResolver{assigned: map[int64]bool{1: true}}
	c, org := newEngagementCache(t, inner)
	id := uuid.New()

	for range 3 {
		a, err := c.ResolveEngagement(ctx, 7, id, 1)
		if err != nil || !a.Assigned {
			t.Fatalf("resolve: %+v %v", a, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d, want 1 (cached)", inner.calls)
	}

	// Unassign: without invalidation the stale answer would be served.
	inner.setAssigned(1, false)
	if err := org.InvalidateOrganization(ctx, 7); err != nil {
		t.Fatal(err)
	}
	a, err := c.ResolveEngagement(ctx, 7, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Assigned {
		t.Fatal("assignment removal not visible after invalidation")
	}
	if inner.calls != 2 {
		t.Fatalf("inner calls = %d, want 2", inner.calls)
	}
}

func TestEngagementCacheIsolatesOrganizationsAndUsers(t *testing.T) {
	ctx := context.Background()
	inner := &fakeEngagementResolver{assigned: map[int64]bool{1: true}}
	c, org := newEngagementCache(t, inner)
	id := uuid.New()

	if a, _ := c.ResolveEngagement(ctx, 7, id, 1); !a.Assigned {
		t.Fatal("user 1 should be assigned")
	}
	if a, _ := c.ResolveEngagement(ctx, 7, id, 2); a.Assigned {
		t.Fatal("user 2 must not get user 1's cached answer")
	}
	// Invalidating another organization keeps org 7's entries.
	if err := org.InvalidateOrganization(ctx, 8); err != nil {
		t.Fatal(err)
	}
	before := inner.calls
	_, _ = c.ResolveEngagement(ctx, 7, id, 1)
	if inner.calls != before {
		t.Fatal("invalidating org 8 dropped org 7's cache")
	}
}

func TestEngagementCacheDoesNotCacheErrors(t *testing.T) {
	ctx := context.Background()
	inner := &fakeEngagementResolver{assigned: map[int64]bool{}, err: engagementctx.ErrNotFound}
	c, _ := newEngagementCache(t, inner)
	id := uuid.New()
	for range 2 {
		if _, err := c.ResolveEngagement(ctx, 7, id, 1); !errors.Is(err, engagementctx.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}
	if inner.calls != 2 {
		t.Fatalf("inner calls = %d, want 2 (errors are not cached)", inner.calls)
	}
}

func TestEngagementCacheWithoutRedis(t *testing.T) {
	inner := &fakeEngagementResolver{assigned: map[int64]bool{1: true}}
	c := NewEngagementCache(inner, New(&fakeResolver{}, nil, 0, "test"))
	for range 2 {
		_, _ = c.ResolveEngagement(context.Background(), 7, uuid.New(), 1)
	}
	if inner.calls != 2 {
		t.Fatalf("inner calls = %d, want pass-through", inner.calls)
	}
}
