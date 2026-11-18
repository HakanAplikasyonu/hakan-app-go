package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// bigSource marks every bulk row of the big demo (source_system).
const bigSource = "demo_big"

// bigBankSource is the bank_transactions source_system (^bank_…$).
const bigBankSource = "bank_demo_big"

// money encodes kuruş as numeric (exact).
func money(k kurus) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(k), Exp: -2, Valid: true}
}

func numf(v float64, scale int) pgtype.Numeric {
	m := int64(v*pow10(scale) + 0.5)
	if v < 0 {
		m = int64(v*pow10(scale) - 0.5)
	}
	return pgtype.Numeric{Int: big.NewInt(m), Exp: int32(-scale), Valid: true} //nolint:gosec // scale ≤ 6
}

func pow10(n int) float64 {
	f := 1.0
	for i := 0; i < n; i++ {
		f *= 10
	}
	return f
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// at is a timestamp on day d: business hours, deterministic per seq.
func at(d time.Time, seq int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 9+seq%9, (seq*7)%60, (seq*13)%60, 0, istanbul)
}

// reserveIDs takes n ids of table's sequence: [first, first+n).
func reserveIDs(ctx context.Context, tx pgx.Tx, table string, n int) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	var first int64
	err := tx.QueryRow(ctx, `WITH s AS (SELECT pg_get_serial_sequence($1, 'id') AS seq)
		SELECT setval((SELECT seq FROM s)::regclass, nextval((SELECT seq FROM s)::regclass) + $2 - 1) - $2 + 1`,
		table, n).Scan(&first)
	return first, err
}

// copyRows inserts rows into table with multi-row INSERTs in the caller's
// transaction. COPY would be faster but Postgres refuses COPY FROM on tables
// with row-level security for a non-superuser owner (FORCE RLS, #312), so
// the rows go in batches of up to 60k parameters.
func copyRows(ctx context.Context, tx pgx.Tx, table string, cols []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	per := 60000 / len(cols)
	if per > 2000 {
		per = 2000
	}
	head := "INSERT INTO " + pgx.Identifier{table}.Sanitize() + " (" + strings.Join(cols, ", ") + ") VALUES "
	for start := 0; start < len(rows); start += per {
		end := min(start+per, len(rows))
		var b strings.Builder
		b.WriteString(head)
		args := make([]any, 0, (end-start)*len(cols))
		for i, r := range rows[start:end] {
			if len(r) != len(cols) {
				return fmt.Errorf("insert %s: row has %d values, want %d", table, len(r), len(cols))
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			for j := range r {
				if j > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "$%d", len(args)+j+1)
			}
			b.WriteByte(')')
			args = append(args, r...)
		}
		if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	return nil
}

// bigCounts collects the row counts for the summary.
type bigCounts struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *bigCounts) add(table string, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[table] += n
}

func (c *bigCounts) print() {
	keys := make([]string, 0, len(c.n))
	for k := range c.n {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-30s %8d\n", k, c.n[k])
	}
}

// buildModels generates every client's model in parallel (independent
// random streams, so the order does not matter).
func buildModels(w *bigWorld, o *bigOffice) []*clientModel {
	out := make([]*clientModel, len(o.taxpayers))
	var wg sync.WaitGroup
	for i := range o.taxpayers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i] = buildClient(w, &o.taxpayers[i])
		}(i)
	}
	wg.Wait()
	return out
}

// docIDs keeps the database ids of a client's documents for later steps.
type docIDs struct {
	invoice  []int64 // by m.invoices index
	customer map[int]int64
	box      int64
	agent    int64
}

// writeDocuments writes customers, invoices (tax summaries, lines), Z
// reports and expenses of one office in one system transaction.
func writeDocuments(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) ([]*docIDs, error) {
	ids := make([]*docIDs, len(ms))
	err := rls.System(ctx, pool, "demo-seed: big documents", func(ctx context.Context, tx pgx.Tx) error {
		var nCust, nInv, nSum, nItem, nZ, nZL, nExp int
		for _, m := range ms {
			nCust += len(m.cps) + 1
			nInv += len(m.invoices)
			for i := range m.invoices {
				nItem += len(m.invoices[i].lines)
				nSum += len(summaries(&m.invoices[i]))
			}
			nZ += len(m.zs)
			for i := range m.zs {
				nZL += len(m.zs[i].bases)
			}
			nExp += len(m.expenses)
		}
		custID, err := reserveIDs(ctx, tx, "customers", nCust)
		if err != nil {
			return err
		}
		invID, err := reserveIDs(ctx, tx, "invoices", nInv)
		if err != nil {
			return err
		}
		zID, err := reserveIDs(ctx, tx, "retail_sales", nZ)
		if err != nil {
			return err
		}
		var cust, inv, sums, items, zs, zls, exps [][]any
		for k, m := range ms {
			d := &docIDs{customer: map[int]int64{}, invoice: make([]int64, len(m.invoices))}
			ids[k] = d
			tp := m.c.taxpayerID
			created := at(periodStart(m.periods[0]), k)
			for _, ci := range m.cps {
				cp := w.cps[ci]
				typ := "both"
				isC, isS := containsInt(m.customers, ci), containsInt(m.suppliers, ci)
				if isC && !isS {
					typ = "customer"
				} else if isS && !isC {
					typ = "supplier"
				}
				d.customer[ci] = custID
				banks, _ := json.Marshal([]map[string]string{{"iban": cp.IBAN}})
				cust = append(cust, []any{custID, tp, bigSource, "cp:" + cp.TaxID, "api", typ, "corporate", cp.Name, cp.Name,
					cp.TaxID, cp.District, cp.City + " " + cp.District + " Mah. Demo Sk. No:" + fmt.Sprint(1+ci%90), cp.City, cp.District,
					"Türkiye", "muhasebe@" + strings.ToLower(numberPrefix(cp.Name)) + fmt.Sprint(ci) + ".example.com",
					fmt.Sprintf("0555 000 %02d %02d", ci/100%100, ci%100), string(banks), created, created})
				custID++
			}
			// Nihai tüketici (e-arşiv / perakende).
			d.customer[-1] = custID
			cust = append(cust, []any{custID, tp, bigSource, "cp:consumer", "api", "customer", "individual", "Nihai Tüketici", "Nihai Tüketici",
				nil, nil, nil, m.c.City.Name, m.c.District, "Türkiye", nil, nil, "[]", created, created})
			custID++
			today := m.today(w)
			for i := range m.invoices {
				doc := &m.invoices[i]
				d.invoice[i] = invID
				var cpTax, cpID any
				name := "Nihai Tüketici"
				var cpIdentity any
				if doc.cp >= 0 {
					cpTax, name = w.cps[doc.cp].TaxID, w.cps[doc.cp].Name
				} else {
					cpIdentity = "11111111111"
				}
				cpID = d.customer[doc.cp]
				status, pay := "active", "paid"
				var remaining any
				switch {
				case doc.cancelled:
					status, pay = "cancelled", "unknown"
				case doc.cp < 0:
				case doc.paidOn.IsZero() && doc.dueOn.Before(today):
					pay, remaining = "overdue", money(doc.payable())
				case doc.paidOn.IsZero():
					pay, remaining = "unpaid", money(doc.payable())
				}
				var rate any
				if doc.currency != "TRY" {
					rate = numf(doc.rate, 6)
				}
				var exemption any
				if doc.exemption != "" {
					exemption = doc.exemption
				}
				ts := at(doc.date, i)
				inv = append(inv, []any{invID, tp, bigSource, doc.srcID, "api", pgtype.UUID{Bytes: doc.ettn, Valid: true}, doc.number,
					doc.dir, doc.typ, doc.typ, doc.profile, doc.profile, statusLabel(doc), status, doc.date, ts.Format("15:04:05"),
					doc.dueOn, doc.period, cpID, cpTax, cpIdentity, name, doc.currency, rate,
					money(doc.base), money(doc.base), money(doc.vat), money(doc.base + doc.vat), money(doc.withholding),
					money(doc.payable()), money(doc.try(doc.payable())), exemption, pay, remaining, true, "items", ts, ts})
				for _, s := range summaries(doc) {
					sums = append(sums, []any{tp, invID, numf(s.rate, 2), money(s.base), money(s.vat), money(s.withholding), nullText(s.whCode), nullText(s.exemption)})
				}
				for li, l := range doc.lines {
					unit := float64(l.base) / 100 / l.qty
					items = append(items, []any{tp, invID, li + 1, l.name, numf(l.qty, 6), l.unit, numf(unit, 6), money(l.base), money(l.base),
						numf(l.rate, 2), money(l.vat), money(l.base + l.vat), nullText(l.whCode), numf(l.whRatio*100, 2), money(l.withholding),
						nullText(l.exemption), l.lineType})
				}
				invID++
			}
			for i := range m.zs {
				z := &m.zs[i]
				b, v := z.totals()
				st := "approved"
				var by, byAt any
				if z.approved {
					by, byAt = o.accountant(), at(z.date.AddDate(0, 0, 2), i)
				} else {
					st = "pending"
				}
				ts := at(z.date, i).Add(10 * time.Hour)
				zs = append(zs, []any{zID, tp, bigSource, fmt.Sprintf("z:%d", z.number), "excel_import", z.date, z.period,
					fmt.Sprintf("%04d", z.number), m.okcSerial, money(b), money(v), money(b + v), st, by, byAt,
					money(z.cash), money(z.card), z.receipts, ts, ts})
				rates := make([]float64, 0, len(z.bases))
				for r := range z.bases {
					rates = append(rates, r)
				}
				sort.Float64s(rates)
				for _, r := range rates {
					zls = append(zls, []any{tp, zID, numf(r, 2), money(z.bases[r]), money(z.vats[r]), ts})
				}
				zID++
			}
			for i := range m.expenses {
				e := &m.expenses[i]
				st := "pending"
				var by, byAt any
				if e.approved {
					st, by, byAt = "approved", o.accountant(), at(e.date.AddDate(0, 0, 3), i)
				}
				ts := at(e.date, i)
				exps = append(exps, []any{tp, bigSource, fmt.Sprintf("exp:%d", i), "ocr", e.date, e.period, money(e.base + e.vat),
					numf(e.rate, 2), money(e.base), money(e.vat), e.description, e.receiptType, e.vendor, e.receiptNo,
					nullText(e.plate), e.payment, "yes", st, numf(0.93, 4), by, byAt, ts, ts})
			}
		}
		steps := []struct {
			table string
			cols  []string
			rows  [][]any
		}{
			{"customers", []string{"id", "taxpayer_id", "source_system", "source_id", "capture_method", "type", "customer_legal_type",
				"trade_name", "name", "tax_number", "tax_office", "address", "city", "district", "country", "email", "phone",
				"bank_accounts", "created_at", "updated_at"}, cust},
			{"invoices", []string{"id", "taxpayer_id", "source_system", "source_id", "capture_method", "ettn", "invoice_number",
				"direction", "type", "invoice_type_code_raw", "profile", "profile_raw", "status", "status_normalized", "issue_date",
				"issue_time", "payment_due_date", "period", "customer_id", "customer_tax_number", "customer_identity_number",
				"customer_name", "currency", "exchange_rate", "line_extension_amount", "tax_exclusive_amount", "tax_amount",
				"tax_inclusive_amount", "withholding_amount", "payable_amount", "payable_amount_try", "global_tax_exemption_code",
				"payment_status", "remaining_amount", "has_items", "tax_summary_source", "created_at", "updated_at"}, inv},
			{"invoice_tax_summaries", []string{"taxpayer_id", "invoice_id", "tax_rate", "base", "vat", "withholding",
				"withholding_code", "tax_exemption_code"}, sums},
			{"invoice_items", []string{"taxpayer_id", "invoice_id", "line_no", "name", "quantity", "unit_code", "unit_price",
				"line_extension_amount", "tax_exclusive_amount", "tax_rate", "total_tax_amount", "tax_inclusive_amount",
				"withholding_code", "withholding_rate", "withholding_amount", "tax_exemption_code", "line_type"}, items},
			{"retail_sales", []string{"id", "taxpayer_id", "source_system", "source_id", "capture_method", "sale_date", "period",
				"z_report_number", "okc_serial_number", "subtotal_amount", "tax_amount", "total_amount", "approval_status",
				"approved_by", "approved_at", "cash_amount", "card_amount", "receipt_count", "created_at", "updated_at"}, zs},
			{"retail_sale_vat_lines", []string{"taxpayer_id", "retail_sale_id", "tax_rate", "tax_base", "tax_amount", "created_at"}, zls},
			{"expenses", []string{"taxpayer_id", "source_system", "source_id", "capture_method", "expense_date", "period", "amount",
				"tax_rate", "tax_exclusive_amount", "tax_amount", "description", "receipt_type", "vendor_name", "receipt_number",
				"vehicle_plate", "payment_method", "is_deductible", "approval_status", "ocr_confidence", "approved_by",
				"approved_at", "created_at", "updated_at"}, exps},
		}
		for _, s := range steps {
			if err := copyRows(ctx, tx, s.table, s.cols, s.rows); err != nil {
				return err
			}
			cnt.add(s.table, len(s.rows))
		}
		return nil
	})
	return ids, err
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func statusLabel(d *invDoc) string {
	if d.cancelled {
		return "İptal"
	}
	return "Onaylandı"
}

type taxSummary struct {
	rate                   float64
	base, vat, withholding kurus
	whCode, exemption      string
}

func summaries(d *invDoc) []taxSummary {
	var out []taxSummary
	for _, l := range d.lines {
		found := false
		for i := range out {
			if out[i].rate == l.rate && out[i].whCode == l.whCode && out[i].exemption == l.exemption {
				out[i].base += l.base
				out[i].vat += l.vat
				out[i].withholding += l.withholding
				found = true
			}
		}
		if !found {
			out = append(out, taxSummary{rate: l.rate, base: l.base, vat: l.vat, withholding: l.withholding, whCode: l.whCode, exemption: l.exemption})
		}
	}
	return out
}

var bankNames = map[string][2]string{
	"00010": {"ziraat", "Ziraat Bankası"}, "00012": {"halkbank", "Halkbank"}, "00015": {"vakifbank", "VakıfBank"},
	"00046": {"akbank", "Akbank"}, "00062": {"garanti", "Garanti BBVA"}, "00064": {"isbank", "İş Bankası"},
	"00067": {"yapikredi", "Yapı Kredi"},
}

// writeLedger writes the bank account, its movements, matches and postings,
// the vouchers (yevmiye) with their Luca exports and transfers, and the
// monthly trial balances (mizan) of one office.
func writeLedger(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, ids []*docIDs, cnt *bigCounts) error {
	return rls.System(ctx, pool, "demo-seed: big ledger", func(ctx context.Context, tx pgx.Tx) error {
		var agent int64
		if err := tx.QueryRow(ctx, `INSERT INTO desktop_agents (organization_id, machine_id, name, platform, version, created_by, last_seen_at)
			VALUES ($1, $2, 'Ofis bilgisayarı', 'windows', '1.4.2', $3, $4)
			ON CONFLICT (organization_id, machine_id) DO UPDATE SET name = EXCLUDED.name RETURNING id`,
			o.orgID, fmt.Sprintf("%s-pc-01", o.def.Slug), o.owner(), w.now).Scan(&agent); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
		var nTxn, nDraft, nBatch, nImp int
		for _, m := range ms {
			nTxn += len(m.bank)
			nDraft += len(m.vouchers)
			for _, p := range m.periods {
				if p < w.periods[len(w.periods)-1] {
					nImp++
				}
			}
			nBatch += len(m.periods)
		}
		boxID, err := reserveIDs(ctx, tx, "cash_boxes", len(ms))
		if err != nil {
			return err
		}
		txnID, err := reserveIDs(ctx, tx, "bank_transactions", nTxn)
		if err != nil {
			return err
		}
		draftID, err := reserveIDs(ctx, tx, "journal_drafts", nDraft)
		if err != nil {
			return err
		}
		batchID, err := reserveIDs(ctx, tx, "export_batches", nBatch)
		if err != nil {
			return err
		}
		impID, err := reserveIDs(ctx, tx, "account_plan_imports", nImp)
		if err != nil {
			return err
		}
		var boxes, ledgers, txns, srcs, matches, postings, drafts, dlines, batches, bitems, transfers, imps, tbl, plan [][]any
		for k, m := range ms {
			c := m.c
			tp, eng := c.taxpayerID, c.engagementID
			bank := bankNames[m.iban[4:9]]
			created := at(periodStart(m.periods[0]), k)
			boxes = append(boxes, []any{boxID, tp, "bank_account", bank[1] + " TL vadesiz", bank[0], m.iban, "TRY", "demand", created, created})
			ledgers = append(ledgers, []any{o.orgID, eng, tp, boxID, "102.01", o.accountant(), created, created})
			ids[k].box = boxID
			var lastDay time.Time
			daySeq := 0
			for i := range m.bank {
				t := &m.bank[i]
				if !t.date.Equal(lastDay) {
					lastDay, daySeq = t.date, 0
				}
				daySeq++
				// Booking time follows the statement order (balance chain).
				ts := time.Date(t.date.Year(), t.date.Month(), t.date.Day(), 9, 0, 0, 0, istanbul).Add(time.Duration(daySeq*7) * time.Minute)
				var cpTax any
				if t.cpTaxID != "" {
					cpTax = t.cpTaxID
				}
				norm := strings.ToUpper(slugASCII(t.desc))
				txns = append(txns, []any{txnID, tp, boxID, bigBankSource, t.srcID, "excel_import", ts, true, daySeq, t.date, t.date, t.dir,
					money(t.amount), "TRY", money(t.balance), t.desc, norm, nullText(t.cpName), nullText(t.cpIBAN), cpTax,
					t.kind, t.kindDetail, "keyword", numf(0.92, 2), ts, t.match, t.period, ts, ts})
				srcs = append(srcs, []any{tp, txnID, boxID, bigBankSource, "excel_import", t.srcID, ts})
				if t.invoice >= 0 && t.match == "matched" {
					matches = append(matches, []any{tp, txnID, "invoice", ids[k].invoice[t.invoice], money(t.amount), 100, "auto", ts})
				}
				if t.match != "unmatched" {
					st, src := "approved", "kind_default"
					var by, byAt any = o.accountant(), ts.Add(48 * time.Hour)
					if t.match == "suggested" {
						st, by, byAt = "suggested", nil, nil
					}
					if t.invoice >= 0 {
						src = "invoice_match"
					}
					postings = append(postings, []any{o.orgID, eng, tp, txnID, t.account, accountNames[t.account], st, numf(0.9, 4),
						src, "Banka hareketi türünden", by, byAt, ts, ts})
				}
				txnID++
			}
			// Vouchers, numbered per period.
			entry := map[string]int{}
			byPeriod := map[string][]int64{}
			for i := range m.vouchers {
				v := &m.vouchers[i]
				entry[v.period]++
				if v.entryType == "" {
					v.entryType = "mahsup"
				}
				ts := at(v.date, i)
				var by, byAt any
				if v.status != "draft" {
					by, byAt = o.accountant(), ts.Add(24*time.Hour)
				}
				origin, _ := json.Marshal(map[string]any{"source": v.source, "demo": "big"})
				drafts = append(drafts, []any{draftID, o.orgID, eng, tp, v.period, entry[v.period], v.entryType, v.date, v.desc,
					v.docType, v.status, string(origin), by, byAt, o.accountant(), ts, ts})
				for li, l := range v.lines {
					dlines = append(dlines, []any{o.orgID, eng, draftID, li + 1, l.account, l.desc, money(l.debit), money(l.credit), v.date})
				}
				if v.status == "exported" {
					byPeriod[v.period] = append(byPeriod[v.period], draftID)
				}
				draftID++
			}
			last := w.periods[len(w.periods)-1]
			for _, p := range m.periods {
				dr := byPeriod[p]
				if len(dr) == 0 {
					continue
				}
				done := periodStart(p).AddDate(0, 1, 8+k%10)
				if done.After(w.now) {
					done = w.now.Add(-time.Duration(2+k%20) * time.Hour)
				}
				files, _ := json.Marshal([]map[string]any{{"name": "luca_fis.xlsx", "size": 6000 + 97*len(dr), "entries": len(dr),
					"sha256": sha(fmt.Sprintf("%d:%s", eng, p)), "content_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
					"storage_key": fmt.Sprintf("exports/journals/%d/demo-big-%s/luca_fis.xlsx", o.orgID, p)}})
				batches = append(batches, []any{batchID, o.orgID, eng, tp, "luca", "unverified-2", false, p, string(files), "[]", o.accountant(), done})
				for _, id := range dr {
					bitems = append(bitems, []any{o.orgID, eng, batchID, id, done})
				}
				status, errCode, errMsg := "imported", any(nil), any(nil)
				var completed any = done.Add(20 * time.Minute)
				if c.Problem && p == w.periods[len(w.periods)-3] {
					status, errCode, errMsg, completed = "failed", "FOLDER_MISSING",
						"Aktarım klasörü bulunamadı. Klasör taşınmış ya da ağ sürücüsü bağlı değil.", nil
				}
				delivered, _ := json.Marshal([]map[string]any{{"name": "luca_fis.xlsx", "size": 6000 + 97*len(dr), "sha256": sha(fmt.Sprintf("%d:%s", eng, p))}})
				transfers = append(transfers, []any{o.orgID, eng, batchID, "luca", "agent_folder", status, agent, string(delivered),
					"Luca aktarım", errCode, errMsg, o.accountant(), o.accountant(), done.Add(time.Minute), done.Add(2 * time.Minute), completed, done, done})
				batchID++
			}
			// Monthly mizan (Luca): closed months; the latest one also carries
			// the Luca-only entries of the problem clients.
			var latest []mizanRow
			var latestImp int64
			for _, p := range m.periods {
				if p >= last {
					continue
				}
				rows := m.mizan(p, m.lucaManual(w, p))
				ts := periodStart(p).AddDate(0, 1, 9+k%5)
				if ts.After(w.now) {
					ts = w.now.Add(-time.Hour)
				}
				imps = append(imps, []any{impID, o.orgID, eng, "luca", "csv", "trial_balance",
					fmt.Sprintf("mizan-%s.csv", p), sha(fmt.Sprintf("mizan:%d:%s", eng, p)), len(rows), 0, o.accountant(), ts, p})
				for _, r := range rows {
					tbl = append(tbl, []any{o.orgID, eng, impID, r.code, r.name, r.code[:3], r.level, r.leaf, money(r.debit), money(r.credit), money(r.debit - r.credit)})
				}
				latest, latestImp = rows, impID
				impID++
			}
			for _, r := range latest {
				plan = append(plan, []any{o.orgID, eng, latestImp, "luca", r.code, r.name, r.code[:3], r.level, r.leaf})
			}
			ids[k].agent = agent
			boxID++
		}
		steps := []struct {
			table string
			cols  []string
			rows  [][]any
		}{
			{"cash_boxes", []string{"id", "taxpayer_id", "type", "name", "bank_id", "iban", "currency", "bank_account_kind", "created_at", "updated_at"}, boxes},
			{"bank_ledger_accounts", []string{"organization_id", "engagement_id", "taxpayer_id", "cash_box_id", "account_code", "updated_by", "created_at", "updated_at"}, ledgers},
			{"bank_transactions", []string{"id", "taxpayer_id", "cash_box_id", "source_system", "source_id", "capture_method", "booked_at",
				"has_time", "bank_sequence", "transaction_date", "value_date", "debit_credit", "amount", "currency", "balance_after", "description",
				"description_normalized", "counterparty_name", "counterparty_iban", "counterparty_tax_number", "kind", "kind_detail",
				"kind_source", "kind_confidence", "classified_at", "match_status", "period", "created_at", "updated_at"}, txns},
			{"bank_transaction_sources", []string{"taxpayer_id", "bank_transaction_id", "cash_box_id", "source_system", "capture_method", "source_id", "received_at"}, srcs},
			{"bank_transaction_matches", []string{"taxpayer_id", "bank_transaction_id", "target_type", "target_id", "amount", "score", "method", "matched_at"}, matches},
			{"bank_transaction_postings", []string{"organization_id", "engagement_id", "taxpayer_id", "bank_transaction_id", "account_code",
				"account_name", "status", "confidence", "source", "reason", "approved_by", "approved_at", "created_at", "updated_at"}, postings},
			{"journal_drafts", []string{"id", "organization_id", "engagement_id", "taxpayer_id", "period", "entry_no", "entry_type", "entry_date",
				"description", "document_type", "status", "origin", "approved_by", "approved_at", "created_by", "created_at", "updated_at"}, drafts},
			{"journal_draft_lines", []string{"organization_id", "engagement_id", "journal_draft_id", "line_no", "account_code", "description",
				"debit", "credit", "document_date"}, dlines},
			{"export_batches", []string{"id", "organization_id", "engagement_id", "taxpayer_id", "driver", "template_version",
				"template_verified", "period", "files", "issues", "created_by", "created_at"}, batches},
			{"export_batch_items", []string{"organization_id", "engagement_id", "export_batch_id", "journal_draft_id", "created_at"}, bitems},
			{"accounting_transfers", []string{"organization_id", "engagement_id", "export_batch_id", "driver", "method", "status", "agent_id",
				"delivered_files", "folder_label", "error_code", "error_message", "requested_by", "completed_by", "claimed_at",
				"delivered_at", "completed_at", "created_at", "updated_at"}, transfers},
			{"account_plan_imports", []string{"id", "organization_id", "engagement_id", "target", "source_format", "source_kind",
				"file_name", "sha256", "account_count", "skipped_rows", "imported_by", "created_at", "period"}, imps},
			{"trial_balance_lines", []string{"organization_id", "engagement_id", "import_id", "code", "name", "main_code", "level",
				"is_leaf", "debit", "credit", "balance"}, tbl},
			{"account_plan_accounts", []string{"organization_id", "engagement_id", "import_id", "target", "code", "name", "main_code",
				"level", "is_leaf"}, plan},
		}
		for _, s := range steps {
			if err := copyRows(ctx, tx, s.table, s.cols, s.rows); err != nil {
				return err
			}
			cnt.add(s.table, len(s.rows))
		}
		return nil
	})
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
