ALTER TABLE data_warnings DROP CONSTRAINT IF EXISTS chk_data_warnings_resolution_note;
ALTER TABLE data_warnings DROP COLUMN IF EXISTS resolution_note;
