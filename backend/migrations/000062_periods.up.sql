-- Dönem yönetimi (issue #32): dönem kilidi, kilit olay günlüğü ve izinler.
-- Kaynak: docs/kaynak/musavir-entegrasyon-paketi/02-adaptor-mimarisi.md §3.4,
-- docs/01 §3 ("command center": mükellef × dönem matrisi).
--
-- Kilit ofisin iş ürünüdür: organization_id + engagement_id (docs/01 §2.3).
-- Fatura ise mükellefin kaynak verisidir; yazım yolu mükellefin aktif
-- engagement'ındaki kilide bakar (D1: tek aktif engagement). Kilitli döneme
-- gelen değişiklik kaydı değiştirmez; change_requests'e (000037) yazılır ve
-- LOCKED_PERIOD_CHANGE uyarısı açılır.

-- Satır varsa dönem kilitlidir; kilit kaldırılınca satır silinir, iz
-- period_lock_events'te kalır.
CREATE TABLE period_locks (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    -- engagement'ın mükellefi; fatura yazım yolu kilidi buradan bulur.
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period           CHAR(7)      NOT NULL,
    note             TEXT         NULL,
    locked_by        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    locked_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_period_locks_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_period_locks_engagement_period UNIQUE (engagement_id, period),
    CONSTRAINT chk_period_locks_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_period_locks_taxpayer ON period_locks (taxpayer_id, period);
CREATE INDEX idx_period_locks_org ON period_locks (organization_id, period);

-- Kilit / kilit kaldırma denetim izi. Kaldırma gerekçesiz olamaz.
CREATE TABLE period_lock_events (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    period           CHAR(7)      NOT NULL,
    action           TEXT         NOT NULL,
    reason           TEXT         NULL,
    actor_id         BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_period_lock_events_uuid UNIQUE (uuid),
    CONSTRAINT fk_period_lock_events_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_period_lock_events_action CHECK (action IN ('lock', 'unlock')),
    CONSTRAINT chk_period_lock_events_reason CHECK (action <> 'unlock' OR (reason IS NOT NULL AND btrim(reason) <> '')),
    CONSTRAINT chk_period_lock_events_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_period_lock_events_list ON period_lock_events (organization_id, engagement_id, period, created_at DESC);

-- Aynı içerik kilitli döneme ikinci kez gelirse (yeniden yükleme, iş
-- tekrarı) açık talep tekrar kullanılır; bu indeks onu bulur.
CREATE INDEX idx_change_requests_open_hash
    ON change_requests (engagement_id, entity_type, new_hash)
    WHERE status = 'open';

-- RLS (issue #6): engagement kapsamlı iki tablo.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY['period_locks', 'period_lock_events']::regclass[]) AS t;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant periods', 'tenant.periods.read'),
    ('Lock tenant periods', 'tenant.periods.lock'),
    ('Decide tenant change requests', 'tenant.change_requests.decide')
ON CONFLICT (slug) DO NOTHING;

-- Sahip hepsini kendiliğinden alır (000044). Yönetici ve mali müşavir hepsi;
-- personel ve stajyer yalnızca görür.
SELECT grant_tenant_role_permission('admin', 'tenant.periods.read');
SELECT grant_tenant_role_permission('admin', 'tenant.periods.lock');
SELECT grant_tenant_role_permission('admin', 'tenant.change_requests.decide');
SELECT grant_tenant_role_permission('accountant', 'tenant.periods.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.periods.lock');
SELECT grant_tenant_role_permission('accountant', 'tenant.change_requests.decide');
SELECT grant_tenant_role_permission('member', 'tenant.periods.read');
SELECT grant_tenant_role_permission('intern', 'tenant.periods.read');
