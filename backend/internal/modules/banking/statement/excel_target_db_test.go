package statement

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	excelusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
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

// #395: banka hareketleri ortak Excel akışından içe aktarılır; satırlar hesap
// IBAN'ına göre gruplanır ve ekstre içe aktarıcısının kurallarıyla yazılır.
func TestCommonExcelImportOfBankMovementsAgainstPostgres(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	user := f.d.User(t)
	sc := engagementctx.Scope{ID: f.eng, OrganizationID: f.org, TaxpayerID: f.tp}
	imp := excelusecase.New(f.d.Pool, &memFiles{m: map[string][]byte{}}, nil, nil).
		WithTarget(mapping.BankTransactions, f.im.ExcelTarget())

	file := "Hesap;İşlem Tarihi;Açıklama;Borç;Alacak;Bakiye\n" +
		ibanZiraat + ";01.10.2026;EFT GELEN DEMIR GIDA;;1.250,00;11.250,00\n" +
		ibanZiraat + ";02.10.2026;KIRA ODEMESI;3.000,00;;8.250,00\n" +
		ibanAkbank + ";02.10.2026;SWIFT GELEN;;100,00;\n"
	v, err := imp.Upload(ctx, sc, user, "hareketler.csv", []byte(file), excelusecase.UploadOptions{DataType: mapping.BankTransactions})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != excelusecase.StatusPreview || v.Summary.OK != 3 {
		t.Fatalf("upload: %s %+v %+v", v.Status, v.Summary, v.Mapping)
	}
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Summary.Added != 3 || f.count(t, f.box) != 2 || f.count(t, f.other) != 1 {
		t.Fatalf("commit %+v ziraat=%d akbank=%d", v.Summary, f.count(t, f.box), f.count(t, f.other))
	}
	// Aynı dosya yeniden: hareketler mükerrer yazılmaz.
	v, err = imp.Upload(ctx, sc, user, "hareketler-2.csv", []byte(file), excelusecase.UploadOptions{DataType: mapping.BankTransactions})
	if err != nil {
		t.Fatal(err)
	}
	v, err = imp.Commit(ctx, sc, v.UUID, user, "k2")
	if err != nil {
		t.Fatal(err)
	}
	if v.Summary.Added != 0 || v.Summary.Duplicates != 3 || f.count(t, f.box) != 2 {
		t.Fatalf("re-import %+v", v.Summary)
	}
	// Hesap kolonu yok ve iki banka hesabı var: yazma reddedilir.
	noAccount := "Tarih;Açıklama;Tutar\n03.10.2026;POS;-45,90\n"
	v, err = imp.Upload(ctx, sc, user, "pos.csv", []byte(noAccount), excelusecase.UploadOptions{DataType: mapping.BankTransactions})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imp.Commit(ctx, sc, v.UUID, user, "k3"); !errors.Is(err, excelusecase.ErrInvalid) {
		t.Fatalf("no account: %v", err)
	}
}
