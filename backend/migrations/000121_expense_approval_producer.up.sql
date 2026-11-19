-- #529: UNAPPROVED_EXPENSE artık control:expense_approval üreticisinden;
-- masraf onaylanınca / reddedilince kendiliğinden kapanır. Eski üretici
-- adıyla yazılmış uyarılar yeni üreticiye taşınır, karar verilmiş
-- masrafların açık uyarıları kapanır.
UPDATE data_warnings
SET producer = 'control:expense_approval', updated_at = NOW()
WHERE code = 'UNAPPROVED_EXPENSE' AND entity_type = 'expense' AND producer = 'receipts';

UPDATE data_warnings w
SET status = 'auto_resolved', resolved_at = NOW(), resolved_by = NULL, updated_at = NOW()
FROM expenses e
WHERE w.code = 'UNAPPROVED_EXPENSE' AND w.entity_type = 'expense' AND w.status = 'open'
  AND e.id = w.entity_id AND e.taxpayer_id = w.taxpayer_id AND e.approval_status <> 'pending';
