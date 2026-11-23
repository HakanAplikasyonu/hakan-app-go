-- Easytrade masraflarının kırılımı yalnızca bu özellikle yazılır (#780).
-- Sahip FORCE altında: veri silen adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DELETE FROM expense_vat_lines l
USING expenses e
WHERE e.taxpayer_id = l.taxpayer_id AND e.id = l.expense_id
  AND e.source_system = 'easytrade' AND e.capture_method = 'api';
