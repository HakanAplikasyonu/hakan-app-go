-- Kaynak finansal işlemleri ve Z raporları (issue #385).

-- name: UpsertSourceFinancialTransaction :one
INSERT INTO source_financial_transactions (
    taxpayer_id, source_system, source_id, transaction_date, due_date, period, transaction_type, transaction_flow,
    direction, payment_method_type, amount, currency, exchange_rate, amount_try, description, reference_document_no,
    status, customer_source_id, customer_name, customer_tax_number, cash_box_source_id, cash_box_name, cash_box_type,
    referenceable_type, referenceable_id, invoice_id, raw_payload, content_hash
) VALUES (
    @taxpayer_id, @source_system, @source_id, @transaction_date, sqlc.narg(due_date), @period, @transaction_type, @transaction_flow,
    @direction, @payment_method_type, @amount, @currency, sqlc.narg(exchange_rate), @amount_try, @description, sqlc.narg(reference_document_no),
    @status, sqlc.narg(customer_source_id), sqlc.narg(customer_name), sqlc.narg(customer_tax_number), sqlc.narg(cash_box_source_id),
    sqlc.narg(cash_box_name), sqlc.narg(cash_box_type), sqlc.narg(referenceable_type), sqlc.narg(referenceable_id),
    (SELECT i.id FROM invoices i WHERE i.taxpayer_id = @taxpayer_id AND i.source_system = @source_system
        AND sqlc.narg(referenceable_type)::text = 'Invoice' AND i.source_id = sqlc.narg(referenceable_id)::text LIMIT 1),
    @raw_payload, @content_hash
)
ON CONFLICT (taxpayer_id, source_system, source_id) DO UPDATE
SET transaction_date = EXCLUDED.transaction_date, due_date = EXCLUDED.due_date, period = EXCLUDED.period,
    transaction_type = EXCLUDED.transaction_type, transaction_flow = EXCLUDED.transaction_flow, direction = EXCLUDED.direction,
    payment_method_type = EXCLUDED.payment_method_type, amount = EXCLUDED.amount, currency = EXCLUDED.currency,
    exchange_rate = EXCLUDED.exchange_rate, amount_try = EXCLUDED.amount_try, description = EXCLUDED.description,
    reference_document_no = EXCLUDED.reference_document_no, status = EXCLUDED.status,
    customer_source_id = EXCLUDED.customer_source_id, customer_name = EXCLUDED.customer_name,
    customer_tax_number = EXCLUDED.customer_tax_number, cash_box_source_id = EXCLUDED.cash_box_source_id,
    cash_box_name = EXCLUDED.cash_box_name, cash_box_type = EXCLUDED.cash_box_type,
    referenceable_type = EXCLUDED.referenceable_type, referenceable_id = EXCLUDED.referenceable_id,
    invoice_id = EXCLUDED.invoice_id, raw_payload = EXCLUDED.raw_payload, content_hash = EXCLUDED.content_hash
WHERE source_financial_transactions.content_hash IS DISTINCT FROM EXCLUDED.content_hash
   OR source_financial_transactions.invoice_id IS DISTINCT FROM EXCLUDED.invoice_id
RETURNING id, (xmax = 0) AS inserted;

-- name: ListSourceFinancialTransactions :many
SELECT id, uuid, source_id, transaction_date, due_date, period, transaction_type, transaction_flow, direction,
       payment_method_type, amount, currency, amount_try, description, reference_document_no, status,
       customer_name, customer_tax_number, cash_box_name, cash_box_type, referenceable_type, referenceable_id,
       COALESCE((SELECT i.uuid::text FROM invoices i WHERE i.taxpayer_id = f.taxpayer_id AND i.id = f.invoice_id), '')::text AS invoice_uuid
FROM source_financial_transactions f
WHERE f.taxpayer_id = @taxpayer_id AND f.source_system = @source_system AND f.period = @period
ORDER BY f.transaction_date DESC, f.id DESC
LIMIT @lim;

-- name: SummarizeSourceFinancialByPayment :many
SELECT payment_method_type, direction, count(*)::int AS count, COALESCE(sum(amount_try), 0)::numeric(18,2) AS total
FROM source_financial_transactions
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND period = @period
GROUP BY payment_method_type, direction
ORDER BY payment_method_type, direction;

-- name: SummarizeSourceFinancialByCustomer :many
SELECT COALESCE(NULLIF(customer_name, ''), '')::text AS customer_name, COALESCE(customer_tax_number, '')::text AS customer_tax_number,
       count(*)::int AS count,
       COALESCE(sum(amount_try) FILTER (WHERE direction = 'in'), 0)::numeric(18,2) AS total_in,
       COALESCE(sum(amount_try) FILTER (WHERE direction = 'out'), 0)::numeric(18,2) AS total_out
FROM source_financial_transactions
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND period = @period
GROUP BY 1, 2
ORDER BY 4 DESC, 5 DESC
LIMIT 50;

-- name: CountSourceFinancialMatches :one
SELECT count(*) FILTER (WHERE referenceable_type = 'Invoice')::int AS invoice_refs,
       count(*) FILTER (WHERE referenceable_type = 'Invoice' AND invoice_id IS NOT NULL)::int AS invoice_matched
FROM source_financial_transactions
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND period = @period;

-- name: UpsertSourceZReport :one
-- Z raporu retail_sales'e günlük toplam olarak yazılır (01 §4.7); kaynak kimliği "z:<id>".
INSERT INTO retail_sales (
    taxpayer_id, source_system, source_id, capture_method, sale_date, period, z_report_number, okc_serial_number,
    subtotal_amount, tax_amount, total_amount, currency, raw_payload, content_hash
) VALUES (
    @taxpayer_id, @source_system, @source_id, 'api', @sale_date, @period, sqlc.narg(z_report_number), sqlc.narg(okc_serial_number),
    @subtotal_amount, @tax_amount, @total_amount, @currency, @raw_payload, @content_hash
)
ON CONFLICT (taxpayer_id, source_system, source_id) DO UPDATE
SET sale_date = EXCLUDED.sale_date, period = EXCLUDED.period, z_report_number = EXCLUDED.z_report_number,
    okc_serial_number = EXCLUDED.okc_serial_number, subtotal_amount = EXCLUDED.subtotal_amount,
    tax_amount = EXCLUDED.tax_amount, total_amount = EXCLUDED.total_amount, currency = EXCLUDED.currency,
    raw_payload = EXCLUDED.raw_payload, content_hash = EXCLUDED.content_hash
WHERE retail_sales.content_hash IS DISTINCT FROM EXCLUDED.content_hash
RETURNING id, (xmax = 0) AS inserted;

-- name: ListSourceZReports :many
SELECT id, uuid, source_id, sale_date, period, z_report_number, okc_serial_number, subtotal_amount, tax_amount,
       total_amount, currency, approval_status, raw_payload
FROM retail_sales
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND period = @period AND source_id LIKE 'z:%'
ORDER BY sale_date DESC, id DESC;
