package statement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
)

// Column origins of an analysis.
const (
	OriginTemplate  = "template"
	OriginSynonym   = "synonym"
	OriginHeuristic = "heuristic"
	OriginAI        = "ai"
)

// ErrNoTemplateNeeded: MT940 carries its own structure.
var ErrNoTemplateNeeded = errors.New("statement: MT940 files need no template")

// ColumnSuggestion is the proposed field of one header cell.
type ColumnSuggestion struct {
	Index      int     `json:"index"`
	Header     string  `json:"header"`
	Field      Field   `json:"field"`
	Confidence float64 `json:"confidence"`
	Origin     string  `json:"origin,omitempty"`
}

// Analysis is what the panel shows before the accountant saves a template.
type Analysis struct {
	Format      string             `json:"format"`
	BankID      string             `json:"bank_id,omitempty"`
	HeaderRow   int                `json:"header_row"`
	Headers     []string           `json:"headers"`
	Fingerprint string             `json:"fingerprint"`
	Matched     *TemplateMatch     `json:"matched,omitempty"`
	Columns     []ColumnSuggestion `json:"columns"`
	Sample      [][]string         `json:"sample"`
	// Lines / RowErrors: what the proposed columns read from the file.
	Lines     int      `json:"lines"`
	RowErrors int      `json:"row_errors"`
	Warnings  []string `json:"warnings"`
	// Rows: the table below the header as the import would read it with the
	// proposed columns (at most MaxPreviewRows), for the Excel-like preview.
	Rows []PreviewRow `json:"rows"`
}

// ColumnRequest is the masked view the AI mapper receives.
type ColumnRequest struct {
	OrganizationID int64
	UserID         int64
	Headers        []string
	Sample         [][]string
}

// ColumnSuggester maps header cells to fields (the AI step). It only maps;
// it never reads or changes amounts.
type ColumnSuggester interface {
	SuggestColumns(ctx context.Context, in ColumnRequest) ([]ColumnSuggestion, error)
}

// Analyze reads a statement file without importing it and proposes the
// field of every header cell: a matching template, else the header
// synonyms, else heuristics refined by the AI mapper (when configured).
func (im *Importer) Analyze(ctx context.Context, org, userID int64, name string, data []byte) (Analysis, error) {
	tpls, err := im.officeTemplates(ctx, org)
	if err != nil {
		return Analysis{}, err
	}
	ai, err := im.columnAI(ctx, org)
	if err != nil {
		return Analysis{}, err
	}
	return AnalyzeFile(ctx, name, data, tpls, ai, ColumnRequest{OrganizationID: org, UserID: userID})
}

// SettingStatementAI is the office setting (organizations.settings) that
// turns the AI column suggestion of statement uploads off. Missing or true:
// on (when an AI provider is configured). Off: the rule-based flow
// (templates, header synonyms, heuristics) runs alone.
const SettingStatementAI = "bank_statement_ai"

// StatementAIEnabled reads SettingStatementAI from an organization's
// settings object.
func StatementAIEnabled(settings []byte) bool {
	var s map[string]any
	if len(settings) == 0 || json.Unmarshal(settings, &s) != nil {
		return true
	}
	on, ok := s[SettingStatementAI].(bool)
	return !ok || on
}

// columnAI is the AI column mapper for the office, nil when the office
// turned it off.
func (im *Importer) columnAI(ctx context.Context, org int64) (ColumnSuggester, error) {
	if im.ai == nil || org <= 0 {
		return im.ai, nil
	}
	o, err := database.QueriesFor(im.pool).GetOrganizationByID(ctx, org)
	if err != nil {
		return nil, err
	}
	if !StatementAIEnabled(o.Settings) {
		return nil, nil
	}
	return im.ai, nil
}

// AnalyzeFile is Analyze without the database.
func AnalyzeFile(ctx context.Context, name string, data []byte, tpls []Template, ai ColumnSuggester, req ColumnRequest) (Analysis, error) {
	format := Detect(name, data)
	if format == "mt940" {
		return Analysis{Format: format}, ErrNoTemplateNeeded
	}
	rows, err := readTable(format, data, tpls)
	if err != nil {
		return Analysis{Format: format}, err
	}
	a := Analysis{Format: format, Warnings: []string{}}
	hdr, cols, match := findHeader(rows, format, tpls)
	if hdr >= 0 {
		a.Matched = match
		origin, conf := OriginTemplate, 1.0
		if match.Source == SourceSynonyms {
			origin, conf = OriginSynonym, 0.8
		}
		byIndex := map[int]Field{}
		for f, c := range fieldColumns {
			if j, ok := cols[c]; ok {
				byIndex[j] = f
			}
		}
		a.setHeader(rows, hdr)
		for k := range a.Columns {
			if f, ok := byIndex[a.Columns[k].Index]; ok {
				a.Columns[k].Field, a.Columns[k].Confidence, a.Columns[k].Origin = f, conf, origin
			}
		}
	} else {
		hdr = guessHeader(rows)
		if hdr < 0 {
			return a, ErrLayoutUnknown
		}
		a.setHeader(rows, hdr)
		heuristicColumns(a.Columns)
		if ai != nil {
			req.Headers, req.Sample = a.Headers, maskRows(a.Sample)
			got, err := ai.SuggestColumns(ctx, req)
			switch {
			case errors.Is(err, mapping.ErrAIUnavailable):
				a.Warnings = append(a.Warnings, "AI yapılandırılmamış; kolonlar başlık adlarından önerildi")
			case err != nil:
				a.Warnings = append(a.Warnings, "AI önerisi alınamadı; kolonlar başlık adlarından önerildi")
			default:
				applyAI(a.Columns, got)
			}
		}
	}
	a.Fingerprint = Fingerprint(a.Headers)
	a.BankID = bankFromText(rows, hdr)
	if a.Matched != nil && a.Matched.BankID != "" {
		a.BankID = a.Matched.BankID
	}
	if a.BankID == "" {
		for i := 0; i < len(rows) && i < hdr; i++ {
			for _, c := range rows[i] {
				if m := reIBANLoose.FindString(strings.ToUpper(c)); m != "" {
					if b := BankFromIBAN(banking.NormalizeIBAN(m)); b != "" {
						a.BankID = b
					}
				}
			}
		}
	}
	// Try the proposal on the file.
	t := a.Template(format)
	if t.Validate() == nil {
		if st, err := ParseTabularWith(rows, format, []Template{t}); err == nil {
			a.Lines, a.RowErrors = len(st.Lines), len(st.Errors)
		}
		a.Rows = previewRows(rows, hdr, format, a.Columns, t)
	} else {
		a.Warnings = append(a.Warnings, "tarih ve tutar (ya da borç ve alacak) kolonları seçilmeli")
		a.Rows = previewRows(rows, hdr, format, a.Columns, Template{})
	}
	return a, nil
}

// Template is the analysis as a template (for saving or trying).
func (a Analysis) Template(format string) Template {
	t := Template{Format: format, BankID: a.BankID, Source: SourceOffice}
	for _, c := range a.Columns {
		t.Columns = append(t.Columns, TemplateColumn{Header: c.Header, Field: c.Field})
	}
	return t
}

func (a *Analysis) setHeader(rows [][]string, hdr int) {
	a.HeaderRow = hdr + 1
	a.Headers, a.Columns = nil, nil
	for j, c := range rows[hdr] {
		if banking.NormalizeDescription(c) == "" {
			continue
		}
		a.Headers = append(a.Headers, strings.TrimSpace(c))
		a.Columns = append(a.Columns, ColumnSuggestion{Index: j, Header: strings.TrimSpace(c)})
	}
	a.Sample = [][]string{}
	for r := hdr + 1; r < len(rows) && len(a.Sample) < 5; r++ {
		if blank(rows[r]) {
			continue
		}
		out := make([]string, len(a.Columns))
		for k, c := range a.Columns {
			if c.Index < len(rows[r]) {
				out[k] = strings.TrimSpace(rows[r][c.Index])
			}
		}
		a.Sample = append(a.Sample, out)
	}
}

// guessHeader: the first row with at least three text cells followed (within
// three rows) by a row that carries a date.
func guessHeader(rows [][]string) int {
	for i := 0; i < len(rows) && i < 40; i++ {
		text := 0
		for _, c := range rows[i] {
			n := banking.NormalizeDescription(c)
			if n == "" {
				continue
			}
			if _, err := parseAmount(c); err == nil {
				continue
			}
			if _, _, err := parseDate(c); err == nil {
				continue
			}
			text++
		}
		if text < 3 {
			continue
		}
		for k := i + 1; k < len(rows) && k <= i+3; k++ {
			for _, c := range rows[k] {
				if _, _, err := parseDate(c); err == nil {
					return i
				}
			}
		}
	}
	return -1
}

// heuristicRules: words in a header and the field they suggest, in order.
var heuristicRules = []struct {
	words []string
	field Field
}{
	{[]string{"VALOR"}, FieldValueDate},
	{[]string{"IBAN"}, FieldCounterpartyIBAN},
	{[]string{"VKN", "TCKN", "VERGI NO"}, FieldCounterpartyTaxID},
	{[]string{"SAAT", "TIME"}, FieldTime},
	{[]string{"TARIH", "DATE"}, FieldDate},
	{[]string{"BAKIYE", "BALANCE"}, FieldBalance},
	{[]string{"BORC", "CIKAN", "CIKIS", "DEBIT"}, FieldDebit},
	{[]string{"ALACAK", "GIREN", "GIRIS", "CREDIT"}, FieldCredit},
	{[]string{"TUTAR", "MIKTAR", "AMOUNT"}, FieldAmount},
	{[]string{"ACIKLAMA", "DETAY", "DESCRIPTION"}, FieldDescription},
	{[]string{"DEKONT", "FIS", "RECEIPT"}, FieldReceipt},
	{[]string{"REFERANS", "REF", "ISLEM NO"}, FieldReference},
	{[]string{"KANAL", "CHANNEL"}, FieldChannel},
	{[]string{"KARSI TARAF", "GONDEREN", "ALICI", "UNVAN"}, FieldCounterparty},
}

func heuristicColumns(cs []ColumnSuggestion) {
	used := map[Field]bool{}
	for k := range cs {
		n := " " + banking.NormalizeDescription(cs[k].Header) + " "
		for _, r := range heuristicRules {
			if used[r.field] {
				continue
			}
			hit := false
			for _, w := range r.words {
				if strings.Contains(n, " "+w+" ") || strings.Contains(n, " "+w) {
					hit = true
					break
				}
			}
			if hit {
				cs[k].Field, cs[k].Confidence, cs[k].Origin = r.field, 0.6, OriginHeuristic
				used[r.field] = true
				break
			}
		}
	}
}

// applyAI takes the AI's answer where it names a valid field, most
// confident first; a field the AI already gave to a column is not given
// twice. A heuristic column whose field the AI put elsewhere is cleared.
func applyAI(cs []ColumnSuggestion, got []ColumnSuggestion) {
	got = append([]ColumnSuggestion(nil), got...)
	sort.SliceStable(got, func(i, j int) bool { return got[i].Confidence > got[j].Confidence })
	taken := map[Field]int{}
	for _, g := range got {
		if g.Index < 0 || g.Index >= len(cs) || g.Field == FieldNone || !g.Field.Valid() {
			continue
		}
		if _, ok := taken[g.Field]; ok || cs[g.Index].Origin == OriginAI {
			continue
		}
		cs[g.Index].Field, cs[g.Index].Origin = g.Field, OriginAI
		cs[g.Index].Confidence = min(max(g.Confidence, 0), 0.9)
		taken[g.Field] = g.Index
	}
	for k, c := range cs {
		if j, ok := taken[c.Field]; ok && c.Origin != OriginAI && j != k {
			cs[k].Field, cs[k].Confidence, cs[k].Origin = FieldNone, 0, ""
		}
	}
}

func maskRows(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = make([]string, len(r))
		for j, c := range r {
			out[i][j] = mapping.MaskCell(c)
		}
	}
	return out
}

// Completer is the AI service subset (internal/modules/ai).
type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, call aiusecase.Call) (aiusecase.Result, error)
}

// AIColumnMapper is the ColumnSuggester backed by the AI service.
type AIColumnMapper struct{ ai Completer }

// NewAIColumnMapper builds the mapper; a nil or disabled service answers
// mapping.ErrAIUnavailable.
func NewAIColumnMapper(ai Completer) *AIColumnMapper { return &AIColumnMapper{ai: ai} }

const columnPrompt = `Sen bir banka ekstresi kolon eşleme aracısın. Bir bankanın Excel/CSV/PDF hesap hareketi dökümünün başlıklarını ve maskelenmiş örnek satırlarını alırsın; her kolonu verilen alanlardan birine eşlersin. Değer hesaplama, satır üretme YOK. Yalnızca şu JSON nesnesini döndür:
{"columns":[{"index":0,"field":"<alan veya null>","confidence":0.0}]}
Kurallar: her başlık için bir giriş (index başlık sırası, 0'dan); emin olmadığın kolonda field=null; bir alan en fazla bir kolona; confidence 0 ile 1 arası. Tek tutar kolonunda işaret yönü verir (amount); ayrı borç/alacak kolonları debit/credit'tir.`

// SuggestColumns implements ColumnSuggester.
func (m *AIColumnMapper) SuggestColumns(ctx context.Context, in ColumnRequest) ([]ColumnSuggestion, error) {
	if m == nil || m.ai == nil || !m.ai.Enabled() {
		return nil, mapping.ErrAIUnavailable
	}
	fields := make([]string, len(Fields))
	for i, f := range Fields {
		fields[i] = string(f)
	}
	sample, _ := json.Marshal(map[string]any{"headers": in.Headers, "rows": in.Sample})
	user := fmt.Sprintf("Alanlar: %s\nÖrnek (maskeli):\n%s", strings.Join(fields, ", "), sample)
	var uid *int64
	if in.UserID > 0 {
		uid = &in.UserID
	}
	res, err := m.ai.Complete(ctx, aiusecase.Call{
		OrganizationID: in.OrganizationID, UserID: uid, Purpose: "bank_statement_column_mapping",
		System:   []provider.SystemBlock{{Text: columnPrompt, Cache: true}},
		Messages: []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{provider.TextBlock(user)}}}, MaxTokens: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("statement: ai call: %w", err)
	}
	return parseColumnAnswer(res.Message.Text(), len(in.Headers))
}

func parseColumnAnswer(text string, n int) ([]ColumnSuggestion, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return nil, errors.New("statement: ai answer has no JSON object")
	}
	var ans struct {
		Columns []struct {
			Index      int      `json:"index"`
			Field      *string  `json:"field"`
			Confidence *float64 `json:"confidence"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(text[i:j+1]), &ans); err != nil {
		return nil, fmt.Errorf("statement: ai answer: %w", err)
	}
	var out []ColumnSuggestion
	for _, c := range ans.Columns {
		if c.Index < 0 || c.Index >= n || c.Field == nil || c.Confidence == nil {
			continue
		}
		f := Field(strings.TrimSpace(*c.Field))
		if f == FieldNone || !f.Valid() {
			continue
		}
		out = append(out, ColumnSuggestion{Index: c.Index, Field: f, Confidence: *c.Confidence, Origin: OriginAI})
	}
	return out, nil
}
