-- name: FindCustomerByTaxNumber :one
SELECT * FROM customers
WHERE taxpayer_id = @taxpayer_id AND tax_number = @tax_number;

-- name: FindCustomerByIdentityNumber :one
SELECT * FROM customers
WHERE taxpayer_id = @taxpayer_id AND identity_number = @identity_number AND tax_number IS NULL;

-- name: FindCustomerBySource :one
SELECT c.* FROM customers c
JOIN customer_source_links l ON l.customer_id = c.id AND l.taxpayer_id = c.taxpayer_id
WHERE l.taxpayer_id = @taxpayer_id AND l.source_system = @source_system AND l.source_id = @source_id;

-- name: InsertCustomer :one
INSERT INTO customers (
    taxpayer_id, source_system, source_id, capture_method, type, customer_legal_type,
    trade_name, first_name, last_name, name, tax_number, identity_number, tax_office,
    address, city, district, postal_code, country, email, phone,
    is_default_retail_customer, is_abroad, raw_payload, content_hash
) VALUES (
    @taxpayer_id, @source_system, @source_id, @capture_method, @type, sqlc.narg(customer_legal_type),
    sqlc.narg(trade_name), sqlc.narg(first_name), sqlc.narg(last_name), @name, sqlc.narg(tax_number), sqlc.narg(identity_number), sqlc.narg(tax_office),
    sqlc.narg(address), sqlc.narg(city), sqlc.narg(district), sqlc.narg(postal_code), sqlc.narg(country), sqlc.narg(email), sqlc.narg(phone),
    @is_default_retail_customer, @is_abroad, @raw_payload, sqlc.narg(content_hash)
)
RETURNING *;

-- name: SetCustomerType :exec
UPDATE customers SET type = @type
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: UpsertCustomerSourceLink :exec
INSERT INTO customer_source_links (taxpayer_id, customer_id, source_system, source_id, raw_payload)
VALUES (@taxpayer_id, @customer_id, @source_system, @source_id, @raw_payload)
ON CONFLICT (taxpayer_id, source_system, source_id)
DO UPDATE SET raw_payload = EXCLUDED.raw_payload;

-- name: ListCustomerSourceLinks :many
SELECT * FROM customer_source_links
WHERE taxpayer_id = @taxpayer_id AND customer_id = @customer_id
ORDER BY id;
