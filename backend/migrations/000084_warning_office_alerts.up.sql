-- Kritik uyarı bildirimi (issue #241): engelleyici bir uyarı açılınca
-- engagement'a atanan ofis üyelerine (atanan yoksa ofis sahiplerine) bir
-- kez bildirim gider. Hangi uyarının bildirildiği burada tutulur; uyarı
-- satırının updated_at'i değişmesin diye ayrı tablo.
CREATE TABLE warning_office_alerts (
    warning_id       BIGINT       PRIMARY KEY REFERENCES data_warnings (id) ON DELETE CASCADE,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    notified_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    recipients       INTEGER      NOT NULL DEFAULT 0,
    CONSTRAINT fk_warning_office_alerts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_warning_office_alerts_recipients CHECK (recipients >= 0)
);

CREATE INDEX idx_warning_office_alerts_engagement ON warning_office_alerts (organization_id, engagement_id);
-- Tarama: açık engelleyiciler, en yeniler.
CREATE INDEX idx_data_warnings_open_blockers ON data_warnings (created_at) WHERE status = 'open' AND severity = 'blocker';

SELECT rls_apply_engagement('warning_office_alerts'::regclass);

-- Kurulumdan önceki açık engelleyiciler bildirilmiş sayılır (ilk çalışmada
-- toplu bildirim gitmesin).
INSERT INTO warning_office_alerts (warning_id, organization_id, engagement_id, recipients)
SELECT id, organization_id, engagement_id, 0 FROM data_warnings
WHERE status = 'open' AND severity = 'blocker'
ON CONFLICT (warning_id) DO NOTHING;

INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('warnings.blocker', 'inapp', 'tr', 'Kritik uyarı',
     '{{taxpayer_name}} için {{count}} engelleyici uyarı açıldı.', TRUE),
    ('warnings.blocker', 'inapp', 'en', 'Critical warning',
     '{{count}} blocking warnings were raised for {{taxpayer_name}}.', TRUE),
    ('warnings.blocker', 'email', 'tr', 'Kritik uyarı: {{taxpayer_name}}',
     'Merhaba, {{taxpayer_name}} için {{count}} engelleyici uyarı açıldı. Engelleyiciler kapanmadan dönem beyana hazır sayılmaz: {{link}}', TRUE),
    ('warnings.blocker', 'email', 'en', 'Critical warning: {{taxpayer_name}}',
     'Hello, {{count}} blocking warnings were raised for {{taxpayer_name}}. The period is not ready until they are closed: {{link}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
