# Veritabanı sahibi geçişi: `app` → `hakan_owner` (#317)

Üretim veritabanı bugün süper kullanıcı `app` (compose'taki `POSTGRES_USER`) ile kurulu ve uygulama bu rolle bağlanıyor. Süper kullanıcı FORCE ROW LEVEL SECURITY'yi atlar; #306, #308 ve #314 ile gelen satır güvenliği sahip rolüne ancak sahip süper kullanıcı değilse uygulanır. Bu runbook sahibi `hakan_owner`'a (`scripts/db-roles.sql`) geçirir.

Ne değişir:
- Veritabanının, `public` şemasının ve içindeki tablo, dizi ve fonksiyonların sahibi `hakan_owner` olur.
- `rls_system` politikaları yeni sahibe yazılır (`TO CURRENT_USER` ile `app` için oluşturulmuşlardı).
- Varsayılan yetkiler `hakan_owner` için yeniden tanımlanır.
- Uygulama, worker ve migrate `hakan_owner` ile bağlanır.
- Yedek `hakan_backup` ile alınır (yalnızca okur, RLS'yi atlar).
- `app` süper kullanıcı olarak kalır, yalnızca yönetim için kullanılır.

Betikler:
- `scripts/db-owner-migrate.sh`: geçiş.
- `scripts/db-owner-rollback.sh`: geri dönüş.
- `scripts/db-owner.sql`: ikisinin ortak çekirdeği.

Her biri tek işlemde çalışır, sonunda doğrular ve bir şey eksikse hepsini geri alır. Tekrar çalıştırmak güvenlidir. `--dry-run` aynı işlemi yapar, doğrular, her adımı listeler ve geri alır. CI provası: `.github/workflows/db-owner-rehearsal.yml`.

## 1. Ön kontroller

```bash
docker compose -f compose.prod.yml exec postgres psql -U app -d app -XtAc "SELECT version()"
```

- Postgres 15 veya üstü olmalı (`GRANT SET ON PARAMETER`; yönetilen Postgres'te bkz. §7).
- Migration'lar güncel ve temiz olmalı:

```bash
docker compose -f compose.prod.yml exec postgres psql -U app -d app -XtAc "SELECT version, dirty FROM schema_migrations"
```

- `dirty = f` olmalı ve sürüm `backend/migrations` içindeki son numara olmalı.
- Bağlı rolün süper kullanıcı olduğu doğrulanmalı (`rolsuper = t`):

```bash
docker compose -f compose.prod.yml exec postgres psql -U app -d app -XtAc "SELECT rolsuper FROM pg_roles WHERE rolname = current_user"
```

- Kuru koşu çalıştırılmalı; hiçbir şey değiştirmez. Çıktıda `ALTER ... OWNER TO hakan_owner` satırları, `db-owner: N object(s) moved` ve `db-owner: verified` görünmeli:

```bash
ADMIN_URL="postgres://app:$DB_PASSWORD@<postgres-host>:5432/app" scripts/db-owner-migrate.sh --dry-run
```

Betikler sunucuda `psql` ister. Sunucuda yoksa postgres konteynerinden çalıştırın: `scripts/` dizinini kopyalayın, `ADMIN_URL` olarak `postgres://app:...@127.0.0.1:5432/app` verin.

## 2. Yedek

Geçişten hemen önce elle bir yedek alın (bugünkü rolle; `app` süper kullanıcı olduğu için RLS engel değil):

```bash
docker compose -f compose.prod.yml exec backup backup-run
```

Yedeğin ve manifestin oluştuğunu doğrulayın (`docs/yedekleme.md`).

## 3. Bakım penceresi

Uygulamanın yazmasını durdurun; geçiş tek işlemdir ve birkaç saniye sürer:

```bash
docker compose -f compose.prod.yml stop backend worker
```

## 4. Geçiş

Parolalar yalnızca ortamdan okunur; komut satırına ve kabuk geçmişine düşmez:

```bash
read -rs DB_OWNER_PASSWORD; read -rs DB_BACKUP_PASSWORD; export DB_OWNER_PASSWORD DB_BACKUP_PASSWORD
ADMIN_URL="postgres://app:$DB_PASSWORD@<postgres-host>:5432/app" scripts/db-owner-migrate.sh
```

Çıktının son satırları `db-owner: verified (owner hakan_owner, N forced tables with rls_system)` ve `COMMIT` olmalı.

## 5. Uygulama ortam değişkenleri (`.env.server` / Dokploy)

| Değişken | Önce | Sonra |
|---|---|---|
| `DB_ADMIN_USER` | (boş) | `app` |
| `DB_ADMIN_PASSWORD` | (boş) | eski `DB_PASSWORD` |
| `DB_USER` | `app` | `hakan_owner` |
| `DB_PASSWORD` | eski parola | `DB_OWNER_PASSWORD` |
| `DB_BACKUP_USER` | (boş) | `hakan_backup` |
| `DB_BACKUP_PASSWORD` | (boş) | `DB_BACKUP_PASSWORD` |

- `DB_ADMIN_*` postgres konteynerinin süper kullanıcısıdır (`POSTGRES_USER`, healthcheck). Konteyner ilk açılışta oluşturduğu kullanıcıyı korur; bu değerler o kullanıcıyla aynı olmalı.
- `DB_RLS_RUNTIME` açık kalmalı (varsayılan). Kapatılırsa uygulama sahip rolüyle çalışır ve FORCE nedeniyle tablolar boş görünür.

```bash
docker compose -f compose.prod.yml up -d
```

## 6. Doğrulama

```sql
-- psql -U app -d app
SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database();          -- hakan_owner
SELECT count(*) FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
 WHERE s.nspname = 'public' AND pg_get_userbyid(c.relowner) = 'app';                         -- 0
SELECT count(*) FROM pg_policies WHERE policyname = 'rls_system' AND 'app' = ANY (roles);    -- 0
SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'hakan_owner';                   -- f | f
SELECT pg_has_role('hakan_owner', 'hakan_rls_runtime', 'USAGE');                             -- f
```

Sahip `app.system` olmadan satır görmemeli, `app.system` açıkken görmeli:

```sql
-- psql -U hakan_owner -d app
SELECT count(*) FROM engagements;                                                            -- 0
BEGIN; SELECT set_config('app.system', 'on', true); SELECT count(*) FROM engagements; ROLLBACK; -- > 0
```

Ardından:
- `backend` ve `worker` sağlık uçları (`/readyz`) yanıt vermeli.
- Ofis panelinde giriş yapılıp bir mükellef açılabilmeli.
- Mükellef panelinden bir belge yüklenebilmeli.
- Yedek yeni rolle alınabilmeli:

```bash
docker compose -f compose.prod.yml exec backup backup-run
```

## 7. Yönetilen Postgres

Bazı sağlayıcılar yönetici rolüne şunları vermez:
- `BYPASSRLS`: yedek rolü için.
- `GRANT SET ON PARAMETER`: migration 000089 için.

**000089 düşüyorsa** (`permission denied to set parameter "app.system"`), aynı işi fonksiyon gövdesinde yapan plpgsql sürümünü kullanın. Ayar fonksiyonun başında açılır ve dönmeden önce (hata olsa da) geri alınır; bunun için yetki gerekmez:

```bash
migrate ... force 88                         # 000089 yarıda kaldıysa
psql "$OWNER_URL" -v ON_ERROR_STOP=1 -f scripts/managed-postgres/definer-functions-plpgsql.sql
migrate ... force 89                         # 000089'u uygulanmış say
migrate ... up
```

Davranış aynıdır (`TestDefinerFunctionsPlpgsql`). `GRANT SET ON PARAMETER` hatası `db-roles.sql`'de alınırsa o satırı atlayın.

**`BYPASSRLS` yoksa** `hakan_backup` FORCE'lu tabloları okuyamaz. Yedek için sağlayıcının anlık görüntüsünü (snapshot) kullanın ya da yedeği yönetici rolüyle alın.

## 8. Geri dönüş

Uygulama yeni sahiple çalışmıyorsa:

1. `docker compose -f compose.prod.yml stop backend worker`
2. Geri dönüşü önce kuru koşuyla, sonra gerçek olarak çalıştırın. Sahiplikler ve `rls_system` politikaları `app`'e döner, roller silinmez:

```bash
ADMIN_URL="postgres://app:$DB_ADMIN_PASSWORD@<postgres-host>:5432/app" scripts/db-owner-rollback.sh --dry-run
ADMIN_URL="postgres://app:$DB_ADMIN_PASSWORD@<postgres-host>:5432/app" scripts/db-owner-rollback.sh
```

3. Ortam değişkenlerini §5'teki "Önce" sütununa döndürün: `DB_USER=app`, `DB_PASSWORD` eski parola; `DB_ADMIN_*` ve `DB_BACKUP_*` boş.
4. `docker compose -f compose.prod.yml up -d` ve §6'daki sağlık kontrolleri.

Geri dönüşten sonra yeniden geçiş aynı adımlarla yapılabilir. CI provası geçiş, geri dönüş ve yeniden geçişi sırayla dener.
