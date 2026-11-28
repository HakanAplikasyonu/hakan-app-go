package testfakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AI fakes an OpenAI-compatible chat completions server (AI_PROVIDER
// openai_compatible, AI_BASE_URL = server URL). It answers by what the
// backend asks (the system prompt and whether an image is attached):
//
//   - receipt OCR (an image): a fuel receipt read with a low-confidence
//     total (AIReceipt; the receipt number is the call number), so the
//     capture becomes a draft and the total goes to the office review queue;
//     when the uploader chose "expense_receipt" an expense receipt with a
//     %1 / %10 / %20 VAT breakdown (AIExpenseReceipt), when "z_report" a Z
//     report with its breakdown and cash / card split (AIZReport) (#780);
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

func fv(v string, c float64) map[string]any { return map[string]any{"value": v, "confidence": c} }

// AIExpenseReceipt is what the fake reads on an expense receipt (#780): a
// market receipt with three VAT rates. The vendor name is read with low
// confidence (the office sees "kontrol et"); the payment method is not on
// the receipt (left empty, never guessed). The date is today.
var AIExpenseReceipt = map[string]any{
	"kind": "expense_receipt", "kind_confidence": 0.96,
	"fields": map[string]any{
		"vendor_tax_number": fv("4560012345", 0.93),
		"vendor_name":       fv("DEMO MARKET GIDA LTD", 0.62),
		"total_amount":      fv("921,00", 0.94),
		"tax_amount":        fv("121,00", 0.92),
		"vat_1_total":       fv("101,00", 0.9), "vat_1_tax": fv("1,00", 0.9),
		"vat_10_total": fv("220,00", 0.9), "vat_10_tax": fv("20,00", 0.9),
		"vat_20_total": fv("600,00", 0.9), "vat_20_tax": fv("100,00", 0.9),
	},
}

// AIZReport is what the fake reads on a Z report (#780).
var AIZReport = map[string]any{
	"kind": "z_report", "kind_confidence": 0.97,
	"fields": map[string]any{
		"okc_serial_number": fv("DM00000780", 0.95),
		"total_amount":      fv("5.430,00", 0.95),
		"tax_amount":        fv("812,00", 0.93),
		"cash_amount":       fv("2.130,00", 0.92),
		"card_amount":       fv("3.300,00", 0.92),
		"receipt_count":     fv("57", 0.9),
		"vat_1_total":       fv("1.010,00", 0.9), "vat_1_tax": fv("10,00", 0.9),
		"vat_10_total": fv("1.100,00", 0.9), "vat_10_tax": fv("100,00", 0.9),
		"vat_20_total": fv("3.320,00", 0.9), "vat_20_tax": fv("702,00", 0.9),
	},
}

// receiptAnswer is the answer for the kind the uploader chose with the call
// number as the receipt / Z number: every image reads as a different
// document (not a duplicate of the last). Expense and Z documents are
// dated today. A Z report also comes from its own register (ÖKC
// DM<call number>): two Z reports of the same day with the same total are
// not duplicates of each other.
func receiptAnswer(n int, hint string) string {
	src := AIReceipt
	switch hint {
	case "expense_receipt":
		src = AIExpenseReceipt
	case "z_report":
		src = AIZReport
	}
	fields := map[string]any{}
	for k, v := range src["fields"].(map[string]any) {
		fields[k] = v
	}
	switch hint {
	case "z_report":
		fields["z_report_number"] = fv(fmt.Sprintf("%04d", n), 0.95)
		fields["okc_serial_number"] = fv(fmt.Sprintf("DM%08d", n), 0.95)
		fields["date"] = fv(time.Now().Format("02.01.2006"), 0.95)
	case "expense_receipt":
		fields["receipt_number"] = fv(fmt.Sprintf("%04d", n), 0.9)
		fields["date"] = fv(time.Now().Format("02.01.2006"), 0.95)
	default:
		fields["receipt_number"] = fv(fmt.Sprintf("%04d", n), 0.9)
	}
	b, _ := json.Marshal(map[string]any{"kind": src["kind"], "kind_confidence": src["kind_confidence"], "fields": fields})
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
	image, hint := false, ""
	for _, m := range in.Messages {
		if m.Role == "system" {
			var s string
			_ = json.Unmarshal(m.Content, &s)
			system.WriteString(s)
		}
		if strings.Contains(string(m.Content), `"image_url"`) {
			image = true
			for _, k := range []string{"expense_receipt", "z_report"} {
				if strings.Contains(string(m.Content), k+" olarak") {
					hint = k
				}
			}
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
		content = receiptAnswer(n, hint)
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
