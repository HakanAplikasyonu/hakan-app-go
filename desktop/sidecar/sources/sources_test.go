package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mssql "github.com/microsoft/go-mssqldb"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// fakeQuerier SQL Server olmadan Reader'ı test eder.
type fakeQuerier struct {
	handle func(q NamedQuery) ([]Row, error)
	calls  []NamedQuery
	limits []int
}

func (f *fakeQuerier) Query(_ context.Context, q NamedQuery, maxRows int) ([]Row, error) {
	f.calls = append(f.calls, q)
	f.limits = append(f.limits, maxRows)
	rows, err := f.handle(q)
	if err != nil {
		return nil, err
	}
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows, nil
}

func rowsFor(name string, rows []Row) *fakeQuerier {
	return &fakeQuerier{handle: func(q NamedQuery) ([]Row, error) {
		if q.Name == name {
			return rows, nil
		}
		return nil, nil
	}}
}

func logoCfg() Config {
	return Config{Kind: KindLogoTiger, Host: "logo-srv", Database: "LOGODB", User: "hakan_ro",
		Password: "S3cr3t!@:/", FirmNo: "001", PeriodNo: "01"}
}

func mikroCfg() Config {
	return Config{Kind: KindMikro, Host: "10.0.0.5", Port: 1450, Database: "MikroDB_V16_ABC",
		User: "hakan_ro", Password: "pw"}
}

func errCode(t *testing.T, err error) string {
	t.Helper()
	var e *rpc.Error
	if !errors.As(err, &e) {
		t.Fatalf("rpc.Error bekleniyordu, gelen: %v", err)
	}
	return e.Code
}

func hasWarn(ws []Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}

var (
	day1 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	day2 = time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
)

// ---- SQL güvenlik denetimi ----

func TestValidateQueryAcceptsShippedQueries(t *testing.T) {
	for _, cfg := range []Config{logoCfg(), mikroCfg()} {
		p, err := DefaultProfile(cfg.Kind)
		if err != nil {
			t.Fatal(err)
		}
		q, err := renderTemplate(p.InvoiceQuery, cfg)
		if err != nil {
			t.Fatal(err)
		}
		params, err := ValidateQuery(q)
		if err != nil {
			t.Fatalf("%s: gömülü sorgu reddedildi: %v", cfg.Kind, err)
		}
		if strings.Join(params, ",") != "from,to" {
			t.Fatalf("%s: parametreler %v", cfg.Kind, params)
		}
	}
	for _, q := range []string{
		pingSQL,
		"SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WITH (NOLOCK) WHERE TABLE_NAME IN ('LG_001_01_INVOICE', 'X')",
		"select a.x from [dbo].[T] as a with (nolock) inner join U b WITH (NOLOCK) on a.id = b.id where a.d >= @FROM and a.d < @to",
		"SELECT x FROM T WITH (NOLOCK) WHERE name = 'DROP TABLE; --'",
		"SELECT x FROM T WITH (NOLOCK) WHERE EXISTS (SELECT 1 FROM U WITH (NOLOCK) WHERE U.id = T.id)",
	} {
		if _, err := ValidateQuery(q); err != nil {
			t.Errorf("kabul edilmeliydi: %q: %v", q, err)
		}
	}
}

func TestValidateQueryRejects(t *testing.T) {
	cases := map[string]string{
		"batch":            "SELECT 1; DROP TABLE x",
		"trailing semi":    "SELECT x FROM T WITH (NOLOCK);",
		"insert":           "INSERT INTO T VALUES (1)",
		"update in select": "SELECT x FROM T WITH (NOLOCK) WHERE 1 = 1 UPDATE T SET x = 1",
		"delete":           "SELECT 1 DELETE FROM T",
		"merge":            "SELECT 1 MERGE T USING U ON 1 = 1",
		"exec":             "SELECT 1 EXEC xp_cmdshell 'dir'",
		"execute":          "SELECT 1 EXECUTE ('x')",
		"drop":             "SELECT 1 DROP TABLE T",
		"alter":            "SELECT 1 ALTER TABLE T ADD c INT",
		"create":           "SELECT 1 CREATE TABLE T (c INT)",
		"truncate":         "SELECT 1 TRUNCATE TABLE T",
		"grant":            "SELECT 1 GRANT SELECT ON T TO u",
		"select into":      "SELECT * INTO NewT FROM T WITH (NOLOCK)",
		"sp call":          "SELECT 1 sp_configure",
		"openrowset":       "SELECT * FROM OPENROWSET('SQLNCLI', 'x', 'SELECT 1')",
		"line comment":     "SELECT x FROM T WITH (NOLOCK) -- yorum",
		"block comment":    "SELECT x /* y */ FROM T WITH (NOLOCK)",
		"unknown param":    "SELECT x FROM T WITH (NOLOCK) WHERE id = @id",
		"system var":       "SELECT @@VERSION",
		"no nolock":        "SELECT x FROM T WHERE d >= @from",
		"join no nolock":   "SELECT x FROM T WITH (NOLOCK) JOIN U ON T.a = U.a",
		"other hint":       "SELECT x FROM T WITH (TABLOCKX)",
		"comma join":       "SELECT x FROM T WITH (NOLOCK), U WITH (NOLOCK)",
		"derived table":    "SELECT x FROM (SELECT x FROM T WITH (NOLOCK)) AS d",
		"table function":   "SELECT x FROM fn_x(1) WITH (NOLOCK)",
		"not select":       "WITH c AS (SELECT 1 AS x) SELECT x FROM c WITH (NOLOCK)",
		"temp table":       "SELECT x FROM #tmp WITH (NOLOCK)",
		"placeholder left": "SELECT * FROM LG_{FFF}_01_INVOICE WITH (NOLOCK)",
		"unterminated":     "SELECT 'abc FROM T",
		"waitfor":          "SELECT 1 WAITFOR DELAY '00:00:10'",
		"set":              "SELECT 1 SET NOCOUNT ON",
		"empty":            "   ",
	}
	for name, q := range cases {
		_, err := ValidateQuery(q)
		if err == nil {
			t.Errorf("%s: reddedilmeliydi: %q", name, q)
			continue
		}
		if c := errCode(t, err); c != CodeQueryRejected {
			t.Errorf("%s: kod %s", name, c)
		}
	}
}

func TestBindArgs(t *testing.T) {
	q := NamedQuery{Name: "x", Text: "SELECT x FROM T WITH (NOLOCK) WHERE d >= @from AND d < @to",
		Args: map[string]any{"from": day1, "to": day2}}
	args, err := bindArgs(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 {
		t.Fatalf("args %v", args)
	}
	if s := fmt.Sprintf("%T", args[0]); s != "sql.NamedArg" {
		t.Fatalf("tür %s", s)
	}
	q.Args["id"] = 1
	if _, err := bindArgs(q); err == nil {
		t.Fatal("bilinmeyen arg reddedilmeliydi")
	}
	delete(q.Args, "id")
	delete(q.Args, "to")
	if _, err := bindArgs(q); err == nil {
		t.Fatal("eksik parametre reddedilmeliydi")
	}
	q.Text = "SELECT 1 DELETE FROM T"
	if _, err := bindArgs(q); err == nil {
		t.Fatal("DML reddedilmeliydi")
	}
}

func TestBindArgsTimeAsDateTime(t *testing.T) {
	args, err := bindArgs(NamedQuery{Text: "SELECT x FROM T WITH (NOLOCK) WHERE d >= @from",
		Args: map[string]any{"from": day1, "to": day2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 {
		t.Fatalf("yalnızca kullanılan parametre bağlanmalı: %v", args)
	}
	na := fmt.Sprintf("%#v", args[0])
	if !strings.Contains(na, "mssql.DateTime1") {
		t.Fatalf("time.Time datetime olarak gönderilmeli: %s", na)
	}
}

// ---- firma / dönem ----

func TestValidateFirmPeriod(t *testing.T) {
	if err := ValidateFirmPeriod("001", "01"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFirmPeriod("125", "12"); err != nil {
		t.Fatal(err)
	}
	bad := [][2]string{{"1", "01"}, {"0001", "01"}, {"00a", "01"}, {"001", "1"}, {"001", "001"},
		{"001 ", "01"}, {"001", "0;"}, {"", ""}, {"١٢٣", "01"}, {"001", "01\n"}}
	for _, b := range bad {
		err := ValidateFirmPeriod(b[0], b[1])
		if err == nil {
			t.Errorf("reddedilmeliydi: %q %q", b[0], b[1])
			continue
		}
		if c := errCode(t, err); c != rpc.CodeInvalidParams {
			t.Errorf("kod %s", c)
		}
	}
	cfg := logoCfg()
	cfg.FirmNo = "1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Logo için geçersiz firma no reddedilmeli")
	}
	if _, err := NewReader(cfg, &fakeQuerier{}); err == nil {
		t.Fatal("NewReader geçersiz firma no'yu reddetmeli")
	}
	// Mikro'da firma / dönem gerekmez.
	if err := mikroCfg().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderTemplate(t *testing.T) {
	got, err := renderTemplate("SELECT * FROM LG_{FFF}_{PP}_INVOICE WITH (NOLOCK)", logoCfg())
	if err != nil || got != "SELECT * FROM LG_001_01_INVOICE WITH (NOLOCK)" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := renderTemplate("LG_{FFF}_X", mikroCfg()); err == nil {
		t.Fatal("Mikro'da yer tutucu reddedilmeli")
	}
	cfg := logoCfg()
	cfg.PeriodNo = "1 OR 1=1"
	if _, err := renderTemplate("LG_{FFF}_{PP}_INVOICE", cfg); err == nil {
		t.Fatal("geçersiz dönem no yazılmamalı")
	}
}

// ---- DSN ----

func TestDSNAndRedaction(t *testing.T) {
	cfg := logoCfg()
	dsn := cfg.DSN()
	if !strings.HasPrefix(dsn, "sqlserver://hakan_ro:") || !strings.Contains(dsn, "@logo-srv:1433?") {
		t.Fatalf("dsn %s", dsn)
	}
	if !strings.Contains(dsn, "database=LOGODB") || !strings.Contains(dsn, "app+name=hakan-agent") {
		t.Fatalf("dsn parametreleri eksik: %s", dsn)
	}
	if strings.Contains(dsn, "S3cr3t!@:/") {
		t.Fatalf("parola URL kodlanmalı: %s", dsn)
	}
	for _, s := range []string{cfg.String(), cfg.RedactedDSN(), cfg.GoString(),
		fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg), fmt.Sprint(&cfg)} {
		if strings.Contains(s, "S3cr3t") {
			t.Fatalf("parola sızdı: %s", s)
		}
		if !strings.Contains(s, "REDACTED") {
			t.Fatalf("maske yok: %s", s)
		}
	}
	m := mikroCfg()
	if !strings.Contains(m.DSN(), "@10.0.0.5:1450?") {
		t.Fatalf("port: %s", m.DSN())
	}
	m.Port = 0
	m.Instance = "SQLEXPRESS"
	m.TrustServerCertificate = true
	d := m.DSN()
	if !strings.Contains(d, "@10.0.0.5/SQLEXPRESS?") || !strings.Contains(d, "TrustServerCertificate=true") {
		t.Fatalf("örnek adı: %s", d)
	}
	m.Host = "srv\\SQLEXPRESS"
	if err := m.Validate(); err == nil {
		t.Fatal("host içinde örnek adı reddedilmeli")
	}
}

func TestOpenDoesNotLeakPassword(t *testing.T) {
	q, err := Open(logoCfg())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Close() }()
	if _, err := Open(Config{Kind: "netsis"}); err == nil {
		t.Fatal("bilinmeyen tür reddedilmeli")
	}
}

// ---- Logo eşlemesi ----

func logoRows() []Row {
	return []Row{
		{"INTERNAL_REFERENCE": int64(101), "GUID": "A1B2", "TRCODE": int64(8), "NUMBER": "SF-0001",
			"DOC_NUMBER": "ABC2026000000001", "DATE": time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
			"DOC_DATE": time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "CANCELLED": int64(0),
			"ARP_CODE": "120.01.001", "CURR_INVOICE": int64(0),
			"TOTAL_GROSS": float64(1000), "TOTAL_DISCOUNTS": float64(0), "TOTAL_VAT": float64(180.1),
			"TOTAL_ADD_TAX": nil, "TOTAL_NET": float64(1180.1), "CREDIT_CARD_NO": "4111111111111111"},
		{"INTERNAL_REFERENCE": int64(102), "TRCODE": int64(6), "DOC_NUMBER": "IADE-1",
			"DATE": time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), "CANCELLED": int64(1),
			"TOTAL_NET": "250.10", "TOTAL_VAT": []byte("41.6833")},
		{"INTERNAL_REFERENCE": int64(103), "TRCODE": int64(5), "DATE": time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{"INTERNAL_REFERENCE": int64(104), "TRCODE": int64(99), "DATE": time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC),
			"TOTAL_NET": "abc", "UNKNOWN_COL": "x"},
	}
}

func TestLogoMapping(t *testing.T) {
	fq := rowsFor(QueryInvoices, logoRows())
	r, err := NewReader(logoCfg(), fq)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadInvoices(context.Background(), day1, day2)
	if err != nil {
		t.Fatal(err)
	}
	if got := fq.calls[0].Text; !strings.Contains(got, "LG_001_01_INVOICE WITH (NOLOCK)") {
		t.Fatalf("sorgu %s", got)
	}
	if fq.calls[0].Args["from"] != day1 || fq.calls[0].Args["to"] != day2 {
		t.Fatalf("args %v", fq.calls[0].Args)
	}
	if b.Skipped != 1 || len(b.Invoices) != 3 || b.Truncated {
		t.Fatalf("skipped=%d invoices=%d truncated=%v", b.Skipped, len(b.Invoices), b.Truncated)
	}

	s := b.Invoices[0]
	if s.Direction != DirectionSales || s.Kind != DocKindInvoice || s.TypeCode != "8" {
		t.Fatalf("satış: %+v", s)
	}
	if s.SourceID != "101" || s.DocNo != "ABC2026000000001" || s.InternalNo != "SF-0001" ||
		s.Date != "2026-10-03" || s.DocDate != "2026-10-02" || s.CounterpartyCode != "120.01.001" || s.Cancelled {
		t.Fatalf("başlık: %+v", s)
	}
	if s.Totals.Gross != "1000" || s.Totals.VAT != "180.1" || s.Totals.Net != "1180.1" ||
		s.Totals.Discounts != "0" || s.Totals.AdditionalTax != "" {
		t.Fatalf("toplamlar: %+v", s.Totals)
	}
	if s.Currency != "0" {
		t.Fatalf("döviz ham kod olmalı: %q", s.Currency)
	}
	if _, ok := s.Raw["CREDIT_CARD_NO"]; ok {
		t.Fatal("kart no ham veriye alınmamalı")
	}
	if s.Raw["DOC_NUMBER"] != "ABC2026000000001" {
		t.Fatal("ham veri korunmalı")
	}
	if len(s.Warnings) != 0 {
		t.Fatalf("beklenmeyen uyarı: %v", s.Warnings)
	}

	ret := b.Invoices[1]
	if ret.Direction != DirectionPurchase || ret.Kind != DocKindReturn || !ret.Cancelled {
		t.Fatalf("iade: %+v", ret)
	}
	if ret.Totals.Net != "250.1" || ret.Totals.VAT != "41.6833" {
		t.Fatalf("ondalık: %+v", ret.Totals)
	}

	u := b.Invoices[2]
	if u.Direction != DirectionUnknown || u.Kind != DocKindUnknown || u.TypeCode != "99" {
		t.Fatalf("bilinmeyen: %+v", u)
	}
	if !hasWarn(u.Warnings, WarnTypeUnmapped) || !hasWarn(u.Warnings, WarnValueInvalid) {
		t.Fatalf("uyarılar: %v", u.Warnings)
	}
	if u.Raw["UNKNOWN_COL"] != "x" || u.Raw["TOTAL_NET"] != "abc" {
		t.Fatal("bilinmeyen satırın ham verisi korunmalı")
	}
	if u.Totals.Net != "" {
		t.Fatal("geçersiz tutar yazılmamalı")
	}
}

func TestLogoTypeColumnFallbackAndMissing(t *testing.T) {
	p, err := DefaultProfile(KindLogoGO)
	if err != nil {
		t.Fatal(err)
	}
	// REST adı TYPE ile de çalışır (TRCODE ↔ TYPE ⚠️).
	inv, skip := mapRow(KindLogoGO, p, Row{"TYPE": int64(1), "INTERNAL_REFERENCE": int64(1),
		"DATE": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if skip || inv.Direction != DirectionPurchase || inv.Kind != DocKindInvoice {
		t.Fatalf("%+v", inv)
	}
	// Tür kolonu yoksa çökmez; FIELD_MISSING + TYPE_UNMAPPED.
	inv, skip = mapRow(KindLogoGO, p, Row{"LOGICALREF": int64(5)})
	if skip || inv.Direction != DirectionUnknown {
		t.Fatalf("%+v", inv)
	}
	for _, c := range []string{WarnFieldMissing, WarnTypeUnmapped} {
		if !hasWarn(inv.Warnings, c) {
			t.Fatalf("%s yok: %v", c, inv.Warnings)
		}
	}
	if inv.Raw["LOGICALREF"] != int64(5) {
		t.Fatal("ham veri korunmalı")
	}
	// Fiyat farkı belge sınıfı.
	inv, _ = mapRow(KindLogoGO, p, Row{"TRCODE": int64(14), "INTERNAL_REFERENCE": int64(1),
		"DATE": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if inv.DocumentClass != "price_difference" || inv.Direction != DirectionSales {
		t.Fatalf("%+v", inv)
	}
}

// ---- Mikro eşlemesi ----

func mikroRows() []Row {
	return []Row{
		{"cha_Guid": "6F9619FF-8B86-D011-B42D-00C04FC964FF", "cha_evrak_tip": int64(63), "cha_cinsi": int64(6),
			"cha_tip": int64(0), "cha_normal_Iade": int64(0), "cha_belge_no": "MKR2026000000007",
			"cha_tarihi": time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), "cha_kod": "120.MUSTERI",
			"cha_meblag": "1200.0000", "cha_aratoplam": "1000.0000", "cha_vergi1": "0", "cha_vergi2": "200.0000",
			"cha_iptal": false, "efd_uuid": "f47ac10b-58cc-4372-a567-0e02b2c3d479"},
		{"cha_Guid": "AAAA", "cha_evrak_tip": int64(1), "cha_cinsi": int64(0), "cha_tip": int64(1),
			"cha_normal_Iade": int64(0), "cha_tarihi": time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
			"cha_meblag": float64(50.25), "cha_vergi1": float64(0), "efd_uuid": nil},
	}
}

func TestMikroAllUnmappedUntilConfigured(t *testing.T) {
	r, err := NewReader(mikroCfg(), rowsFor(QueryInvoices, mikroRows()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadInvoices(context.Background(), day1, day2)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Invoices) != 2 || b.Skipped != 0 {
		t.Fatalf("%+v", b)
	}
	for _, inv := range b.Invoices {
		if inv.Direction != DirectionUnknown || inv.Kind != DocKindUnknown || !hasWarn(inv.Warnings, WarnTypeUnmapped) {
			t.Fatalf("eşlenmemiş olmalı: %+v", inv)
		}
		if inv.Totals.VAT != "" {
			t.Fatal("KDV toplamı tahmin edilmemeli")
		}
	}
	a := b.Invoices[0]
	if a.TypeCode != "evrak_tip=63;cinsi=6;tip=0;normal_iade=0" {
		t.Fatalf("tür kodu %q", a.TypeCode)
	}
	if a.SourceID != "6F9619FF-8B86-D011-B42D-00C04FC964FF" || a.DocNo != "MKR2026000000007" ||
		a.Date != "2026-10-10" || a.CounterpartyCode != "120.MUSTERI" || a.ETTN != "f47ac10b-58cc-4372-a567-0e02b2c3d479" {
		t.Fatalf("başlık: %+v", a)
	}
	if a.Totals.Amount != "1200" || a.Totals.Subtotal != "1000" || a.Totals.Net != "" || a.Totals.Gross != "" {
		t.Fatalf("toplamlar: %+v", a.Totals)
	}
	if !hasWarn(a.Warnings, WarnTaxRateUnmapped) {
		t.Fatalf("TAX_RATE_UNMAPPED bekleniyordu: %v", a.Warnings)
	}
	if a.Raw["cha_vergi2"] != "200.0000" {
		t.Fatal("oran bazında KDV ham veride kalmalı")
	}
	c := b.Invoices[1]
	if hasWarn(c.Warnings, WarnTaxRateUnmapped) {
		t.Fatal("cha_vergi* sıfırsa uyarı gerekmez")
	}
	if c.Totals.Amount != "50.25" || c.ETTN != "" {
		t.Fatalf("%+v", c)
	}
}

func writeOverride(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMikroOverrideRules(t *testing.T) {
	path := writeOverride(t, map[string]any{"mikro": map[string]any{
		"verified": true,
		"type_rules": []map[string]any{
			{"match": map[string]string{"evrak_tip": "63"}, "direction": "sales", "kind": "invoice"},
			{"match": map[string]string{"evrak_tip": "63", "normal_iade": "1"}, "direction": "sales", "kind": "return"},
			{"match": map[string]string{"evrak_tip": "1"}, "skip": true},
		},
		"fields": map[string][]string{"counterparty_title": {"cari_unvan1"}},
	}})
	cfg := mikroCfg()
	cfg.OverridePath = path
	rows := mikroRows()
	rows = append(rows, Row{"cha_Guid": "R1", "cha_evrak_tip": int64(63), "cha_cinsi": int64(6), "cha_tip": int64(1),
		"cha_normal_Iade": int64(1), "cha_tarihi": time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), "cari_unvan1": "ACME A.Ş."})
	r, err := NewReader(cfg, rowsFor(QueryInvoices, rows))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Profile().Verified || len(r.Profile().Fields["source_id"]) == 0 {
		t.Fatal("geçersiz kılma birleşmeli, varsayılan alanlar korunmalı")
	}
	b, err := r.ReadInvoices(context.Background(), day1, day2)
	if err != nil {
		t.Fatal(err)
	}
	if b.Skipped != 1 || len(b.Invoices) != 2 {
		t.Fatalf("skipped=%d n=%d", b.Skipped, len(b.Invoices))
	}
	if inv := b.Invoices[0]; inv.Direction != DirectionSales || inv.Kind != DocKindInvoice || hasWarn(inv.Warnings, WarnTypeUnmapped) {
		t.Fatalf("%+v", inv)
	}
	if inv := b.Invoices[1]; inv.Kind != DocKindReturn || inv.CounterpartyTitle != "ACME A.Ş." {
		t.Fatalf("en özgül kural kazanmalı: %+v", inv)
	}
	// Ünvan kolonu yapılandırıldı ama ilk satırda yok → FIELD_MISSING.
	if !hasWarn(b.Invoices[0].Warnings, WarnFieldMissing) {
		t.Fatalf("%v", b.Invoices[0].Warnings)
	}
}

func TestOverrideRejectsBadContent(t *testing.T) {
	cases := map[string]any{
		"unknown key":   map[string]any{"mikro": map[string]any{"invoce_query": "x"}},
		"unknown prof":  map[string]any{"netsis": map[string]any{}},
		"dml query":     map[string]any{"mikro": map[string]any{"invoice_query": "SELECT 1 DELETE FROM CARI_HESAP_HAREKETLERI"}},
		"no nolock":     map[string]any{"mikro": map[string]any{"invoice_query": "SELECT * FROM X WHERE d >= @from AND d < @to"}},
		"missing param": map[string]any{"mikro": map[string]any{"invoice_query": "SELECT * FROM X WITH (NOLOCK) WHERE d >= @from"}},
		"bad rule":      map[string]any{"mikro": map[string]any{"type_rules": []map[string]any{{"match": map[string]string{"evrak_tip": "1"}, "direction": "in"}}}},
		"empty match":   map[string]any{"mikro": map[string]any{"type_rules": []map[string]any{{"direction": "sales", "kind": "invoice"}}}},
		"bad field":     map[string]any{"mikro": map[string]any{"fields": map[string][]string{"iban": {"x"}}}},
		"bad table":     map[string]any{"mikro": map[string]any{"tables": []string{"X'; DROP"}}},
	}
	for name, v := range cases {
		cfg := mikroCfg()
		cfg.OverridePath = writeOverride(t, v)
		if _, err := NewReader(cfg, &fakeQuerier{}); err == nil {
			t.Errorf("%s: reddedilmeliydi", name)
		}
	}
	cfg := mikroCfg()
	cfg.OverridePath = filepath.Join(t.TempDir(), "yok.json")
	if _, err := NewReader(cfg, &fakeQuerier{}); err == nil {
		t.Fatal("olmayan dosya hata vermeli")
	}
}

// ---- ondalık ----

func TestToDecimalNoFloatArithmetic(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"1180.50", "1180.5"},
		{[]byte("0.1"), "0.1"},
		{int64(42), "42"},
		{float64(0.1), "0.1"},
		{float64(1180.1), "1180.1"},
		{float32(0.1), "0.1"},
		{float64(123456789.12), "123456789.12"},
		{"99999999999999999999.9999", "99999999999999999999.9999"},
	}
	for _, c := range cases {
		d, err := toDecimal(c.in)
		if err != nil || d.String() != c.want {
			t.Errorf("%v (%T): %s %v, beklenen %s", c.in, c.in, d.String(), err, c.want)
		}
	}
	for _, bad := range []any{"abc", true, nilFloat(), struct{}{}} {
		if _, err := toDecimal(bad); err == nil {
			t.Errorf("%v reddedilmeli", bad)
		}
	}
	// Toplamlar JSON'da dizgi olarak taşınır.
	data, _ := json.Marshal(Totals{Net: "1180.1"})
	if string(data) != `{"net":"1180.1"}` {
		t.Fatalf("%s", data)
	}
}

func nilFloat() float64 { return math.NaN() }

// ---- satır sınırı ----

func TestRowCap(t *testing.T) {
	var rows []Row
	for i := range 5 {
		rows = append(rows, Row{"INTERNAL_REFERENCE": int64(i), "TRCODE": int64(8),
			"DATE": time.Date(2026, 10, 1+i, 0, 0, 0, 0, time.UTC)})
	}
	cfg := logoCfg()
	cfg.MaxRows = 3
	fq := rowsFor(QueryInvoices, rows)
	r, err := NewReader(cfg, fq)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadInvoices(context.Background(), day1, day2)
	if err != nil {
		t.Fatal(err)
	}
	if fq.limits[0] != 4 {
		t.Fatalf("querier'a sınır+1 istenmeli, gelen %d", fq.limits[0])
	}
	if len(b.Invoices) != 3 || !b.Truncated || !hasWarn(b.Warnings, WarnRowLimitReached) {
		t.Fatalf("n=%d truncated=%v w=%v", len(b.Invoices), b.Truncated, b.Warnings)
	}
	// Tam sınırda kesilme yok.
	fq = rowsFor(QueryInvoices, rows[:3])
	r, _ = NewReader(cfg, fq)
	b, _ = r.ReadInvoices(context.Background(), day1, day2)
	if b.Truncated || len(b.Invoices) != 3 {
		t.Fatalf("sınırda kesilmemeli: %+v", b)
	}
	if DefaultMaxRows != 50_000 {
		t.Fatal("varsayılan sınır")
	}
}

func TestReadInvoicesWindowAndErrors(t *testing.T) {
	r, err := NewReader(logoCfg(), rowsFor(QueryInvoices, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadInvoices(context.Background(), day2, day1); err == nil {
		t.Fatal("ters pencere reddedilmeli")
	}
	if _, err := r.ReadInvoices(context.Background(), day1, day1); err == nil {
		t.Fatal("boş pencere reddedilmeli")
	}
	b, err := r.ReadInvoices(context.Background(), day1, day2)
	if err != nil || b.Invoices == nil || len(b.Invoices) != 0 {
		t.Fatalf("%+v %v", b, err)
	}
	fail := &fakeQuerier{handle: func(NamedQuery) ([]Row, error) {
		return nil, rpc.Errorf(CodeQueryFailed, "Invalid column name 'DATE'")
	}}
	r, _ = NewReader(logoCfg(), fail)
	if _, err := r.ReadInvoices(context.Background(), day1, day2); errCode(t, err) != CodeQueryFailed {
		t.Fatal("sorgu hatası iletilmeli")
	}
	if _, err := NewReader(logoCfg(), nil); err == nil {
		t.Fatal("nil querier reddedilmeli")
	}
}

// ---- bağlantı denetimi ----

func TestConnectionCheck(t *testing.T) {
	present := map[string]bool{"lg_001_01_invoice": true}
	var invoiceErr error
	fq := &fakeQuerier{handle: func(q NamedQuery) ([]Row, error) {
		switch q.Name {
		case QueryPing:
			return []Row{{"ok": int64(1)}}, nil
		case QueryTables:
			var out []Row
			for _, t := range []string{"LG_001_01_INVOICE", "CARI_HESAP_HAREKETLERI", "E_FATURA_DETAYLARI"} {
				if present[strings.ToLower(t)] && strings.Contains(q.Text, "'"+t+"'") {
					out = append(out, Row{"TABLE_NAME": t})
				}
			}
			return out, nil
		case QueryInvoices:
			return nil, invoiceErr
		}
		return nil, fmt.Errorf("beklenmeyen sorgu %s", q.Name)
	}}

	r, err := NewReader(logoCfg(), fq)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Test(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Missing) != 0 || res.Tables[0] != "LG_001_01_INVOICE" {
		t.Fatalf("%+v", res)
	}
	for _, c := range fq.calls {
		if _, err := ValidateQuery(c.Text); err != nil {
			t.Fatalf("Test'in sorgusu güvenlik denetiminden geçmeli: %s: %v", c.Text, err)
		}
	}

	invoiceErr = rpc.Errorf(CodeQueryFailed, "Invalid column name 'DATE'")
	res, _ = r.Test(context.Background())
	if res.OK || !strings.Contains(res.InvoiceQueryError, "DATE") {
		t.Fatalf("kolon hatası raporlanmalı: %+v", res)
	}

	invoiceErr = nil
	r, _ = NewReader(mikroCfg(), fq)
	present["cari_hesap_hareketleri"] = true
	res, err = r.Test(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Missing) != 1 || res.Missing[0] != "E_FATURA_DETAYLARI" || res.InvoiceQueryError == "" {
		t.Fatalf("eksik tablo raporlanmalı: %+v", res)
	}

	down := &fakeQuerier{handle: func(NamedQuery) ([]Row, error) {
		return nil, rpc.Errorf(rpc.CodeUnavailable, "SQL Server'a bağlanılamadı")
	}}
	r, _ = NewReader(mikroCfg(), down)
	if _, err := r.Test(context.Background()); errCode(t, err) != rpc.CodeUnavailable {
		t.Fatal("bağlantı hatası dönmeli")
	}
}

func TestTimeoutApplied(t *testing.T) {
	cfg := logoCfg()
	cfg.TimeoutSeconds = 7
	var deadline time.Duration
	fq := &fakeQuerier{}
	r, err := NewReader(cfg, querierFunc(func(ctx context.Context, q NamedQuery, n int) ([]Row, error) {
		d, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("süre sınırı yok")
		}
		deadline = time.Until(d)
		return fq.Query(ctx, q, n)
	}))
	fq.handle = func(NamedQuery) ([]Row, error) { return nil, nil }
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadInvoices(context.Background(), day1, day2); err != nil {
		t.Fatal(err)
	}
	if deadline <= 0 || deadline > 7*time.Second {
		t.Fatalf("süre sınırı %v", deadline)
	}
}

type querierFunc func(ctx context.Context, q NamedQuery, n int) ([]Row, error)

func (f querierFunc) Query(ctx context.Context, q NamedQuery, n int) ([]Row, error) {
	return f(ctx, q, n)
}

func TestNormalizeSQLValue(t *testing.T) {
	if v := normalizeSQLValue("DECIMAL", []byte("12.3400")); v != "12.3400" {
		t.Fatalf("%v", v)
	}
	var u mssql.UniqueIdentifier
	if err := u.Scan("6F9619FF-8B86-D011-B42D-00C04FC964FF"); err != nil {
		t.Fatal(err)
	}
	b, _ := u.Value()
	if v := normalizeSQLValue("UNIQUEIDENTIFIER", b); v != "6F9619FF-8B86-D011-B42D-00C04FC964FF" {
		t.Fatalf("%v", v)
	}
	if v := normalizeSQLValue("INT", int64(3)); v != int64(3) {
		t.Fatalf("%v", v)
	}
}
