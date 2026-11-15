package ai

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
)

func TestNewServiceDisabledWithoutKey(t *testing.T) {
	svc := NewService(config.AIConfig{Provider: "anthropic", Model: "claude-opus-5-5", Masking: true}, nil, nil)
	if svc.Enabled() {
		t.Fatal("service without key must be disabled")
	}
}

func TestNewServiceWithFallback(t *testing.T) {
	svc := NewService(config.AIConfig{
		Provider: "anthropic", APIKey: "test-key", Model: "claude-opus-5-5",
		FallbackProvider: "openai_compatible", FallbackBaseURL: "http://localhost:1234/v1", FallbackModel: "local",
		Masking: true, DisableProviderStore: true,
	}, nil, nil)
	p := svc.Privacy()
	if !svc.Enabled() || p.Primary == nil || p.Fallback == nil || p.Fallback.Model != "local" {
		t.Fatalf("privacy = %+v", p)
	}
}

func TestNewServiceSkipsFallbackWithoutModel(t *testing.T) {
	svc := NewService(config.AIConfig{
		Provider: "anthropic", APIKey: "k", Model: "m",
		FallbackProvider: "openai_compatible", FallbackBaseURL: "http://x/v1",
	}, nil, nil)
	if svc.Privacy().Fallback != nil {
		t.Fatal("fallback without model must be skipped")
	}
}
