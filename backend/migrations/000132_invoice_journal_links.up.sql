-- Faturadan üretilen muhasebe fişi (#669, Oğuz kararı 20.11: faturaların
-- fişini uygulama üretir, diğer fişlerle Luca'ya aktarır).
--
-- Bir iş (engagement) bir faturadan en fazla bir fiş üretir: istek
-- tekrarlanırsa mevcut fiş döner. Fiş silinirse bağ da gider (fiş yeniden
-- üretilebilir). Fatura silinmez (kaynak belge), bağ fatura ile kalır.
CREATE TABLE invoice_journal_links (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    invoice_id       BIGINT       NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    journal_draft_id BIGINT       NOT NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invoice_journal_links_invoice UNIQUE (engagement_id, invoice_id),
    CONSTRAINT uq_invoice_journal_links_journal UNIQUE (engagement_id, journal_draft_id),
    CONSTRAINT fk_invoice_journal_links_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_journal_links_journal
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE CASCADE
);

SELECT rls_apply_engagement('invoice_journal_links'::regclass);
