-- name: InsertInvoice :one
INSERT INTO invoices (
    taxpayer_id, source_system, source_id, capture_method, ettn, invoice_number, direction, type,
    invoice_type_code_raw, profile, profile_raw, document_class, status, status_normalized,
    issue_date, issue_time, payment_due_date, period,
    customer_id, customer_tax_number, customer_identity_number, customer_name,
    currency, exchange_rate,
    line_extension_amount, allowance_total_amount, tax_exclusive_amount, tax_amount, tax_inclusive_amount,
    withholding_amount, excise_amount, communications_tax_amount, income_tax_withholding_amount,
    income_tax_withholding_rate, payable_amount, payable_amount_try,
    global_tax_exemption_code, original_invoice_number, original_invoice_issue_date, order_reference,
    notes, has_items, tax_summary_source, ubl_attachment_id, raw_payload, content_hash, source_updated_at
) VALUES (
    @taxpayer_id, @source_system, @source_id, @capture_method, sqlc.narg(ettn), @invoice_number, @direction, @type,
    sqlc.narg(invoice_type_code_raw), sqlc.narg(profile), sqlc.narg(profile_raw), @document_class, sqlc.narg(status), @status_normalized,
    @issue_date, sqlc.narg(issue_time), sqlc.narg(payment_due_date), @period,
    sqlc.narg(customer_id), sqlc.narg(customer_tax_number), sqlc.narg(customer_identity_number), sqlc.narg(customer_name),
    @currency, sqlc.narg(exchange_rate),
    @line_extension_amount, @allowance_total_amount, @tax_exclusive_amount, @tax_amount, @tax_inclusive_amount,
    @withholding_amount, @excise_amount, @communications_tax_amount, @income_tax_withholding_amount,
    sqlc.narg(income_tax_withholding_rate), @payable_amount, sqlc.narg(payable_amount_try),
    sqlc.narg(global_tax_exemption_code), sqlc.narg(original_invoice_number), sqlc.narg(original_invoice_issue_date), sqlc.narg(order_reference),
    sqlc.narg(notes), @has_items, @tax_summary_source, sqlc.narg(ubl_attachment_id), @raw_payload, sqlc.narg(content_hash), sqlc.narg(source_updated_at)
)
RETURNING id, uuid;

-- name: GetInvoice :one
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetInvoiceBySource :one
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND source_id = @source_id;

-- name: FindInvoicesByETTN :many
-- 01 §6 kural 1.
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND ettn = @ettn
ORDER BY id;

-- name: FindInvoicesByPartyNumberDate :many
-- 01 §6 kural 2: karşı taraf VKN/TCKN + fatura no + tarih.
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND invoice_number = @invoice_number
  AND issue_date = @issue_date
  AND (customer_tax_number = sqlc.narg(customer_tax_number) OR customer_identity_number = sqlc.narg(customer_identity_number))
ORDER BY id;

-- name: FindPossibleDuplicateInvoices :many
-- 01 §6 kural 3: karşı taraf + tarih + ödenecek tutar (±tolerans).
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND issue_date = @issue_date
  AND direction = @direction
  AND (customer_tax_number = sqlc.narg(customer_tax_number) OR customer_identity_number = sqlc.narg(customer_identity_number))
  AND payable_amount BETWEEN @payable_min AND @payable_max
  AND id <> @exclude_id
ORDER BY id;

-- name: ListInvoicesByPeriod :many
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND period = @period
ORDER BY issue_date, id
LIMIT @lim OFFSET @off;

-- name: InsertInvoiceItem :one
INSERT INTO invoice_items (
    taxpayer_id, invoice_id, line_no, name, description, product_code, quantity, unit_code, unit_price,
    line_extension_amount, own_allowance_amount, allowance_total_amount, tax_exclusive_amount,
    tax_rate, total_tax_amount, tax_inclusive_amount, withholding_code, withholding_rate, withholding_amount,
    tax_exemption_code, special_tax_base_code, special_tax_base_amount, export_registered_code, gtip_code,
    excise_rate, excise_amount, communications_tax_rate, communications_tax_amount,
    income_tax_withholding_amount, line_type, tax_breakdowns
) VALUES (
    @taxpayer_id, @invoice_id, @line_no, @name, sqlc.narg(description), sqlc.narg(product_code), @quantity, sqlc.narg(unit_code), @unit_price,
    @line_extension_amount, @own_allowance_amount, @allowance_total_amount, @tax_exclusive_amount,
    @tax_rate, @total_tax_amount, @tax_inclusive_amount, sqlc.narg(withholding_code), @withholding_rate, @withholding_amount,
    sqlc.narg(tax_exemption_code), sqlc.narg(special_tax_base_code), sqlc.narg(special_tax_base_amount), sqlc.narg(export_registered_code), sqlc.narg(gtip_code),
    @excise_rate, @excise_amount, @communications_tax_rate, @communications_tax_amount,
    @income_tax_withholding_amount, @line_type, @tax_breakdowns
)
RETURNING id;

-- name: ListInvoiceItems :many
SELECT * FROM invoice_items
WHERE taxpayer_id = @taxpayer_id AND invoice_id = @invoice_id
ORDER BY line_no;

-- name: InsertInvoiceTaxSummary :exec
INSERT INTO invoice_tax_summaries (taxpayer_id, invoice_id, tax_rate, base, vat, withholding, withholding_code, tax_exemption_code)
VALUES (@taxpayer_id, @invoice_id, @tax_rate, @base, @vat, @withholding, sqlc.narg(withholding_code), sqlc.narg(tax_exemption_code));

-- name: ListInvoiceTaxSummaries :many
SELECT * FROM invoice_tax_summaries
WHERE taxpayer_id = @taxpayer_id AND invoice_id = @invoice_id
ORDER BY tax_rate, withholding_code NULLS FIRST, tax_exemption_code NULLS FIRST;

-- name: DeleteInvoiceChildren :exec
-- Yeniden işlemede (ör. kaynak önceliği daha yüksek bir sürüm geldi) satırlar ve özet baştan yazılır.
WITH del_items AS (
    DELETE FROM invoice_items WHERE invoice_items.taxpayer_id = @taxpayer_id AND invoice_items.invoice_id = @invoice_id
)
DELETE FROM invoice_tax_summaries WHERE invoice_tax_summaries.taxpayer_id = @taxpayer_id AND invoice_tax_summaries.invoice_id = @invoice_id;

-- name: PeriodVATSummary :many
-- KDV1 için dönem özeti (yalnızca aktif faturalar).
SELECT i.direction, s.tax_rate, s.withholding_code, s.tax_exemption_code,
       SUM(s.base)::numeric AS base, SUM(s.vat)::numeric AS vat, SUM(s.withholding)::numeric AS withholding
FROM invoices i
JOIN invoice_tax_summaries s ON s.invoice_id = i.id AND s.taxpayer_id = i.taxpayer_id
WHERE i.taxpayer_id = @taxpayer_id AND i.period = @period AND i.status_normalized = 'active'
GROUP BY i.direction, s.tax_rate, s.withholding_code, s.tax_exemption_code
ORDER BY i.direction, s.tax_rate;

-- name: GetTaxpayerOwnTaxID :one
-- Mükellefin kendi VKN/TCKN'si (issue #3); UBL'de kendi taraf doğrulaması buna göre.
SELECT tax_id FROM taxpayers WHERE id = @taxpayer_id;

-- name: ReplaceInvoiceContent :execrows
-- Daha yüksek öncelikli bir kaynak geldiğinde faturanın içeriği o kaynaktan
-- yeniden yazılır (01 §6). Kimlik (id, uuid) ve ETTN korunur.
UPDATE invoices SET
    source_system = @source_system, source_id = @source_id, capture_method = @capture_method,
    ettn = COALESCE(sqlc.narg(ettn), ettn), invoice_number = @invoice_number, direction = @direction, type = @type,
    invoice_type_code_raw = sqlc.narg(invoice_type_code_raw), profile = sqlc.narg(profile), profile_raw = sqlc.narg(profile_raw),
    document_class = @document_class, issue_date = @issue_date, issue_time = sqlc.narg(issue_time),
    payment_due_date = sqlc.narg(payment_due_date), period = @period,
    customer_id = sqlc.narg(customer_id), customer_tax_number = sqlc.narg(customer_tax_number),
    customer_identity_number = sqlc.narg(customer_identity_number), customer_name = sqlc.narg(customer_name),
    -- Kaynak kur vermezse ofisin girdiği kur korunur (#571).
    currency = @currency, exchange_rate = COALESCE(sqlc.narg(exchange_rate), exchange_rate),
    -- Kaynak kur verirse otomatik / elle kur etiketi düşer (#780).
    exchange_rate_source = CASE WHEN sqlc.narg(exchange_rate)::numeric IS NULL THEN exchange_rate_source END,
    exchange_rate_date = CASE WHEN sqlc.narg(exchange_rate)::numeric IS NULL THEN exchange_rate_date END,
    line_extension_amount = @line_extension_amount, allowance_total_amount = @allowance_total_amount,
    tax_exclusive_amount = @tax_exclusive_amount, tax_amount = @tax_amount, tax_inclusive_amount = @tax_inclusive_amount,
    withholding_amount = @withholding_amount, excise_amount = @excise_amount, communications_tax_amount = @communications_tax_amount,
    income_tax_withholding_amount = @income_tax_withholding_amount, income_tax_withholding_rate = sqlc.narg(income_tax_withholding_rate),
    payable_amount = @payable_amount, payable_amount_try = sqlc.narg(payable_amount_try),
    global_tax_exemption_code = sqlc.narg(global_tax_exemption_code), original_invoice_number = sqlc.narg(original_invoice_number),
    original_invoice_issue_date = sqlc.narg(original_invoice_issue_date), order_reference = sqlc.narg(order_reference),
    notes = sqlc.narg(notes), has_items = @has_items, tax_summary_source = @tax_summary_source,
    ubl_attachment_id = COALESCE(sqlc.narg(ubl_attachment_id), ubl_attachment_id),
    raw_payload = @raw_payload, content_hash = sqlc.narg(content_hash)
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: GetInvoiceByID :one
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: SetMissingInvoiceExchangeRate :execrows
-- Dövizli faturada kaynakta kur yoksa ofis girer (#571); TL karşılığı da
-- boşsa kurdan hesaplanır. TCMB'den otomatik gelen kur (#780) elle
-- değiştirilebilir; elle girilen kur bir daha otomatikle ezilmez. Kaynakta
-- kuru olan ya da TL faturaya dokunulmaz.
UPDATE invoices SET
    exchange_rate = @rate::numeric,
    payable_amount_try = CASE WHEN exchange_rate_source = 'tcmb' THEN ROUND(payable_amount * @rate::numeric, 2)
                              ELSE COALESCE(payable_amount_try, ROUND(payable_amount * @rate::numeric, 2)) END,
    exchange_rate_source = 'manual', exchange_rate_date = NULL,
    exchange_rate_set_by = sqlc.narg(set_by), exchange_rate_set_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id
  AND currency <> 'TRY' AND (exchange_rate IS NULL OR exchange_rate = 0 OR exchange_rate_source = 'tcmb');

-- name: InvoiceExchangeRateEntry :one
-- Elle girilen kurun kim tarafından ne zaman girildiği (#571).
SELECT i.exchange_rate_set_at, COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.name, u.surname)), ''), u.email::text, '')::text AS set_by_name
FROM invoices i
LEFT JOIN users u ON u.id = i.exchange_rate_set_by
WHERE i.taxpayer_id = @taxpayer_id AND i.id = @id AND i.exchange_rate_set_at IS NOT NULL;

-- name: SetMissingInvoiceCounterpartyTaxID :execrows
-- Karşı tarafın VKN/TCKN'si kaynakta yoksa ya da geçersizse ofis girer
-- (#669, INVALID_TAX_ID): cari bağlanır. Numarası olan faturaya dokunulmaz.
UPDATE invoices SET
    customer_id = @customer_id,
    customer_tax_number = sqlc.narg(tax_number),
    customer_identity_number = sqlc.narg(identity_number),
    counterparty_tax_id_set_by = sqlc.narg(set_by), counterparty_tax_id_set_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id
  AND customer_tax_number IS NULL AND customer_identity_number IS NULL;

-- name: InvoiceCounterpartyTaxIDEntry :one
-- Elle girilen VKN/TCKN'nin kim tarafından ne zaman girildiği (#669).
SELECT i.counterparty_tax_id_set_at, COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.name, u.surname)), ''), u.email::text, '')::text AS set_by_name
FROM invoices i
LEFT JOIN users u ON u.id = i.counterparty_tax_id_set_by
WHERE i.taxpayer_id = @taxpayer_id AND i.id = @id AND i.counterparty_tax_id_set_at IS NOT NULL;

-- name: SetInvoiceVATDeductionPeriod :execrows
-- KDV indirim dönemi (#640): alış faturasının KDV'si bu aya alınır; NULL
-- belge ayına döndürür. Sınırları tablo kısıtı da denetler.
UPDATE invoices SET
    vat_deduction_period = sqlc.narg(vat_period)::text,
    vat_deduction_set_by = sqlc.narg(set_by)::bigint,
    vat_deduction_set_at = CASE WHEN sqlc.narg(vat_period)::text IS NULL THEN NULL ELSE NOW() END
WHERE taxpayer_id = @taxpayer_id AND id = @id AND direction = 'incoming';

-- name: InvoiceVATDeductionEntry :one
-- KDV indirim dönemini kimin ne zaman değiştirdiği (#640).
SELECT i.vat_deduction_set_at, COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.name, u.surname)), ''), u.email::text, '')::text AS set_by_name
FROM invoices i
LEFT JOIN users u ON u.id = i.vat_deduction_set_by
WHERE i.taxpayer_id = @taxpayer_id AND i.id = @id AND i.vat_deduction_set_at IS NOT NULL;
