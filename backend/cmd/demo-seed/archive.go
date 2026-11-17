package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/archive"
)

// seedArchive (#393): Atlas Gıda'nın belge arşivi ve İSG evrakları. Tarihler
// tohumlama gününe göre: temel İSG eğitimi 20 gün içinde bitiyor, bir
// sağlık raporunun süresi dolmuş, risk değerlendirmesi geçerli; ofis
// mobilde İSG sekmesi yaklaşan bitiş uyarısını gösterir.
func seedArchive(ctx context.Context, pool *pgxpool.Pool, taxpayerID, actor int64, now time.Time) error {
	svc := archive.New(database.Ambient{Pool: pool}, payrollStore{up: seedStore})
	day := func(d int) string { return now.AddDate(0, 0, d).Format(time.DateOnly) }
	docs := []struct {
		file string
		in   archive.Input
	}{
		{"kira-sozlesmesi.pdf", archive.Input{Category: "contract", Title: "Depo kira sözleşmesi", Note: "Tuzla depo, 3 yıllık.",
			DocumentDate: day(-400), ValidUntil: day(695)}},
		{"vergi-levhasi-2026.pdf", archive.Input{Category: "tax", Title: "Vergi levhası 2026", DocumentDate: day(-200)}},
		{"ticaret-sicil-gazetesi.pdf", archive.Input{Category: "trade_registry", Title: "Ticaret sicil gazetesi (kuruluş)", DocumentDate: day(-1500)}},
		{"sgk-isyeri-tescil.pdf", archive.Input{Category: "sgk", Title: "SGK işyeri tescil belgesi", DocumentDate: day(-1400)}},
		{"isg-temel-egitim.pdf", archive.Input{Category: "isg", ISGKind: "isg_training", Title: "Temel İSG eğitimi sertifikaları (4 personel)",
			DocumentDate: day(-345), ValidUntil: day(20)}},
		{"risk-degerlendirmesi.pdf", archive.Input{Category: "isg", ISGKind: "risk_assessment", Title: "Risk değerlendirmesi raporu",
			Note: "Az tehlikeli sınıf, 6 yılda bir yenilenir.", DocumentDate: day(-300), ValidUntil: day(1890)}},
		{"saglik-raporu-burak-sen.pdf", archive.Input{Category: "isg", ISGKind: "health_report", Title: "İşe giriş sağlık raporu · Burak Şen",
			DocumentDate: day(-1830), ValidUntil: day(-5)}},
	}
	for _, d := range docs {
		_, err := svc.Upload(ctx, taxpayerID, actor, d.file, demoPDF(d.in.Title), d.in)
		if errors.Is(err, archive.ErrExists) {
			continue // the taxpayer outlived a -reset: its archive is already there
		}
		if err != nil {
			return fmt.Errorf("archive %s: %w", d.file, err)
		}
	}
	return nil
}
