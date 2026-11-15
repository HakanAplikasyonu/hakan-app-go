package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/tools"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

type memStore struct {
	mu   sync.Mutex
	recs []UsageRecord
	err  error
}

func (m *memStore) RecordUsage(_ context.Context, r UsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, r)
	return m.err
}

func (m *memStore) UsageReport(context.Context, int64, time.Time, time.Time) ([]ReportRow, error) {
	return []ReportRow{}, nil
}

func textResp(text string, model string) provider.Response {
	return provider.Response{
		Message:    provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{provider.TextBlock(text)}},
		StopReason: provider.StopEndTurn,
		Usage:      provider.Usage{InputTokens: 1000, OutputTokens: 100},
		Model:      model,
	}
}

func userMsg(text string) []provider.Message {
	return []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{provider.TextBlock(text)}}}
}

func TestCompleteMasksRequestAndUnmasksAnswer(t *testing.T) {
	fake := &provider.Fake{Responses: []provider.Response{textResp("[AD_1] için [VKN_1] doğrulandı, IBAN [IBAN_1].", "claude-opus-5-5")}}
	store := &memStore{}
	svc := New(&Backend{Provider: fake, Model: "claude-opus-5-5"}, nil, store, Options{Masking: true})

	taxpayer := int64(7)
	res, err := svc.Complete(context.Background(), Call{
		OrganizationID: 42, TaxpayerID: &taxpayer, Purpose: "test",
		System:   []provider.SystemBlock{{Text: "Mükellef: Demir Gıda A.Ş."}},
		Messages: userMsg("Demir Gıda A.Ş. VKN 1234567890 IBAN TR330006100519786457841326"),
		Names:    []string{"Demir Gıda A.Ş."},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent, _ := json.Marshal(fake.Requests[0])
	for _, leak := range []string{"1234567890", "TR33", "Demir"} {
		if strings.Contains(string(sent), leak) {
			t.Fatalf("%q reached the provider: %s", leak, sent)
		}
	}
	want := "Demir Gıda A.Ş. için 1234567890 doğrulandı, IBAN TR330006100519786457841326."
	if got := res.Message.Text(); got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if res.Masked != 3 {
		t.Fatalf("masked = %d", res.Masked)
	}
	if len(store.recs) != 1 {
		t.Fatalf("records = %d", len(store.recs))
	}
	rec := store.recs[0]
	if rec.OrganizationID != 42 || rec.TaxpayerID == nil || *rec.TaxpayerID != 7 || rec.Fallback || rec.Purpose != "test" {
		t.Fatalf("record = %+v", rec)
	}
	// 1000 x 4 + 100 x 20 milli-USD/MTok... = 0.006 USD
	if rec.CostUSD != "0.006000" || rec.MaskedValues != 3 {
		t.Fatalf("cost = %q masked = %d", rec.CostUSD, rec.MaskedValues)
	}
}

func TestCompleteWithoutMaskingSendsRawText(t *testing.T) {
	fake := &provider.Fake{Responses: []provider.Response{textResp("ok", "m")}}
	svc := New(&Backend{Provider: fake, Model: "m"}, nil, nil, Options{Masking: false})
	if _, err := svc.Complete(context.Background(), Call{OrganizationID: 1, Messages: userMsg("VKN 1234567890")}); err != nil {
		t.Fatal(err)
	}
	if got := fake.Requests[0].Messages[0].Text(); got != "VKN 1234567890" {
		t.Fatalf("sent %q", got)
	}
}

func TestCompleteFallsBackAndRecordsFallback(t *testing.T) {
	primary := &provider.Fake{Err: errors.New("overloaded")}
	fallback := &provider.Fake{Responses: []provider.Response{textResp("yedek", "")}}
	store := &memStore{}
	svc := New(
		&Backend{Provider: primary, Model: "claude-opus-5-5"},
		&Backend{Provider: fallback, Model: "local-qwen"},
		store, Options{Masking: true},
	)
	res, err := svc.Complete(context.Background(), Call{OrganizationID: 1, Messages: userMsg("merhaba")})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fallback || res.Model != "local-qwen" || res.Message.Text() != "yedek" {
		t.Fatalf("result = %+v", res)
	}
	if fallback.Requests[0].Model != "local-qwen" {
		t.Fatalf("fallback model = %q", fallback.Requests[0].Model)
	}
	rec := store.recs[0]
	if !rec.Fallback || rec.CostUSD != "" || rec.Purpose != "unspecified" {
		t.Fatalf("record = %+v (unknown model must have empty cost)", rec)
	}
}

func TestCompleteNoFallbackReturnsError(t *testing.T) {
	store := &memStore{}
	svc := New(&Backend{Provider: &provider.Fake{Err: errors.New("down")}, Model: "m"}, nil, store, Options{})
	if _, err := svc.Complete(context.Background(), Call{OrganizationID: 1, Messages: userMsg("x")}); err == nil {
		t.Fatal("want error")
	}
	if len(store.recs) != 0 {
		t.Fatal("failed call must not be recorded")
	}
}

func TestCompleteLedgerFailureStillReturnsAnswer(t *testing.T) {
	fake := &provider.Fake{Responses: []provider.Response{textResp("ok", "m")}}
	svc := New(&Backend{Provider: fake, Model: "m"}, nil, &memStore{err: errors.New("db down")}, Options{})
	if _, err := svc.Complete(context.Background(), Call{OrganizationID: 1, Messages: userMsg("x")}); err != nil {
		t.Fatalf("ledger failure leaked: %v", err)
	}
}

func TestCompleteGuards(t *testing.T) {
	if _, err := New(nil, nil, nil, Options{}).Complete(context.Background(), Call{OrganizationID: 1}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
	svc := New(&Backend{Provider: &provider.Fake{}, Model: "m"}, nil, nil, Options{})
	if _, err := svc.Complete(context.Background(), Call{}); !errors.Is(err, ErrMissingOrganization) {
		t.Fatalf("err = %v", err)
	}
}

type lookupTool struct {
	gotInput string
	kind     tools.Kind
}

func (l *lookupTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "lookup_party", Kind: l.kind,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"vkn": map[string]any{"type": "string"}},
			"required":   []string{"vkn"},
		},
	}
}

func (l *lookupTool) Run(_ context.Context, _ tools.Env, in json.RawMessage) (tools.Result, error) {
	l.gotInput = string(in)
	return tools.Result{Content: `{"vkn":"1234567890","title":"Demir Gıda A.Ş.","iban":"TR330006100519786457841326"}`}, nil
}

func TestRunToolsUnmasksInputAndMasksResult(t *testing.T) {
	toolUse := provider.Response{
		Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{
			{Type: provider.BlockToolUse, ID: "tu1", Name: "lookup_party", Input: json.RawMessage(`{"vkn":"[VKN_1]"}`)},
		}},
		StopReason: provider.StopToolUse,
		Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
		Model:      "claude-opus-5-5",
	}
	fake := &provider.Fake{Responses: []provider.Response{toolUse, textResp("tamam [IBAN_1]", "claude-opus-5-5")}}
	store := &memStore{}
	svc := New(&Backend{Provider: fake, Model: "claude-opus-5-5"}, nil, store, Options{Masking: true})
	tool := &lookupTool{kind: tools.KindRead}

	res, err := svc.RunTools(context.Background(),
		Call{OrganizationID: 1, Messages: userMsg("1234567890 kim?"), Names: []string{"Demir Gıda A.Ş."}},
		tools.NewRegistry(tool), tools.Env{Scope: orgctx.Scope{}}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if tool.gotInput != `{"vkn":"1234567890"}` {
		t.Fatalf("tool got %s, want unmasked input", tool.gotInput)
	}
	second, _ := json.Marshal(fake.Requests[1])
	for _, leak := range []string{"1234567890", "Demir", "TR33"} {
		if strings.Contains(string(second), leak) {
			t.Fatalf("%q leaked in tool round: %s", leak, second)
		}
	}
	if len(fake.Requests[0].Tools) != 1 {
		t.Fatalf("tools offered = %d", len(fake.Requests[0].Tools))
	}
	if res.Message.Text() != "tamam TR330006100519786457841326" {
		t.Fatalf("answer = %q", res.Message.Text())
	}
	if res.Usage.InputTokens != 1010 || len(store.recs) != 2 {
		t.Fatalf("usage = %+v recs = %d", res.Usage, len(store.recs))
	}
}

func TestRunToolsRefusesWriteTools(t *testing.T) {
	toolUse := provider.Response{
		Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{
			{Type: provider.BlockToolUse, ID: "tu1", Name: "lookup_party", Input: json.RawMessage(`{"vkn":"x"}`)},
		}},
		StopReason: provider.StopToolUse,
	}
	fake := &provider.Fake{Responses: []provider.Response{toolUse, textResp("ok", "m")}}
	svc := New(&Backend{Provider: fake, Model: "m"}, nil, nil, Options{})
	tool := &lookupTool{kind: tools.KindWrite}
	if _, err := svc.RunTools(context.Background(), Call{OrganizationID: 1, Messages: userMsg("x")},
		tools.NewRegistry(tool), tools.Env{}, 3); err != nil {
		t.Fatal(err)
	}
	if tool.gotInput != "" {
		t.Fatal("write tool was run")
	}
	last := fake.Requests[1].Messages[len(fake.Requests[1].Messages)-1].Content[0]
	if !last.IsError {
		t.Fatalf("want error tool_result, got %+v", last)
	}
}

func TestPrivacyStatement(t *testing.T) {
	svc := New(
		&Backend{Provider: provider.NewAnthropic(provider.Config{APIKey: "x"}), Model: "claude-opus-5-5"},
		&Backend{Provider: provider.NewOpenAICompatible(provider.Config{BaseURL: "http://x"}), Model: "q"},
		nil, Options{Masking: true, DisableProviderStore: true},
	)
	p := svc.Privacy()
	if !p.Enabled || !p.Masking || !p.Primary.NoTraining || !p.Fallback.NoTraining || p.Fallback.Basis != "store_false" {
		t.Fatalf("privacy = %+v primary=%+v fallback=%+v", p, p.Primary, p.Fallback)
	}
	svc = New(&Backend{Provider: provider.NewOpenAICompatible(provider.Config{BaseURL: "http://x"}), Model: "q"}, nil, nil, Options{})
	if svc.Privacy().Primary.NoTraining {
		t.Fatal("openai without store=false must not claim no-training")
	}
}
