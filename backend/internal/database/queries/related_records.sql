-- İlişkili kayıtlar (#469): bir mükellef kaydının (fatura, banka hareketi,
-- fiş, uyarı, masraf, Z raporu, bordro, Easytrade hareketi) bağlı olduğu
-- kayıtlar. Yeni tablo yok; bağlar mevcut sütunlardan okunur:
--   bank_transaction_matches (hareket ↔ fatura / bordro),
--   journal_draft_lines.source_kind/source_id, bank_transaction_postings.
--   journal_draft_uuid, receipt_postings.journal_draft_id,
--   payroll_runs.journal_draft_id (kayıt → fiş),
--   data_warnings.entity_type/entity_id (uyarı ↔ kaynak belge),
--   suggestions.target_type/target_id (inceleme kaydı),
--   retail_sales.invoice_id, source_financial_transactions.invoice_id.
-- Her sorgu taxpayer_id / engagement_id ile süzülür; RLS ayrıca korur.

-- name: ResolveRelatedSource :one
-- Kaynak kaydın iç kimliği; kayıt bu engagement'ta görünmüyorsa satır yok.
SELECT src.id::bigint AS id FROM (
    SELECT i.id FROM invoices i
    WHERE @kind::text = 'invoice' AND i.taxpayer_id = @taxpayer_id AND i.uuid = @uuid
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
    UNION ALL
    SELECT b.id FROM bank_transactions b
    WHERE @kind::text = 'bank_transaction' AND b.taxpayer_id = @taxpayer_id AND b.uuid = @uuid
    UNION ALL
    SELECT j.id FROM journal_drafts j
    WHERE @kind::text = 'journal_draft' AND j.organization_id = @organization_id AND j.engagement_id = @engagement_id AND j.uuid = @uuid
    UNION ALL
    SELECT w.id FROM data_warnings w
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.uuid = @uuid
    UNION ALL
    SELECT e.id FROM expenses e
    WHERE @kind::text = 'expense' AND e.taxpayer_id = @taxpayer_id AND e.uuid = @uuid
    UNION ALL
    SELECT r.id FROM retail_sales r
    WHERE @kind::text = 'retail_sale' AND r.taxpayer_id = @taxpayer_id AND r.uuid = @uuid
    UNION ALL
    SELECT p.id FROM payroll_runs p
    WHERE @kind::text = 'payroll_run' AND p.taxpayer_id = @taxpayer_id AND p.uuid = @uuid
    UNION ALL
    SELECT f.id FROM source_financial_transactions f
    WHERE @kind::text = 'financial_transaction' AND f.taxpayer_id = @taxpayer_id AND f.uuid = @uuid
) src
LIMIT 1;

-- name: ListRelatedRecords :many
-- Kaynağın (kind, source_id) ilişkili kayıtları. Bütün sütunlar metin:
-- tarih YYYY-MM-DD, tutar ondalık; olmayan değer boş metin.
SELECT r.kind::text AS kind, r.uuid::text AS uuid, r.relation::text AS relation,
       r.rec_date::text AS rec_date, r.amount::text AS amount, r.currency::text AS currency,
       r.status::text AS status, r.label::text AS label, r.detail::text AS detail
FROM (
    -- Fatura → ödeme / tahsilat hareketleri (eşleşme).
    SELECT 'bank_transaction' AS kind, b.uuid::text AS uuid, 'payment' AS relation,
           COALESCE(to_char(b.transaction_date, 'YYYY-MM-DD'), '') AS rec_date,
           COALESCE(m.amount::text, '') AS amount, b.currency AS currency,
           b.match_status AS status, b.description AS label, b.debit_credit AS detail
    FROM bank_transaction_matches m
    JOIN bank_transactions b ON b.taxpayer_id = m.taxpayer_id AND b.id = m.bank_transaction_id
    WHERE @kind::text IN ('invoice', 'payroll_run') AND m.taxpayer_id = @taxpayer_id
      AND m.target_type = @kind::text AND m.target_id = @source_id

    UNION ALL
    -- Banka hareketi → eşlenen faturalar.
    SELECT 'invoice', i.uuid::text, 'payment',
           COALESCE(to_char(i.issue_date, 'YYYY-MM-DD'), ''), COALESCE(m.amount::text, ''), i.currency,
           i.status_normalized, i.invoice_number, COALESCE(i.customer_name, '')
    FROM bank_transaction_matches m
    JOIN invoices i ON i.taxpayer_id = m.taxpayer_id AND i.id = m.target_id
    WHERE @kind::text = 'bank_transaction' AND m.taxpayer_id = @taxpayer_id
      AND m.bank_transaction_id = @source_id AND m.target_type = 'invoice'
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)

    UNION ALL
    -- Banka hareketi → eşlenen bordro (net ücret ödemesi).
    SELECT 'payroll_run', p.uuid::text, 'payment', '', COALESCE(m.amount::text, ''), 'TRY',
           p.source, p.period, ''
    FROM bank_transaction_matches m
    JOIN payroll_runs p ON p.taxpayer_id = m.taxpayer_id AND p.id = m.target_id
    WHERE @kind::text = 'bank_transaction' AND m.taxpayer_id = @taxpayer_id
      AND m.bank_transaction_id = @source_id AND m.target_type = 'payroll_run'

    UNION ALL
    -- Kayıt → oluşan fiş: fiş satırının kaynağı (fatura, fatura satırı,
    -- banka hareketi), banka kaydı, masraf / Z raporu kaydı ya da bordro.
    SELECT DISTINCT 'journal_draft', j.uuid::text, 'journal',
           COALESCE(to_char(j.entry_date, 'YYYY-MM-DD'), ''),
           COALESCE((SELECT SUM(l.debit) FROM journal_draft_lines l
                     WHERE l.organization_id = j.organization_id AND l.engagement_id = j.engagement_id AND l.journal_draft_id = j.id)::text, ''),
           'TRY', j.status, j.description, j.entry_no::text
    FROM journal_drafts j
    WHERE j.organization_id = @organization_id AND j.engagement_id = @engagement_id AND (
        (@kind::text = 'invoice' AND EXISTS (
            SELECT 1 FROM journal_draft_lines l
            WHERE l.organization_id = j.organization_id AND l.engagement_id = j.engagement_id AND l.journal_draft_id = j.id
              AND ((l.source_kind = 'invoice' AND l.source_id = (@source_id::bigint)::text)
                OR (l.source_kind = 'invoice_item' AND l.source_id IN (
                    SELECT ii.id::text FROM invoice_items ii
                    WHERE ii.taxpayer_id = @taxpayer_id AND ii.invoice_id = @source_id)))))
        OR (@kind::text = 'bank_transaction' AND (
            EXISTS (SELECT 1 FROM journal_draft_lines l
                    WHERE l.organization_id = j.organization_id AND l.engagement_id = j.engagement_id AND l.journal_draft_id = j.id
                      AND l.source_kind = 'bank_transaction' AND l.source_id = (@source_id::bigint)::text)
            OR j.uuid IN (SELECT bp.journal_draft_uuid FROM bank_transaction_postings bp
                          WHERE bp.organization_id = @organization_id AND bp.engagement_id = @engagement_id AND bp.bank_transaction_id = @source_id
                            AND bp.journal_draft_uuid IS NOT NULL)))
        OR (@kind::text IN ('expense', 'retail_sale') AND j.id IN (
            SELECT rp.journal_draft_id FROM receipt_postings rp
            WHERE rp.organization_id = @organization_id AND rp.engagement_id = @engagement_id AND rp.target_type = @kind::text
              AND rp.target_id = @source_id AND rp.journal_draft_id IS NOT NULL))
        OR (@kind::text = 'payroll_run' AND j.id IN (
            SELECT p.journal_draft_id FROM payroll_runs p
            WHERE p.taxpayer_id = @taxpayer_id AND p.id = @source_id AND p.journal_draft_id IS NOT NULL))
    )

    UNION ALL
    -- Fiş → kaynak faturalar (fatura ya da fatura satırı kaynaklı satırlar).
    SELECT DISTINCT 'invoice', i.uuid::text, 'source',
           COALESCE(to_char(i.issue_date, 'YYYY-MM-DD'), ''), COALESCE(i.payable_amount::text, ''), i.currency,
           i.status_normalized, i.invoice_number, COALESCE(i.customer_name, '')
    FROM invoices i
    WHERE @kind::text = 'journal_draft' AND i.taxpayer_id = @taxpayer_id
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
      AND i.id IN (
        SELECT CASE WHEN l.source_kind = 'invoice' THEN l.source_id::bigint
                    ELSE (SELECT ii.invoice_id FROM invoice_items ii
                          WHERE ii.taxpayer_id = @taxpayer_id AND ii.id = l.source_id::bigint) END
        FROM journal_draft_lines l
        WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = @source_id
          AND l.source_kind IN ('invoice', 'invoice_item') AND l.source_id ~ '^[0-9]+$')

    UNION ALL
    -- Fiş → kaynak banka hareketleri.
    SELECT DISTINCT 'bank_transaction', b.uuid::text, 'source',
           COALESCE(to_char(b.transaction_date, 'YYYY-MM-DD'), ''), COALESCE(b.amount::text, ''), b.currency,
           b.match_status, b.description, b.debit_credit
    FROM bank_transactions b
    WHERE @kind::text = 'journal_draft' AND b.taxpayer_id = @taxpayer_id AND (
        b.id IN (SELECT l.source_id::bigint FROM journal_draft_lines l
                 WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.journal_draft_id = @source_id
                   AND l.source_kind = 'bank_transaction' AND l.source_id ~ '^[0-9]+$')
        OR b.id IN (SELECT bp.bank_transaction_id FROM bank_transaction_postings bp
                    JOIN journal_drafts j ON j.uuid = bp.journal_draft_uuid
                    WHERE bp.organization_id = @organization_id AND bp.engagement_id = @engagement_id AND j.organization_id = @organization_id AND j.engagement_id = @engagement_id
                      AND j.id = @source_id))

    UNION ALL
    -- Fiş → kaynak masraf.
    SELECT 'expense', e.uuid::text, 'source',
           COALESCE(to_char(e.expense_date, 'YYYY-MM-DD'), ''), COALESCE(e.amount::text, ''), e.currency,
           e.approval_status, COALESCE(e.vendor_name, e.description), e.receipt_type
    FROM receipt_postings rp
    JOIN expenses e ON e.taxpayer_id = rp.taxpayer_id AND e.id = rp.target_id
    WHERE @kind::text = 'journal_draft' AND rp.organization_id = @organization_id AND rp.engagement_id = @engagement_id
      AND rp.target_type = 'expense' AND rp.journal_draft_id = @source_id

    UNION ALL
    -- Fiş → kaynak Z raporu.
    SELECT 'retail_sale', s.uuid::text, 'source',
           COALESCE(to_char(s.sale_date, 'YYYY-MM-DD'), ''), COALESCE(s.total_amount::text, ''), s.currency,
           s.approval_status, COALESCE(s.z_report_number, ''), COALESCE(s.okc_serial_number, '')
    FROM receipt_postings rp
    JOIN retail_sales s ON s.taxpayer_id = rp.taxpayer_id AND s.id = rp.target_id
    WHERE @kind::text = 'journal_draft' AND rp.organization_id = @organization_id AND rp.engagement_id = @engagement_id
      AND rp.target_type = 'retail_sale' AND rp.journal_draft_id = @source_id

    UNION ALL
    -- Fiş → kaynak bordro.
    SELECT 'payroll_run', p.uuid::text, 'source', '', COALESCE(p.net_total::text, ''), 'TRY',
           p.source, p.period, ''
    FROM payroll_runs p
    WHERE @kind::text = 'journal_draft' AND p.taxpayer_id = @taxpayer_id AND p.journal_draft_id = @source_id

    UNION ALL
    -- Kayıt → hakkındaki uyarılar (fatura için satır uyarıları da).
    SELECT 'warning', w.uuid::text, 'warning',
           to_char(w.created_at, 'YYYY-MM-DD'), '', '', w.status, w.code, w.severity
    FROM data_warnings w
    WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
      AND @kind::text IN ('invoice', 'bank_transaction', 'expense', 'retail_sale', 'financial_transaction')
      AND ((w.entity_type = @kind::text AND w.entity_id = @source_id)
        OR (@kind::text = 'invoice' AND w.entity_type = 'invoice_item' AND w.entity_id IN (
            SELECT ii.id FROM invoice_items ii WHERE ii.taxpayer_id = @taxpayer_id AND ii.invoice_id = @source_id))
        -- Banka kontrolünün dönem uyarıları (BNK_*) hareketleri
        -- params.groups[].transactions içinde uuid ile taşır.
        OR (@kind::text = 'bank_transaction' AND w.entity_type = 'period' AND w.producer = 'control:bank'
            AND position((SELECT b.uuid::text FROM bank_transactions b
                          WHERE b.taxpayer_id = @taxpayer_id AND b.id = @source_id) IN w.params::text) > 0))

    UNION ALL
    -- Kayıt → inceleme kaydı (öneri).
    SELECT 'suggestion', s.uuid::text, 'review',
           to_char(s.created_at, 'YYYY-MM-DD'), '', '', s.status, s.kind, s.target_field
    FROM suggestions s
    WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id
      AND @kind::text IN ('invoice', 'bank_transaction', 'expense', 'retail_sale')
      AND ((s.target_type = @kind::text AND s.target_id = @source_id)
        OR (@kind::text = 'invoice' AND s.target_type = 'invoice_item' AND s.target_id IN (
            SELECT ii.id FROM invoice_items ii WHERE ii.taxpayer_id = @taxpayer_id AND ii.invoice_id = @source_id)))

    UNION ALL
    -- Uyarı → kaynak belge (fatura satırı uyarısı faturaya bağlanır).
    SELECT 'invoice', i.uuid::text, 'source',
           COALESCE(to_char(i.issue_date, 'YYYY-MM-DD'), ''), COALESCE(i.payable_amount::text, ''), i.currency,
           i.status_normalized, i.invoice_number, COALESCE(i.customer_name, '')
    FROM data_warnings w
    JOIN invoices i ON i.taxpayer_id = w.taxpayer_id AND i.id = CASE
        WHEN w.entity_type = 'invoice' THEN w.entity_id
        ELSE (SELECT ii.invoice_id FROM invoice_items ii WHERE ii.taxpayer_id = w.taxpayer_id AND ii.id = w.entity_id) END
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id
      AND w.entity_type IN ('invoice', 'invoice_item')
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)

    UNION ALL
    SELECT 'bank_transaction', b.uuid::text, 'source',
           COALESCE(to_char(b.transaction_date, 'YYYY-MM-DD'), ''), COALESCE(b.amount::text, ''), b.currency,
           b.match_status, b.description, b.debit_credit
    FROM data_warnings w
    JOIN bank_transactions b ON b.taxpayer_id = w.taxpayer_id AND b.id = w.entity_id
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id
      AND w.entity_type = 'bank_transaction'

    UNION ALL
    -- Banka kontrolünün dönem uyarısı → andığı hareketler.
    SELECT 'bank_transaction', b.uuid::text, 'source',
           COALESCE(to_char(b.transaction_date, 'YYYY-MM-DD'), ''), COALESCE(b.amount::text, ''), b.currency,
           b.match_status, b.description, b.debit_credit
    FROM data_warnings w
    JOIN bank_transactions b ON b.taxpayer_id = w.taxpayer_id
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id
      AND w.id = @source_id AND w.entity_type = 'period' AND w.producer = 'control:bank'
      AND b.uuid::text IN (
        SELECT jsonb_array_elements_text(g -> 'transactions')
        FROM jsonb_array_elements(CASE WHEN jsonb_typeof(w.params -> 'groups') = 'array'
                                       THEN w.params -> 'groups' ELSE '[]'::jsonb END) g
        WHERE jsonb_typeof(g -> 'transactions') = 'array')

    UNION ALL
    SELECT 'expense', e.uuid::text, 'source',
           COALESCE(to_char(e.expense_date, 'YYYY-MM-DD'), ''), COALESCE(e.amount::text, ''), e.currency,
           e.approval_status, COALESCE(e.vendor_name, e.description), e.receipt_type
    FROM data_warnings w
    JOIN expenses e ON e.taxpayer_id = w.taxpayer_id AND e.id = w.entity_id
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id
      AND w.entity_type = 'expense'

    UNION ALL
    SELECT 'retail_sale', s.uuid::text, 'source',
           COALESCE(to_char(s.sale_date, 'YYYY-MM-DD'), ''), COALESCE(s.total_amount::text, ''), s.currency,
           s.approval_status, COALESCE(s.z_report_number, ''), COALESCE(s.okc_serial_number, '')
    FROM data_warnings w
    JOIN retail_sales s ON s.taxpayer_id = w.taxpayer_id AND s.id = w.entity_id
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id
      AND w.entity_type = 'retail_sale'

    UNION ALL
    SELECT 'financial_transaction', f.uuid::text, 'source',
           COALESCE(to_char(f.transaction_date, 'YYYY-MM-DD'), ''), COALESCE(f.amount::text, ''), f.currency,
           f.status, f.description, f.transaction_type
    FROM data_warnings w
    JOIN source_financial_transactions f ON f.taxpayer_id = w.taxpayer_id AND f.id = w.entity_id
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id
      AND w.entity_type = 'financial_transaction'

    UNION ALL
    -- Uyarı → ilgili kural (kod; kataloğu ve ayarı arayüz gösterir).
    SELECT 'rule', '', 'rule', '', '', '', w.status, w.code, w.severity
    FROM data_warnings w
    WHERE @kind::text = 'warning' AND w.organization_id = @organization_id AND w.engagement_id = @engagement_id AND w.id = @source_id

    UNION ALL
    -- Fatura → Z raporu ve Easytrade hareketleri (invoice_id bağı).
    SELECT 'retail_sale', s.uuid::text, 'source',
           COALESCE(to_char(s.sale_date, 'YYYY-MM-DD'), ''), COALESCE(s.total_amount::text, ''), s.currency,
           s.approval_status, COALESCE(s.z_report_number, ''), COALESCE(s.okc_serial_number, '')
    FROM retail_sales s
    WHERE @kind::text = 'invoice' AND s.taxpayer_id = @taxpayer_id AND s.invoice_id = @source_id

    UNION ALL
    SELECT 'financial_transaction', f.uuid::text, 'source',
           COALESCE(to_char(f.transaction_date, 'YYYY-MM-DD'), ''), COALESCE(f.amount::text, ''), f.currency,
           f.status, f.description, f.transaction_type
    FROM source_financial_transactions f
    WHERE @kind::text = 'invoice' AND f.taxpayer_id = @taxpayer_id AND f.invoice_id = @source_id

    UNION ALL
    -- Z raporu / Easytrade hareketi → bağlı fatura.
    SELECT 'invoice', i.uuid::text, 'source',
           COALESCE(to_char(i.issue_date, 'YYYY-MM-DD'), ''), COALESCE(i.payable_amount::text, ''), i.currency,
           i.status_normalized, i.invoice_number, COALESCE(i.customer_name, '')
    FROM invoices i
    WHERE @kind::text IN ('retail_sale', 'financial_transaction') AND i.taxpayer_id = @taxpayer_id
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
      AND i.id IN (
        SELECT s.invoice_id FROM retail_sales s
        WHERE @kind::text = 'retail_sale' AND s.taxpayer_id = @taxpayer_id AND s.id = @source_id
        UNION ALL
        SELECT f.invoice_id FROM source_financial_transactions f
        WHERE @kind::text = 'financial_transaction' AND f.taxpayer_id = @taxpayer_id AND f.id = @source_id)

    UNION ALL
    -- Banka hareketi → cari / karşı hesap (muhasebeleştirme kaydı).
    SELECT 'account', '', 'counterparty_account', '', '', '', bp.status, bp.account_code, bp.account_name
    FROM bank_transaction_postings bp
    WHERE @kind::text = 'bank_transaction' AND bp.organization_id = @organization_id AND bp.engagement_id = @engagement_id
      AND bp.bank_transaction_id = @source_id
) r
ORDER BY r.relation, r.kind, r.rec_date, r.label
LIMIT 200;
