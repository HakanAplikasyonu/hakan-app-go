package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Mizan hafızası uçtan uca (#310): gerçek Postgres ve gerçek HTTP katmanıyla,
// Luca mizanı içe aktarma → öneri → düzeltme → kural adayı → terfi → ikinci
// ayda otomatik kodlama → Luca çıktısında plan denetimi.
//
// Fikstürler: testdata/mizan (go run testdata/mizan/generate.go). Mükellef
// Oğuz Gıda; Korkmaz Gıda planda 120.01.006, Deniz Ambalaj planda
// 320.01.002 (kalemleri 153 → 150.01 düzeltilir), Yeni Lojistik planda yok.
const (
	mizanVKN     = "4730074556"
	mizanTitle   = "Oğuz Gıda Pazarlama Ltd. Şti."
	denizVKN     = "2981056178"
	korkmazVKN   = "5610023435"
	yeniVKN      = "9044100286"
	correctedTo  = "150.01"
	planAccounts = 34
)

type mizanEnv struct {
	*sweepEnv
	eng string // engagement uuid of Oğuz Gıda
}

func newMizanEnv(t *testing.T) *mizanEnv {
	t.Helper()
	e := newSweepEnv(t)
	m := &mizanEnv{sweepEnv: e}
	ctx := context.Background()
	if err := rls.System(ctx, e.d.Pool, "test: mizan memory fixture", func(ctx context.Context, tx pgx.Tx) error {
		var tp int64
		if err := tx.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', $1, $2) RETURNING id`,
			mizanVKN, mizanTitle).Scan(&tp); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING uuid::text`,
			e.orgID, tp).Scan(&m.eng)
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

// api sends a JSON request as the office owner and decodes `data`.
func (m *mizanEnv) api(t *testing.T, method, path string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	return m.send(t, req, out)
}

func (m *mizanEnv) upload(t *testing.T, path, file string, out any) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(m.backend, "testdata", "mizan", file))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(raw)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return m.send(t, req, out)
}

func (m *mizanEnv) send(t *testing.T, req *http.Request, out any) int {
	t.Helper()
	req.Header.Set("Authorization", "Bearer "+m.ownerTok)
	req.Header.Set("X-Organization", "demo-smmm")
	rec := httptest.NewRecorder()
	m.h.ServeHTTP(rec, req)
	if out != nil && rec.Code < 300 {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s %s: decode: %v (%s)", req.Method, req.URL.Path, err, rec.Body)
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			t.Fatalf("%s %s: decode data: %v (%s)", req.Method, req.URL.Path, err, env.Data)
		}
	}
	if rec.Code >= 500 {
		t.Fatalf("%s %s = %d: %s", req.Method, req.URL.Path, rec.Code, rec.Body)
	}
	if rec.Code >= 300 {
		t.Logf("%s %s = %d: %s", req.Method, req.URL.Path, rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	return rec.Code
}

func (m *mizanEnv) base() string { return "/v1/tenant/engagements/" + m.eng }

// invoiceUUID returns the invoice of the Oğuz engagement with that number.
func (m *mizanEnv) invoiceUUID(t *testing.T, number string) string {
	t.Helper()
	var id string
	if err := rls.System(context.Background(), m.d.Pool, "test: mizan invoice lookup", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT i.uuid::text FROM invoices i JOIN taxpayers t ON t.id = i.taxpayer_id
			WHERE t.tax_id = $1 AND i.invoice_number = $2`, mizanVKN, number).Scan(&id)
	}); err != nil {
		t.Fatalf("invoice %s: %v", number, err)
	}
	return id
}

type suggestion struct {
	UUID        string `json:"uuid"`
	TargetType  string `json:"target_type"`
	TargetField string `json:"target_field"`
	LineNo      int    `json:"line_no"`
	AccountCode string `json:"account_code"`
	Confidence  string `json:"confidence"`
	Status      string `json:"status"`
	RuleRef     string `json:"rule_ref"`
	OpenNew     *bool  `json:"open_new"`
	AccountName string `json:"account_name"`
}

func (m *mizanEnv) suggest(t *testing.T, number string) []suggestion {
	t.Helper()
	var out struct {
		Items []suggestion `json:"items"`
	}
	if code := m.api(t, http.MethodPost, m.base()+"/invoices/"+m.invoiceUUID(t, number)+"/account-suggestions", map[string]any{}, &out); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("suggest %s: %d", number, code)
	}
	return out.Items
}

func pick(items []suggestion, targetType, field string) []suggestion {
	var out []suggestion
	for _, s := range items {
		if s.TargetType == targetType && s.TargetField == field {
			out = append(out, s)
		}
	}
	return out
}

func TestMizanMemoryEndToEnd(t *testing.T) {
	m := newMizanEnv(t)

	// 1. Luca mizanı: xlsx ve csv aynı planı verir; son içe aktarma etkin plan.
	for _, f := range []string{"oguz-luca-mizan.xlsx", "oguz-luca-mizan.csv"} {
		var imp struct {
			Import struct {
				SourceFormat string `json:"source_format"`
				SourceKind   string `json:"source_kind"`
				AccountCount int    `json:"account_count"`
			} `json:"import"`
		}
		if code := m.upload(t, m.base()+"/account-plan/imports", f, &imp); code != http.StatusCreated {
			t.Fatalf("import %s: %d", f, code)
		}
		if imp.Import.AccountCount != planAccounts || imp.Import.SourceKind != "trial_balance" {
			t.Fatalf("import %s: %+v, want %d accounts of a trial balance", f, imp.Import, planAccounts)
		}
	}

	// 2. İlk ay faturaları.
	for _, f := range []string{"2026-10-korkmaz-satis.xml", "2026-10-deniz-alis.xml", "2026-10-yeni-lojistik-alis.xml"} {
		if code := m.upload(t, m.base()+"/ubl-uploads", "ubl/"+f, nil); code != http.StatusAccepted {
			t.Fatalf("upload %s: %d", f, code)
		}
	}

	// Planda olan cari: mevcut hesap önerilir.
	korkmaz := pick(m.suggest(t, "OGZ2026000001001"), "customer", "account_code")
	if len(korkmaz) != 1 || korkmaz[0].AccountCode != "120.01.006" || korkmaz[0].OpenNew == nil || *korkmaz[0].OpenNew {
		t.Fatalf("Korkmaz cari önerisi = %+v, want 120.01.006 without open_new", korkmaz)
	}
	// Planda olmayan cari: sıradaki boş kod, "açılmalı".
	yeni := pick(m.suggest(t, "YNL2026000000318"), "customer", "account_code")
	if len(yeni) != 1 || yeni[0].AccountCode != "320.01.007" || yeni[0].OpenNew == nil || !*yeni[0].OpenNew ||
		yeni[0].AccountName != "Yeni Lojistik Taşımacılık A.Ş." {
		t.Fatalf("Yeni Lojistik cari önerisi = %+v, want 320.01.007 open_new", yeni)
	}

	// 3. Deniz Ambalaj kalemleri varsayılan 153'e düşer; müşavir 150.01'e düzeltir.
	deniz := m.suggest(t, "DNZ2026000004521")
	if c := pick(deniz, "customer", "account_code"); len(c) != 1 || c[0].AccountCode != "320.01.002" {
		t.Fatalf("Deniz cari önerisi = %+v, want 320.01.002", c)
	}
	lines := pick(deniz, "invoice_item", "account_code")
	if len(lines) != 3 {
		t.Fatalf("Deniz kalem önerileri = %+v, want 3", lines)
	}
	for _, s := range lines {
		if s.AccountCode != "153" || s.Status != "pending" {
			t.Fatalf("Deniz kalem önerisi = %+v, want pending 153", s)
		}
	}
	candidates := func() []map[string]any {
		var out struct {
			Items []map[string]any `json:"items"`
		}
		if code := m.api(t, http.MethodGet, m.base()+"/account-rule-candidates", nil, &out); code != http.StatusOK {
			t.Fatalf("candidates: %d", code)
		}
		return out.Items
	}
	for i, s := range lines {
		if got := candidates(); len(got) != 0 {
			t.Fatalf("aday kural %d düzeltmeden sonra çıktı: %v", i, got)
		}
		var dto map[string]any
		code := m.api(t, http.MethodPost, m.base()+"/suggestions/"+s.UUID+"/correct",
			map[string]any{"reason": "Ambalaj malzemesi ilk madde hesabına", "value": map[string]any{"account_code": correctedTo}}, &dto)
		if code != http.StatusOK || dto["status"] != "corrected" {
			t.Fatalf("correct %s: %d %v", s.UUID, code, dto)
		}
	}

	// 4. Eşik (3 karar) dolunca aday çıkar.
	got := candidates()
	if len(got) != 1 || got[0]["counterparty_key"] != denizVKN || got[0]["account_code"] != correctedTo || got[0]["direction"] != "incoming" {
		t.Fatalf("aday kurallar = %v, want Deniz Ambalaj → %s", got, correctedTo)
	}
	if n, _ := got[0]["decisions"].(float64); n != 3 {
		t.Fatalf("aday karar sayısı = %v, want 3", got[0]["decisions"])
	}

	// 5. Terfi.
	var rule map[string]any
	if code := m.api(t, http.MethodPost, m.base()+"/account-rule-candidates/promote", map[string]any{
		"counterparty_key": got[0]["counterparty_key"], "counterparty_name": got[0]["counterparty_name"],
		"direction": got[0]["direction"], "account_code": correctedTo,
	}, &rule); code != http.StatusCreated {
		t.Fatalf("promote: %d", code)
	}
	if rule["account_code"] != correctedTo || rule["scope"] != "engagement" {
		t.Fatalf("kural = %v", rule)
	}
	if left := candidates(); len(left) != 0 {
		t.Fatalf("terfiden sonra aday kaldı: %v", left)
	}

	// 6. İkinci ay: Deniz Ambalaj kalemleri soru sorulmadan 150.01.
	for _, f := range []string{"2026-11-korkmaz-satis.xml", "2026-11-deniz-alis.xml", "2026-11-yeni-lojistik-alis.xml"} {
		if code := m.upload(t, m.base()+"/ubl-uploads", "ubl/"+f, nil); code != http.StatusAccepted {
			t.Fatalf("upload %s: %d", f, code)
		}
	}
	auto := pick(m.suggest(t, "DNZ2026000004890"), "invoice_item", "account_code")
	if len(auto) != 3 {
		t.Fatalf("ikinci ay Deniz kalemleri = %+v, want 3", auto)
	}
	autoIDs := map[string]bool{}
	for _, s := range auto {
		if s.AccountCode != correctedTo || s.Status != "auto_accepted" || !strings.HasPrefix(s.RuleRef, "account_rule:") {
			t.Fatalf("ikinci ay kalemi = %+v, want auto_accepted %s by the promoted rule", s, correctedTo)
		}
		autoIDs[s.UUID] = true
	}
	// İnceleme kuyruğuna düşmez.
	var queue struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
	}
	if code := m.api(t, http.MethodGet, m.base()+"/suggestions?limit=200", nil, &queue); code != http.StatusOK {
		t.Fatalf("queue: %d", code)
	}
	for _, q := range queue.Items {
		if autoIDs[q.UUID] {
			t.Fatalf("otomatik kodlanan kalem kuyrukta: %s", q.UUID)
		}
	}
	// Kural başka firmanın kalemlerine dokunmaz.
	for _, s := range pick(m.suggest(t, "YNL2026000000402"), "invoice_item", "account_code") {
		if s.AccountCode == correctedTo || s.Status == "auto_accepted" {
			t.Fatalf("Yeni Lojistik kalemi kurala takıldı: %+v", s)
		}
	}

	// 7. Luca çıktısı: plandaki kodlarla uyarı yok; planda olmayan kodla var.
	journal := func(desc, date string, lines ...map[string]any) string {
		var d struct {
			UUID string `json:"uuid"`
		}
		if code := m.api(t, http.MethodPost, m.base()+"/journal-drafts", map[string]any{
			"entry_date": date + "T00:00:00Z", "description": desc, "lines": lines,
		}, &d); code != http.StatusCreated {
			t.Fatalf("journal draft %q: %d", desc, code)
		}
		if code := m.api(t, http.MethodPost, m.base()+"/journal-drafts/"+d.UUID+"/approve", map[string]any{}, nil); code >= 300 {
			t.Fatalf("approve %q: %d", desc, code)
		}
		return d.UUID
	}
	ln := func(account, debit, credit string) map[string]any {
		return map[string]any{"account": account, "debit": debit, "credit": credit}
	}
	known := journal("Deniz Ambalaj alış (Kasım)", "2026-11-10",
		ln(correctedTo, "15540.00", "0"), ln("191.01", "3108.00", "0"), ln("320.01.002", "0", "18648.00"))
	var batch struct {
		Issues []struct {
			Code string `json:"code"`
		} `json:"issues"`
	}
	if code := m.api(t, http.MethodPost, m.base()+"/journal-exports", map[string]any{"driver": "luca", "drafts": []string{known}}, &batch); code != http.StatusCreated {
		t.Fatalf("Luca çıktısı (plandaki kodlar): %d", code)
	}
	for _, is := range batch.Issues {
		if is.Code == "ACCOUNT_NOT_IN_TARGET_PLAN" {
			t.Fatalf("plandaki kodlarla ACCOUNT_NOT_IN_TARGET_PLAN çıktı: %+v", batch.Issues)
		}
	}

	unknown := journal("Yeni Lojistik taşıma (Kasım)", "2026-11-18",
		ln("153", "9200.00", "0"), ln("191.01", "1840.00", "0"), ln("320.01.007", "0", "11040.00"))
	req := httptest.NewRequest(http.MethodPost, m.base()+"/journal-exports",
		strings.NewReader(fmt.Sprintf(`{"driver":"luca","drafts":["%s"]}`, unknown)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.ownerTok)
	req.Header.Set("X-Organization", "demo-smmm")
	rec := httptest.NewRecorder()
	m.h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "ACCOUNT_NOT_IN_TARGET_PLAN") || !strings.Contains(body, "320.01.007") {
		t.Fatalf("Luca çıktısı (planda olmayan 320.01.007) = %d %s, want EXPORT_BLOCKED with ACCOUNT_NOT_IN_TARGET_PLAN", rec.Code, body)
	}
	_ = korkmazVKN
	_ = yeniVKN
}
