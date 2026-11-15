package ai

import (
	"errors"
	"log/slog"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
)

// NewService builds the AI service from configuration. A missing primary key
// yields a disabled service (calls return usecase.ErrDisabled); a broken
// fallback is logged and skipped so it never blocks the primary.
func NewService(cfg config.AIConfig, store usecase.UsageStore, log *slog.Logger) *usecase.Service {
	if log == nil {
		log = slog.Default()
	}
	var primary, fallback *usecase.Backend
	if p, err := provider.New(provider.Config{
		Kind: cfg.Provider, APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, DisableStore: cfg.DisableProviderStore,
	}); err == nil {
		primary = &usecase.Backend{Provider: p, Model: cfg.Model}
	} else if !errors.Is(err, provider.ErrNotConfigured) {
		log.Warn("ai: primary provider not usable", "provider", cfg.Provider, "err", err)
	}
	if cfg.FallbackProvider != "" {
		p, err := provider.New(provider.Config{
			Kind: cfg.FallbackProvider, APIKey: cfg.FallbackAPIKey, BaseURL: cfg.FallbackBaseURL,
			DisableStore: cfg.DisableProviderStore,
		})
		switch {
		case err != nil:
			log.Warn("ai: fallback provider not usable", "provider", cfg.FallbackProvider, "err", err)
		case cfg.FallbackModel == "":
			log.Warn("ai: fallback provider has no AI_FALLBACK_MODEL; skipped", "provider", cfg.FallbackProvider)
		default:
			fallback = &usecase.Backend{Provider: p, Model: cfg.FallbackModel}
		}
	}
	return usecase.New(primary, fallback, store, usecase.Options{
		Masking:              cfg.Masking,
		DisableProviderStore: cfg.DisableProviderStore,
		MaxTokens:            cfg.MaxTokens,
		Logger:               log,
	})
}
