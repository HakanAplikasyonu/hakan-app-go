package main

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	excelimport "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// demoOtherProgramExpenses is Atlas Gıda's early November receipts as another
// program exports them (#395): its own headers, a banner line and a total
// row. The common Excel import maps the columns from the expense aliases,
// the office saves the layout as a template and the rows land in Masraflar
// as pending.
const demoOtherProgramExpenses = "Masraf Raporu · 01.11.2026 - 05.11.2026\n" +
	"Fiş Tarihi;Firma;Vergi No;Belge No;Açıklama;Tutar;Ödeme Şekli\n" +
	"02.11.2026;Demirtepe Petrol Ltd. Şti.;9000000033;0061;Dağıtım aracı yakıt;1.350,00;Kredi kartı\n" +
	"03.11.2026;Köşe Lokantası;;0140;Tedarikçi toplantısı;620,00;Nakit\n" +
	"04.11.2026;Hızlı Kargo A.Ş.;9000000041;5590;Numune gönderimi;210,00;Nakit\n" +
	"Toplam;;;;;2.180,00;\n"

// excelStore keeps the uploaded demo file: the object store when it can
// also read back, else memory.
type excelStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// seedExcelImport runs the common Excel import once (#395): upload, the
// office template, commit.
func seedExcelImport(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.System(ctx, pool, "demo-seed: common Excel import (#395)", func(ctx context.Context, _ pgx.Tx) error {
		q := db.New(pool)
		warn := warningsengine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
		rec := receipts.New(pool, seedStore, nil, nil, warn, nil)
		var store excelStore = storage.NewMemory()
		if s, ok := seedStore.(excelStore); ok {
			store = s
		}
		svc := excelimport.New(pool, store, nil, nil).WithTarget(mapping.Expenses, rec.ExpenseTarget())
		v, err := svc.Upload(ctx, scope, actor, "masraf-raporu-kasim.csv", []byte(demoOtherProgramExpenses),
			excelimport.UploadOptions{DataType: mapping.Expenses, SourceSystem: "masraf_raporu", Period: "2026-11"})
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		if v.Status != excelimport.StatusPreview {
			return fmt.Errorf("upload: status %s, want preview", v.Status)
		}
		if v, err = svc.SetMapping(ctx, scope, v.UUID, actor, excelimport.MappingInput{SaveTemplate: excelimport.ScopeTenant}); err != nil {
			return fmt.Errorf("template: %w", err)
		}
		if _, err := svc.Commit(ctx, scope, v.UUID, actor, "demo-seed-395"); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		return nil
	})
}
