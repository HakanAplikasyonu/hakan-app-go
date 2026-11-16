package pushtext

import (
	"strings"
	"testing"
)

// Müşavir mobil (#241) ve belge görevi (#240) bildirimleri kilit ekranında
// kendi metinleriyle görünür; mükellef adı ya da tutar taşımaz.
func TestOfficeKinds(t *testing.T) {
	for _, kind := range []string{"doc_tasks", "doc_task", "doc_task_submitted", "warnings_blocker"} {
		tr, en := For(kind, "tr", nil), For(kind, "en", nil)
		if tr == fallback.tr || en == fallback.en || strings.Contains(tr.Body, "{{") || strings.Contains(en.Body, "{{") {
			t.Errorf("%s: tr=%+v en=%+v", kind, tr, en)
		}
	}
}
