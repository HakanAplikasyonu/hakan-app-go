# Yedekleme ve geri yükleme (#178)

Üretimde `backup` servisi (`compose.prod.yml`) Postgres veritabanını ve nesne
deposunu (SeaweedFS) zamanlanmış olarak yedekler. Yedekler
[age](https://age-encryption.org) ile şifrelenir. Sunucuda yalnızca **açık
anahtar** durur; yedeği açan özel anahtar sunucunun dışında saklanır. Sunucu
ele geçirilse bile eski yedekler okunamaz.

Kod: `deploy/backup/` (imaj ve betikler), tatbikat: `scripts/backup-drill.sh`,
`deploy/backup/drill.compose.yml`, CI işi `backup-drill`.

## Ne yedeklenir

Her çalıştırmada aynı zaman damgasıyla şu dosyalar yazılır:

| Dosya | İçerik |
|---|---|
| `db/db-<zaman>.dump.age` | `pg_dump --format=custom`, şifreli |
| `db/db-<zaman>.roles.sql.age` | `pg_dumpall --roles-only` (RLS rolleri, yetkiler), şifreli; parola özetleri içerdiği için şifrelenir |
| `db/db-<zaman>.manifest.json` | Şema sürümü ve her tablonun satır sayısı (düz metin, içerik yok) |
| `objects/objects-<zaman>.tar.age` | Uygulama bucket'ının (`S3_BUCKET`) tam kopyası, tar, şifreli |
| `objects/objects-<zaman>.manifest.json` | Nesne sayısı |

Redis ve Meilisearch yedeklenmez: Redis kuyruk/önbellek, Meilisearch arama
dizini veritabanından yeniden kurulur (`search-reindex`).

## Kurulum

1. Sunucu dışında (kendi bilgisayarında) bir anahtar çifti üret:

   ```bash
   age-keygen -o hakan-backup-identity.txt
   ```

   Çıktıdaki `Public key: age1…` satırı açık anahtardır. `hakan-backup-identity.txt`
   özel anahtardır; parola yöneticisinde ya da çevrimdışı bir yerde sakla, en
   az iki kişide bulunsun. Sunucuya koyma.
2. `.env.server`'a açık anahtarı yaz (birden fazla kişi için virgülle ayır):
   `BACKUP_AGE_RECIPIENTS=age1…,age1…`
3. Şemayı güncelle: `scripts/prod-env.sh upgrade` (`BACKUP_IMAGE` eklenir). Ardından deploy.

Açık anahtar tanımlı değilse servis hiçbir şey yazmaz, sağlık kontrolü
`unhealthy` olur ve logda sebebi yazar. Deploy'u engellemez.

## Ayarlar

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `BACKUP_AGE_RECIPIENTS` | boş | age açık anahtarları (zorunlu) |
| `BACKUP_SCHEDULE` | `17 2 * * *` | cron ifadesi, saat dilimi Europe/Istanbul |
| `BACKUP_RETENTION_DAYS` | `14` | Sunucudaki (`<NAME_PREFIX>-prod-backups` volume) saklama süresi |
| `BACKUP_OBJECTS` | `true` | Nesne deposu da yedeklensin mi |
| `BACKUP_MAX_AGE_HOURS` | `26` | Son başarılı yedek bundan eskiyse servis `unhealthy` olur |
| `BACKUP_OFFSITE_S3_BUCKET` ve diğer `BACKUP_OFFSITE_S3_*` | boş | Başka bir S3 uyumlu depoya kopya. Bucket boşsa kapalı. |
| `BACKUP_OFFSITE_RETENTION_DAYS` | `BACKUP_RETENTION_DAYS` | Uzak kopyanın saklama süresi |

Yedekler aynı sunucudaki bir volume'da durduğu için, sunucu kaybına karşı
koruma ancak uzak kopya açıksa sağlanır (bkz. Karar gerekiyor).

Elle yedek almak için:

```bash
docker compose -f compose.prod.yml exec backup backup-run
```

Durumu görmek için `docker inspect --format '{{.State.Health.Status}}' <NAME_PREFIX>-backup`
ve `docker logs <NAME_PREFIX>-backup` kullanılır.

## Geri yükleme

Felaket senaryosu: yeni sunucu, boş volume'lar.

1. Yığını yeni sunucuda, eski `.env.server` ile kur ve yalnızca bağımlılıkları
   başlat: `docker compose -f compose.prod.yml up -d postgres seaweedfs seaweedfs-init backup`.
   Backend/worker/migrate'i **başlatma** (migration boş veritabanında şema kurar,
   geri yükleme boş veritabanı ister).
2. Yedek dosyalarını ve özel anahtarı backup konteynerine kopyala
   (anahtarı işten sonra sil):

   ```bash
   docker cp hakan-backup-identity.txt <NAME_PREFIX>-backup:/tmp/identity.txt
   ```

   ```bash
   docker cp ./yedek/. <NAME_PREFIX>-backup:/backups/restore/
   ```

3. Veritabanı (önce roller, sonra döküm; ardından manifestle doğrular):

   ```bash
   docker compose -f compose.prod.yml exec backup restore-db --identity /tmp/identity.txt /backups/restore/db-<zaman>.dump.age
   ```

4. Nesneler:

   ```bash
   docker compose -f compose.prod.yml exec backup restore-objects --identity /tmp/identity.txt /backups/restore/objects-<zaman>.tar.age
   ```

5. Anahtarı sil (`docker compose … exec backup rm /tmp/identity.txt`), ardından
   tüm yığını başlat (`up -d`). `migrate` yalnızca yedekten sonraki migration'ları
   uygular. Arama dizini için `search-reindex` çalıştır.

`restore-db` tabloları olan bir veritabanını reddeder. Mevcut bir veritabanının
üzerine yazmak için `--clean` gerekir; bunu yalnızca ne yaptığından eminsen kullan.

## Veritabanı rolleri ve yedek rolü (#312)

`scripts/db-roles.sql` rolleri bir kez, bir süper kullanıcıyla kurar; tekrar çalıştırmak güvenlidir:

```bash
psql "$ADMIN_URL" -v ON_ERROR_STOP=1 -v owner_password="$DB_OWNER_PASSWORD" -v backup_password="$DB_BACKUP_PASSWORD" -f scripts/db-roles.sql
```

| Rol | Ne için | Özellik |
|---|---|---|
| `hakan_owner` | tablo sahibi, migration ve uygulama bağlantısı (`DB_USER`) | süper kullanıcı değil, `BYPASSRLS` yok; FORCE RLS ona da uygulanır, tablolara yalnızca `rls.System` (`app.system`) içinden erişir |
| `hakan_rls_runtime`, `hakan_rls_app`, `hakan_rls_worker` | uygulamanın geçtiği roller | `NOLOGIN`; sahip `ADMIN`, `SET` ile ve `INHERIT FALSE` ile üye (miras alsaydı `rls_legacy` sahibe de uygulanır, FORCE etkisiz kalırdı) |
| `hakan_backup` | `pg_dump` | yalnızca okur (`pg_read_all_data`), RLS'yi atlar (`BYPASSRLS`), yazamaz |

Yedek servisi `DB_BACKUP_USER` / `DB_BACKUP_PASSWORD` doluysa bu rolle bağlanır, boşsa `DB_USER` ile. Sahip süper kullanıcı değilken `pg_dump` sahiple çalışmaz: FORCE'lu tablolarda `query would be affected by row-level security policy` hatası verir. Bu yüzden yedek rolü gerekir. Süper kullanıcı olmayan rol parola özetlerini okuyamaz. Bu durumda roller parolasız yedeklenir (`--no-role-passwords`); geri yüklemeden sonra parolalar `db-roles.sql` ile yeniden verilir.

### Geri yükleme kuru koşusu (süper kullanıcı olmayan sahip)

#312'de yerel Postgres 16 üzerinde denendi:

- **Kaynak:** `hakan_owner` sahipli bir veritabanı; migration'lar bu rolle uygulandı, üzerine demo ofis verisi kuruldu.
- **Yedek:**
  1. `pg_dump --format=custom` `hakan_owner` ile çalıştırıldı: RLS hatasıyla durdu (beklenen).
  2. Aynı komut `hakan_backup` ile çalıştırıldı: başarılı.
  3. `pg_dumpall --roles-only --no-role-passwords` `hakan_backup` ile çalıştırıldı: başarılı.
  4. `hakan_backup` ile `UPDATE` denendi: `permission denied` (rol yazamıyor).
- **Geri yükleme:**
  1. Boş bir veritabanına `pg_restore --exit-on-error --single-transaction`, hem süper kullanıcıyla hem `hakan_owner` ile ayrı ayrı denendi; ikisi de başarılı. `pg_dump`, RLS'yi açan ve FORCE'u koyan komutları ve politikaları veriden sonra yazar.
  2. 109 tablonun satır sayıları kaynakla aynıydı. Şema sürümü 89, 71 tablo FORCE'lu ve sahip `hakan_owner`.

Üretimi bu rollere geçirme adımları: `docs/runbook/db-sahip-gecisi.md` (#317).

CI tatbikatı (`scripts/backup-drill.sh`) yedeği `hakan_backup` ile alır, geri yüklemeyi hedef kümenin yöneticisiyle yapar.

## Tatbikat (CI)

`backup-drill` işi altyapı değişikliklerinde (`deploy/**`, migration'lar,
Dockerfile'lar) ve haftalık tam çalıştırmada koşar. Adımlar:

1. Boş bir Postgres'e tüm migration'lar uygulanır, bir işaret satırı ve üç nesne eklenir.
2. `backup-run` çalışır: şifreli döküm, roller, manifest ve şifreli bucket arşivi.
3. Yedek dosyalarında işaret metninin düz hali aranır; bulunursa iş düşer.
4. Döküm **ikinci, bomboş bir Postgres kümesine** (yeni sunucu gibi, roller yok)
   geri yüklenir. Şema sürümü ve her tablonun satır sayısı manifestle
   karşılaştırılır. İşaret satırı kontrol edilir. Dolu veritabanına ikinci
   yüklemenin reddedildiği doğrulanır.
5. Nesneler yeni bir bucket'a geri yüklenir ve `rclone check` ile içerikleri karşılaştırılır.

Yerelde:

```bash
docker build -t local/hakan-backup:main deploy/backup
```

```bash
scripts/backup-drill.sh
```

## Karar gerekiyor

Aşağıdakiler **öneridir**; ürün sahibi onaylamalı.

- **RPO (en çok ne kadar veri kaybı kabul edilir): 24 saat.** Günlük gece yedeği
  bunu sağlar. Daha sıkı bir hedef (ör. 1 saat) gerekirse ya `BACKUP_SCHEDULE`
  saatliğe çekilir ya da sürekli WAL arşivleme (pgBackRest / WAL-G) kurulur.
  Muhasebe verisinin gün içinde kaynaklardan yeniden çekilebildiği düşünülürse
  24 saat makul bir başlangıç.
- **RTO (geri dönüş süresi): 4 saat.** Yeni sunucu, imaj çekme, geri yükleme ve
  doğrulama. Veritabanı büyüdükçe geri yükleme süresi tatbikatla ölçülmeli.
- **Uzak kopya zorunlu olmalı mı?** Şu an isteğe bağlı. Öneri: canlıya
  çıkmadan önce farklı bir sağlayıcıda (ya da farklı bölgede) S3 bucket açılsın
  ve `BACKUP_OFFSITE_*` doldurulsun. Bu yapılmazsa sunucu kaybında yedek de
  kaybolur.
- **Saklama süresi:** sunucuda 14 gün öneriliyor; uzak kopyada 35 gün, ve
  ayda bir alınan bir yedeğin daha uzun (ör. 1 yıl) saklanması öneriliyor.
  Yasal saklama yükümlülükleri (VUK, KVKK imha politikası) hukukla netleşmeli.
- **Nesne yedeği tam kopya:** her gün bucket'ın tamamı arşivleniyor. Depo
  büyüyünce artımlı yönteme (`rclone sync` ile şifreli uzak depo) geçilmeli.
- **Özel anahtarın kimde duracağı:** en az iki kişi, çevrimdışı. Anahtar
  kaybolursa yedekler açılamaz.
