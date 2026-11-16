package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/mask"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/redact"
)

// Image redaction policies (AI_IMAGE_REDACTION, issue #169).
const (
	// RedactionRequired: images are sent only after their personal data is
	// blacked out; without an OCR engine image calls are refused.
	RedactionRequired = "required"
	// RedactionOff: images are sent as they are.
	RedactionOff = "off"
)

// ErrImageRedactionUnavailable: masking is on, the policy requires image
// redaction and no OCR engine is configured, so the image is not sent.
var ErrImageRedactionUnavailable = errors.New("ai: image redaction is required but no OCR engine is configured")

// ImageRedactor blacks out personal data in an image. *redact.Redactor
// implements it.
type ImageRedactor interface {
	Redact(ctx context.Context, m *mask.Masker, mediaType string, data []byte) (redact.Result, error)
}

func (s *Service) redactionPolicy() string {
	if s.opts.ImageRedaction == RedactionOff {
		return RedactionOff
	}
	return RedactionRequired
}

// redactImages replaces every image block with its redacted copy followed by
// a note naming the tokens of the blacked-out regions.
func (s *Service) redactImages(ctx context.Context, m *mask.Masker, req provider.Request) (provider.Request, error) {
	if s.redactionPolicy() == RedactionOff || !hasImages(req) {
		return req, nil
	}
	if s.opts.ImageRedactor == nil {
		return req, ErrImageRedactionUnavailable
	}
	msgs := make([]provider.Message, len(req.Messages))
	for i, msg := range req.Messages {
		blocks := make([]provider.Block, 0, len(msg.Content))
		for _, b := range msg.Content {
			if b.Type != provider.BlockImage {
				blocks = append(blocks, b)
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(b.Data)
			if err != nil {
				return req, fmt.Errorf("ai: image is not base64: %w", err)
			}
			res, err := s.opts.ImageRedactor.Redact(ctx, m, b.MediaType, raw)
			if err != nil {
				return req, fmt.Errorf("ai: image redaction: %w", err)
			}
			blocks = append(blocks, provider.ImageBlock(res.MediaType, base64.StdEncoding.EncodeToString(res.Data)))
			if note := redact.Note(res.Regions); note != "" {
				blocks = append(blocks, provider.TextBlock(note))
			}
		}
		msgs[i] = provider.Message{Role: msg.Role, Content: blocks}
	}
	req.Messages = msgs
	return req, nil
}

func hasImages(req provider.Request) bool {
	for _, msg := range req.Messages {
		for _, b := range msg.Content {
			if b.Type == provider.BlockImage {
				return true
			}
		}
	}
	return false
}
