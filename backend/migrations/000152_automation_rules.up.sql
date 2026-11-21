-- İletişim merkezi (#780 C bölümü): otomasyon kuralları (Ayarlar >
-- Otomasyonlar). Kural bir tetikleyiciye (takvim, olay ya da kişiye özel
-- ödeme) bağlanır, hedef mükelleflere seçilen kanallardan (SMS, e-posta,
-- uygulama içi) değişkenli metin gönderir. Gönderim 000152'nin servisinden
-- geçer; her çalışma automation_runs'ta kanıt olarak durur.
--
-- Güvenlik kuralları:
--   * Aynı kural aynı mükellefe aynı dönemde bir kez çalışır: run_key
--     (takvim: dönem + gün, son gün öncesi: beyan + dönem, olay: olayın
--     dedupe_key'i) kural ve mükellefle birlikte tekildir.
--   * Kural geçmişi (çalışmalar ve düzenlemeler) tutulur; silinen kural
--     gizlenir (deleted_at), geçmişi kalır.
--   * Ofis bütün otomasyonları tek anahtarla kapatabilir
--     (office_automation_settings.enabled).

-- Ofis geneli ana anahtar. Satır yoksa otomasyonlar açıktır (kurallar
-- tek tek pasif gelir).
CREATE TABLE office_automation_settings (
    organization_id BIGINT      PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_office_automation_settings_updated_by ON office_automation_settings (updated_by);
SELECT rls_apply_org('office_automation_settings');

-- Kural. trigger_kind: calendar (ayın belli günleri / son günden X gün
-- önce), event (olay sonrası), payment (kişiye özel ödeme tarihi).
-- trigger_config JSON'u tetikleyicinin ayrıntısıdır (günler, saat, beyan, olay,
-- gecikme, tekrar gönderim, ödeme tarihi). template_code hazır kuralların
-- kodudur ("kdv_28", ...); ofisin kendi kuralında boştur.
CREATE TABLE automation_rules (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    template_code         TEXT         NOT NULL DEFAULT '',
    name                  TEXT         NOT NULL,
    trigger_kind          TEXT         NOT NULL,
    trigger_config        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- Hedef: all (tüm mükellefler), group (grup; #780 B), selected (seçili
    -- mükellef dosyaları, engagement uuid'leri).
    target_kind           TEXT         NOT NULL DEFAULT 'all',
    target_group_uuid     UUID         NULL,
    target_engagements    UUID[]       NOT NULL DEFAULT '{}',
    -- "Yalnız ödemesi/eksiği olanlar".
    only_due              BOOLEAN      NOT NULL DEFAULT FALSE,
    channels              TEXT[]       NOT NULL DEFAULT '{sms}',
    -- "E-posta açılmadıysa SMS": e-posta gider, bu kadar saat sonra hâlâ
    -- açılmadıysa SMS gider.
    sms_if_unopened_hours INTEGER      NULL,
    subject               TEXT         NOT NULL DEFAULT '',
    body                  TEXT         NOT NULL,
    -- auto: otomatik gönder; draft: taslak oluştur, ofis onaylasın.
    approval_mode         TEXT         NOT NULL DEFAULT 'auto',
    active                BOOLEAN      NOT NULL DEFAULT FALSE,
    -- Kuralın açıldığı an: olay tetikleyicisi bundan eski olaylara gitmez.
    active_since          TIMESTAMPTZ  NULL,
    send_count            INTEGER      NOT NULL DEFAULT 0,
    last_run_at           TIMESTAMPTZ  NULL,
    created_by            BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by            BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ  NULL,
    CONSTRAINT uq_automation_rules_uuid UNIQUE (uuid),
    CONSTRAINT chk_automation_rules_name CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT chk_automation_rules_trigger_kind CHECK (trigger_kind IN ('calendar', 'event', 'payment')),
    CONSTRAINT chk_automation_rules_trigger CHECK (jsonb_typeof(trigger_config) = 'object'),
    CONSTRAINT chk_automation_rules_target CHECK (target_kind IN ('all', 'group', 'selected')),
    CONSTRAINT chk_automation_rules_channels CHECK (cardinality(channels) >= 1
        AND channels <@ ARRAY['sms', 'email', 'inapp']::text[]),
    CONSTRAINT chk_automation_rules_unopened CHECK (sms_if_unopened_hours IS NULL OR sms_if_unopened_hours BETWEEN 1 AND 168),
    CONSTRAINT chk_automation_rules_body CHECK (char_length(btrim(body)) BETWEEN 1 AND 2000),
    CONSTRAINT chk_automation_rules_subject CHECK (char_length(subject) <= 200),
    CONSTRAINT chk_automation_rules_approval CHECK (approval_mode IN ('auto', 'draft'))
);
CREATE INDEX idx_automation_rules_org ON automation_rules (organization_id, active) WHERE deleted_at IS NULL;
CREATE INDEX idx_automation_rules_org_all ON automation_rules (organization_id, created_at);
-- Hazır kural ofiste bir kez kurulur; silinen hazır kural geri gelmez.
CREATE UNIQUE INDEX uq_automation_rules_template ON automation_rules (organization_id, template_code)
    WHERE template_code <> '';
CREATE INDEX idx_automation_rules_created_by ON automation_rules (created_by);
CREATE INDEX idx_automation_rules_updated_by ON automation_rules (updated_by);
CREATE TRIGGER trg_automation_rules_set_updated_at
    BEFORE UPDATE ON automation_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
SELECT rls_apply_org('automation_rules');

-- Bir kuralın bir mükellefe bir çalışması. Durum: scheduled (zamanı
-- bekliyor), draft (ofis onayı bekliyor), sent (gönderildi; deneme modu
-- dahil), skipped (atlandı: ödemesi yok, kural kapalı, süre geçti...),
-- failed (hiçbir kanaldan gidemedi), rejected (ofis taslağı reddetti).
CREATE TABLE automation_runs (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rule_id             BIGINT       NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    engagement_id       BIGINT       NULL,
    run_key             TEXT         NOT NULL,
    period              TEXT         NOT NULL DEFAULT '',
    status              TEXT         NOT NULL DEFAULT 'scheduled',
    scheduled_for       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- Olay ya da takvimden gelen değişkenler (:donem, :tutar, :son_gun...).
    vars                JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- Gönderilen (ya da onaya sunulan) metin, değişkenler yerleşmiş.
    subject             TEXT         NOT NULL DEFAULT '',
    body                TEXT         NOT NULL DEFAULT '',
    channels            TEXT[]       NOT NULL DEFAULT '{}',
    -- Kanal başına sonuç ({"sms": {"status": "sent", "uuid": "..."}, ...}).
    result              JSONB        NOT NULL DEFAULT '{}'::jsonb,
    note                TEXT         NOT NULL DEFAULT '',
    source_ref          TEXT         NOT NULL DEFAULT '',
    mail_message_id     BIGINT       NULL REFERENCES mail_messages (id) ON DELETE SET NULL,
    -- "E-posta açılmadıysa SMS": kontrol zamanı ve sonucu ('' bekliyor,
    -- sent, skipped).
    sms_followup_at     TIMESTAMPTZ  NULL,
    sms_followup_status TEXT         NOT NULL DEFAULT '',
    -- "Olaydan X gün sonra hâlâ ödenmediyse tekrar gönder" kontrol zamanı.
    resend_check_at     TIMESTAMPTZ  NULL,
    decided_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at          TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at             TIMESTAMPTZ  NULL,
    CONSTRAINT uq_automation_runs_uuid UNIQUE (uuid),
    CONSTRAINT uq_automation_runs_key UNIQUE (organization_id, rule_id, run_key),
    CONSTRAINT fk_automation_runs_engagement FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_automation_runs_status CHECK (status IN ('scheduled', 'draft', 'sent', 'skipped', 'failed', 'rejected')),
    CONSTRAINT chk_automation_runs_followup CHECK (sms_followup_status IN ('', 'sent', 'skipped')),
    CONSTRAINT chk_automation_runs_vars CHECK (jsonb_typeof(vars) = 'object'),
    CONSTRAINT chk_automation_runs_result CHECK (jsonb_typeof(result) = 'object')
);
CREATE INDEX idx_automation_runs_org_created ON automation_runs (organization_id, created_at DESC);
CREATE INDEX idx_automation_runs_rule ON automation_runs (rule_id, created_at DESC);
CREATE INDEX idx_automation_runs_engagement ON automation_runs (organization_id, engagement_id);
CREATE INDEX idx_automation_runs_status ON automation_runs (organization_id, status, created_at DESC);
CREATE INDEX idx_automation_runs_due ON automation_runs (scheduled_for) WHERE status = 'scheduled';
CREATE INDEX idx_automation_runs_followup ON automation_runs (sms_followup_at)
    WHERE sms_followup_at IS NOT NULL AND sms_followup_status = '';
CREATE INDEX idx_automation_runs_resend ON automation_runs (resend_check_at) WHERE resend_check_at IS NOT NULL;
CREATE INDEX idx_automation_runs_mail ON automation_runs (mail_message_id);
CREATE INDEX idx_automation_runs_decided_by ON automation_runs (decided_by);
SELECT rls_apply_org('automation_runs');

-- Kural düzenleme geçmişi: kim, ne zaman, ne yaptı (oluşturdu, düzenledi,
-- açtı, kapattı, sildi) ve o anki kuralın görüntüsü.
CREATE TABLE automation_rule_history (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rule_id         BIGINT       NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    action          TEXT         NOT NULL,
    actor_id        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    snapshot        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_automation_rule_history_action CHECK (action IN ('created', 'updated', 'enabled', 'disabled', 'deleted'))
);
CREATE INDEX idx_automation_rule_history_org ON automation_rule_history (organization_id, rule_id, created_at DESC);
CREATE INDEX idx_automation_rule_history_rule ON automation_rule_history (rule_id);
CREATE INDEX idx_automation_rule_history_actor ON automation_rule_history (actor_id);
SELECT rls_apply_org('automation_rule_history');
