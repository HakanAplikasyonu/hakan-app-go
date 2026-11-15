package methods

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/agent"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// RegisterAgent yerel ajan yöntemlerini bağlar (issue #51).
//
// auth.resume ve auth.login'in döndürdüğü yenileme belirteci ön yüze gitmez:
// sidecar onu `auth.rotated` olayıyla kabuğa yollar, kabuk anahtar zincirine
// yazar ve olayı ön yüze iletmez.
func RegisterAgent(s *rpc.Server, rt *agent.Runtime) {
	s.Handle("agent.status", func(context.Context, json.RawMessage) (any, error) {
		return rt.Status(), nil
	})
	s.Handle("auth.login", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			BaseURL  string `json:"base_url"`
			Email    string `json:"email"`
			Password string `json:"password"`
			TOTPCode string `json:"totp_code"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		if p.Email == "" || p.Password == "" {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "e-posta ve parola gerekli")
		}
		orgs, err := rt.Login(ctx, p.BaseURL, p.Email, p.Password, p.TOTPCode)
		if err != nil {
			return nil, apiErr(err)
		}
		return map[string]any{"organizations": orgs}, nil
	})
	s.Handle("auth.resume", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		if err := rt.Resume(ctx, p.RefreshToken); err != nil {
			return nil, apiErr(err)
		}
		return rt.Status(), nil
	})
	s.Handle("auth.logout", func(context.Context, json.RawMessage) (any, error) {
		rt.Logout()
		return rt.Status(), nil
	})
	s.Handle("agent.organizations", func(ctx context.Context, _ json.RawMessage) (any, error) {
		orgs, err := rt.Client.Organizations(ctx)
		if err != nil {
			return nil, apiErr(err)
		}
		return map[string]any{"organizations": orgs}, nil
	})
	s.Handle("agent.use_organization", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Slug      string `json:"slug"`
			AgentName string `json:"agent_name"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		st, err := rt.UseOrganization(ctx, p.Slug, p.AgentName)
		if err != nil {
			return nil, apiErr(err)
		}
		return st, nil
	})
	s.Handle("agent.engagements", func(ctx context.Context, _ json.RawMessage) (any, error) {
		items, err := rt.Client.Engagements(ctx)
		if err != nil {
			return nil, apiErr(err)
		}
		return map[string]any{"items": items}, nil
	})
	s.Handle("watch.add", func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Path           string `json:"path"`
			EngagementUUID string `json:"engagement_uuid"`
			Label          string `json:"label"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		w, err := rt.AddWatch(p.Path, p.EngagementUUID, p.Label)
		if err != nil {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "%v", err)
		}
		return w, nil
	})
	s.Handle("watch.remove", func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Path string `json:"path"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		if err := rt.RemoveWatch(p.Path); err != nil {
			return nil, err
		}
		return rt.Status(), nil
	})
}

// apiErr API hatasının kodunu protokole taşır (ör. MFA_REQUIRED,
// INVALID_CREDENTIALS); oturum yoksa SIGNED_OUT.
func apiErr(err error) error {
	var ae *agent.APIError
	switch {
	case errors.As(err, &ae):
		return rpc.Errorf(ae.Code, "%s", ae.Message)
	case errors.Is(err, agent.ErrSignedOut):
		return rpc.Errorf("SIGNED_OUT", "oturum açılmalı")
	}
	return rpc.Errorf(rpc.CodeUnavailable, "%v", err)
}
