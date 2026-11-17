package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	excelusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/sheet"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warnrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type memFiles struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memFiles) Upload(_ context.Context, f storage.File, key string) error {
	b, _ := io.ReadAll(f.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = b
	return nil
}

func (s *memFiles) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[key]
	if !ok {
		return nil, 0, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

// #395: masraf ve Z dosyaları ortak Excel akışından (eşleme → önizleme →
// yazma) içe aktarılır; satırları masraf / Z kurallarıyla bu modül yazar.
func TestCommonExcelImportOfExpensesAndZAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'S') RETURNING id`, "ex-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '5610023435', 'S') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	q := db.New(pool)
	svc := New(pool, nopStore{}, &fakeReader{}, nil, engine.New(warnrepo.NewWarnings(q), warnrepo.NewConfig(q)), nil)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	imp := excelusecase.New(pool, &memFiles{m: map[string][]byte{}}, nil, nil).
		WithTarget(mapping.Expenses, svc.ExpenseTarget()).
		WithTarget(mapping.RetailSales, svc.RetailSaleTarget())

	// Başka bir programın başlıkları: takma adlarla eşlenir, AI gerekmez.
	file := "Masraf listesi\n\nFiş Tarihi;Firma;Vergi No;Belge No;Kategori;Tutar;Ödeme Şekli;Not\n" +
		"03.10.2026;Opet Petrolcülük;1234567890;0042;Akaryakıt;1.200,00;Kredi kartı;depo\n" +
		"04.10.2026;Köşe Lokantası;;0007;;550,00;nakit;\n" +
		"05.10.2026;Sıfır;;;;0,00;;\n"
	v, err := imp.Upload(ctx, sc, user, "masraf.csv", []byte(file), excelusecase.UploadOptions{DataType: mapping.Expenses})
	must(err)
	if v.Status != excelusecase.StatusPreview || v.AIUsed || v.Summary.OK != 3 || v.Summary.Total != "1750.00" {
		t.Fatalf("upload: %s %+v %+v", v.Status, v.Summary, v.Mapping)
	}
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k1")
	must(err)
	// Sıfır toplamlı satırı masraf modülü reddeder; satır numarası dosyadaki.
	if v.Status != excelusecase.StatusCommitted || v.Summary.Added != 2 || v.Summary.Rejected != 1 || v.Summary.RowErrors[0].RowNo != 5 {
		t.Fatalf("commit: %s %+v", v.Status, v.Summary)
	}
	page, err := svc.Expenses(ctx, sc, ExpenseFilter{Limit: 50})
	must(err)
	if page.Total != 2 {
		t.Fatalf("expenses %d", page.Total)
	}
	if _, err := imp.Rollback(ctx, sc, v.UUID, user); !errors.Is(err, excelusecase.ErrNoRollback) {
		t.Fatalf("rollback: %v", err)
	}
	// Masraf ekranının dışa aktarımı ortak akıştan geri yüklenir: mükerrer yok.
	xlsx, err := svc.ExportExpenses(ctx, sc, ExpenseFilter{})
	must(err)
	v, err = imp.Upload(ctx, sc, user, "masraf.xlsx", xlsx, excelusecase.UploadOptions{DataType: mapping.Expenses})
	must(err)
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k2")
	must(err)
	if v.Summary.Added != 0 || v.Summary.Duplicates != 2 {
		t.Fatalf("re-import %+v", v.Summary)
	}

	// Z raporu: AI yokken eksik zorunlu alan eşlemede kalır, müşavir eşler.
	z := "Tarih;Cihaz;Z No;Nakit;Kart;Tutar Toplam\n" +
		"01.10.2026;AB123;0101;300,00;900,00;1.200,00\n"
	v, err = imp.Upload(ctx, sc, user, "z.csv", []byte(z), excelusecase.UploadOptions{DataType: mapping.RetailSales})
	must(err)
	if v.Status != excelusecase.StatusMappingReview {
		t.Fatalf("z upload: %s %+v", v.Status, v.Mapping)
	}
	in := excelusecase.MappingInput{}
	for _, c := range [][2]string{{"B", "okc"}, {"F", "total"}} {
		in.Columns = append(in.Columns, struct {
			Column string `json:"column"`
			Field  string `json:"field"`
		}{c[0], c[1]})
	}
	v, err = imp.SetMapping(ctx, sc, v.UUID, user, in)
	must(err)
	if v.Status != excelusecase.StatusPreview {
		t.Fatalf("z mapping: %s %+v", v.Status, v.Summary)
	}
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k3")
	must(err)
	if v.Summary.Added != 1 {
		t.Fatalf("z commit %+v", v.Summary)
	}
	var okc string
	must(pool.QueryRow(ctx, `SELECT okc_serial_number FROM retail_sales WHERE taxpayer_id = $1`, tp).Scan(&okc))
	if okc != "AB123" {
		t.Fatalf("okc %q", okc)
	}
}

func TestCatalogKeysAreSheetKeys(t *testing.T) {
	for dt, cols := range map[string][]sheet.Column{mapping.Expenses: sheet.ExpenseColumns, mapping.RetailSales: sheet.ZColumns} {
		keys := map[string]bool{}
		for _, c := range cols {
			keys[c.Key] = true
		}
		c, _ := mapping.CatalogFor(dt)
		for _, f := range c.Fields {
			if !keys[f.Key] {
				t.Fatalf("%s: catalog field %s is not a sheet column", dt, f.Key)
			}
		}
	}
}
