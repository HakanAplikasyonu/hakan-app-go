DROP TABLE IF EXISTS luca_koza_settings;
DELETE FROM source_connections WHERE source_system = 'luca_koza';
ALTER TABLE source_connections DROP CONSTRAINT chk_source_connections_source_system;
ALTER TABLE source_connections ADD CONSTRAINT chk_source_connections_source_system
    CHECK (source_system ~ '^(easytrade|parasut|kolaybi|bizimhesap|logo_isbasi|logo_tiger|logo_go|mikro|robom|manual_upload|entegrator_[a-z0-9_]+|bank_[a-z0-9_]+)$');
