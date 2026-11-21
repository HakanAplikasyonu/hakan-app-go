package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// demoHistoryAllRef marks the per-taxpayer history rows (idempotency).
const demoHistoryAllRef = "demo-gecmis-tum"

// seedCommunicationHistoryAll (#780 D): fills the "İletişim geçmişi" tab of
// every demo taxpayer with automatic sends spread over the last ~60 days:
// SMS (KDV raporu hazır, beyanname verildi, belge hatırlatma: some failed
// with NetGSM code 30, some sent but not yet delivered) and e-mails (KDV
// raporu, aylık mali rapor, tahakkuk fişi: opened / downloaded or not).
// Every unopened e-mail gets the "e-posta açılmadı" SMS two days later.
// Rows are automatic (sent_by NULL). Only new rows; a second run changes
// nothing.
func seedCommunicationHistoryAll(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: communication history (all taxpayers)", func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM sms_messages WHERE organization_id = $1 AND source_ref = $2`,
			orgID, demoHistoryAllRef).Scan(&n); err != nil {
			return fmt.Errorf("count history: %w", err)
		}
		if n > 0 {
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, t.title, COALESCE(t.contact_email::text, ''), COALESCE(t.contact_phone, '')
			FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 ORDER BY e.id`, orgID)
		if err != nil {
			return fmt.Errorf("engagements: %w", err)
		}
		type eng struct {
			id                  int64
			title, email, phone string
		}
		var engs []eng
		for rows.Next() {
			var e eng
			if err := rows.Scan(&e.id, &e.title, &e.email, &e.phone); err != nil {
				rows.Close()
				return err
			}
			engs = append(engs, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		now := time.Now()
		// at is 09:30 on the day `days` ago, a few minutes apart per taxpayer.
		at := func(i, days int) time.Time {
			d := now.AddDate(0, 0, -days)
			return time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, d.Location()).Add(time.Duration(i%40) * 4 * time.Minute)
		}
		job := func(i, k int) string { return fmt.Sprintf("1731%06d%04d", 200000+i*37+k, 1000+k*111+i%97) }
		delivered := func(t time.Time) *time.Time { d := t.Add(40 * time.Second); return &d }

		sms := func(e eng, body, status, jobNo, code, errMsg, rule, ref string, created time.Time, deliveredAt *time.Time) error {
			var sentAt *time.Time
			if status == "sent" {
				s := created.Add(5 * time.Second)
				sentAt = &s
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, provider_job_id,
					error_code, error_message, source_rule, source_ref, sent_by, created_at, sent_at, delivered_at)
				VALUES ($1, $2, $3, $4, 'DEMOSMMM', $5, $6, $7, $8, $9, $10, NULL, $11, $12, $13)`,
				orgID, e.id, e.phone, body, status, jobNo, code, errMsg, rule, ref, created, sentAt, deliveredAt)
			if err != nil {
				return fmt.Errorf("history sms: %w", err)
			}
			return nil
		}
		mail := func(e eng, subject, body, rule string, created time.Time, opened, downloaded bool) (string, error) {
			var openedAt, downloadedAt *time.Time
			if opened {
				o := created.Add(3 * time.Hour)
				openedAt = &o
				if downloaded {
					d := o.Add(2 * time.Minute)
					downloadedAt = &d
				}
			}
			var id string
			err := tx.QueryRow(ctx, `
				INSERT INTO mail_messages (organization_id, engagement_id, to_email, subject, body, status, source_rule, source_ref,
					sent_by, created_at, sent_at, opened_at, downloaded_at)
				VALUES ($1, $2, $3, $4, $5, 'sent', $6, $7, NULL, $8, $9, $10, $11)
				RETURNING uuid::text`,
				orgID, e.id, e.email, subject, body, rule, demoHistoryAllRef, created, created.Add(3*time.Second),
				openedAt, downloadedAt).Scan(&id)
			if err != nil {
				return "", fmt.Errorf("history mail: %w", err)
			}
			return id, nil
		}

		for i, e := range engs {
			if strings.TrimSpace(e.email) == "" {
				e.email = fmt.Sprintf("muhasebe%d@ornek-mukellef.com.tr", i+1)
			}
			if p := demoPhone(e.phone); p != "" {
				e.phone = p
			} else {
				e.phone = fmt.Sprintf("5320002%03d", i%1000)
			}
			sayin := "Sayın " + e.title + ", "

			// SMS: KDV raporu hazır (Eylül, Ekim) ve beyanname verildi; iletildi.
			for k, s := range []struct {
				body, rule string
				days       int
			}{
				{sayin + "Eylül 2026 KDV raporunuz hazır. Ayrıntılar e-postanızda. DEMO SMMM", "KDV raporu hazır — ayın 29'u", 53},
				{sayin + "Ekim 2026 KDV raporunuz hazır. Ödenecek KDV tutarı e-postanıza gönderildi. DEMO SMMM", "KDV raporu hazır — ayın 29'u", 23},
				{sayin + "Ekim dönemi KDV beyannameniz verildi. Son ödeme günü 26 Kasım. DEMO SMMM", "Beyanname verildi — verildiği gün", 6},
			} {
				t := at(i, s.days)
				if err := sms(e, s.body, "sent", job(i, k), "", "", s.rule, demoHistoryAllRef, t, delivered(t)); err != nil {
					return err
				}
			}

			// SMS: belge hatırlatma; bazılarında numara geçersiz (NetGSM 30),
			// bazılarında gönderildi ama henüz telefona iletilmedi.
			t := at(i, 2)
			body := sayin + "Kasım ayı faturalarınızı ve banka ekstrenizi ayın 5'ine kadar iletmenizi rica ederiz. DEMO SMMM"
			const rule = "Belge hatırlatma — ayın 1'i"
			var err error
			switch {
			case i%5 == 3:
				err = sms(e, body, "failed", "", "30", "Numara geçersiz veya kapalı", rule, demoHistoryAllRef, t, nil)
			case i%3 == 1:
				err = sms(e, body, "sent", job(i, 3), "", "", rule, demoHistoryAllRef, t, nil)
			default:
				err = sms(e, body, "sent", job(i, 3), "", "", rule, demoHistoryAllRef, t, delivered(t))
			}
			if err != nil {
				return err
			}

			// E-postalar; açılmayana iki gün sonra "e-posta açılmadı" SMS'i.
			for k, m := range []struct {
				subject, body, rule, smsBody string
				days                         int
				opened, downloaded           bool
			}{
				{"Eylül 2026 KDV raporunuz", sayin + "Eylül 2026 KDV raporunuz ektedir.\n\nSaygılarımızla",
					"KDV raporu hazır — ayın 29'u", "", 53, true, i%2 == 1},
				{"Ekim 2026 KDV raporunuz", sayin + "Ekim 2026 dönemine ait KDV raporunuz ektedir.\n\nSaygılarımızla",
					"KDV raporu hazır — ayın 29'u",
					sayin + "Ekim KDV raporunuzu e-postayla gönderdik, henüz açılmamış görünüyor. Kontrol etmenizi rica ederiz. DEMO SMMM",
					23, i%4 != 0, true},
				{"Ekim 2026 aylık mali raporunuz", sayin + "aylık gelir-gider ve vergi özetiniz ektedir.\n\nSaygılarımızla",
					"Aylık mali rapor — ayın 3'ü",
					sayin + "aylık mali raporunuzu e-postayla gönderdik, henüz açılmamış görünüyor. Kontrol etmenizi rica ederiz. DEMO SMMM",
					21, i%3 != 0, i%2 == 0},
				{"KDV tahakkuk fişi ve ödeme bilgisi", sayin + "Ekim dönemi KDV tahakkuk fişiniz ektedir. Son ödeme günü 26 Kasım 2026.\n\nSaygılarımızla",
					"Tahakkuk fişi — beyanname verildiğinde", "", 6, true, true},
			} {
				created := at(i, m.days).Add(10 * time.Minute)
				id, err := mail(e, m.subject, m.body, m.rule, created, m.opened, m.downloaded)
				if err != nil {
					return err
				}
				if m.opened || m.smsBody == "" {
					continue
				}
				ft := created.Add(48 * time.Hour)
				if err := sms(e, m.smsBody, "sent", job(i, 10+k), "", "",
					m.rule+" — e-posta açılmadı, SMS gitti (eposta_acilmadi)", "mail:"+id, ft, delivered(ft)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
