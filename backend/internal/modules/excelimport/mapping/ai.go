package mapping

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Completer is the AI service subset (internal/modules/ai, #24).
type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, call aiusecase.Call) (aiusecase.Result, error)
}

// ErrAIUnavailable: no AI provider is configured.
var ErrAIUnavailable = errors.New("mapping: AI is not configured")

// AIRequest is what the mapper needs.
type AIRequest struct {
	OrganizationID int64
	TaxpayerID     int64
	UserID         int64
	DataType       string // may be empty: the model guesses
	Sheet          parse.Sheet
	Layout         parse.Layout
}

// AIMapper maps columns with the model. It only maps: it never computes or
// changes values (04 §4.3 rule 1).
type AIMapper struct{ ai Completer }

// NewAIMapper builds the mapper.
func NewAIMapper(ai Completer) *AIMapper { return &AIMapper{ai: ai} }

type aiColumn struct {
	Column     string   `json:"column"`
	Header     string   `json:"header"`
	Field      *string  `json:"field"`
	Confidence *float64 `json:"confidence"`
}

type aiAnswer struct {
	DataType    string     `json:"data_type"`
	SourceGuess string     `json:"source_guess"`
	Columns     []aiColumn `json:"columns"`
	Warnings    []string   `json:"warnings"`
}

const systemPrompt = `Sen bir muhasebe veri eşleme aracısın. Bir Excel/CSV dışa aktarımının başlıklarını ve maskelenmiş örnek satırlarını alırsın; her kolonu verilen hedef alanlardan birine eşlersin. Değer hesaplama, satır üretme, değer değiştirme YOK. Yalnızca şu JSON nesnesini döndür, başka metin yazma:
{"data_type":"outgoing_invoices|incoming_invoices","source_guess":"<program adı veya unknown>","columns":[{"column":"B","header":"...","field":"<alan veya null>","confidence":0.0}],"warnings":["..."]}
Kurallar: her kolon için bir giriş; emin olmadığın veya hedefi olmayan kolonda field=null; bir alan en fazla bir kolona; confidence 0 ile 1 arası. Oran bazında KDV kolonları için "tax_summary[<oran>].base" ve "tax_summary[<oran>].vat" kullan (ör. "KDV %20" -> tax_summary[20].vat).`

// Map asks the model; an invalid answer is retried once (04 §4.3 rule 4).
// On a second failure it returns an all-unmapped mapping for the
// accountant, with a warning, and no error.
func (m *AIMapper) Map(ctx context.Context, req AIRequest) (Mapping, error) {
	if m == nil || m.ai == nil || !m.ai.Enabled() {
		return Mapping{}, ErrAIUnavailable
	}
	headers, rows := MaskSample(req.Sheet, req.Layout)
	sample, _ := json.Marshal(map[string]any{"headers": headers, "rows": rows})
	var fields []string
	for _, f := range InvoiceFields {
		fields = append(fields, f.Key+" ("+f.Description+")")
	}
	user := fmt.Sprintf("Beklenen veri türü: %s\nHedef alanlar:\n- %s\n- tax_summary[<oran>].base / tax_summary[<oran>].vat\nÖrnek (maskeli):\n%s",
		orUnknown(req.DataType), strings.Join(fields, "\n- "), sample)
	var tp *int64
	if req.TaxpayerID > 0 {
		tp = &req.TaxpayerID
	}
	var uid *int64
	if req.UserID > 0 {
		uid = &req.UserID
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		msgs := []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{provider.TextBlock(user)}}}
		if attempt == 1 && lastErr != nil {
			msgs[0].Content = append(msgs[0].Content, provider.TextBlock("Önceki yanıt geçersizdi ("+lastErr.Error()+"). Yalnızca şemaya uyan JSON döndür."))
		}
		res, err := m.ai.Complete(ctx, aiusecase.Call{
			OrganizationID: req.OrganizationID, TaxpayerID: tp, UserID: uid,
			Purpose: "excel_column_mapping", System: []provider.SystemBlock{{Text: systemPrompt, Cache: true}},
			Messages: msgs, MaxTokens: 2048,
		})
		if err != nil {
			return Mapping{}, fmt.Errorf("mapping: ai call: %w", err)
		}
		mp, err := parseAnswer(res.Message.Text(), req)
		if err == nil {
			return mp, nil
		}
		lastErr = err
	}
	return unmapped(req, "AI eşlemesi iki denemede de geçersizdi; eşleme müşavire bırakıldı"), nil
}

func parseAnswer(text string, req AIRequest) (Mapping, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return Mapping{}, errors.New("no JSON object")
	}
	var a aiAnswer
	dec := json.NewDecoder(strings.NewReader(text[i : j+1]))
	if err := dec.Decode(&a); err != nil {
		return Mapping{}, fmt.Errorf("invalid JSON: %v", err)
	}
	dt := req.DataType
	if dt == "" {
		dt = a.DataType
	}
	if !Supported(dt) {
		return Mapping{}, fmt.Errorf("data_type %q", a.DataType)
	}
	byLetter := map[string]aiColumn{}
	for _, c := range a.Columns {
		if c.Confidence == nil || *c.Confidence < 0 || *c.Confidence > 1 {
			return Mapping{}, fmt.Errorf("column %s: confidence out of range", c.Column)
		}
		byLetter[strings.ToUpper(strings.TrimSpace(c.Column))] = c
	}
	mp := Mapping{DataType: dt, SourceSystem: a.SourceGuess, HeaderRow: req.Layout.HeaderRow, Warnings: a.Warnings}
	used := map[string]bool{}
	for _, col := range req.Layout.Columns {
		cm := ColumnMap{Column: col.Letter, Header: col.Header, Origin: OriginAI}
		if c, ok := byLetter[col.Letter]; ok {
			cm.Confidence = *c.Confidence
			if c.Field != nil && *c.Field != "" {
				f := strings.TrimSpace(*c.Field)
				if !ValidField(dt, f) {
					return Mapping{}, fmt.Errorf("column %s: unknown field %q", col.Letter, f)
				}
				if used[f] {
					return Mapping{}, fmt.Errorf("field %q mapped twice", f)
				}
				used[f] = true
				cm.Field = f
			}
		}
		cm.NeedsReview = cm.Field != "" && cm.Confidence < ReviewBelow
		mp.Columns = append(mp.Columns, cm)
	}
	if mp.SourceSystem == "" {
		mp.SourceSystem = "unknown"
	}
	return mp, nil
}

func unmapped(req AIRequest, warning string) Mapping {
	mp := Mapping{DataType: req.DataType, SourceSystem: "unknown", HeaderRow: req.Layout.HeaderRow, Warnings: []string{warning}}
	for _, c := range req.Layout.Columns {
		mp.Columns = append(mp.Columns, ColumnMap{Column: c.Letter, Header: c.Header, Origin: OriginUser, NeedsReview: true})
	}
	return mp
}

// Unmapped is an empty mapping for manual work (no AI, no template).
func Unmapped(dataType string, l parse.Layout, warning string) Mapping {
	return unmapped(AIRequest{DataType: dataType, Layout: l}, warning)
}

func orUnknown(s string) string {
	if s == "" {
		return "bilinmiyor, sen tahmin et"
	}
	return s
}
