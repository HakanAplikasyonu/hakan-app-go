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

// demoMizanAccount is a leaf account of Atlas's September 2026 trial
// balance with its cumulative borç / alacak totals (01.01 – 30.09).
type demoMizanAccount struct {
	code, name    string
	debit, credit string
}

// Demo mizan (#393, #456): Atlas Gıda'nın Eylül 2026 Luca genel geçici
// mizanı, dengeli (borç = alacak). 335 / 360 / 361 bakiyeleri Eylül
// bordrosu ve beyanlarının açık borcudur (sepPayrollNet, sepKDVPayable +
// sepMUHSGKPayable, sepSGKPayable); Ekim banka ekstresi bunları öder. Ekim
// mizanı sabit değildir: seedMizanOctober onu Eylül'den ve Luca'ya aktarılan
// Ekim fişlerinden üretir.
var demoMizan = []demoMizanAccount{
	{"100.01", "MERKEZ KASA TL", "185.400,00", "171.250,00"},
	{"102.01", "ZİRAAT BANKASI TL", "2.450.300,00", "2.112.800,00"},
	{"120.01.001", "ÇEŞİTLİ ALICILAR", "1.842.100,00", "1.560.400,00"},
	{"153.01", "TİCARİ MALLAR", "1.120.000,00", "820.000,00"},
	{"254.01", "TAŞITLAR", "564.050,90", "0,00"},
	{"191.01", "İNDİRİLECEK KDV %20", "210.400,00", "174.300,00"},
	{"320.01.001", "ÇEŞİTLİ SATICILAR", "905.000,00", "1.250.300,00"},
	{"335.01", "PERSONELE BORÇLAR", "997.932,08", "1.122.673,59"},
	{"360.01", "ÖDENECEK VERGİ VE FONLAR", "141.225,40", "159.238,56"},
	{"361.01", "ÖDENECEK SGK PRİMLERİ", "405.614,32", "456.316,11"},
	{"391.01", "HESAPLANAN KDV %20", "174.300,00", "338.200,00"},
	{"500.01", "SERMAYE", "0,00", "500.000,00"},
	{"600.01", "YURTİÇİ SATIŞLAR", "0,00", "1.691.000,00"},
	{"621.01", "SATILAN TİCARİ MALLAR MALİYETİ", "820.000,00", "0,00"},
	{"770.01", "GENEL YÖNETİM GİDERLERİ", "540.155,56", "0,00"},
}

var demoMizanNames = map[string]string{
	"100": "KASA", "102": "BANKALAR", "108": "DİĞER HAZIR DEĞERLER", "120": "ALICILAR", "120.01": "YURTİÇİ ALICILAR",
	"153": "TİCARİ MALLAR", "191": "İNDİRİLECEK KDV", "254": "TAŞITLAR", "320": "SATICILAR", "320.01": "YURTİÇİ SATICILAR",
	"335": "PERSONELE BORÇLAR", "360": "ÖDENECEK VERGİ VE FONLAR", "361": "ÖDENECEK SOSYAL GÜVENLİK KESİNTİLERİ",
	"391": "HESAPLANAN KDV", "500": "SERMAYE", "600": "YURTİÇİ SATIŞLAR", "621": "SATILAN TİCARİ MALLAR MALİYETİ",
	"646": "KAMBİYO KARLARI", "770": "GENEL YÖNETİM GİDERLERİ", "780": "FİNANSMAN GİDERLERİ",
}

// mizanAmt is a borç / alacak pair.
type mizanAmt struct{ debit, credit decimal.Decimal }

func trAmount(s string) decimal.Decimal {
	return decimal.RequireFromString(strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", "."))
}

// septemberPostings is demoMizan as leaf postings.
func septemberPostings() map[string]mizanAmt {
	out := map[string]mizanAmt{}
	for _, a := range demoMizan {
		out[a.code] = mizanAmt{trAmount(a.debit), trAmount(a.credit)}
	}
	return out
}

func mizanImporter(pool *pgxpool.Pool, scope engagementctx.Scope) (*usecase.Service, usecase.Subject) {
	return usecase.New(repository.New(pool, db.New(pool))), usecase.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}
}

// seedMizan uploads Atlas's September 2026 trial balance (Luca CSV). The
// upload is also the engagement's Luca plan, so every code the demo already
// uses (vouchers, assignments, cari) is in it with a zero balance and no
// export is blocked by ACCOUNT_NOT_IN_TARGET_PLAN.
func seedMizan(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	used, err := demoUsedCodes(ctx, pool, scope)
	if err != nil {
		return err
	}
	svc, sub := mizanImporter(pool, scope)
	csv := demoMizanCSV("01.01.2026 - 30.09.2026", septemberPostings(), used)
	if _, err := svc.Import(ctx, sub, actor, usecase.DefaultTarget, "atlas-mizan-2026-09.csv", "2026-09", []byte(csv)); err != nil {
		return fmt.Errorf("mizan 2026-09: %w", err)
	}
	return nil
}

// seedMizanOctober uploads the October trial balance: September plus every
// October voucher the portal exported to Luca plus lucaManualEntry. Runs
// after all sources, before the reconciliation, so the office app's Mizan
// tab compares October with September and the reconciliation has a story.
func seedMizanOctober(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	used, err := demoUsedCodes(ctx, pool, scope)
	if err != nil {
		return err
	}
	exported, err := exportedLines(ctx, pool, scope, "2026-10")
	if err != nil {
		return err
	}
	postings := septemberPostings()
	for code, a := range exported {
		addPosting(postings, code, a)
	}
	for _, m := range lucaManualEntry {
		if demoMizanNames[m.code] == "" {
			demoMizanNames[m.code] = m.name
		}
		addPosting(postings, m.code, mizanAmt{decimal.RequireFromString(m.debit), decimal.RequireFromString(m.credit)})
	}
	svc, sub := mizanImporter(pool, scope)
	csv := demoMizanCSV("01.01.2026 - 31.10.2026", postings, used)
	if _, err := svc.Import(ctx, sub, actor, usecase.DefaultTarget, "atlas-mizan-2026-10.csv", "2026-10", []byte(csv)); err != nil {
		return fmt.Errorf("mizan 2026-10: %w", err)
	}
	return nil
}

// addPosting books a voucher line on the mizan. A main-account code ("335")
// lands on the demo leaf of that account ("335.01") the way Luca books it
// on the office's sub-account.
func addPosting(p map[string]mizanAmt, raw string, a mizanAmt) {
	code := usecase.NormalizeCode(raw)
	if !strings.Contains(code, ".") {
		for _, m := range demoMizan {
			if m.code[:3] == code {
				code = m.code
				break
			}
		}
	}
	cur := p[code]
	p[code] = mizanAmt{cur.debit.Add(a.debit), cur.credit.Add(a.credit)}
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

// demoMizanCSV builds a Luca "genel geçici mizan" from leaf postings:
// every upper level sums the leaves under it; used codes outside the
// postings are added with zero amounts. A posting on a code that has codes
// under it moves to its first leaf so no amount sits on a parent.
func demoMizanCSV(title string, postings map[string]mizanAmt, used []string) string {
	type line struct {
		name          string
		debit, credit decimal.Decimal
	}
	lines := map[string]*line{}
	ensure := func(code string) {
		parts := strings.Split(code, ".")
		for i := range parts {
			c := strings.Join(parts[:i+1], ".")
			if lines[c] == nil {
				name := demoMizanNames[c]
				if name == "" {
					name = "HESAP " + c
				}
				lines[c] = &line{name: name}
			}
		}
	}
	for _, a := range demoMizan {
		ensure(a.code)
		lines[a.code].name = a.name
	}
	for code := range postings {
		ensure(code)
	}
	for _, raw := range used {
		if code := usecase.NormalizeCode(raw); usecase.ValidCode(code) {
			ensure(code)
		}
	}
	codes := make([]string, 0, len(lines))
	for c := range lines {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	isLeaf := func(c string) bool {
		i := sort.SearchStrings(codes, c)
		return i+1 >= len(codes) || !strings.HasPrefix(codes[i+1], c+".")
	}
	firstLeaf := func(c string) string {
		for _, x := range codes {
			if strings.HasPrefix(x, c+".") && isLeaf(x) {
				return x
			}
		}
		return c
	}
	for code, a := range postings {
		leaf := code
		if !isLeaf(code) {
			leaf = firstLeaf(code)
		}
		parts := strings.Split(leaf, ".")
		for i := range parts {
			l := lines[strings.Join(parts[:i+1], ".")]
			l.debit, l.credit = l.debit.Add(a.debit), l.credit.Add(a.credit)
		}
	}
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
