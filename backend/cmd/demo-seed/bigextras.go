package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// bigExtras fills what makes the office screens look lived-in: open and
// resolved data warnings, document requests to taxpayers, the review queue,
// notifications, WhatsApp saves, monthly report emails, uploads of the last
// days, the activity log and the locks of the closed periods.
//
// Most rows are derived in SQL from the documents already written (the
// unmatched bank lines become warnings and document tasks, the pending
// receipts become review items), so they point at real records.
func bigExtras(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	last := w.periods[len(w.periods)-1]
	prev := w.periods[len(w.periods)-2]
	lockUntil := w.periods[len(w.periods)-3] // filed by everyone, locked
	engs := make([]int64, 0, len(ms))
	problem := make([]int64, 0, len(ms))
	for _, m := range ms {
		engs = append(engs, m.c.engagementID)
		if m.c.Problem {
			problem = append(problem, m.c.engagementID)
		}
	}
	return rls.System(ctx, pool, "demo-seed: big extras", func(ctx context.Context, tx pgx.Tx) error {
		steps := []struct {
			name string
			sql  string
			args []any
		}{
			// Eşleşmeyen banka hareketleri: açık uyarı + mükellefe belge görevi.
			{"data_warnings (banka)", `
				INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, code, severity,
					params, producer, status, created_at, updated_at)
				SELECT e.organization_id, e.id, b.taxpayer_id, b.period, 'bank_transaction', b.id,
				       CASE WHEN b.debit_credit = 'A' THEN 'BNK_UNINVOICED_COLLECTION' ELSE 'BNK_UNDOCUMENTED_PAYMENT' END, 'warning',
				       jsonb_build_object('amount', b.amount::text, 'counterparty', COALESCE(b.counterparty_name, '')), 'bank_rules', 'open',
				       b.booked_at + interval '1 hour', b.booked_at + interval '1 hour'
				FROM bank_transactions b JOIN engagements e ON e.taxpayer_id = b.taxpayer_id AND e.id = ANY($1::bigint[])
				WHERE b.source_system = $2 AND b.match_status = 'unmatched'`, []any{engs, bigBankSource}},
			{"document_tasks (banka)", `
				INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, bank_transaction_id, warning_id, code, kind, period,
					status, note, due_at, reminder_count, last_reminded_at, next_reminder_at, created_by, submitted_at, created_at, updated_at)
				SELECT w.organization_id, w.engagement_id, w.taxpayer_id, w.entity_id, w.id, w.code,
				       CASE WHEN w.code = 'BNK_UNINVOICED_COLLECTION' THEN 'uninvoiced_collection' ELSE 'undocumented_payment' END,
				       w.period, CASE WHEN w.id % 4 = 0 THEN 'submitted' ELSE 'open' END,
				       'Bu hareketin faturasını ya da dekontunu yükler misiniz?', w.created_at + interval '5 days',
				       (w.id % 3)::int, CASE WHEN w.id % 3 > 0 THEN w.created_at + interval '3 days' END,
				       w.created_at + interval '6 days', $2, CASE WHEN w.id % 4 = 0 THEN w.created_at + interval '2 days' END,
				       w.created_at, w.created_at
				FROM data_warnings w WHERE w.engagement_id = ANY($1::bigint[]) AND w.producer = 'bank_rules' AND w.status = 'open'`,
				[]any{engs, o.accountant()}},
			// Belge talepleri: kira sözleşmesi, SGK bildirgesi, ekstre…
			{"document_tasks (talep)", `
				INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, code, kind, period, status, title, note, due_at,
					reminder_count, created_by, submitted_at, matched_at, cancelled_at, cancelled_by, cancel_reason, created_at, updated_at)
				SELECT e.organization_id, e.id, e.taxpayer_id, 'DOC_REQUEST', 'document_request', p.period,
				       s.status, t.title, 'Dönem kapanışı için gerekli.', p.due, (e.id % 3)::int, $2,
				       CASE WHEN s.status IN ('submitted', 'matched') THEN p.due - interval '2 days' END,
				       CASE WHEN s.status = 'matched' THEN p.due - interval '1 day' END,
				       CASE WHEN s.status = 'cancelled' THEN p.due - interval '3 days' END,
				       CASE WHEN s.status = 'cancelled' THEN $2::bigint END,
				       CASE WHEN s.status = 'cancelled' THEN 'Mükellef belgeyi e-posta ile gönderdi.' END,
				       p.due - interval '10 days', p.due - interval '10 days'
				FROM engagements e
				CROSS JOIN (VALUES ($3::text, $5::timestamptz), ($4::text, $6::timestamptz)) p(period, due)
				CROSS JOIN LATERAL (SELECT (ARRAY['Kira sözleşmesi ve kira ödeme dekontu', 'Kredi kartı hesap ekstresi',
				       'SGK işe giriş bildirgesi', 'Araç ruhsat fotokopisi', 'Gider pusulası', 'Kasa sayım tutanağı'])[1 + (e.id + length(p.period)) % 6] AS title) t
				CROSS JOIN LATERAL (SELECT CASE
				       WHEN p.period = $4 THEN (ARRAY['open', 'open', 'submitted', 'open'])[1 + e.id % 4]
				       ELSE (ARRAY['matched', 'matched', 'cancelled', 'open'])[1 + e.id % 4] END AS status) s
				WHERE e.id = ANY($1::bigint[])`,
				[]any{engs, o.accountant(), prev, last, w.now.AddDate(0, 0, -6), w.now.AddDate(0, 0, 4)}},
			// Onay bekleyen masraf fişleri: düşük güvenli OCR alanları kuyrukta.
			{"suggestions (fiş)", `
				INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id, target_field, period,
					proposed_value, confidence, rationale, origin, model, status, created_at, updated_at)
				SELECT e.organization_id, e.id, x.taxpayer_id, 'receipt_field', 'expense', x.id, f.field, x.period,
				       to_jsonb(CASE f.field WHEN 'total_amount' THEN x.amount::numeric(18,2)::text
				                             WHEN 'tax_amount' THEN x.tax_amount::numeric(18,2)::text ELSE x.receipt_number END),
				       f.conf, 'OCR alan güveni ' || f.conf::text, 'model', $3, 'pending', x.created_at, x.created_at
				FROM expenses x JOIN engagements e ON e.taxpayer_id = x.taxpayer_id AND e.id = ANY($1::bigint[])
				CROSS JOIN LATERAL (VALUES ('total_amount', 0.52 + (x.id % 30) / 100.0), ('receipt_number', 0.41 + (x.id % 20) / 100.0)) f(field, conf)
				WHERE x.source_system = $2 AND x.approval_status = 'pending' AND (x.id % 3 = 0 OR e.id = ANY($4::bigint[]))`,
				[]any{engs, bigSource, DemoReceiptModel, problem}},
			// Geçmiş aylarda karara bağlanmış öneriler (kabul / düzeltme).
			{"suggestions (geçmiş)", `
				INSERT INTO suggestions (organization_id, engagement_id, taxpayer_id, kind, target_type, target_id, target_field, period,
					proposed_value, confidence, rationale, origin, model, status, applied_value, decided_by, decided_at, created_at, updated_at)
				SELECT e.organization_id, e.id, x.taxpayer_id, 'receipt_field', 'expense', x.id, 'total_amount', x.period,
				       to_jsonb(x.amount::numeric(18,2)::text), 0.64, 'OCR alan güveni 0.64', 'model', $3,
				       CASE WHEN x.id % 5 = 0 THEN 'corrected' ELSE 'accepted' END, to_jsonb(x.amount::numeric(18,2)::text), $4,
				       x.approved_at, x.created_at, x.approved_at
				FROM expenses x JOIN engagements e ON e.taxpayer_id = x.taxpayer_id AND e.id = ANY($1::bigint[])
				WHERE x.source_system = $2 AND x.approval_status = 'approved' AND x.id % 7 = 0`,
				[]any{engs, bigSource, DemoReceiptModel, o.accountant()}},
			{"data_warnings (fiş)", `
				INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, code, severity,
					producer, status, created_at, updated_at)
				SELECT e.organization_id, e.id, x.taxpayer_id, x.period, 'expense', x.id, 'UNAPPROVED_EXPENSE', 'info', 'control:expense_approval', 'open',
				       x.created_at, x.created_at
				FROM expenses x JOIN engagements e ON e.taxpayer_id = x.taxpayer_id AND e.id = ANY($1::bigint[])
				WHERE x.source_system = $2 AND x.approval_status = 'pending'`, []any{engs, bigSource}},
			// Faturalar: olağan dışı tutar (geçmişte çözülmüş, bu ay açık).
			{"data_warnings (fatura)", `
				INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, code, severity,
					params, producer, status, resolved_by, resolved_at, created_at, updated_at)
				SELECT e.organization_id, e.id, i.taxpayer_id, i.period, 'invoice', i.id,
				       (ARRAY['UNUSUAL_AMOUNT', 'NEW_PARTY_HIGH_AMOUNT', 'POSSIBLE_DUPLICATE', 'OUT_OF_PERIOD'])[1 + i.id % 4], 'warning',
				       jsonb_build_object('amount', i.payable_amount::numeric(18,2)::text), 'invoice_rules',
				       CASE WHEN i.period >= $3 THEN 'open' WHEN i.id % 2 = 0 THEN 'resolved' ELSE 'acknowledged' END,
				       CASE WHEN i.period < $3 THEN $4::bigint END, CASE WHEN i.period < $3 THEN i.created_at + interval '4 days' END,
				       i.created_at, i.created_at
				FROM invoices i JOIN engagements e ON e.taxpayer_id = i.taxpayer_id AND e.id = ANY($1::bigint[])
				WHERE i.source_system = $2 AND i.id % 41 = 0`, []any{engs, bigSource, prev, o.accountant()}},
			// Beyan son günü yaklaşıyor (geçen ay, hazırlanmamışlar) ve sorunlu
			// mükelleflerde dönemi kilitleyen engelleyiciler.
			{"data_warnings (dönem)", `
				INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, code, severity, params,
					producer, status, created_at, updated_at)
				SELECT e.organization_id, e.id, e.taxpayer_id, $2, 'period', 'VAT_DEADLINE_NEAR', 'warning',
				       jsonb_build_object('due_date', $3::text), 'tax_calendar', 'open', $4::timestamptz, $4::timestamptz
				FROM engagements e WHERE e.id = ANY($1::bigint[])
				  AND NOT EXISTS (SELECT 1 FROM tax_returns r WHERE r.engagement_id = e.id AND r.kind = 'kdv1' AND r.period = $2 AND r.status <> 'draft')
				UNION ALL
				SELECT e.organization_id, e.id, e.taxpayer_id, $2, 'period', 'BNK_BALANCE_MISMATCH', 'blocker',
				       '{}'::jsonb, 'bank_rules', 'open', $4::timestamptz, $4::timestamptz
				FROM engagements e WHERE e.id = ANY($5::bigint[])`,
				[]any{engs, prev, periodStart(prev).AddDate(0, 1, 27).Format(time.DateOnly), w.now.AddDate(0, 0, -2), problem}},
			// Kapanmış ve verilmiş dönemler kilitli.
			{"period_locks", `
				INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period, note, locked_by, locked_at)
				SELECT r.organization_id, r.engagement_id, e.taxpayer_id, r.period, 'Beyanlar verildi, dönem kapandı.', r.filed_by,
				       r.filed_at + interval '3 days'
				FROM tax_returns r JOIN engagements e ON e.id = r.engagement_id
				WHERE r.engagement_id = ANY($1::bigint[]) AND r.kind = 'kdv1' AND r.status = 'filed' AND r.period <= $2`,
				[]any{engs, lockUntil}},
			// Son bir haftanın yüklemeleri (pano: son yüklemeler).
			{"ubl_upload_jobs", `
				INSERT INTO ubl_upload_jobs (organization_id, engagement_id, taxpayer_id, actor_id, file_name, file_key, sha256, size_bytes,
					status, report, started_at, finished_at, created_at, updated_at)
				SELECT e.organization_id, e.id, e.taxpayer_id, $2, 'efatura-' || $3 || '-' || e.id || '.zip',
				       'uploads/ubl/' || e.id || '/' || $3 || '.zip', encode(sha256(convert_to('ubl:' || e.id || $3, 'UTF8')), 'hex'),
				       48000 + e.id * 97, 'done', jsonb_build_object('added', 6 + e.id % 9, 'updated', e.id % 3, 'skipped', 0),
				       $4::timestamptz - make_interval(hours => (e.id % 140)::int), $4::timestamptz - make_interval(hours => (e.id % 140)::int) + interval '40 seconds',
				       $4::timestamptz - make_interval(hours => (e.id % 140)::int), $4::timestamptz - make_interval(hours => (e.id % 140)::int)
				FROM engagements e WHERE e.id = ANY($1::bigint[]) AND e.id % 3 <> 1`,
				[]any{engs, o.accountant(), last, w.now}},
			{"whatsapp_chat_links", `
				INSERT INTO whatsapp_chat_links (organization_id, engagement_id, chat_key, label, created_by, updated_by, created_at, updated_at)
				SELECT e.organization_id, e.id, '90555000' || lpad((e.id % 10000)::text, 4, '0'), left(t.title, 100) || ' (WhatsApp)',
				       $2, $2, e.started_at, e.started_at
				FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id WHERE e.id = ANY($1::bigint[]) AND e.id % 4 <> 3
				ON CONFLICT (organization_id, chat_key) DO NOTHING`, []any{engs, o.owner()}},
			{"whatsapp_captures", `
				INSERT INTO whatsapp_captures (organization_id, engagement_id, taxpayer_id, target, chat_key, file_name, content_type,
					size_bytes, sha256, created_by, created_at)
				SELECT l.organization_id, l.engagement_id, e.taxpayer_id, (ARRAY['expense', 'document', 'income'])[1 + g % 3], l.chat_key,
				       (ARRAY['akaryakit-fisi-', 'kira-dekontu-', 'satis-faturasi-'])[1 + g % 3] || g || CASE WHEN g % 3 = 1 THEN '.pdf' ELSE '.jpg' END,
				       CASE WHEN g % 3 = 1 THEN 'application/pdf' ELSE 'image/jpeg' END, 90000 + g * 1311,
				       encode(sha256(convert_to('wa:' || l.engagement_id || ':' || g, 'UTF8')), 'hex'), $2,
				       $3::timestamptz - make_interval(days => (g * 9 + l.engagement_id % 7)::int)
				FROM whatsapp_chat_links l JOIN engagements e ON e.id = l.engagement_id
				CROSS JOIN generate_series(1, 4) g
				WHERE l.engagement_id = ANY($1::bigint[])`, []any{engs, o.accountant(), w.now}},
			// Yanıt bekleyen cari mektubu: gönderildi, yanıt gelmedi.
			{"reconciliation_letters (gönderildi)", `
				UPDATE reconciliation_letters SET status = 'sent', sent_at = $2::timestamptz, attempts = 1
				WHERE engagement_id = ANY($1::bigint[]) AND status = 'draft'`, []any{engs, w.now.AddDate(0, 0, -3)}},
			{"report_schedules", `
				INSERT INTO report_schedules (organization_id, engagement_id, taxpayer_id, enabled, day_of_month, recipient, last_period, updated_by)
				SELECT e.organization_id, e.id, e.taxpayer_id, e.id % 5 <> 0, 1 + e.id % 5, t.contact_email, $2, $3
				FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id WHERE e.id = ANY($1::bigint[])
				ON CONFLICT (organization_id, engagement_id) DO NOTHING`, []any{engs, prev, o.owner()}},
			// Mükellefe özel inceleme eşiği (#518 A5): bazı mükelleflerde eşik
			// yüksek, bazılarında otomatik onay kapalı.
			{"review_settings (mükellef)", `
				INSERT INTO review_settings (organization_id, engagement_id, kind, auto_accept, threshold, updated_by)
				SELECT e.organization_id, e.id, '*', e.id % 14 <> 0, CASE WHEN e.id % 14 <> 0 THEN 0.98 END, $2
				FROM engagements e WHERE e.id = ANY($1::bigint[]) AND e.id % 7 = 0
				ON CONFLICT DO NOTHING`, []any{engs, o.owner()}},
			// KVKK ilgili kişi talepleri (#518 B3): bekleyen dışa aktarım,
			// reddedilmiş silme, süresi dolmuş dışa aktarım. Tamamlanmış silme
			// yok: demo verisi anonimleştirilmiş görünmesin.
			{"privacy_requests", `
				INSERT INTO privacy_requests (organization_id, engagement_id, kind, subject_type, status, reason, decision_note,
					summary, requested_by, decided_by, decided_at, completed_at, expires_at, created_at, updated_at)
				SELECT e.organization_id, e.id, v.kind, 'taxpayer', v.status, v.reason, v.note,
				       CASE WHEN v.status = 'completed' THEN jsonb_build_object('export_expires_at', $4::timestamptz - interval '3 days') ELSE '{}'::jsonb END,
				       $2, CASE WHEN v.status <> 'pending' THEN $3::bigint END,
				       CASE WHEN v.status <> 'pending' THEN $4::timestamptz - v.age + interval '1 day' END,
				       CASE WHEN v.status = 'completed' THEN $4::timestamptz - v.age + interval '1 day' END,
				       CASE WHEN v.status = 'completed' THEN $4::timestamptz - interval '3 days' END,
				       $4::timestamptz - v.age, $4::timestamptz - v.age
				FROM engagements e
				JOIN (VALUES
					(0, 'export', 'pending', 'Mükellefin yazılı başvurusu: sistemdeki kişisel verilerinin dökümü.', '', interval '2 days'),
					(1, 'erasure', 'rejected', 'Mükellef telefonla silme istedi.', 'Yazılı başvuru olmadan işlem yapılamaz; mükellefe bilgi verildi.', interval '20 days'),
					(2, 'export', 'completed', 'Yıllık KVKK bilgilendirmesi kapsamında döküm.', '', interval '40 days')
				) v(slot, kind, status, reason, note, age) ON e.id % 5 = v.slot
				WHERE e.id = ANY($1::bigint[])`, []any{engs, o.accountant(), o.owner(), w.now}},
		}
		for _, s := range steps {
			tag, err := tx.Exec(ctx, s.sql, s.args...)
			if err != nil {
				return fmt.Errorf("%s: %w", s.name, err)
			}
			cnt.add(s.name, int(tag.RowsAffected()))
		}
		if err := bigReports(ctx, tx, w, o, ms, cnt); err != nil {
			return err
		}
		if err := bigNotifications(ctx, tx, w, o, ms, cnt); err != nil {
			return err
		}
		return bigActivity(ctx, tx, w, o, ms, cnt)
	})
}

// bigReports: the last two monthly reports were emailed to every client
// (scheduled), built by the report service from the seeded data.
func bigReports(ctx context.Context, tx pgx.Tx, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	q := db.New(tx)
	var rows [][]any
	for k, m := range ms {
		for back := 3; back >= 2; back-- {
			p := w.periods[len(w.periods)-back]
			if indexOf(m.periods, p) < 0 {
				continue
			}
			rg, _ := reports.ParseRange(p, "")
			rep, err := reports.Build(ctx, q, m.c.taxpayerID, rg, reports.Options{OrganizationID: o.orgID, EngagementID: m.c.engagementID, ApprovedOnly: true}, w.now)
			if err != nil {
				return fmt.Errorf("report %s: %w", p, err)
			}
			snap, _ := json.Marshal(rep)
			sent := periodStart(p).AddDate(0, 1, 0).Add(time.Duration(7*60+k) * time.Minute)
			rows = append(rows, []any{o.orgID, m.c.engagementID, m.c.taxpayerID, "month", p, m.c.Email, reports.OriginScheduled,
				"sent", 1, string(snap), o.def.Name, sent, sent, sent})
		}
	}
	cnt.add("report_deliveries", len(rows))
	return copyRows(ctx, tx, "report_deliveries", []string{"organization_id", "engagement_id", "taxpayer_id", "kind", "period",
		"recipient", "origin", "status", "attempts", "report", "office_name", "sent_at", "created_at", "updated_at"}, rows)
}

// bigNotifications: in-app notifications of the office members (some read).
func bigNotifications(ctx context.Context, tx pgx.Tx, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	prev := w.periods[len(w.periods)-2]
	pl := periodLabelTR(prev)
	var rows [][]any
	add := func(user int64, k int, prio, title, body, url, tmpl string, hoursAgo int) {
		created := w.now.Add(-time.Duration(hoursAgo) * time.Hour)
		status, read := "delivered", any(nil)
		if hoursAgo > 30 && k%3 != 0 {
			status, read = "read", created.Add(3*time.Hour)
		}
		rows = append(rows, []any{user, o.orgID, "inapp", status, prio, title, body, url, tmpl, "demo.big", created, created, read, created, created})
	}
	base := "/t/" + o.def.Slug
	for k, m := range ms {
		staff := m.c.assignees[0]
		switch {
		case m.c.Problem:
			add(staff, k, "high", m.c.Title+": eşleşmeyen banka hareketi", "Son haftalarda belgesiz tahsilat ve ödemeler var.", base+"/warnings", "bank_unmatched", 6+k)
			add(o.owner(), k, "critical", m.c.Title+": dönem kapanışını engelleyen uyarı", "Banka bakiyesi mizanla tutmuyor.", base+"/warnings", "blocker_alert", 20+k)
		case k%3 == 0:
			add(staff, k, "normal", m.c.Title+": yeni belge yüklendi", "Mükellef portalından e-fatura paketi yüklendi.", base+"/upload", "upload_received", 2+k*3)
		}
		if k%2 == 0 {
			add(staff, k, "normal", m.c.Title+": mutabakat yanıtı geldi", pl+" cari mutabakat mektubuna yanıt verildi.", base+"/engagements", "reconciliation_reply", 40+k*5)
		}
	}
	for _, u := range o.staff[:len(o.staff)-1] {
		add(u, 1, "high", pl+" KDV beyannamesi son gün yaklaşıyor", "Hazırlanmamış KDV1 beyanları var; son gün ayın 28'i.", base+"/review", "vat_deadline", 12)
		add(u, 2, "normal", pl+" MUHSGK beyannameleri", "Bordrolar içe aktarıldı; MUHSGK beyanları hazırlanabilir.", base, "muhsgk_ready", 70)
	}
	cnt.add("notifications", len(rows))
	return copyRows(ctx, tx, "notifications", []string{"user_id", "organization_id", "channel", "status", "priority", "title", "body",
		"action_url", "template_code", "source_event", "sent_at", "delivered_at", "read_at", "created_at", "updated_at"}, rows)
}

var monthLabelsTR = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

func periodLabelTR(p string) string {
	t := periodStart(p)
	return fmt.Sprintf("%s %d", monthLabelsTR[t.Month()-1], t.Year())
}

// bigActivity: the office's audit log (sign-ins, filings, exports, uploads)
// over the last 60 days.
func bigActivity(ctx context.Context, tx pgx.Tx, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	r := demo.Rand(o.idx, 0, 17)
	actions := []struct{ action, resource string }{
		{"login", "auth.session"}, {"tax_returns.filed", "tax_return"}, {"tax_returns.prepared", "tax_return"},
		{"journals.exported", "export_batch"}, {"invoices.uploaded", "ubl_upload_job"}, {"reconciliation.run", "reconciliation_run"},
		{"expenses.approved", "expense"}, {"bank.postings_approved", "bank_transaction"}, {"payroll.imported", "payroll_run"},
		{"document_tasks.created", "document_task"},
	}
	staff := o.staff[:len(o.staff)-1]
	var rows [][]any
	for i := 0; i < 60*6; i++ {
		a := actions[r.IntN(len(actions))]
		m := ms[r.IntN(len(ms))]
		user := staff[r.IntN(len(staff))]
		at := w.now.Add(-time.Duration(r.IntN(60*24*60)) * time.Minute)
		payload := map[string]any{"taxpayer": m.c.Title, "period": at.AddDate(0, -1, 0).Format("2006-01")}
		if a.action == "login" {
			payload = map[string]any{"method": "password"}
		}
		pj, _ := json.Marshal(payload)
		rows = append(rows, []any{user, o.orgID, a.action, a.resource, string(pj),
			fmt.Sprintf("10.20.%d.%d", o.idx+1, 10+r.IntN(200)), "Mozilla/5.0 (demo)", at})
	}
	cnt.add("activity_events", len(rows))
	return copyRows(ctx, tx, "activity_events", []string{"actor_user_id", "organization_id", "action", "resource", "payload",
		"ip_address", "user_agent", "created_at"}, rows)
}
