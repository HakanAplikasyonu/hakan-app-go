-- Parçalı Z raporu (Oğuz'un demo geri bildirimi): bir fiş yakalamasının
-- sıralı fotoğrafları.

-- name: InsertReceiptCapturePart :exec
INSERT INTO receipt_capture_parts (organization_id, engagement_id, taxpayer_id, receipt_capture_id, part_no, attachment_id, file_name)
SELECT c.organization_id, c.engagement_id, c.taxpayer_id, c.id, @part_no, @attachment_id, @file_name
FROM receipt_captures c
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id AND c.uuid = @capture_uuid
ON CONFLICT (receipt_capture_id, part_no) DO NOTHING;

-- name: ListReceiptCaptureParts :many
SELECT p.part_no, p.file_name
FROM receipt_capture_parts p
JOIN receipt_captures c ON c.id = p.receipt_capture_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND c.uuid = @capture_uuid
ORDER BY p.part_no;

-- name: GetEngagementReceiptPartImage :one
SELECT a.storage_key, a.content_type, a.size_bytes
FROM receipt_capture_parts p
JOIN receipt_captures c ON c.id = p.receipt_capture_id
JOIN attachments a ON a.taxpayer_id = p.taxpayer_id AND a.id = p.attachment_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id
  AND c.uuid = @receipt_uuid AND p.part_no = @part_no;

-- name: ListRetailSaleCaptures :many
-- Z raporlarının yakalaması ve parça sayısı (ofis Z listesinde tek satır).
SELECT c.retail_sale_id::bigint AS retail_sale_id, c.uuid,
       (SELECT count(*) FROM receipt_capture_parts p
        WHERE p.organization_id = c.organization_id AND p.engagement_id = c.engagement_id
          AND p.receipt_capture_id = c.id)::int AS part_count
FROM receipt_captures c
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
  AND c.retail_sale_id = ANY(@retail_sale_ids::bigint[])
ORDER BY c.id;
