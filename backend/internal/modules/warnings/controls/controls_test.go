package controls

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// sample is one invoice of the labeled data set: expected lists the codes a
// careful accountant would want raised on it (on the invoice or its line).
type sample struct {
	label    string
	dir      string
	party    string
	date     string
	amount   string
	item     string
	expected []string
}

// The office's history: a consulting firm buying office supplies.
func history() []sample {
	var out []sample
	for m := 1; m <= 9; m++ {
		d := fmt.Sprintf("2026-%02d-10", m)
		out = append(out,
			sample{dir: "incoming", party: "1111111111", date: d, amount: "10000", item: "Kırtasiye malzemesi"},
			sample{dir: "incoming", party: "2222222222", date: d, amount: "1500", item: "Toner kartuş"},
			sample{dir: "incoming", party: "3333333333", date: d, amount: "800", item: "Temizlik malzemesi"},
			sample{dir: "outgoing", party: "5555555555", date: d, amount: "12000", item: "Danışmanlık hizmeti"},
			sample{dir: "outgoing", party: "6666666666", date: d, amount: "15000", item: "Yazılım danışmanlık hizmeti"},
		)
	}
	return out
}

// The period under test (2026-10), labeled.
func period() []sample {
	return []sample{
		{label: "olağan kırtasiye", dir: "incoming", party: "1111111111", date: "2026-10-05", amount: "10500", item: "Kırtasiye malzemesi"},
		// 45.000 TL is 4.5 times this supplier's median (10.000): below the
		// 10× multiplier and the 50.000 TL floor (000255), not unusual.
		{label: "dizüstü bilgisayar, yüksek tutar", dir: "incoming", party: "1111111111", date: "2026-10-06", amount: "45000", item: "Dizüstü bilgisayar",
			expected: []string{"FIXED_ASSET_CANDIDATE", "UNUSUAL_RECORD_TYPE"}},
		{label: "yeni tedarikçi, ofis masası", dir: "incoming", party: "9999999991", date: "2026-10-07", amount: "80000", item: "Ofis masası ve sandalye",
			// No history with this supplier: 05 compares with the taxpayer's
			// own purchases (median 1.500). Below the 100.000 TL new party limit.
			expected: []string{"FIXED_ASSET_CANDIDATE", "UNUSUAL_AMOUNT", "UNUSUAL_RECORD_TYPE"}},
		{label: "masa lambası (demirbaş değil)", dir: "incoming", party: "2222222222", date: "2026-10-08", amount: "300", item: "Masa lambası"},
		{label: "yazıcı toneri (sarf)", dir: "incoming", party: "2222222222", date: "2026-10-09", amount: "1400", item: "Yazıcı toneri"},
		{label: "olağan temizlik", dir: "incoming", party: "3333333333", date: "2026-10-10", amount: "850", item: "Temizlik malzemesi"},
		{label: "olağan danışmanlık", dir: "outgoing", party: "5555555555", date: "2026-10-11", amount: "12500", item: "Danışmanlık hizmeti Ekim"},
		{label: "araç satışı", dir: "outgoing", party: "6666666666", date: "2026-10-12", amount: "950000", item: "Fiat Egea araç satışı",
			expected: []string{"UNUSUAL_AMOUNT", "UNUSUAL_RECORD_TYPE"}},
		{label: "yeni müşteri, olağan tutar", dir: "outgoing", party: "7777777777", date: "2026-10-13", amount: "20000", item: "Danışmanlık hizmeti"},
		{label: "yeni müşteri, yüksek tutar", dir: "outgoing", party: "8888888888", date: "2026-10-14", amount: "120000", item: "Yazılım danışmanlık hizmeti",
			expected: []string{"NEW_PARTY_HIGH_AMOUNT"}},
		{label: "olağan danışmanlık 2", dir: "outgoing", party: "5555555555", date: "2026-10-15", amount: "11800", item: "Danışmanlık hizmeti"},
	}
}

// TestControlsOnLabeledSample is the #31 acceptance check: the expected
// warnings appear and the false-positive rate is measured (and logged).
func TestControlsOnLabeledSample(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 4000000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	insert := func(s sample, n int) int64 {
		t.Helper()
		var invID int64
		day, _ := time.Parse("2006-01-02", s.date)
		if err := d.Pool.QueryRow(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
			direction, type, issue_date, period, currency, tax_summary_source, customer_tax_number, customer_name, payable_amount, tax_exclusive_amount)
			VALUES ($1, 'test', $2::text, 'manual', $2::text, $3::text, CASE WHEN $3::text = 'incoming' THEN 'ALIS' ELSE 'SATIS' END, $4, $5, 'TRY', 'items', $6::text, $6::text, $7::numeric, $7::numeric)
			RETURNING id`, tp, fmt.Sprintf("T-%d", n), s.dir, day, day.Format("2006-01"), s.party, s.amount).Scan(&invID); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Pool.Exec(ctx, `INSERT INTO invoice_items (taxpayer_id, invoice_id, line_no, name, tax_exclusive_amount)
			VALUES ($1, $2, 1, $3, $4::numeric)`, tp, invID, s.item, s.amount); err != nil {
			t.Fatal(err)
		}
		return invID
	}
	n := 0
	for _, s := range history() {
		n++
		insert(s, n)
	}
	labeled := period()
	invIDs := make([]int64, len(labeled))
	for i, s := range labeled {
		n++
		invIDs[i] = insert(s, n)
	}

	q := db.New(d.Pool)
	r := New(q, engine.New(repository.NewWarnings(q), repository.NewConfig(q)))
	subj := engine.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	res, err := r.Run(ctx, subj, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if res.Invoices != len(labeled) || res.Items != len(labeled) {
		t.Fatalf("result %+v", res)
	}

	// What was raised, per sample (invoice warnings + its line's warnings).
	got := map[int64]map[string]bool{}
	rows, err := d.Pool.Query(ctx, `
		SELECT w.code, CASE WHEN w.entity_type = 'invoice' THEN w.entity_id ELSE it.invoice_id END
		FROM data_warnings w
		LEFT JOIN invoice_items it ON w.entity_type = 'invoice_item' AND it.taxpayer_id = w.taxpayer_id AND it.id = w.entity_id
		WHERE w.engagement_id = $1 AND w.status = 'open'`, eng)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code string
		var inv int64
		if err := rows.Scan(&code, &inv); err != nil {
			t.Fatal(err)
		}
		if got[inv] == nil {
			got[inv] = map[string]bool{}
		}
		got[inv][code] = true
	}
	rows.Close()

	type score struct{ tp, fp, fn int }
	scores := map[string]*score{}
	codes := []string{"FIXED_ASSET_CANDIDATE", "UNUSUAL_RECORD_TYPE", "UNUSUAL_AMOUNT", "NEW_PARTY_HIGH_AMOUNT"}
	for _, c := range codes {
		scores[c] = &score{}
	}
	var fps []string
	for i, s := range labeled {
		want := map[string]bool{}
		for _, c := range s.expected {
			want[c] = true
		}
		for _, c := range codes {
			switch {
			case want[c] && got[invIDs[i]][c]:
				scores[c].tp++
			case want[c]:
				scores[c].fn++
				t.Errorf("%s: expected %s, not raised", s.label, c)
			case got[invIDs[i]][c]:
				scores[c].fp++
				fps = append(fps, s.label+" → "+c)
			}
		}
	}
	var b strings.Builder
	b.WriteString("\n| Kod | Doğru pozitif | Yanlış pozitif | Kaçan | Yanlış pozitif oranı |\n|---|---|---|---|---|\n")
	totalFP, totalRaised := 0, 0
	for _, c := range codes {
		s := scores[c]
		rate := 0.0
		if s.tp+s.fp > 0 {
			rate = float64(s.fp) / float64(s.tp+s.fp) * 100
		}
		totalFP += s.fp
		totalRaised += s.tp + s.fp
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %%%.0f |\n", c, s.tp, s.fp, s.fn, rate)
	}
	sort.Strings(fps)
	fmt.Fprintf(&b, "\nToplam: %d uyarı, %d yanlış pozitif (%%%.0f). Yanlış pozitifler: %s\n",
		totalRaised, totalFP, float64(totalFP)/float64(totalRaised)*100, strings.Join(fps, "; "))
	t.Log(b.String())
	// Known false positives of the method, kept as a regression baseline:
	// "masa" / "yazıcı" keywords hit a lamp and a toner; a lamp is also a new
	// kind of purchase for this firm. A change that adds or removes one must
	// update this list (and the PR's report).
	wantFP := []string{
		"masa lambası (demirbaş değil) → FIXED_ASSET_CANDIDATE",
		"masa lambası (demirbaş değil) → UNUSUAL_RECORD_TYPE",
		"yazıcı toneri (sarf) → FIXED_ASSET_CANDIDATE",
	}
	if strings.Join(fps, "|") != strings.Join(wantFP, "|") {
		t.Errorf("false positives changed:\n got %v\nwant %v", fps, wantFP)
	}

	// Re-run: no duplicates.
	again, err := r.Run(ctx, subj, "2026-10")
	if err != nil || again.Report.Created != 0 {
		t.Fatalf("re-run created %d (%v)", again.Report.Created, err)
	}
	if _, err := r.Run(ctx, subj, "2026-13"); err != ErrInvalidPeriod {
		t.Fatalf("bad period: %v", err)
	}
}
