// Package usecase is the AI module's service layer: one entry point that
// masks the request, calls the primary provider (falling back to the
// secondary one on failure), unmasks the answer and records the call's cost
// against the office and taxpayer (docs/01 G11, issue #24).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/mask"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/tools"
)

// ErrDisabled is returned when no primary provider is configured.
var ErrDisabled = errors.New("ai: no provider configured")

// ErrMissingOrganization is returned for a call without an office: every
// call is billed to an office.
var ErrMissingOrganization = errors.New("ai: call has no organization")

// Backend is one configured provider + model.
type Backend struct {
	Provider provider.Provider
	Model    string
}

// UsageRecord is one row of the cost ledger.
type UsageRecord struct {
	OrganizationID int64
	TaxpayerID     *int64
	UserID         *int64
	Purpose        string
	Provider       string
	Model          string
	Fallback       bool
	Usage          provider.Usage
	// CostUSD is a 6-decimal string; empty when the model has no known price.
	CostUSD      string
	MaskedValues int
}

// ReportRow is one month x taxpayer x model line of the office cost report.
type ReportRow struct {
	Month            string `json:"month"`
	TaxpayerID       *int64 `json:"taxpayer_id"`
	Model            string `json:"model"`
	Calls            int64  `json:"calls"`
	FallbackCalls    int64  `json:"fallback_calls"`
	UnpricedCalls    int64  `json:"unpriced_calls"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CostUSD          string `json:"cost_usd"`
}

// UsageStore persists and reports AI usage.
type UsageStore interface {
	RecordUsage(ctx context.Context, rec UsageRecord) error
	UsageReport(ctx context.Context, organizationID int64, from, to time.Time) ([]ReportRow, error)
}

// Options tune the service.
type Options struct {
	// Masking replaces identifiers before a request leaves the server.
	Masking bool
	// DisableProviderStore records that OpenAI-compatible backends get
	// store=false (reported by Privacy).
	DisableProviderStore bool
	MaxTokens            int
	Logger               *slog.Logger
}

// Service runs AI calls.
type Service struct {
	primary  *Backend
	fallback *Backend
	store    UsageStore
	opts     Options
	log      *slog.Logger
}

// New builds the service. A nil primary (or one without a provider) yields
// a disabled service whose calls return ErrDisabled.
func New(primary, fallback *Backend, store UsageStore, opts Options) *Service {
	if primary != nil && primary.Provider == nil {
		primary = nil
	}
	if fallback != nil && fallback.Provider == nil {
		fallback = nil
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 4096
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Service{primary: primary, fallback: fallback, store: store, opts: opts, log: log}
}

// Enabled reports whether a primary provider is configured.
func (s *Service) Enabled() bool { return s != nil && s.primary != nil }

// Call is one model request on behalf of an office (and optionally a taxpayer).
type Call struct {
	OrganizationID int64
	TaxpayerID     *int64
	UserID         *int64
	// Purpose names the feature for the cost report (e.g. "account_code_suggestion").
	Purpose  string
	System   []provider.SystemBlock
	Messages []provider.Message
	Tools    []provider.ToolDef
	// MaxTokens overrides Options.MaxTokens when > 0.
	MaxTokens int
	// Names are extra names to mask (taxpayer title, counterparties, contacts).
	Names []string
	// Masker carries tokens across calls of one conversation; nil = new one.
	Masker *mask.Masker
}

// Result is a completed call with the answer unmasked.
type Result struct {
	Message    provider.Message
	StopReason string
	Usage      provider.Usage
	Model      string
	Provider   string
	Fallback   bool
	// Masked is the number of distinct values masked in the request.
	Masked int
}

// Complete runs one call.
func (s *Service) Complete(ctx context.Context, call Call) (Result, error) {
	if !s.Enabled() {
		return Result{}, ErrDisabled
	}
	if call.OrganizationID == 0 {
		return Result{}, ErrMissingOrganization
	}
	m := s.masker(call)
	req := provider.Request{
		System:    call.System,
		Messages:  call.Messages,
		Tools:     call.Tools,
		MaxTokens: call.MaxTokens,
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = s.opts.MaxTokens
	}
	if m != nil {
		req = maskRequest(m, req)
	}

	backend, isFallback := s.primary, false
	resp, err := s.call(ctx, backend, req)
	if err != nil && s.fallback != nil && ctx.Err() == nil {
		s.log.WarnContext(ctx, "ai primary provider failed; trying fallback",
			"provider", s.primary.Provider.Kind(), "model", s.primary.Model, "err", err)
		backend, isFallback = s.fallback, true
		resp, err = s.call(ctx, backend, req)
	}
	if err != nil {
		return Result{}, err
	}

	masked := 0
	if m != nil {
		resp.Message = unmaskMessage(m, resp.Message)
		masked = m.Total()
	}
	model := resp.Model
	if model == "" {
		model = backend.Model
	}
	out := Result{
		Message:    resp.Message,
		StopReason: resp.StopReason,
		Usage:      resp.Usage,
		Model:      model,
		Provider:   backend.Provider.Kind(),
		Fallback:   isFallback,
		Masked:     masked,
	}
	s.record(ctx, call, out)
	return out, nil
}

// RunTools runs a tool loop: the model may call read tools from reg (only
// those the scope allows) up to maxSteps rounds. Tool inputs are unmasked
// before Run and tool results are masked again before they go back to the
// model. Write tools are refused in the loop; they belong in the review
// queue. Every round is a separate Complete, so every round is billed.
func (s *Service) RunTools(ctx context.Context, call Call, reg *tools.Registry, env tools.Env, maxSteps int) (Result, error) {
	if maxSteps <= 0 {
		maxSteps = 5
	}
	if call.Masker == nil {
		call.Masker = s.masker(call)
	}
	available := map[string]tools.Tool{}
	call.Tools = nil
	if reg != nil {
		for _, t := range reg.Available(env.Scope) {
			spec := t.Spec()
			available[spec.Name] = t
			call.Tools = append(call.Tools, provider.ToolDef{
				Name: spec.Name, Description: spec.Description, InputSchema: spec.InputSchema,
			})
		}
	}
	msgs := append([]provider.Message(nil), call.Messages...)
	var total provider.Usage
	var last Result
	for step := 0; step < maxSteps; step++ {
		call.Messages = msgs
		res, err := s.Complete(ctx, call)
		if err != nil {
			return Result{}, err
		}
		total.Add(res.Usage)
		last = res
		uses := res.Message.ToolUses()
		if res.StopReason != provider.StopToolUse || len(uses) == 0 {
			break
		}
		msgs = append(msgs, res.Message)
		results := make([]provider.Block, 0, len(uses))
		for _, use := range uses {
			results = append(results, s.runTool(ctx, available, env, use))
		}
		msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: results})
	}
	last.Usage = total
	return last, nil
}

func (s *Service) runTool(ctx context.Context, available map[string]tools.Tool, env tools.Env, use provider.Block) provider.Block {
	t, ok := available[use.Name]
	if !ok {
		return provider.ToolResultBlock(use.ID, "unknown or unavailable tool: "+use.Name, true)
	}
	spec := t.Spec()
	if spec.Kind == tools.KindWrite {
		return provider.ToolResultBlock(use.ID, "write tools are not run directly; propose the change instead", true)
	}
	if err := tools.Validate(spec.InputSchema, use.Input); err != nil {
		return provider.ToolResultBlock(use.ID, err.Error(), true)
	}
	r, err := t.Run(ctx, env, use.Input)
	if err != nil {
		s.log.WarnContext(ctx, "ai tool failed", "tool", use.Name, "err", err)
		return provider.ToolResultBlock(use.ID, "tool failed", true)
	}
	return provider.ToolResultBlock(use.ID, r.Content, r.IsError)
}

// Privacy describes the data-handling settings of the configured providers
// (the "your data is not used for model training" statement, docs/01 §3).
type Privacy struct {
	Enabled     bool             `json:"enabled"`
	Masking     bool             `json:"masking"`
	MaskedKinds []string         `json:"masked_kinds"`
	Primary     *ProviderPrivacy `json:"primary"`
	Fallback    *ProviderPrivacy `json:"fallback"`
}

// ProviderPrivacy is one provider's statement.
type ProviderPrivacy struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// NoTraining is true when the provider does not train on request data
	// under the configured settings.
	NoTraining bool `json:"no_training"`
	// Basis says why: the provider's API terms or the store=false flag.
	Basis string `json:"basis"`
}

// Privacy returns the current data-handling statement.
func (s *Service) Privacy() Privacy {
	p := Privacy{
		Enabled:     s.Enabled(),
		Masking:     s.opts.Masking,
		MaskedKinds: []string{string(mask.KindVKN), string(mask.KindTCKN), string(mask.KindIBAN), string(mask.KindName)},
	}
	if s.primary != nil {
		p.Primary = s.providerPrivacy(s.primary)
	}
	if s.fallback != nil {
		p.Fallback = s.providerPrivacy(s.fallback)
	}
	return p
}

func (s *Service) providerPrivacy(b *Backend) *ProviderPrivacy {
	out := &ProviderPrivacy{Provider: b.Provider.Kind(), Model: b.Model}
	switch b.Provider.Kind() {
	case provider.KindAnthropic:
		out.NoTraining = true
		out.Basis = "anthropic_commercial_terms"
	case provider.KindOpenAICompatible:
		out.NoTraining = s.opts.DisableProviderStore
		if out.NoTraining {
			out.Basis = "store_false"
		} else {
			out.Basis = "not_guaranteed"
		}
	default:
		out.Basis = "unknown"
	}
	return out
}

// Report returns the office's cost report for [from, to).
func (s *Service) Report(ctx context.Context, organizationID int64, from, to time.Time) ([]ReportRow, error) {
	if s.store == nil {
		return []ReportRow{}, nil
	}
	if !to.After(from) {
		return nil, fmt.Errorf("ai: report range is empty")
	}
	return s.store.UsageReport(ctx, organizationID, from, to)
}

func (s *Service) call(ctx context.Context, b *Backend, req provider.Request) (provider.Response, error) {
	req.Model = b.Model
	return b.Provider.Complete(ctx, req)
}

func (s *Service) masker(call Call) *mask.Masker {
	if !s.opts.Masking {
		return nil
	}
	if call.Masker != nil {
		call.Masker.AddNames(call.Names...)
		return call.Masker
	}
	return mask.New(call.Names...)
}

func (s *Service) record(ctx context.Context, call Call, r Result) {
	if s.store == nil {
		return
	}
	rec := UsageRecord{
		OrganizationID: call.OrganizationID,
		TaxpayerID:     call.TaxpayerID,
		UserID:         call.UserID,
		Purpose:        strings.TrimSpace(call.Purpose),
		Provider:       r.Provider,
		Model:          r.Model,
		Fallback:       r.Fallback,
		Usage:          r.Usage,
		MaskedValues:   r.Masked,
	}
	if rec.Purpose == "" {
		rec.Purpose = "unspecified"
	}
	if nano, ok := EstimateCostNanoUSD(r.Model, r.Usage); ok {
		rec.CostUSD = FormatUSD(nano)
	}
	// The answer is already paid for: a ledger failure is logged, not
	// returned, so the caller still gets its result.
	if err := s.store.RecordUsage(context.WithoutCancel(ctx), rec); err != nil {
		s.log.ErrorContext(ctx, "ai usage record failed", "organization_id", call.OrganizationID, "err", err)
	}
}

func maskRequest(m *mask.Masker, req provider.Request) provider.Request {
	sys := make([]provider.SystemBlock, len(req.System))
	for i, b := range req.System {
		b.Text = m.Mask(b.Text)
		sys[i] = b
	}
	req.System = sys
	msgs := make([]provider.Message, len(req.Messages))
	for i, msg := range req.Messages {
		blocks := make([]provider.Block, len(msg.Content))
		for j, b := range msg.Content {
			b.Text = m.Mask(b.Text)
			b.Content = m.Mask(b.Content)
			if len(b.Input) > 0 {
				b.Input = maskJSON(b.Input, m.Mask)
			}
			blocks[j] = b
		}
		msgs[i] = provider.Message{Role: msg.Role, Content: blocks}
	}
	req.Messages = msgs
	return req
}

func unmaskMessage(m *mask.Masker, msg provider.Message) provider.Message {
	blocks := make([]provider.Block, len(msg.Content))
	for i, b := range msg.Content {
		b.Text = m.Unmask(b.Text)
		if len(b.Input) > 0 {
			b.Input = maskJSON(b.Input, m.Unmask)
		}
		blocks[i] = b
	}
	msg.Content = blocks
	return msg
}

// maskJSON applies fn to every string value of a JSON document (so the
// result stays valid JSON). Non-JSON input is returned unchanged.
func maskJSON(raw json.RawMessage, fn func(string) string) json.RawMessage {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.Marshal(walkStrings(v, fn))
	if err != nil {
		return raw
	}
	return out
}

func walkStrings(v any, fn func(string) string) any {
	switch t := v.(type) {
	case string:
		return fn(t)
	case []any:
		for i := range t {
			t[i] = walkStrings(t[i], fn)
		}
		return t
	case map[string]any:
		for k, val := range t {
			t[k] = walkStrings(val, fn)
		}
		return t
	default:
		return v
	}
}
