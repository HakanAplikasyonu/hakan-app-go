package httpserver

import (
	"context"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
)

// newRateLimiter builds the shared limiter from RATE_LIMIT_* / LOGIN_BACKOFF_*
// and sends abuse events to the activity log as security.<kind> (#180).
func newRateLimiter(cfg config.Config, deps Deps, rec *activity.Recorder) *ratelimit.Limiter {
	rules := make(map[string]ratelimit.Rule, len(cfg.RateLimit.Rules))
	for action, r := range cfg.RateLimit.Rules {
		rules[action] = ratelimit.Rule{Limit: r.Limit, Window: r.Window}
	}
	l := ratelimit.NewWithConfig(deps.Redis, cfg.App.Env, ratelimit.Config{
		Store: cfg.RateLimit.Store,
		Rules: rules,
		Backoff: ratelimit.Backoff{
			Threshold:   cfg.RateLimit.BackoffThreshold,
			IPThreshold: cfg.RateLimit.BackoffIPThreshold,
			Base:        cfg.RateLimit.BackoffBase,
			Max:         cfg.RateLimit.BackoffMax,
			Window:      cfg.RateLimit.BackoffWindow,
		},
	})
	if rec != nil {
		l.SetAbuseHook(func(r *http.Request, ev ratelimit.Event) {
			payload := map[string]any{
				"action":              ev.Action,
				"subject":             ev.Subject,
				"retry_after_seconds": int(ev.Retry.Seconds()),
			}
			if ev.Email != "" {
				payload["email"] = ev.Email
			}
			ctx := context.Background()
			if r != nil {
				ctx = r.Context()
				payload["method"] = r.Method
				payload["path"] = r.URL.Path
			}
			// The IP is stored in the event's own column by the recorder.
			rec.Record(ctx, nil, "security."+ev.Kind, "security", nil, payload, r)
		})
	}
	return l
}
