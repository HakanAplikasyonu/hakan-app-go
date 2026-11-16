package testfakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// AI fakes an OpenAI-compatible chat completions server (AI_PROVIDER
// openai_compatible, AI_BASE_URL = server URL). It answers by what the
// backend asks (the system prompt and whether an image is attached):
//
//   - receipt OCR (an image): a fuel receipt read with a low-confidence
//     total (AIReceipt; the receipt number is the call number), so the
//     capture becomes a draft and the total goes to the office review queue;
//   - Excel column mapping: no column mapped (the accountant maps them);
//   - period summary: a short text without numbers.
type AI struct {
	mu       sync.Mutex
	Calls    int
	Purposes []string // "receipt", "excel_mapping", "summary", "other"
}

// AIReceipt is what the fake reads on every receipt.
var AIReceipt = map[string]any{
	"kind": "fuel_receipt", "kind_confidence": 0.95,
	"fields": map[string]any{
		"date":              map[string]any{"value": "2026-10-14", "confidence": 0.95},
		"vendor_tax_number": map[string]any{"value": "1234567890", "confidence": 0.92},
		"vendor_name":       map[string]any{"value": "ATLAS AKARYAKIT", "confidence": 0.9},
		"total_amount":      map[string]any{"value": "1250.00", "confidence": 0.55},
		"tax_amount":        map[string]any{"value": "208.33", "confidence": 0.9},
		"tax_rate":          map[string]any{"value": "20", "confidence": 0.9},
		"vehicle_plate":     map[string]any{"value": "34ABC123", "confidence": 0.9},
		"receipt_number":    map[string]any{"value": "0042", "confidence": 0.9},
	},
}

// receiptAnswer is AIReceipt with the call number as the receipt number:
// every image reads as a different receipt (not a duplicate of the last).
func receiptAnswer(n int) string {
	fields := map[string]any{}
	for k, v := range AIReceipt["fields"].(map[string]any) {
		fields[k] = v
	}
	fields["receipt_number"] = map[string]any{"value": fmt.Sprintf("%04d", n), "confidence": 0.9}
	b, _ := json.Marshal(map[string]any{"kind": AIReceipt["kind"], "kind_confidence": AIReceipt["kind_confidence"], "fields": fields})
	return string(b)
}

type aiRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func (f *AI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var in aiRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var system strings.Builder
	image := false
	for _, m := range in.Messages {
		if m.Role == "system" {
			var s string
			_ = json.Unmarshal(m.Content, &s)
			system.WriteString(s)
		}
		if strings.Contains(string(m.Content), `"image_url"`) {
			image = true
		}
	}
	f.mu.Lock()
	f.Calls++
	n := f.Calls
	f.mu.Unlock()
	purpose, content := "other", "Tamam."
	switch sys := system.String(); {
	case image:
		purpose = "receipt"
		content = receiptAnswer(n)
	case strings.Contains(sys, "veri eşleme"):
		purpose = "excel_mapping"
		content = `{"data_type":"outgoing_invoices","source_guess":"unknown","columns":[],"warnings":[]}`
	case strings.Contains(sys, "dönem özeti"):
		purpose = "summary"
		content = "Bu dönemin kayıtları işlendi. Açık uyarılar müşaviriniz tarafından inceleniyor."
	}
	f.mu.Lock()
	f.Purposes = append(f.Purposes, purpose)
	f.mu.Unlock()
	model := in.Model
	if model == "" {
		model = "fake"
	}
	writeJSON(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion", "model": model,
		"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
	})
}
