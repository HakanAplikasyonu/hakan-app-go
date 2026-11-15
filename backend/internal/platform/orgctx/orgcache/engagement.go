package orgcache

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/google/uuid"
)

// EngagementCache caches engagementctx resolutions in Redis under the
// organization's version counter:
//
//	e:{orgID}:{ver}:{userID}:{engagementUUID} -> JSON Access, TTL
//
// Every change that can alter an answer bumps that counter through
// InvalidateOrganization: assigning or unassigning a member, ending an
// engagement, and the membership and role changes the organizations module
// already invalidates on (removing a member also drops their assignments).
// Only successful resolutions are cached; Redis errors fail open.
type EngagementCache struct {
	inner engagementctx.Resolver
	org   *CachedResolver
}

var _ engagementctx.Resolver = (*EngagementCache)(nil)

// NewEngagementCache wraps inner using org's Redis client, TTL and version
// counters. A nil org (or one without Redis) passes straight through.
func NewEngagementCache(inner engagementctx.Resolver, org *CachedResolver) *EngagementCache {
	return &EngagementCache{inner: inner, org: org}
}

// ResolveEngagement implements engagementctx.Resolver.
func (c *EngagementCache) ResolveEngagement(ctx context.Context, orgID int64, id uuid.UUID, userID int64) (engagementctx.Access, error) {
	if c.org == nil || c.org.rdb == nil {
		return c.inner.ResolveEngagement(ctx, orgID, id, userID)
	}
	ver, err := c.org.version(ctx, orgID)
	if err != nil {
		return c.inner.ResolveEngagement(ctx, orgID, id, userID)
	}
	key := fmt.Sprintf("%se:%d:%s:%d:%s", c.org.prefix, orgID, ver, userID, id)
	if raw, err := c.org.rdb.Get(ctx, key).Bytes(); err == nil {
		var a engagementctx.Access
		if json.Unmarshal(raw, &a) == nil {
			return a, nil
		}
	}
	a, err := c.inner.ResolveEngagement(ctx, orgID, id, userID)
	if err != nil {
		return a, err
	}
	if raw, err := json.Marshal(a); err == nil {
		c.org.rdb.Set(ctx, key, raw, c.org.ttl)
	}
	return a, nil
}
