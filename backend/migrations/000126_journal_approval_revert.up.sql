-- Onayı geri alınan fiş (#566): onaylanmış ama henüz programa aktarılmamış
-- fiş taslağa döner; kimin, ne zaman ve neden geri aldığı fişte durur.
-- Yeniden onaylanınca bu üç alan son geri almanın izi olarak kalır.
ALTER TABLE journal_drafts
    ADD COLUMN approval_reverted_by     BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN approval_reverted_at     TIMESTAMPTZ NULL,
    ADD COLUMN approval_revert_reason   TEXT        NULL;
