-- Eksik ekstre takibi (Oğuz'un geri bildirimi): mükellefin kullanımdaki her
-- banka hesabı için kapanmış aylarda ekstre geldi mi. Ekstre geldi = o ay o
-- hesapta hareket ya da gün sonu bakiyesi var, ya da müşavir "geldi (hareket
-- yok)" diye işaretledi. Hesabın beklendiği ilk ay: ilk hareketinin ayı ile
-- listeye eklendiği ayın bir öncesinden erken olanı (yeni eklenen hesabın
-- geçen ayki ekstresi beklenir). Pasif hesap sayılmaz.

-- name: ListBankStatementCoverage :many
SELECT b.id AS cash_box_id, b.uuid AS cash_box_uuid, b.name, b.bank_id, b.iban, b.currency,
       LEAST(
           COALESCE((SELECT min(t.period) FROM bank_transactions t
                     WHERE t.taxpayer_id = b.taxpayer_id AND t.cash_box_id = b.id), '9999-12'),
           to_char((b.created_at AT TIME ZONE 'Europe/Istanbul') - interval '1 month', 'YYYY-MM')
       )::text AS start_period,
       p.period::text AS period,
       (EXISTS (SELECT 1 FROM bank_transactions t
                WHERE t.taxpayer_id = b.taxpayer_id AND t.cash_box_id = b.id AND t.period = p.period)
        OR EXISTS (SELECT 1 FROM bank_daily_summaries s
                   WHERE s.taxpayer_id = b.taxpayer_id AND s.cash_box_id = b.id
                     AND s.date >= to_date(p.period || '-01', 'YYYY-MM-DD')
                     AND s.date < to_date(p.period || '-01', 'YYYY-MM-DD') + interval '1 month'))::bool AS has_movement,
       EXISTS (SELECT 1 FROM bank_statement_marks m
               WHERE m.organization_id = @organization_id AND m.engagement_id = @engagement_id
                 AND m.cash_box_id = b.id AND m.period = p.period)::bool AS marked
FROM cash_boxes b
CROSS JOIN unnest(@periods::text[]) AS p(period)
WHERE b.taxpayer_id = @taxpayer_id AND b.type = 'bank_account' AND b.deactivated_at IS NULL
ORDER BY b.id, p.period
LIMIT 1200;

-- name: ListBankStatementGaps :many
-- Ofis yapılacakları "Eksik ekstreler": görünen her mükellefin kullanımdaki
-- hesaplarında ekstresi gelmemiş kapanmış aylar; satır başına mükellef +
-- hesap + ay. Atanmış mükelleflerle sınırlı (engagements.read_all yoksa).
-- same_kind: mükellefin aynı banka ve para birimindeki hesap sayısı (istek
-- başlığı ayırt edilsin); taxpayer_missing: mükellefin toplam eksiği.
WITH acc AS (
    SELECT e.id AS engagement_id, e.uuid AS engagement_uuid, tp.title AS taxpayer_title, b.taxpayer_id,
           b.id AS cash_box_id, b.uuid AS cash_box_uuid, b.name, b.bank_id, b.iban, b.currency,
           LEAST(
               COALESCE((SELECT min(t.period) FROM bank_transactions t
                         WHERE t.taxpayer_id = b.taxpayer_id AND t.cash_box_id = b.id), '9999-12'),
               to_char((b.created_at AT TIME ZONE 'Europe/Istanbul') - interval '1 month', 'YYYY-MM')
           ) AS start_period,
           count(*) OVER (PARTITION BY e.id, b.bank_id, b.currency) AS same_kind
    FROM engagements e
    JOIN taxpayers tp ON tp.id = e.taxpayer_id
    JOIN cash_boxes b ON b.taxpayer_id = e.taxpayer_id AND b.type = 'bank_account' AND b.deactivated_at IS NULL
    WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
      AND (@all_engagements::bool OR EXISTS (
          SELECT 1 FROM engagement_assignments a
          WHERE a.organization_id = e.organization_id AND a.engagement_id = e.id AND a.user_id = @user_id))
), gaps AS (
    SELECT a.*, p.period::text AS period
    FROM acc a
    JOIN unnest(@periods::text[]) AS p(period) ON p.period >= a.start_period
    WHERE NOT EXISTS (SELECT 1 FROM bank_transactions t
                      WHERE t.taxpayer_id = a.taxpayer_id
                        AND t.cash_box_id = a.cash_box_id AND t.period = p.period)
      AND NOT EXISTS (SELECT 1 FROM bank_daily_summaries s
                      WHERE s.taxpayer_id = a.taxpayer_id
                        AND s.cash_box_id = a.cash_box_id
                        AND s.date >= to_date(p.period || '-01', 'YYYY-MM-DD')
                        AND s.date < to_date(p.period || '-01', 'YYYY-MM-DD') + interval '1 month')
      AND NOT EXISTS (SELECT 1 FROM bank_statement_marks m
                      WHERE m.organization_id = @organization_id AND m.engagement_id = a.engagement_id
                        AND m.cash_box_id = a.cash_box_id AND m.period = p.period)
)
SELECT g.engagement_id, g.engagement_uuid, g.taxpayer_title::text AS taxpayer_title,
       g.cash_box_uuid, g.name::text AS name, g.bank_id, g.iban, g.currency::text AS currency, g.period,
       g.same_kind::bigint AS same_kind,
       (count(*) OVER (PARTITION BY g.engagement_id))::bigint AS taxpayer_missing,
       (count(*) OVER ())::bigint AS total
FROM gaps g
ORDER BY g.taxpayer_title, g.engagement_id, g.name, g.cash_box_id, g.period DESC
LIMIT @lim OFFSET @off;

-- name: InsertBankStatementMark :exec
INSERT INTO bank_statement_marks (organization_id, engagement_id, taxpayer_id, cash_box_id, period, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @cash_box_id, @period, sqlc.narg(created_by))
ON CONFLICT (engagement_id, cash_box_id, period) DO NOTHING;

-- name: DeleteBankStatementMark :exec
DELETE FROM bank_statement_marks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND cash_box_id = @cash_box_id AND period = @period;
