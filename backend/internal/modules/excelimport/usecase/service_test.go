package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memStore) Upload(_ context.Context, f storage.File, key string) error {
	b, _ := io.ReadAll(f.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = b
	return nil
}

func (s *memStore) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[key]
	if !ok {
		return nil, 0, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

type countingAI struct {
	calls  int
	answer string
}

func (c *countingAI) Enabled() bool { return true }
func (c *countingAI) Complete(_ context.Context, _ aiusecase.Call) (aiusecase.Result, error) {
	c.calls++
	return aiusecase.Result{Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{provider.TextBlock(c.answer)}}}, nil
}

// E (KDV) is below 0.80 -> mapping_review on first sight.
const answer = `{"data_type":"outgoing_invoices","source_guess":"bizimhesap","columns":[
{"column":"A","field":"issue_date","confidence":0.98},{"column":"B","field":"invoice_number","confidence":0.97},
{"column":"C","field":"customer_tax_number","confidence":0.95},{"column":"D","field":"customer_name","confidence":0.92},
{"column":"E","field":"tax_summary[20].vat","confidence":0.6},{"column":"F","field":"amounts.payable_amount","confidence":0.9},
{"column":"G","field":null,"confidence":0.99}],"warnings":[]}`

const csvFile = "Rapor: Satışlar\n\nTarih;Belge No;Vergi No;Ünvan;KDV %20;Toplam;Plasiyer\n" +
	"15.10.2026;A-1;1234567890;Demir Gıda;20,00;120,00;Ali\n" +
	"16.10.2026;A-2;9876543210;Kaya Ltd;200,00;1.200,00;Veli\n" +
	"Genel Toplam;;;;220,00;1.320,00;\n"

func TestExcelImportFlowAgainstPostgres(t *testing.T) {
	url := os.Getenv("EXCEL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("EXCEL_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'X') RETURNING id`, "x-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM invoices WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM attachments WHERE taxpayer_id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})
	ai := &countingAI{answer: answer}
	svc := New(pool, &memStore{m: map[string][]byte{}}, ai, nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	invoices := func() int {
		var n int
		must(pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`, tp).Scan(&n))
		return n
	}

	v, err := svc.Upload(ctx, sc, user, "satis.csv", []byte(csvFile), UploadOptions{Period: "2026-10"})
	must(err)
	if v.Status != StatusMappingReview || !v.AIUsed || ai.calls != 1 || v.Layout.HeaderRow != 2 {
		t.Fatalf("first upload: %s ai=%v calls=%d layout=%+v", v.Status, v.AIUsed, ai.calls, v.Layout)
	}
	// Commit is refused before preview.
	if _, err := svc.Commit(ctx, sc, v.UUID, user, "k1"); !errors.Is(err, ErrState) {
		t.Fatalf("early commit: %v", err)
	}
	in := MappingInput{SaveTemplate: ScopeTenant}
	for _, c := range [][2]string{{"E", "tax_summary[20].vat"}} {
		in.Columns = append(in.Columns, struct {
			Column string `json:"column"`
			Field  string `json:"field"`
		}{c[0], c[1]})
	}
	v, err = svc.SetMapping(ctx, sc, v.UUID, user, in)
	must(err)
	if v.Status != StatusPreview || v.Summary.OK != 2 || v.Summary.Total != "1320.00" || !v.FromTemplate {
		t.Fatalf("after mapping: %s %+v", v.Status, v.Summary)
	}
	if invoices() != 0 {
		t.Fatal("rows written before commit")
	}
	v, err = svc.Commit(ctx, sc, v.UUID, user, "k1")
	must(err)
	if v.Status != StatusCommitted || v.Summary.Added != 2 || invoices() != 2 {
		t.Fatalf("commit: %s %+v invoices=%d", v.Status, v.Summary, invoices())
	}
	if again, err := svc.Commit(ctx, sc, v.UUID, user, "k1"); err != nil || again.Summary.Added != 2 {
		t.Fatalf("idempotent commit: %v", err)
	}
	if _, err := svc.Commit(ctx, sc, v.UUID, user, "other"); !errors.Is(err, ErrIdempotency) {
		t.Fatalf("other key: %v", err)
	}
	first := v.UUID

	// Same report again: the tenant template matches, no AI call, nothing new.
	v2, err := svc.Upload(ctx, sc, user, "satis-2.csv", []byte(strings.Replace(csvFile, "Rapor: Satışlar", "Rapor 2", 1)), UploadOptions{})
	must(err)
	if ai.calls != 1 || v2.AIUsed || !v2.FromTemplate || v2.Status != StatusPreview || v2.Summary.Existing != 2 {
		t.Fatalf("second upload: calls=%d %s %+v", ai.calls, v2.Status, v2.Summary)
	}
	v2, err = svc.Commit(ctx, sc, v2.UUID, user, "k2")
	must(err)
	if v2.Summary.Added != 0 || invoices() != 2 {
		t.Fatalf("re-import duplicated: %+v invoices=%d", v2.Summary, invoices())
	}
	rows, total, err := svc.Rows(ctx, sc, first, "", 50, 0)
	must(err)
	if total != 2 || !rows[0].Written {
		t.Fatalf("rows %d %+v", total, rows)
	}
	// Rollback of the second import touches nothing; of the first removes both.
	if v2, err = svc.Rollback(ctx, sc, v2.UUID, user); err != nil || v2.Summary.RolledBack != 0 || invoices() != 2 {
		t.Fatalf("rollback dup import: %v %d", err, invoices())
	}
	v, err = svc.Rollback(ctx, sc, first, user)
	must(err)
	if v.Status != StatusRolledBack || v.Summary.RolledBack != 2 || invoices() != 0 {
		t.Fatalf("rollback: %s %+v invoices=%d", v.Status, v.Summary, invoices())
	}

	// A broken row: has_errors until skipped.
	bad := strings.Replace(csvFile, "16.10.2026;A-2", "xx;A-2", 1)
	v3, err := svc.Upload(ctx, sc, user, "bad.csv", []byte(bad), UploadOptions{})
	must(err)
	if v3.Status != StatusHasErrors || v3.Summary.Errors != 1 {
		t.Fatalf("bad upload %s %+v", v3.Status, v3.Summary)
	}
	if _, err := svc.Commit(ctx, sc, v3.UUID, user, "k3"); !errors.Is(err, ErrState) {
		t.Fatal("commit with errors allowed")
	}
	v3, err = svc.SkipRow(ctx, sc, v3.UUID, 4, true)
	must(err)
	if v3.Status != StatusPreview || v3.Summary.Skipped != 1 {
		t.Fatalf("after skip %s %+v", v3.Status, v3.Summary)
	}

	// Unreadable file -> rejected; templates list.
	if _, err := svc.Upload(ctx, sc, user, "x.xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0, 0}, UploadOptions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("xls: %v", err)
	}
	tpls, err := svc.Templates(ctx, org)
	must(err)
	if len(tpls) != 1 || tpls[0].Scope != ScopeTenant {
		t.Fatalf("templates %+v", tpls)
	}
	must(svc.DeleteTemplate(ctx, org, tpls[0].UUID))
}
