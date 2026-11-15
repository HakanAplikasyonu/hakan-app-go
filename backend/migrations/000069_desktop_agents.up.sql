-- Masaüstü yerel ajanı (issue #51, 02 §6): kayıt ve sağlık.
--
-- Ajan, ofisin (ya da mükellefin) makinesinde çalışan masaüstü
-- uygulamasıdır. Oturum açmış ofis kullanıcısının yetkisiyle kendini kaydeder
-- ve 5 dakikada bir heartbeat gönderir. 30 dakika sessiz kalan ajan
-- `offline` olur ve kaydeden kullanıcıya AGENT_OFFLINE bildirimi gider
-- (05 §4: bağlantı olayı; data_warnings'e yazılmaz).
--
-- machine_id: kurulumun kalıcı kimliği (ajan üretir). Aynı makine yeniden
-- kayıt olursa satır güncellenir, ikinci satır açılmaz.
CREATE TABLE desktop_agents (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    machine_id           TEXT         NOT NULL,
    name                 TEXT         NOT NULL,
    platform             TEXT         NOT NULL,
    version              TEXT         NOT NULL DEFAULT '',
    created_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    status               TEXT         NOT NULL DEFAULT 'online',
    last_seen_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_heartbeat       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    offline_since        TIMESTAMPTZ  NULL,
    revoked_at           TIMESTAMPTZ  NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_desktop_agents_uuid UNIQUE (uuid),
    CONSTRAINT uq_desktop_agents_machine UNIQUE (organization_id, machine_id),
    CONSTRAINT chk_desktop_agents_machine_id CHECK (machine_id ~ '^[A-Za-z0-9_.:-]{8,128}$'),
    CONSTRAINT chk_desktop_agents_name CHECK (btrim(name) <> '' AND length(name) <= 120),
    CONSTRAINT chk_desktop_agents_platform CHECK (platform IN ('windows', 'macos', 'linux')),
    CONSTRAINT chk_desktop_agents_version CHECK (length(version) <= 64),
    CONSTRAINT chk_desktop_agents_status CHECK (status IN ('online', 'offline'))
);

-- Sessizlik taraması: yalnızca çevrimiçi ve geri alınmamış ajanlar.
CREATE INDEX idx_desktop_agents_sweep ON desktop_agents (last_seen_at)
    WHERE status = 'online' AND revoked_at IS NULL;

CREATE TRIGGER trg_desktop_agents_set_updated_at
    BEFORE UPDATE ON desktop_agents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6).
SELECT rls_apply_org('desktop_agents');

INSERT INTO permissions (name, slug) VALUES
    ('Read desktop agents', 'tenant.agents.read'),
    ('Write desktop agents', 'tenant.agents.write')
ON CONFLICT (slug) DO NOTHING;

-- Masaüstü uygulamasını ofisin her çalışanı kullanabilir; stajyer yalnızca
-- listeyi görür.
SELECT grant_tenant_role_permission('admin', 'tenant.agents.read');
SELECT grant_tenant_role_permission('admin', 'tenant.agents.write');
SELECT grant_tenant_role_permission('accountant', 'tenant.agents.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.agents.write');
SELECT grant_tenant_role_permission('member', 'tenant.agents.read');
SELECT grant_tenant_role_permission('member', 'tenant.agents.write');
SELECT grant_tenant_role_permission('intern', 'tenant.agents.read');

-- Bildirim şablonları. Değişkenler: agent_name, minutes, link.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('agents.offline', 'inapp', 'tr', 'Masaüstü ajanı bağlantısı koptu',
     '{{agent_name}} {{minutes}} dakikadır sessiz. Bilgisayarın açık ve uygulamanın çalışır olduğunu kontrol edin.', TRUE),
    ('agents.offline', 'inapp', 'en', 'Desktop agent went offline',
     '{{agent_name}} has been silent for {{minutes}} minutes. Check that the computer is on and the app is running.', TRUE),
    ('agents.offline', 'email', 'tr', 'Masaüstü ajanı bağlantısı koptu: {{agent_name}}',
     'Merhaba, {{agent_name}} adlı masaüstü ajanından {{minutes}} dakikadır haber alınamıyor. Klasör izleme ve yerel program okuma bu sürede durur. Ayrıntılar: {{link}}', TRUE),
    ('agents.offline', 'email', 'en', 'Desktop agent offline: {{agent_name}}',
     'Hello, the desktop agent {{agent_name}} has been silent for {{minutes}} minutes. Folder watching and local program reads are paused meanwhile. Details: {{link}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
