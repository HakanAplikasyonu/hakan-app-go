package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// demoHistoryRef marks the communication history demo rows (idempotency).
const demoHistoryRef = "demo-gecmis"

// seedCommunicationHistory (#780 D): the taxpayer page's "İletişim geçmişi"
// tab on three demo taxpayers: e-mails with open / download times, SMS with
// NetGSM job numbers and delivery times, one SMS the operator could not
// deliver, and on the second taxpayer the "e-posta açılmadı, SMS gitti"
// example (an unopened e-mail, then an SMS whose source_rule notes the
// fallback). Only new rows; a second run changes nothing.
func seedCommunicationHistory(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: communication history", func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM mail_messages WHERE organization_id = $1 AND source_ref = $2`,
			orgID, demoHistoryRef).Scan(&n); err != nil {
			return fmt.Errorf("count history: %w", err)
		}
		if n > 0 {
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, t.title, COALESCE(t.contact_email::text, ''), COALESCE(t.contact_phone, '')
			FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 ORDER BY e.id LIMIT 3`, orgID)
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
		for i := range engs {
			if strings.TrimSpace(engs[i].email) == "" {
				engs[i].email = fmt.Sprintf("muhasebe%d@ornek-mukellef.com.tr", i+1)
			}
			if p := demoPhone(engs[i].phone); p != "" {
				engs[i].phone = p
			} else {
				engs[i].phone = fmt.Sprintf("53200001%02d", i+1)
			}
		}

		mail := func(e eng, subject, body, rule, sent, opened, downloaded string) (string, error) {
			var id string
			err := tx.QueryRow(ctx, `
				INSERT INTO mail_messages (organization_id, engagement_id, to_email, subject, body, status, source_rule, source_ref,
					created_at, sent_at, opened_at, downloaded_at)
				VALUES ($1, $2, $3, $4, $5, 'sent', $6, $7, $8::timestamptz, $8::timestamptz,
					NULLIF($9, '')::timestamptz, NULLIF($10, '')::timestamptz)
				RETURNING uuid::text`,
				orgID, e.id, e.email, subject, body, rule, demoHistoryRef, sent, opened, downloaded).Scan(&id)
			if err != nil {
				return "", fmt.Errorf("history mail: %w", err)
			}
			return id, nil
		}
		sms := func(e eng, body, status, job, code, errMsg, rule, ref, sent, delivered string) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO sms_messages (organization_id, engagement_id, phone, body, msgheader, status, provider_job_id,
					error_code, error_message, source_rule, source_ref, created_at, sent_at, delivered_at)
				VALUES ($1, $2, $3, $4, 'OFISDEMO', $5, $6, $7, $8, $9, $10, $11::timestamptz, $11::timestamptz,
					NULLIF($12, '')::timestamptz)`,
				orgID, e.id, e.phone, body, status, job, code, errMsg, rule, ref, sent, delivered)
			if err != nil {
				return fmt.Errorf("history sms: %w", err)
			}
			return nil
		}

		if len(engs) > 0 {
			e := engs[0]
			if _, err := mail(e, "Ekim 2026 KDV beyannameniz verildi",
				"Sayın "+e.title+",\n\nEkim 2026 KDV beyannameniz verildi. Tahakkuk fişi ektedir. Ödenecek tutar 18.450 TL, son ödeme günü 28 Kasım 2026.\n\nSaygılarımızla",
				"kdv_tahakkuk", "2026-11-10T09:00:00+03:00", "2026-11-10T09:42:13+03:00", "2026-11-10T09:44:02+03:00"); err != nil {
				return err
			}
			if err := sms(e, "Sayın "+e.title+", KDV son ödeme günü 28 Kasım. Tutar 18.450 TL.\n", "sent", "17312466018321",
				"", "", "kdv_hatirlatma", "", "2026-11-19T10:00:00+03:00", "2026-11-19T10:00:06+03:00"); err != nil {
				return err
			}
		}
		if len(engs) > 1 {
			// "E-posta açılmadı, SMS gitti": the e-mail is never opened; two
			// days later the rule's fallback sends the SMS.
			e := engs[1]
			mailID, err := mail(e, "Kasım 2026 banka ekstreleriniz",
				"Sayın "+e.title+",\n\nKasım 2026 banka ekstrelerinizi 5 Aralık'a kadar yüklemenizi rica ederiz.\n\nSaygılarımızla",
				"eksik_ekstre", "2026-11-12T09:30:00+03:00", "", "")
			if err != nil {
				return err
			}
			if err := sms(e, "Sayın "+e.title+", Kasım banka ekstrelerinizi 5 Aralık'a kadar yüklemenizi rica ederiz. E-postamız açılmadığı için SMS ile hatırlatıyoruz.\n",
				"sent", "17314208774410", "", "", "eksik_ekstre:eposta_acilmadi_sms", "mail:"+mailID,
				"2026-11-14T09:30:00+03:00", "2026-11-14T09:30:09+03:00"); err != nil {
				return err
			}
		}
		if len(engs) > 2 {
			e := engs[2]
			if _, err := mail(e, "Ekim 2026 muhtasar ve prim hizmet beyannameniz",
				"Sayın "+e.title+",\n\nEkim 2026 muhtasar ve prim hizmet beyannameniz verildi. Ödenecek tutar 6.210 TL, son ödeme günü 26 Kasım 2026.\n\nSaygılarımızla",
				"muhtasar_tahakkuk", "2026-11-08T14:20:00+03:00", "2026-11-09T08:05:41+03:00", ""); err != nil {
				return err
			}
			if err := sms(e, "Sayın "+e.title+", muhtasar son ödeme günü 26 Kasım. Tutar 6.210 TL.\n", "failed", "17311890013377",
				"dlr_2", "Operatör iletemedi: Zaman aşımı: mesaj geçerlilik süresi içinde telefona iletilemedi.",
				"muhtasar_hatirlatma", "", "2026-11-18T10:00:00+03:00", ""); err != nil {
				return err
			}
			if err := sms(e, "Sayın "+e.title+", muhtasar son ödeme günü 26 Kasım, tekrar hatırlatırız. Tutar 6.210 TL.\n", "sent", "17312338450092",
				"", "", "muhtasar_hatirlatma_tekrar", "", "2026-11-19T09:00:00+03:00", "2026-11-19T09:00:04+03:00"); err != nil {
				return err
			}
		}
		return nil
	})
}

// demoPhone keeps a stored mobile number in the 10-digit form, "" otherwise.
func demoPhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	if len(d) == 10 && d[0] == '5' {
		return d
	}
	return ""
}
