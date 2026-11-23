package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ledgers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Mükellef defterleri (#780): "Defterler" sekmesi boş görünmesin. Kaya
// Kırtasiye'nin (işletme defteri) üç ay önceki ve geçen ayki Luca işletme
// defteri dökümü yüklü, aradaki ay eksik görünür; Atlas Gıda'nın (bilanço)
// geçen ayki e-defter yevmiyesi yüklü. Dosyalar gerçek servisten geçer
// (depoya yazılır, özet okunur); aynı içerik tekrar çalıştırmada çoğalmaz.

// demoIsletmeCSV is a small Luca style işletme defteri export of a month.
func demoIsletmeCSV(period string, grow int) string {
	y, m := period[:4], period[5:7]
	d := func(day string) string { return day + "." + m + "." + y }
	amt := func(base int) string {
		v := base + base*grow/10
		s := fmt.Sprintf("%d", v)
		var b strings.Builder
		for i, c := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(c)
		}
		return b.String() + ",00"
	}
	rows := []string{
		"İŞLETME DEFTERİ (" + period + ")",
		"Tarih;Evrak No;Açıklama;Satın Alınan Mallar;Giderler;KDV;Satılan Mallar",
		d("03") + ";KRT-01;Anadolu Kağıt Toptan - kağıt alışı;" + amt(42000) + ";;" + amt(8400) + ";",
		d("08") + ";OFS-02;Ofis Malzemeleri Dağıtım - kırtasiye alışı;" + amt(18500) + ";;" + amt(3700) + ";",
		d("10") + ";KRA-03;Dükkân kirası;;" + amt(15000) + ";;",
		d("15") + ";ELK-04;Elektrik faturası;;" + amt(1900) + ";" + amt(380) + ";",
		d("20") + ";Z-05;Perakende satışlar (Z raporları);;;;" + amt(61000),
		d("28") + ";KYA-06;Yıldız Koleji - toptan satış;;;;" + amt(26000),
		d("30") + ";;Toplam;" + amt(60500) + ";" + amt(16900) + ";" + amt(12480) + ";" + amt(87000),
	}
	return strings.Join(rows, "\n") + "\n"
}

// demoYevmiyeXML is a short e-defter yevmiye (XBRL-GL) of a month.
func demoYevmiyeXML(period string) string {
	line := func(code, name, amount, dc string) string {
		return `<gl-cor:entryDetail><gl-cor:account><gl-cor:accountMainID>` + code +
			`</gl-cor:accountMainID><gl-cor:accountMainDescription>` + name +
			`</gl-cor:accountMainDescription></gl-cor:account><gl-cor:amount>` + amount +
			`</gl-cor:amount><gl-cor:debitCreditCode>` + dc + `</gl-cor:debitCreditCode></gl-cor:entryDetail>`
	}
	entries := [][]string{
		{line("153", "TİCARİ MALLAR", "250000.00", "D"), line("191", "İNDİRİLECEK KDV", "50000.00", "D"), line("320", "SATICILAR", "300000.00", "C")},
		{line("120", "ALICILAR", "480000.00", "D"), line("600", "YURTİÇİ SATIŞLAR", "400000.00", "C"), line("391", "HESAPLANAN KDV", "80000.00", "C")},
		{line("102", "BANKALAR", "350000.00", "D"), line("120", "ALICILAR", "350000.00", "C")},
		{line("320", "SATICILAR", "200000.00", "D"), line("102", "BANKALAR", "200000.00", "C")},
		{line("770", "GENEL YÖNETİM GİDERLERİ", "45000.00", "D"), line("102", "BANKALAR", "45000.00", "C")},
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<edefter:defter xmlns:edefter="http://www.edefter.gov.tr" xmlns:xbrli="http://www.xbrl.org/2003/instance" xmlns:gl-cor="http://www.xbrl.org/int/gl/cor/2006-10-25"><xbrli:xbrl><gl-cor:accountingEntries>`)
	b.WriteString(`<gl-cor:documentInfo><gl-cor:entriesType>journal</gl-cor:entriesType><gl-cor:periodCoveredStart>` + period +
		`-01</gl-cor:periodCoveredStart></gl-cor:documentInfo>`)
	for _, e := range entries {
		b.WriteString("<gl-cor:entryHeader>" + strings.Join(e, "") + "</gl-cor:entryHeader>")
	}
	b.WriteString("</gl-cor:accountingEntries></xbrli:xbrl></edefter:defter>\n")
	return b.String()
}

// seedLedgers uploads the demo books.
func seedLedgers(ctx context.Context, pool *pgxpool.Pool, orgID int64, now time.Time) error {
	month := func(back int) string {
		t := now.In(foreigninvoices.Istanbul)
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -back, 0).Format("2006-01")
	}
	type upload struct {
		taxID, name, kind, period, body string
	}
	kaya, atlas := "20780000120", "9000000017"
	files := []upload{
		{kaya, "Kaya_isletme_defteri_" + month(3) + ".csv", "isletme_defteri", month(3), demoIsletmeCSV(month(3), 0)},
		{kaya, "Kaya_isletme_defteri_" + month(1) + ".csv", "isletme_defteri", month(1), demoIsletmeCSV(month(1), 2)},
		{atlas, atlas + "-" + strings.ReplaceAll(month(1), "-", "") + "-Y-000001.xml", "e_defter_yevmiye", month(1), demoYevmiyeXML(month(1))},
	}
	return rls.System(ctx, pool, "demo-seed: ledgers", func(ctx context.Context, tx pgx.Tx) error {
		q := db.New(database.Ambient{Pool: pool})
		svc := ledgers.New(q, payrollStore{up: seedStore}, nil)
		for _, f := range files {
			var engID int64
			err := tx.QueryRow(ctx, `SELECT e.id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
				WHERE e.organization_id = $1 AND t.tax_id = $2 AND e.status = 'active' ORDER BY e.id DESC LIMIT 1`,
				orgID, f.taxID).Scan(&engID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("ledger engagement: %w", err)
			}
			if _, err := svc.Upload(ctx, ledgers.Subject{OrganizationID: orgID, EngagementID: engID}, ledgers.UploadInput{
				FileName: f.name, Data: []byte(f.body), Period: f.period, Kind: f.kind,
			}); err != nil {
				return fmt.Errorf("ledger %s: %w", f.name, err)
			}
		}
		return nil
	})
}
