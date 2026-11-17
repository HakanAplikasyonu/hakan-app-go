package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// demoMizanAccount is a leaf account of Atlas's demo trial balance with its
// cumulative borç / alacak totals at the end of September and October 2026.
type demoMizanAccount struct {
	code, name          string
	sepDebit, sepCredit string
	octDebit, octCredit string
}

// Demo mizan (#393): Atlas Gıda'nın Eylül ve Ekim 2026 Luca genel geçici
// mizanı. Tutarlar kümülatif; her ay borç = alacak (dengeli mizan).
var demoMizan = []demoMizanAccount{
	{"100.01", "MERKEZ KASA TL", "185.400,00", "171.250,00", "212.900,00", "196.300,00"},
	{"102.01", "ZİRAAT BANKASI TL", "2.450.300,00", "2.112.800,00", "2.810.650,00", "2.405.100,00"},
	{"120.01", "YURTİÇİ ALICILAR", "1.842.100,00", "1.560.400,00", "2.115.300,00", "1.790.800,00"},
	{"153.01", "TİCARİ MALLAR", "1.120.000,00", "820.000,00", "1.310.000,00", "955.000,00"},
	{"254.01", "TAŞITLAR", "564.050,90", "0,00", "564.050,90", "0,00"},
	{"191.01", "İNDİRİLECEK KDV %20", "210.400,00", "174.300,00", "246.800,00", "210.400,00"},
	{"320.01", "YURTİÇİ SATICILAR", "905.000,00", "1.250.300,00", "1.060.000,00", "1.442.600,00"},
	{"335.01", "PERSONELE BORÇLAR", "249.481,02", "249.481,02", "373.222,53", "373.222,53"},
	{"360.01", "ÖDENECEK GELİR VERGİSİ", "14.938,80", "22.408,20", "22.408,20", "29.877,60"},
	{"361.01", "ÖDENECEK SGK PRİMLERİ", "61.900,00", "94.287,64", "94.287,64", "126.675,28"},
	{"391.01", "HESAPLANAN KDV %20", "174.300,00", "338.200,00", "210.400,00", "396.100,00"},
	{"500.01", "SERMAYE", "0,00", "500.000,00", "0,00", "500.000,00"},
	{"600.01", "YURTİÇİ SATIŞLAR", "0,00", "1.691.000,00", "0,00", "2.026.478,60"},
	{"621.01", "SATILAN TİCARİ MALLAR MALİYETİ", "820.000,00", "0,00", "955.000,00", "0,00"},
	{"770.01", "GENEL YÖNETİM GİDERLERİ", "386.556,14", "0,00", "477.534,74", "0,00"},
}

var demoMizanNames = map[string]string{
	"100": "KASA", "102": "BANKALAR", "120": "ALICILAR", "153": "TİCARİ MALLAR", "191": "İNDİRİLECEK KDV",
	"254": "TAŞITLAR", "320": "SATICILAR", "335": "PERSONELE BORÇLAR", "360": "ÖDENECEK VERGİ VE FONLAR",
	"361": "ÖDENECEK SOSYAL GÜVENLİK KESİNTİLERİ", "391": "HESAPLANAN KDV", "500": "SERMAYE",
	"600": "YURTİÇİ SATIŞLAR", "621": "SATILAN TİCARİ MALLAR MALİYETİ", "770": "GENEL YÖNETİM GİDERLERİ",
}

// seedMizan uploads Atlas's September and October 2026 trial balances (Luca
// CSV) so the office app's Mizan tab shows the latest mizan compared with
// the previous month. The upload is also the engagement's Luca plan, so
// every code the demo already uses (vouchers, assignments, cari) is in it
// with a zero balance and no export is blocked by ACCOUNT_NOT_IN_TARGET_PLAN.
func seedMizan(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	used, err := demoUsedCodes(ctx, pool, scope)
	if err != nil {
		return err
	}
	svc := usecase.New(repository.New(pool, db.New(pool)))
	sub := usecase.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}
	for i, m := range []struct{ period, title string }{
		{"2026-09", "01.01.2026 - 30.09.2026"},
		{"2026-10", "01.01.2026 - 31.10.2026"},
	} {
		csv := demoMizanCSV(m.title, i == 1, used)
		if _, err := svc.Import(ctx, sub, actor, usecase.DefaultTarget, "atlas-mizan-"+m.period+".csv", m.period, []byte(csv)); err != nil {
			return fmt.Errorf("mizan %s: %w", m.period, err)
		}
	}
	return nil
}

// demoUsedCodes lists the account codes the demo engagement already uses.
func demoUsedCodes(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope) ([]string, error) {
	var codes []string
	err := rls.System(ctx, pool, "demo-seed: mizan codes", func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT account_code::text FROM journal_draft_lines WHERE organization_id = $1 AND engagement_id = $2
			UNION SELECT account_code::text FROM account_assignments WHERE organization_id = $1 AND engagement_id = $2
			UNION SELECT account_code::text FROM counterparty_accounts WHERE organization_id = $1 AND engagement_id = $2`,
			scope.OrganizationID, scope.ID)
		if err != nil {
			return err
		}
		codes, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mizan codes: %w", err)
	}
	return codes, nil
}

// demoMizanCSV builds a Luca "genel geçici mizan": main accounts sum their
// leaves; used codes outside the list are added with zero amounts.
func demoMizanCSV(title string, october bool, used []string) string {
	type amt struct {
		name          string
		debit, credit decimal.Decimal
	}
	parse := func(s string) decimal.Decimal {
		return decimal.RequireFromString(strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", "."))
	}
	lines := map[string]*amt{}
	add := func(code, name string, debit, credit decimal.Decimal) {
		a := lines[code]
		if a == nil {
			a = &amt{name: name}
			lines[code] = a
		}
		a.debit, a.credit = a.debit.Add(debit), a.credit.Add(credit)
	}
	for _, a := range demoMizan {
		d, c := a.sepDebit, a.sepCredit
		if october {
			d, c = a.octDebit, a.octCredit
		}
		add(a.code, a.name, parse(d), parse(c))
		add(a.code[:3], demoMizanNames[a.code[:3]], parse(d), parse(c))
	}
	for _, raw := range used {
		code := usecase.NormalizeCode(raw)
		if !usecase.ValidCode(code) {
			continue
		}
		parts := strings.Split(code, ".")
		for i := range parts {
			c := strings.Join(parts[:i+1], ".")
			if lines[c] == nil {
				name := demoMizanNames[c]
				if name == "" {
					name = "HESAP " + c
				}
				lines[c] = &amt{name: name}
			}
		}
	}
	codes := make([]string, 0, len(lines))
	for c := range lines {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	tr := func(d decimal.Decimal) string {
		if d.IsZero() {
			return ""
		}
		s := d.StringFixed(2)
		intPart, frac := s[:len(s)-3], s[len(s)-2:]
		var b strings.Builder
		for i, r := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(r)
		}
		return b.String() + "," + frac
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ATLAS GIDA TİCARET A.Ş. GENEL GEÇİCİ MİZAN %s\n", title)
	b.WriteString("Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n")
	for _, c := range codes {
		a := lines[c]
		bal := a.debit.Sub(a.credit)
		var db, cb decimal.Decimal
		if bal.IsPositive() {
			db = bal
		} else {
			cb = bal.Neg()
		}
		fmt.Fprintf(&b, "%s;%s;%s;%s;%s;%s\n", c, a.name, tr(a.debit), tr(a.credit), tr(db), tr(cb))
	}
	return b.String()
}
