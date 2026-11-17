package planexport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/text/encoding/charmap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const lucaPlan = "Hesap Kodu;Hesap Adı\n" +
	"120;ALICILAR\n120.01;Yurt İçi Alıcılar\n120.01.001;Eski Müşteri A\n120.01.002;Eski Müşteri B\n" +
	"320;SATICILAR\n320.01;Yurt İçi Satıcılar\n320.01.001;Eski Satıcı\n600;YURTİÇİ SATIŞLAR\n"

type env struct {
	d     *testdb.DB
	svc   *Service
	scope engagementctx.Scope
	user  int64
}

func setup(t *testing.T, withPlan bool) env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, org, tp)
	e := env{d: d, svc: New(d.Pool), scope: engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}, user: d.User(t)}
	if withPlan {
		plan := usecase.New(repository.New(d.Pool, db.New(d.Pool)))
		if _, err := plan.Import(context.Background(), usecase.Subject{OrganizationID: org, EngagementID: eng}, 0, "luca", "luca-plan.csv", "", []byte(lucaPlan)); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e env) customer(t *testing.T, typ, name, taxNo string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	var tax any
	if taxNo != "" {
		tax = taxNo
	}
	if err := e.d.Pool.QueryRow(context.Background(), `
		INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, type, name, tax_number, tax_office, address, city, email)
		VALUES ($1, 'test', $2, 'api', $3, $4, $5, 'Kadıköy', 'Bağdat Cad. 5', 'İstanbul', 'muhasebe@example.com')
		RETURNING uuid`, e.scope.TaxpayerID, uuid.NewString(), typ, name, tax).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Codes come from the plan's layout and never clash; the same cari keeps
// its code.
func TestAllocateFollowsPlanAndIsStable(t *testing.T) {
	e := setup(t, true)
	ctx := context.Background()
	a := e.customer(t, "customer", "Şirin Gıda Ltd. Şti.", "1234567890")
	sub, created, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", CustomerUUID: &a})
	if err != nil || !created || sub.Code != "120.01.003" || sub.ParentCode != "120.01" || sub.Source != "generated" {
		t.Fatalf("first: %+v %v %v", sub, created, err)
	}
	again, created, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", CustomerUUID: &a})
	if err != nil || created || again.Code != sub.Code {
		t.Fatalf("second: %+v %v %v", again, created, err)
	}
	// Same tax id without the card: same code.
	byTax, created, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", TaxID: "1234567890", Name: "Şirin"})
	if err != nil || created || byTax.Code != sub.Code {
		t.Fatalf("by tax id: %+v %v %v", byTax, created, err)
	}
	// Another main account: its own code.
	sup, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "320", CustomerUUID: &a})
	if err != nil || sup.Code != "320.01.002" {
		t.Fatalf("supplier side: %+v %v", sup, err)
	}
	// A code already in the cari memory (review decision) is reused.
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code, account_code, account_name)
		VALUES ($1, $2, $3, '9876543210', '120', '120.01.050', 'Hafızadaki Cari')`, e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID); err != nil {
		t.Fatal(err)
	}
	mem, created, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", TaxID: "9876543210", Name: "X"})
	if err != nil || created || mem.Code != "120.01.050" || mem.Source != "memory" {
		t.Fatalf("memory: %+v %v %v", mem, created, err)
	}
	// The next new code skips the memory code range: highest used + 1.
	b := e.customer(t, "customer", "Öztürk İnşaat", "")
	next, created, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", CustomerUUID: &b})
	if err != nil || !created || next.Code != "120.01.051" || !strings.HasPrefix(next.CounterpartyID, "customer:") {
		t.Fatalf("after memory: %+v %v %v", next, created, err)
	}
	// The generated code is remembered for the review (cari memory).
	var remembered string
	if err := e.d.Pool.QueryRow(ctx, `SELECT account_code FROM counterparty_accounts WHERE engagement_id = $1 AND counterparty_tax_id = '1234567890' AND main_code = '120'`, e.scope.ID).Scan(&remembered); err != nil || remembered != sub.Code {
		t.Fatalf("memory row: %q %v", remembered, err)
	}
	if _, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "12", TaxID: "1234567890", Name: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad main: %v", err)
	}
	if _, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", TaxID: "12", Name: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad tax id: %v", err)
	}
}

// No plan: the fallback layout 120.01.001.
func TestAllocateWithoutPlan(t *testing.T) {
	e := setup(t, false)
	sub, _, err := e.svc.Allocate(context.Background(), e.scope, e.user, AllocateInput{MainCode: "329", TaxID: "1111111111", Name: "Ortak"})
	if err != nil || sub.Code != "329.01.001" {
		t.Fatalf("fallback: %+v %v", sub, err)
	}
}

// Parallel requests: distinct caris get distinct codes, one cari asked
// many times at once gets one code.
func TestAllocateConcurrency(t *testing.T) {
	e := setup(t, true)
	ctx := context.Background()
	const n = 12
	var wg sync.WaitGroup
	codes := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", TaxID: fmt.Sprintf("55500000%02d", i), Name: fmt.Sprintf("Cari %d", i)})
			codes[i], errs[i] = s.Code, err
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, c := range codes {
		if errs[i] != nil {
			t.Fatalf("allocate %d: %v", i, errs[i])
		}
		if seen[c] {
			t.Fatalf("code %s given twice: %v", c, codes)
		}
		seen[c] = true
	}
	same := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "320", TaxID: "4440000000", Name: "Aynı Cari"})
			if err != nil {
				t.Errorf("same cari: %v", err)
			}
			same[i] = s.Code
		}()
	}
	wg.Wait()
	for _, c := range same {
		if c != same[0] || c == "" {
			t.Fatalf("same cari got different codes: %v", same)
		}
	}
}

func decode1254(t *testing.T, b []byte) string {
	t.Helper()
	out, err := charmap.Windows1254.NewDecoder().Bytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// "Yeni hesaplar" has only what Luca lacks; "tüm hesap planı" has the plan
// too; both are Windows-1254 with ";" and recorded in the history.
func TestPlanFiles(t *testing.T) {
	e := setup(t, true)
	ctx := context.Background()
	a := e.customer(t, "customer", "Şirin Gıda", "1234567890")
	if _, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "120", CustomerUUID: &a}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Allocate(ctx, e.scope, e.user, AllocateInput{MainCode: "159", TaxID: "2222222222", Name: "Avans Verilen"}); err != nil {
		t.Fatal(err)
	}
	f, err := e.svc.BuildFile(ctx, e.scope, e.user, FileInput{Kind: KindNew})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(f.Data, []byte{0xC5}) || !bytes.Contains(f.Data, []byte{0xDE, 0x69, 0x72}) || !bytes.Contains(f.Data, []byte{0x47, 0xFD, 0x64, 0x61}) { // "Şir", "Gıda"; no UTF-8
		t.Fatalf("encoding: % X", f.Data)
	}
	text := decode1254(t, f.Data)
	lines := strings.Split(text, "\r\n")
	want := []string{
		"Hesap Kodu;Hesap Adı;Vergi No;Vergi Dairesi;T.C. Kimlik No;Adres;Döviz;E-Posta;Kdv Oran;Kdv Hesap Kodu",
		"120.01.003;Şirin Gıda;1234567890;Kadıköy;;Bağdat Cad. 5 İstanbul;;muhasebe@example.com;;",
		"159.01;159 01;;;;;;;;",
		"159.01.001;Avans Verilen;;;;;;;;",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("new file:\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if f.Kind != KindNew || f.TemplateVerified || f.Accounts != 3 || f.ComparedImport == nil || f.ComparedFileName != "luca-plan.csv" || len(f.SHA256) != 64 {
		t.Fatalf("record: %+v", f.Export)
	}
	full, err := e.svc.BuildFile(ctx, e.scope, e.user, FileInput{Kind: KindFull})
	if err != nil {
		t.Fatal(err)
	}
	fullText := decode1254(t, full.Data)
	for _, must := range []string{"\r\n120;ALICILAR;", "\r\n120.01.001;Eski Müşteri A;", "\r\n120.01.003;Şirin Gıda;1234567890", "\r\n600;YURTİÇİ SATIŞLAR;"} {
		if !strings.Contains(fullText, must) {
			t.Fatalf("full file lacks %q:\n%s", must, fullText)
		}
	}
	if strings.HasSuffix(fullText, "\r\n") {
		t.Fatal("last row ends with a line ending (template: final_line_end false)")
	}
	hist, err := e.svc.Exports(ctx, e.scope, 10, 0)
	if err != nil || len(hist) != 2 || hist[0].Kind != KindFull || hist[1].Kind != KindNew || hist[0].ComparedFileName != "luca-plan.csv" {
		t.Fatalf("history: %+v %v", hist, err)
	}
	subs, err := e.svc.List(ctx, e.scope)
	if err != nil || len(subs) != 2 || subs[0].InPlan {
		t.Fatalf("list: %+v %v", subs, err)
	}
}

// Nothing new: a clear error instead of an empty file; Generate fills the
// customers and suppliers.
func TestNothingNewAndGenerate(t *testing.T) {
	e := setup(t, true)
	ctx := context.Background()
	if _, err := e.svc.BuildFile(ctx, e.scope, e.user, FileInput{Kind: KindNew}); !errors.Is(err, ErrNothingNew) {
		t.Fatalf("nothing new: %v", err)
	}
	if _, err := e.svc.BuildFile(ctx, e.scope, e.user, FileInput{Kind: "other"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad kind: %v", err)
	}
	e.customer(t, "customer", "Müşteri 1", "1000000001")
	e.customer(t, "supplier", "Satıcı 1", "1000000002")
	e.customer(t, "both", "İkisi", "1000000003")
	res, err := e.svc.Generate(ctx, e.scope, e.user, GenerateInput{})
	if err != nil || len(res.Created) != 4 || res.Existing != 0 {
		t.Fatalf("generate: %+v %v", res, err)
	}
	again, err := e.svc.Generate(ctx, e.scope, e.user, GenerateInput{})
	if err != nil || len(again.Created) != 0 || again.Existing != 4 {
		t.Fatalf("second generate: %+v %v", again, err)
	}
	if _, err := e.svc.Generate(ctx, e.scope, e.user, GenerateInput{MainCodes: []string{"600"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad main: %v", err)
	}
}
