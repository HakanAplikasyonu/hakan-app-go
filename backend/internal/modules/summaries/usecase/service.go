// Package usecase drafts the AI period summary (issue #35): facts from
// the portal data, prose from the AI with {{Fn}} placeholders only, an
// accountant edits and approves. No automatic sending.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/facts"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Errors.
var (
	ErrInvalid  = errors.New("summaries: invalid input")
	ErrNotFound = errors.New("summaries: not found")
	ErrState    = errors.New("summaries: only drafts can be changed")
)

// UnmatchedError lists numbers in the body that are not facts.
type UnmatchedError struct{ Numbers []string }

func (e UnmatchedError) Error() string {
	return "summaries: numbers not backed by portal data: " + strings.Join(e.Numbers, ", ")
}

// Completer is the AI service subset (#24).
type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, call aiusecase.Call) (aiusecase.Result, error)
}

// Queries is the db subset.
type Queries interface {
	SummaryInvoiceTotals(ctx context.Context, arg db.SummaryInvoiceTotalsParams) ([]db.SummaryInvoiceTotalsRow, error)
	SummaryExpenseTotals(ctx context.Context, arg db.SummaryExpenseTotalsParams) (db.SummaryExpenseTotalsRow, error)
	SummaryOpenWarnings(ctx context.Context, arg db.SummaryOpenWarningsParams) ([]db.SummaryOpenWarningsRow, error)
	SupersedePeriodSummaryDrafts(ctx context.Context, arg db.SupersedePeriodSummaryDraftsParams) error
	InsertPeriodSummary(ctx context.Context, arg db.InsertPeriodSummaryParams) (db.PeriodSummary, error)
	GetPeriodSummary(ctx context.Context, arg db.GetPeriodSummaryParams) (db.PeriodSummary, error)
	ListPeriodSummaries(ctx context.Context, arg db.ListPeriodSummariesParams) ([]db.PeriodSummary, error)
	UpdatePeriodSummaryBody(ctx context.Context, arg db.UpdatePeriodSummaryBodyParams) (db.PeriodSummary, error)
	ApprovePeriodSummary(ctx context.Context, arg db.ApprovePeriodSummaryParams) (db.PeriodSummary, error)
}

// Service drafts summaries.
type Service struct {
	q   Queries
	ai  Completer
	log *slog.Logger
}

// New builds the service; ai may be nil (a fixed template is used).
func New(q Queries, ai Completer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: q, ai: ai, log: log}
}

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Facts computes the period's numbers.
func (s *Service) Facts(ctx context.Context, sc engagementctx.Scope, period string) ([]facts.Fact, error) {
	if !periodRe.MatchString(period) {
		return nil, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	start, _ := time.Parse("2006-01", period)
	prev := start.AddDate(0, -1, 0).Format("2006-01")
	cur, err := s.invoiceTotals(ctx, sc.TaxpayerID, period)
	if err != nil {
		return nil, err
	}
	before, err := s.invoiceTotals(ctx, sc.TaxpayerID, prev)
	if err != nil {
		return nil, err
	}
	exp, err := s.q.SummaryExpenseTotals(ctx, db.SummaryExpenseTotalsParams{TaxpayerID: sc.TaxpayerID, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("summaries: expenses: %w", err)
	}
	warns, err := s.q.SummaryOpenWarnings(ctx, db.SummaryOpenWarningsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: pgtype.Text{String: period, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("summaries: warnings: %w", err)
	}
	inv := func(dir, p string) facts.Source {
		return facts.Source{Table: "invoices", Filter: fmt.Sprintf("direction=%s period=%s status=active", dir, p)}
	}
	sales, purch := cur["outgoing"], cur["incoming"]
	srcS, srcP := inv("outgoing", period), inv("incoming", period)
	srcS.Records, srcP.Records = sales.count, purch.count
	fs := []facts.Fact{
		facts.New("F1", "Satış matrahı (KDV hariç)", sales.base, facts.UnitTRY, srcS),
		facts.New("F2", "Hesaplanan KDV", sales.vat, facts.UnitTRY, srcS),
		facts.New("F3", "Satış faturası sayısı", decimal.NewFromInt(sales.count), facts.UnitCount, srcS),
		facts.New("F4", "Alış matrahı (KDV hariç)", purch.base, facts.UnitTRY, srcP),
		facts.New("F5", "İndirilecek KDV", purch.vat, facts.UnitTRY, srcP),
		facts.New("F6", "Alış faturası sayısı", decimal.NewFromInt(purch.count), facts.UnitCount, srcP),
	}
	diff := sales.vat.Sub(purch.vat)
	diffLabel := "Ödenecek KDV tahmini (hesaplanan − indirilecek, devir hariç)"
	if diff.IsNegative() {
		diffLabel = "Devreden KDV tahmini (indirilecek − hesaplanan, devir hariç)"
	}
	fs = append(fs, facts.New("F7", diffLabel, diff, facts.UnitTRY, facts.Source{Table: "invoices", Filter: "F2 − F5"}))
	fs = append(fs,
		facts.New("F8", "Onaylı masraf toplamı", dec(exp.Amount), facts.UnitTRY,
			facts.Source{Table: "expenses", Filter: "period=" + period + " approval_status=approved", Records: exp.Expenses}),
		facts.New("F9", "Onaylı masraf sayısı", decimal.NewFromInt(exp.Expenses), facts.UnitCount,
			facts.Source{Table: "expenses", Filter: "period=" + period + " approval_status=approved", Records: exp.Expenses}))
	next := 10
	addChange := func(label string, now, was decimal.Decimal, dir string) {
		if !was.IsPositive() {
			return
		}
		pct := now.Sub(was).Div(was).Mul(decimal.NewFromInt(100))
		way := "artış"
		if pct.IsNegative() {
			way = "azalış"
		}
		src := inv(dir, prev)
		fs = append(fs,
			facts.New(fmt.Sprintf("F%d", next), "Önceki dönem "+label, was, facts.UnitTRY, src),
			facts.New(fmt.Sprintf("F%d", next+1), label+" değişimi ("+way+", önceki döneme göre)", pct, facts.UnitPercent, facts.Source{Table: "invoices", Filter: period + " / " + prev}))
		next += 2
	}
	addChange("satış matrahı", sales.base, before["outgoing"].base, "outgoing")
	addChange("alış matrahı", purch.base, before["incoming"].base, "incoming")
	var blockers, others int64
	for _, w := range warns {
		if w.Severity == "blocker" {
			blockers += w.Warnings
		} else {
			others += w.Warnings
		}
	}
	wsrc := facts.Source{Table: "data_warnings", Filter: "period=" + period + " status=open"}
	fs = append(fs,
		facts.New(fmt.Sprintf("F%d", next), "Açık engelleyici uyarı sayısı", decimal.NewFromInt(blockers), facts.UnitCount, wsrc),
		facts.New(fmt.Sprintf("F%d", next+1), "Açık diğer uyarı sayısı", decimal.NewFromInt(others), facts.UnitCount, wsrc))
	return fs, nil
}

type totals struct {
	count       int64
	base, vat   decimal.Decimal
	withholding decimal.Decimal
	payable     decimal.Decimal
}

func (s *Service) invoiceTotals(ctx context.Context, taxpayerID int64, period string) (map[string]totals, error) {
	rows, err := s.q.SummaryInvoiceTotals(ctx, db.SummaryInvoiceTotalsParams{TaxpayerID: taxpayerID, Period: period})
	if err != nil {
		return nil, fmt.Errorf("summaries: invoices: %w", err)
	}
	out := map[string]totals{}
	for _, r := range rows {
		out[r.Direction] = totals{count: r.Invoices, base: dec(r.Base), vat: dec(r.Vat), withholding: dec(r.Withholding), payable: dec(r.Payable)}
	}
	return out, nil
}

const systemPrompt = `Bir mali müşavirin mükellefine göndereceği kısa dönem özetinin taslağını yazarsın. Kurallar:
- Türkçe, sade, 2-3 kısa paragraf; resmi ama samimi.
- Metinde HİÇ RAKAM yazma. Her sayıyı yalnızca verilen yer tutucuyla an: {{F1}} gibi. Yer tutucu dışında rakam, yüzde veya tutar uydurma.
- Yalnızca verilen olgulara dayan; vergi tavsiyesi verme, tahmin etme, yorumunu olgulara bağla.
- Açık uyarı varsa müşavirin bunları incelediğini belirt.
- Kişi veya firma adı yazma. Selamlama ve imza ekleme.
Yalnızca metni döndür.`

// Generate drafts a summary; an existing draft of the period is superseded.
func (s *Service) Generate(ctx context.Context, sc engagementctx.Scope, userID int64, period string) (View, error) {
	fs, err := s.Facts(ctx, sc, period)
	if err != nil {
		return View{}, err
	}
	tpl, model := s.write(ctx, sc, userID, period, fs)
	factsJSON, _ := json.Marshal(fs)
	if err := s.q.SupersedePeriodSummaryDrafts(ctx, db.SupersedePeriodSummaryDraftsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: period}); err != nil {
		return View{}, err
	}
	row, err := s.q.InsertPeriodSummary(ctx, db.InsertPeriodSummaryParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Period: period,
		Facts: factsJSON, Template: tpl, Body: facts.Fill(tpl, fs), Model: text(model), CreatedBy: optID(userID),
	})
	if err != nil {
		return View{}, err
	}
	return view(row), nil
}

// write asks the model for a placeholder template (one retry), else uses
// the fixed template. It returns the template and the model ("" = fixed).
func (s *Service) write(ctx context.Context, sc engagementctx.Scope, userID int64, period string, fs []facts.Fact) (string, string) {
	if s.ai == nil || !s.ai.Enabled() {
		return Fallback(fs), ""
	}
	var lines []string
	for _, f := range facts.Sorted(fs) {
		lines = append(lines, fmt.Sprintf("{{%s}} = %s (%s)", f.ID, f.Label, f.Rendered))
	}
	user := "Dönem: " + periodName(period) + "\nOlgular:\n" + strings.Join(lines, "\n")
	tp, uid := sc.TaxpayerID, userID
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		msg := user
		if lastErr != nil {
			msg += "\n\nÖnceki taslak reddedildi: " + lastErr.Error() + ". Rakamları yalnızca yer tutucuyla yaz."
		}
		call := aiusecase.Call{OrganizationID: sc.OrganizationID, TaxpayerID: &tp, Purpose: "period_summary",
			System:   []provider.SystemBlock{{Text: systemPrompt, Cache: true}},
			Messages: []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{provider.TextBlock(msg)}}}, MaxTokens: 1200}
		if userID > 0 {
			call.UserID = &uid
		}
		res, err := s.ai.Complete(ctx, call)
		if err != nil {
			s.log.Warn("period_summary_ai_failed", "error", err)
			break
		}
		tpl := strings.TrimSpace(res.Message.Text())
		if lastErr = facts.CheckTemplate(tpl, fs); lastErr == nil {
			return tpl, res.Model
		}
	}
	return Fallback(fs), ""
}

// Fallback is the fixed template used without (or after a failed) AI.
func Fallback(fs []facts.Fact) string {
	byLabel := map[string]string{}
	for _, f := range fs {
		byLabel[f.Label] = f.ID
	}
	var b strings.Builder
	b.WriteString("Bu dönemde {{F3}} satış faturasıyla {{F1}} satış matrahı ve {{F2}} hesaplanan KDV oluştu. ")
	b.WriteString("Alış tarafında {{F6}} faturayla {{F4}} matrah ve {{F5}} indirilecek KDV kaydedildi; buna göre {{F7}} (" + labelOf(fs, "F7") + ") görünüyor.")
	if id, ok := findPrefix(fs, "Satış matrahı değişimi"); ok {
		b.WriteString(" Satış matrahındaki değişim {{" + id + "}} (" + labelOf(fs, id) + ").")
	}
	b.WriteString("\n\nOnaylı {{F9}} masrafın toplamı {{F8}}.")
	if id, ok := findPrefix(fs, "Açık engelleyici"); ok {
		b.WriteString(" Dönemde açık {{" + id + "}} engelleyici uyarı var; bunlar beyandan önce incelenecek.")
	}
	return b.String()
}

func labelOf(fs []facts.Fact, id string) string {
	for _, f := range fs {
		if f.ID == id {
			return lowerFirst(f.Label)
		}
	}
	return ""
}

func findPrefix(fs []facts.Fact, prefix string) (string, bool) {
	for _, f := range fs {
		if strings.HasPrefix(f.Label, prefix) || strings.HasPrefix(f.Label, lowerFirst(prefix)) {
			return f.ID, true
		}
	}
	return "", false
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[0])) + string(r[1:])
}

// Edit replaces the draft body; numbers are checked on approval.
func (s *Service) Edit(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, body string) (View, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 20000 {
		return View{}, fmt.Errorf("%w: body must be 1-20000 characters", ErrInvalid)
	}
	row, err := s.q.UpdatePeriodSummaryBody(ctx, db.UpdatePeriodSummaryBodyParams{Body: body, OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, s.notFoundOrState(ctx, sc, id)
	}
	if err != nil {
		return View{}, err
	}
	return view(row), nil
}

// Approve accepts the draft when every number in it is a fact.
func (s *Service) Approve(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64) (View, error) {
	row, err := s.q.GetPeriodSummary(ctx, db.GetPeriodSummaryParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if row.Status != "draft" {
		return View{}, ErrState
	}
	var fs []facts.Fact
	_ = json.Unmarshal(row.Facts, &fs)
	if bad := facts.Verify(row.Body, fs, row.Period); len(bad) > 0 {
		return View{}, UnmatchedError{Numbers: bad}
	}
	row, err = s.q.ApprovePeriodSummary(ctx, db.ApprovePeriodSummaryParams{ApprovedBy: optID(userID), OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrState
	}
	if err != nil {
		return View{}, err
	}
	return view(row), nil
}

// Get returns one summary.
func (s *Service) Get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (View, error) {
	row, err := s.q.GetPeriodSummary(ctx, db.GetPeriodSummaryParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	return view(row), nil
}

// List returns the engagement's summaries (latest 50).
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, period string) ([]View, error) {
	if period != "" && !periodRe.MatchString(period) {
		return nil, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	rows, err := s.q.ListPeriodSummaries(ctx, db.ListPeriodSummariesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Period: text(period)})
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

func (s *Service) notFoundOrState(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) error {
	if _, err := s.q.GetPeriodSummary(ctx, db.GetPeriodSummaryParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id}); err == nil {
		return ErrState
	}
	return ErrNotFound
}

// View is the API view.
type View struct {
	UUID       uuid.UUID    `json:"uuid"`
	Period     string       `json:"period"`
	Status     string       `json:"status"`
	Facts      []facts.Fact `json:"facts"`
	Template   string       `json:"template"`
	Body       string       `json:"body"`
	Edited     bool         `json:"edited"`
	Model      *string      `json:"model"`
	ApprovedAt *time.Time   `json:"approved_at"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

func view(r db.PeriodSummary) View {
	v := View{UUID: r.Uuid, Period: r.Period, Status: r.Status, Template: r.Template, Body: r.Body, Edited: r.Edited,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	_ = json.Unmarshal(r.Facts, &v.Facts)
	if r.Model.Valid {
		m := r.Model.String
		v.Model = &m
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		v.ApprovedAt = &t
	}
	return v
}

var months = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

func periodName(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return p
	}
	return months[t.Month()-1] + " " + t.Format("2006")
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optID(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}
