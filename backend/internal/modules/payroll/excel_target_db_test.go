package payroll

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	excelusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
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

// #395: bordro ortak Excel akışından bir bordro dönemi olarak yazılır.
func TestCommonExcelImportOfPayrollAgainstPostgres(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	svc := New(database.Ambient{Pool: d.Pool}, nil, "test-secret")
	imp := excelusecase.New(d.Pool, &memFiles{m: map[string][]byte{}}, nil, nil).
		WithTarget(mapping.Payroll, svc.ExcelTarget())

	file := "Adı Soyadı;TC Kimlik No;Gün;Brüt Ücret;SGK İşçi Payı;İşsizlik İşçi Payı;Net Ücret\n" +
		"Ayşe Yılmaz;10000000146;30;30.000,00;4.200,00;300,00;24.150,00\n" +
		"Mehmet Kaya;10000000078;30;40.000,00;5.600,00;400,00;31.700,00\n"
	v, err := imp.Upload(ctx, sc, user, "bordro.csv", []byte(file), excelusecase.UploadOptions{DataType: mapping.Payroll})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != excelusecase.StatusPreview || v.Summary.OK != 2 || v.Summary.Total != "55850.00" {
		t.Fatalf("upload: %s %+v %+v", v.Status, v.Summary, v.Mapping)
	}
	// Dönem ne dosyada ne yüklemede: yazma reddedilir, önizlemede kalır.
	if _, err := imp.Commit(ctx, sc, v.UUID, user, "k1"); !errors.Is(err, excelusecase.ErrInvalid) {
		t.Fatalf("no period: %v", err)
	}
	v, err = imp.Upload(ctx, sc, user, "bordro.csv", []byte(file), excelusecase.UploadOptions{DataType: mapping.Payroll, Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k2")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != excelusecase.StatusCommitted || v.Summary.Added != 2 || v.Summary.Note == "" {
		t.Fatalf("commit: %s %+v", v.Status, v.Summary)
	}
	page, err := svc.ListRuns(ctx, sc, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Period != "2026-10" || page.Items[0].Totals.EmployeeCount != 2 ||
		page.Items[0].Totals.Net.StringFixed(2) != "55850.00" {
		t.Fatalf("runs %+v", page.Items)
	}
}

// #395: bordro ekranlarının listeleri Excel'e aktarılır (TC maskeli, tr-TR
// tarih, tutarlar sayı).
func TestPayrollListExportsAgainstPostgres(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}
	svc := New(database.Ambient{Pool: d.Pool}, nil, "test-secret")
	file := "Adı Soyadı;TC Kimlik No;İşe Giriş;Gün;Brüt Ücret;SGK İşçi Payı;İşsizlik İşçi Payı;SGK İşveren Payı;Net Ücret\n" +
		"Ayşe Yılmaz;10000000146;01.03.2024;30;30.000,00;4.200,00;300,00;6.150,00;24.150,00\n"
	if _, err := svc.Import(ctx, sc, user, "bordro.csv", []byte(file), ImportOptions{Period: "2026-10"}); err != nil {
		t.Fatal(err)
	}
	read := func(data []byte, err error) [][]string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		_, sheets, err := parse.Read("x.xlsx", data)
		if err != nil {
			t.Fatal(err)
		}
		return sheets[0].Rows
	}
	emp := read(svc.ExportEmployees(ctx, tp, EmployeeFilter{}))
	if len(emp) != 2 || emp[0][0] != "Adı Soyadı" || emp[1][0] != "Ayşe Yılmaz" || emp[1][1] == "10000000146" || emp[1][8] != "Çalışıyor" {
		t.Fatalf("employees %q", emp)
	}
	runs := read(svc.ExportRuns(ctx, sc, "2026"))
	if len(runs) != 2 || runs[1][0] != "2026-10" || runs[1][1] != "1" || runs[1][3] != "24150" {
		t.Fatalf("runs %q", runs)
	}
	if none := read(svc.ExportRuns(ctx, sc, "2025")); len(none) != 1 {
		t.Fatalf("2025 runs %q", none)
	}
	prem := read(svc.ExportPremiums(ctx, tp, ""))
	if len(prem) < 1 || prem[0][0] != "Dönem" {
		t.Fatalf("premiums %q", prem)
	}
}
