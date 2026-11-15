package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// Adlı sorgular.
const (
	QueryPing     = "ping"
	QueryTables   = "tables"
	QueryInvoices = "invoices"
)

const pingSQL = "SELECT 1 AS ok"

var renderedTableRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Reader bir yerel kaynaktan fatura başlıklarını okur.
type Reader struct {
	cfg          Config
	profile      *Profile
	q            Querier
	invoiceQuery string
	tables       []string
}

// NewReader yapılandırmayı ve profili (gömülü + cfg.OverridePath) yükler,
// sorgu metinlerini doldurup güvenlik denetiminden geçirir. Hatalı
// yapılandırma bağlantı kurulmadan burada yakalanır.
func NewReader(cfg Config, q Querier) (*Reader, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if q == nil {
		return nil, rpc.Errorf(rpc.CodeInternal, "querier verilmedi")
	}
	p, err := LoadProfile(cfg.Kind, cfg.OverridePath)
	if err != nil {
		return nil, err
	}
	iq, err := renderTemplate(p.InvoiceQuery, cfg)
	if err != nil {
		return nil, err
	}
	used, err := ValidateQuery(iq)
	if err != nil {
		return nil, err
	}
	if strings.Join(used, ",") != "from,to" {
		return nil, rejectf("fatura sorgusu @from ve @to parametrelerinin ikisini de kullanmalı")
	}
	tables := make([]string, 0, len(p.Tables))
	for _, t := range p.Tables {
		rt, err := renderTemplate(t, cfg)
		if err != nil {
			return nil, err
		}
		if !renderedTableRe.MatchString(rt) {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "profil geçersiz: tablo adı %q", rt)
		}
		tables = append(tables, rt)
	}
	return &Reader{cfg: cfg, profile: p, q: q, invoiceQuery: iq, tables: tables}, nil
}

// Profile, kullanılan (birleştirilmiş) profili döner.
func (r *Reader) Profile() *Profile { return r.profile }

// Tables, Test'in denetleyeceği doldurulmuş tablo adları.
func (r *Reader) Tables() []string { return append([]string(nil), r.tables...) }

func (r *Reader) run(ctx context.Context, q NamedQuery, maxRows int) ([]Row, error) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.timeout())
	defer cancel()
	if _, err := ValidateQuery(q.Text); err != nil {
		return nil, err
	}
	return r.q.Query(ctx, q, maxRows)
}

// ReadInvoices [from, to) penceresindeki satış / alış fatura başlıklarını
// okur. from / to sunucunun yerel duvar saati olarak yorumlanır (saat
// dilimi dönüşümü yapılmaz). Satır sınırına ulaşılırsa sonuç kesilir,
// Truncated=true ve ROW_LIMIT_REACHED uyarısı döner; pencere daraltılıp
// yeniden okunmalıdır.
func (r *Reader) ReadInvoices(ctx context.Context, from, to time.Time) (*InvoiceBatch, error) {
	if !from.Before(to) {
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "tarih penceresi geçersiz: başlangıç bitişten önce olmalı")
	}
	limit := r.cfg.maxRows()
	rows, err := r.run(ctx, NamedQuery{
		Name: QueryInvoices,
		Text: r.invoiceQuery,
		Args: map[string]any{"from": from, "to": to},
	}, limit+1)
	if err != nil {
		return nil, err
	}
	b := &InvoiceBatch{Source: r.cfg.Kind, From: from, To: to, Invoices: []Invoice{}}
	if len(rows) > limit {
		rows = rows[:limit]
		b.Truncated = true
		b.Warnings = append(b.Warnings, Warning{
			Code:    WarnRowLimitReached,
			Message: fmt.Sprintf("%d satır sınırına ulaşıldı; sonuç eksik, tarih penceresini daraltın", limit),
		})
	}
	for _, row := range rows {
		inv, skip := mapRow(r.cfg.Kind, r.profile, row)
		if skip {
			b.Skipped++
			continue
		}
		b.Invoices = append(b.Invoices, inv)
	}
	return b, nil
}

// CheckResult bağlantı denetiminin sonucudur.
type CheckResult struct {
	OK bool `json:"ok"`
	// Tables beklenen (profildeki) tablolar; Missing bunlardan bulunamayanlar
	// (yok ya da SQL kullanıcısının SELECT yetkisi yok).
	Tables  []string `json:"tables"`
	Missing []string `json:"missing"`
	// InvoiceQueryError, fatura sorgusu boş pencereyle denendiğinde dönen
	// hata (ör. ⚠️ kolon adı yanlış). Boşsa sorgu çalışıyor.
	InvoiceQueryError string `json:"invoice_query_error,omitempty"`
}

// Test bağlantıyı (SELECT 1), beklenen tabloların varlığını
// (INFORMATION_SCHEMA.TABLES) ve fatura sorgusunun derlendiğini (boş
// pencereyle) denetler. Pilot mükellefte ⚠️ tablo ve kolon adları böyle
// doğrulanır. Bağlantı kurulamazsa hata döner; diğer sorunlar sonuçta.
func (r *Reader) Test(ctx context.Context) (*CheckResult, error) {
	if _, err := r.run(ctx, NamedQuery{Name: QueryPing, Text: pingSQL}, 1); err != nil {
		return nil, err
	}
	res := &CheckResult{Tables: r.Tables(), Missing: []string{}}
	// Tablo adları profil doğrulamasından ([A-Za-z0-9_]) geçtiği için dize
	// sabiti olarak yazılması güvenlidir.
	quoted := make([]string, len(r.tables))
	for i, t := range r.tables {
		quoted[i] = "'" + t + "'"
	}
	tq := "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WITH (NOLOCK) WHERE TABLE_NAME IN (" +
		strings.Join(quoted, ", ") + ")"
	rows, err := r.run(ctx, NamedQuery{Name: QueryTables, Text: tq}, len(r.tables)*4+1)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	for _, row := range rows {
		for k, v := range row {
			if strings.EqualFold(k, "TABLE_NAME") {
				if s, ok := v.(string); ok {
					found[strings.ToUpper(s)] = true
				}
			}
		}
	}
	for _, t := range r.tables {
		if !found[strings.ToUpper(t)] {
			res.Missing = append(res.Missing, t)
		}
	}
	if len(res.Missing) == 0 {
		now := time.Now()
		_, err := r.run(ctx, NamedQuery{
			Name: QueryInvoices,
			Text: r.invoiceQuery,
			Args: map[string]any{"from": now, "to": now},
		}, 1)
		if err != nil {
			res.InvoiceQueryError = err.Error()
		}
	} else {
		res.InvoiceQueryError = "beklenen tablolar eksik olduğundan fatura sorgusu denenmedi"
	}
	res.OK = len(res.Missing) == 0 && res.InvoiceQueryError == ""
	return res, nil
}
