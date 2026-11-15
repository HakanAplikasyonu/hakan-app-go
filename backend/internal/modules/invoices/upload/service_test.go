package upload

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownVKN = "1288331521"

// makeInvoice clones a sample with a fresh ETTN and invoice number.
func makeInvoice(t *testing.T, base string, n int) ([]byte, string) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + base)
	if err != nil {
		t.Fatal(err)
	}
	ettn := uuid.NewString()
	b = uuidRe.ReplaceAll(b, []byte("${1}"+ettn+"${2}"))
	b = idRe.ReplaceAll(b, []byte(fmt.Sprintf("${1}TST2026%09d${2}", n)))
	return b, ettn
}

var (
	uuidRe = regexp.MustCompile(`(<UUID[^>]*>)[^<]+(</UUID>)`)
	idRe   = regexp.MustCompile(`(<ID xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">)GIB2026[0-9]{9}(</ID>)`)
)

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type env struct {
	pool  *pgxpool.Pool
	svc   *Service
	scope engagementctx.Scope
	actor int64
}

func setup(t *testing.T) *env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	user := d.User(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	eng := d.Engagement(t, org, tp)
	return &env{
		pool:  d.Pool,
		svc:   New(d.Pool, storage.NewMemory(), nil, nil),
		scope: engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp},
		actor: user,
	}
}

func (e *env) count(t *testing.T, sql string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, e.scope.TaxpayerID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Kabul: 100 faturalık ZIP tek işte işleniyor, rapor doğru; aynı ZIP ikinci
// kez yüklenince kayıt çoğalmıyor.
func TestUpload100InvoiceZipTwice(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	files := map[string][]byte{}
	for i := range 100 {
		base := "temelfatura-satis.xml"
		if i%4 == 0 {
			base = "temelfatura-tevkifat.xml"
		}
		b, _ := makeInvoice(t, base, i+1)
		files[fmt.Sprintf("fatura-%03d.xml", i+1)] = b
	}
	data := zipOf(t, files)

	start := time.Now()
	job, err := e.svc.Upload(ctx, e.scope, e.actor, "portal-indirme.zip", data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("100 invoices processed in %s", time.Since(start))
	if job.Status != "done" || job.Report == nil {
		t.Fatalf("job: %+v", job)
	}
	r := job.Report
	if r.Total != 100 || r.Added != 100 || r.Duplicate != 0 || r.Failed != 0 {
		t.Fatalf("first report: total=%d added=%d dup=%d failed=%d (%+v)", r.Total, r.Added, r.Duplicate, r.Failed, r.Items[:1])
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 100 {
		t.Fatalf("invoices = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM attachments WHERE taxpayer_id = $1 AND kind = 'ubl_xml' AND owner_type = 'invoice'`); n != 100 {
		t.Fatalf("attachments = %d", n)
	}
	// Two counterparties in the samples: one VKN, one TCKN.
	if n := e.count(t, `SELECT count(*) FROM customers WHERE taxpayer_id = $1`); n != 2 {
		t.Fatalf("customers = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM invoice_tax_summaries WHERE taxpayer_id = $1`); n == 0 {
		t.Fatal("no tax summaries")
	}

	// Same counterparty, date and amount, different ETTN (`01` §6 rule 3):
	// every clone after the first of its kind is flagged, none is merged.
	if n := e.count(t, `SELECT count(*) FROM data_warnings WHERE taxpayer_id = $1 AND code = 'POSSIBLE_DUPLICATE'`); n != 98 {
		t.Fatalf("possible duplicate warnings = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM invoice_sources WHERE taxpayer_id = $1`); n != 100 {
		t.Fatalf("invoice sources = %d", n)
	}

	job2, err := e.svc.Upload(ctx, e.scope, e.actor, "portal-indirme.zip", data)
	if err != nil {
		t.Fatal(err)
	}
	r2 := job2.Report
	if r2.Total != 100 || r2.Added != 0 || r2.Duplicate != 100 || r2.Failed != 0 {
		t.Fatalf("second report: %+v", *r2)
	}
	if it := r2.Items[0]; it.Outcome != "seen" || it.MatchRule != "source" {
		t.Fatalf("second upload item: %+v", it)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 100 {
		t.Fatalf("invoices after re-upload = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM attachments WHERE taxpayer_id = $1`); n != 100 {
		t.Fatalf("attachments after re-upload = %d", n)
	}
	// List and Get see the jobs.
	list, err := e.svc.List(ctx, e.scope, 10, 0)
	if err != nil || len(list) != 2 || list[0].Report.Items != nil {
		t.Fatalf("list: %v %d", err, len(list))
	}
	got, err := e.svc.Get(ctx, e.scope, job.UUID)
	if err != nil || got.Report.Added != 100 {
		t.Fatalf("get: %v", err)
	}
	other := e.scope
	other.ID = -1
	if _, err := e.svc.Get(ctx, other, job.UUID); err == nil {
		t.Fatal("job visible from another engagement")
	}
}

func TestUploadMixedFailuresAndWarnings(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	good, _ := makeInvoice(t, "temelfatura-satis.xml", 1)
	foreign := bytes.ReplaceAll(good, []byte(ownVKN), []byte("1111111111"))
	foreign = bytes.Replace(foreign, []byte("TST2026"), []byte("FRN2026"), 1)
	// Schema-valid but breaks a GİB rule: an unknown currency code.
	rule, _ := makeInvoice(t, "temelfatura-satis.xml", 2)
	rule = bytes.Replace(rule, []byte(">TRY<"), []byte(">XXX<"), 1)
	notSchema, _ := makeInvoice(t, "temelfatura-satis.xml", 3)
	notSchema = uuidRe.ReplaceAll(notSchema, []byte("${1}"+uuid.NewString()+"${2}<Bogus xmlns=\"urn:x\">x</Bogus>"))
	files := map[string][]byte{
		"a-iyi.xml":    good,
		"b-baska.xml":  foreign,
		"c-kural.xml":  rule,
		"d-sema.xml":   notSchema,
		"e-bozuk.xml":  []byte("<Invoice xmlns=\"urn:oasis:names:specification:ubl:schema:xsd:Invoice-2\"><broken"),
		"f-notlar.txt": []byte("skip"),
	}
	job, err := e.svc.Upload(ctx, e.scope, e.actor, "karisik.zip", zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ItemResult{}
	for _, it := range job.Report.Items {
		byName[it.Name] = it
	}
	if byName["a-iyi.xml"].Status != ItemAdded {
		t.Errorf("good: %+v", byName["a-iyi.xml"])
	}
	if it := byName["b-baska.xml"]; it.Status != ItemFailed || it.ErrorCode != CodePartyMismatch {
		t.Errorf("foreign: %+v", it)
	}
	if it := byName["d-sema.xml"]; it.Status != ItemFailed || it.ErrorCode != CodeXSD {
		t.Errorf("xsd: %+v", it)
	}
	if it := byName["e-bozuk.xml"]; it.Status != ItemFailed || it.ErrorCode != CodeUnreadable {
		t.Errorf("broken: %+v", it)
	}
	it := byName["c-kural.xml"]
	if it.Status != ItemAdded || !contains(it.Warnings, "UBL_SCHEMATRON_FAIL") {
		t.Errorf("schematron: %+v", it)
	}
	if n := e.count(t, `SELECT count(*) FROM data_warnings WHERE taxpayer_id = $1 AND code = 'UBL_SCHEMATRON_FAIL' AND status = 'open'`); n != 1 {
		t.Errorf("schematron warnings = %d", n)
	}
	if job.Report.Total != 5 || job.Report.Added != 2 || job.Report.Failed != 3 {
		t.Errorf("report: %+v", *job.Report)
	}

	if _, err := e.svc.Upload(ctx, e.scope, e.actor, "x.pdf", []byte("%PDF-1.7")); err != ErrUnsupported {
		t.Errorf("pdf upload: %v", err)
	}
}

func TestUploadWithoutOwnTaxIDFailsJob(t *testing.T) {
	e := setup(t)
	e.svc.WithOwnTaxIDResolver(func(context.Context, int64) (string, error) { return "", nil })
	good, _ := makeInvoice(t, "temelfatura-satis.xml", 1)
	job, err := e.svc.Upload(context.Background(), e.scope, e.actor, "a.xml", good)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" || !strings.Contains(job.Error, "VKN") {
		t.Fatalf("job: %+v", job)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
