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

// Payroll is the data type of a payroll (bordro) sheet (#382). Payroll
// sheets are committed by the payroll module (its own upload, or the
// common Excel import flow through excelimport/target, #395).
const Payroll = "payroll"

// PayrollFields is the target catalog of a payroll sheet.
var PayrollFields = []Field{
	{Key: "employee_name", Description: "personel adı soyadı"},
	{Key: "first_name", Description: "personel adı (soyadı ayrı kolonda)"},
	{Key: "last_name", Description: "personel soyadı"},
	{Key: "national_id", Description: "TC kimlik numarası"},
	{Key: "sgk_no", Description: "SGK sicil numarası"},
	{Key: "job_title", Description: "görevi / unvanı"},
	{Key: "department", Description: "bölüm / departman"},
	{Key: "hire_date", Description: "işe giriş tarihi"},
	{Key: "exit_date", Description: "işten çıkış tarihi"},
	{Key: "period", Description: "bordro dönemi (ay/yıl)"},
	{Key: "days_worked", Description: "SGK prim günü / çalışılan gün"},
	{Key: "gross", Description: "brüt ücret / toplam kazanç"},
	{Key: "sgk_base", Description: "SGK matrahı"},
	{Key: "sgk_employee", Description: "SGK işçi payı (%14)"},
	{Key: "unemployment_employee", Description: "işsizlik sigortası işçi payı (%1)"},
	{Key: "income_tax_base", Description: "gelir vergisi matrahı"},
	{Key: "income_tax", Description: "ödenecek gelir vergisi"},
	{Key: "income_tax_exemption", Description: "asgari ücret gelir vergisi istisnası"},
	{Key: "stamp_tax", Description: "ödenecek damga vergisi"},
	{Key: "stamp_tax_exemption", Description: "asgari ücret damga vergisi istisnası"},
	{Key: "agi", Description: "asgari geçim indirimi (AGİ)"},
	{Key: "other_deductions", Description: "diğer kesintiler (avans, icra, sendika)"},
	{Key: "net", Description: "net ücret / ele geçen"},
	{Key: "sgk_employer", Description: "SGK işveren payı"},
	{Key: "unemployment_employer", Description: "işsizlik sigortası işveren payı (%2)"},
	{Key: "sgk_incentive", Description: "SGK teşvik indirimi"},
	{Key: "employer_cost", Description: "işverene toplam maliyet"},
}

// PayrollField reports whether f is a payroll target field.
func PayrollField(f string) bool {
	for _, x := range PayrollFields {
		if x.Key == f {
			return true
		}
	}
	return false
}

const payrollSystemPrompt = `Sen bir bordro veri eşleme aracısın. Bir bordro programının (Luca, Zirve, Logo ya da serbest Excel) dışa aktarımının başlıklarını ve maskelenmiş örnek satırlarını alırsın; her kolonu verilen hedef alanlardan birine eşlersin. Değer hesaplama, satır üretme, değer değiştirme YOK. Yalnızca şu JSON nesnesini döndür, başka metin yazma:
{"source_guess":"luca|zirve|logo|unknown","columns":[{"column":"B","header":"...","field":"<alan veya null>","confidence":0.0}],"warnings":["..."]}
Kurallar: her kolon için bir giriş; emin olmadığın veya hedefi olmayan kolonda field=null; bir alan en fazla bir kolona; confidence 0 ile 1 arası. SGK işçi payı ile işsizlik işçi payını, SGK işveren payı ile işsizlik işveren payını ayır.`

// MapPayroll asks the model to map a payroll sheet. Same rules as Map: an
// invalid answer is retried once, then an all-unmapped mapping comes back
// with a warning.
func (m *AIMapper) MapPayroll(ctx context.Context, req AIRequest) (Mapping, error) {
	if m == nil || m.ai == nil || !m.ai.Enabled() {
		return Mapping{}, ErrAIUnavailable
	}
	headers, rows := MaskSample(req.Sheet, req.Layout)
	sample, _ := json.Marshal(map[string]any{"headers": headers, "rows": rows})
	fields := make([]string, 0, len(PayrollFields))
	for _, f := range PayrollFields {
		fields = append(fields, f.Key+" ("+f.Description+")")
	}
	user := fmt.Sprintf("Hedef alanlar:\n- %s\nÖrnek (maskeli):\n%s", strings.Join(fields, "\n- "), sample)
	var tp, uid *int64
	if req.TaxpayerID > 0 {
		tp = &req.TaxpayerID
	}
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
			Purpose: "payroll_column_mapping", System: []provider.SystemBlock{{Text: payrollSystemPrompt, Cache: true}},
			Messages: msgs, MaxTokens: 2048,
		})
		if err != nil {
			return Mapping{}, fmt.Errorf("mapping: ai call: %w", err)
		}
		mp, err := ParsePayrollAnswer(res.Message.Text(), req.Layout)
		if err == nil {
			return mp, nil
		}
		lastErr = err
	}
	mp := Unmapped(Payroll, req.Layout, "AI eşlemesi iki denemede de geçersizdi; eşleme müşavire bırakıldı")
	return mp, nil
}

// ParsePayrollAnswer validates the model's payroll mapping.
func ParsePayrollAnswer(text string, l parse.Layout) (Mapping, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return Mapping{}, errors.New("no JSON object")
	}
	var a aiAnswer
	if err := json.NewDecoder(strings.NewReader(text[i : j+1])).Decode(&a); err != nil {
		return Mapping{}, fmt.Errorf("invalid JSON: %v", err)
	}
	byLetter := map[string]aiColumn{}
	for _, c := range a.Columns {
		if c.Confidence == nil || *c.Confidence < 0 || *c.Confidence > 1 {
			return Mapping{}, fmt.Errorf("column %s: confidence out of range", c.Column)
		}
		byLetter[strings.ToUpper(strings.TrimSpace(c.Column))] = c
	}
	mp := Mapping{DataType: Payroll, SourceSystem: a.SourceGuess, HeaderRow: l.HeaderRow, Warnings: a.Warnings}
	used := map[string]bool{}
	for _, col := range l.Columns {
		cm := ColumnMap{Column: col.Letter, Header: col.Header, Origin: OriginAI}
		if c, ok := byLetter[col.Letter]; ok {
			cm.Confidence = *c.Confidence
			if c.Field != nil && *c.Field != "" {
				f := strings.TrimSpace(*c.Field)
				if !PayrollField(f) {
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
