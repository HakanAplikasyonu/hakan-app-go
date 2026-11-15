# desktop/

HAKAN masaüstü uygulaması (D5, docs/01 §4.2): **Tauri v2** kabuğu ve yanında
çalışan **Go sidecar**.

```
desktop/
├── sidecar/          Go sidecar (ayrı Go modülü; backend modülünü replace ile kullanır)
│   ├── rpc/          stdin/stdout satır tabanlı JSON protokolü
│   └── methods/      ping, version, ubl.inspect (go-ubltr), api.health (backend zarfı)
├── src-tauri/        İnce Rust kabuk: pencere, sidecar yaşam döngüsü, OS anahtar zinciri, güncelleyici
│   ├── src/bridge.rs protokolün Rust tarafı (Tauri'den bağımsız, test edilebilir)
│   ├── src/secrets.rs Keychain / Credential Manager / Secret Service
│   └── tests/        kabuk ↔ gerçek Go sidecar uçtan uca testi
├── ui/               Statik arayüz (derleme adımı yok)
└── scripts/build-sidecar.mjs   sidecar'ı binaries/hakan-sidecar-<target-triple> olarak derler
```

## Mimari

- İş mantığı Go sidecar'da. Rust kabuk sidecar'ı başlatır, yanıtları id ile
  eşler, olayları (`ready`) ön yüze `sidecar://event` olarak iletir.
- Ön yüz sidecar'a doğrudan erişemez: `sidecar_call` yalnızca izin
  listesindeki yöntemleri (`ALLOWED_METHODS`, `src/lib.rs`) iletir. Kabuk
  (shell) ve dosya izinleri ön yüze açılmadı.
- Sırlar (API anahtarı vb.) OS anahtar zincirinde, servis adı
  `app.hakan.desktop`. Diske düz yazılmaz.
- Sidecar backend ile aynı Go kodunu kullanır: go-ubltr (UBL okuma) ve
  `backend/pkg/response` (API zarfı). `backend/internal/...` Go kuralı
  gereği başka modülden içe aktarılamaz; paylaşılacak kod `backend/pkg/`
  altına taşınmalı.
- Backend'de yalnızca mevcut API'ler kullanılır (`/healthz`,
  `/v1/app/config`; yükleme uçları #51'de).

## Protokol

Her satır bir JSON nesnesi:

```
→ {"id":1,"method":"ping","params":{"echo":"x"}}
← {"id":1,"result":{"pong":true,"echo":"x","time":"…"}}
← {"id":2,"error":{"code":"NOT_UBL","message":"…"}}
← {"event":"ready","data":{"version":"0.1.0",…}}
```

stdout yalnızca protokol içindir; sidecar günlükleri stderr'e yazar.

## Geliştirme

Gerekenler: Go (backend ile aynı sürüm), Rust (stable), Node 22. Linux'ta
ek olarak `libwebkit2gtk-4.1-dev libayatana-appindicator3-dev librsvg2-dev patchelf libdbus-1-dev`.

```bash
cd desktop
npm ci
npm run dev          # sidecar'ı derler ve tauri dev
```

Testler:

```bash
(cd sidecar && go test ./...)
npm run sidecar && (cd src-tauri && cargo test)
# Gerçek anahtar zinciriyle (masaüstü oturumu gerekir):
(cd src-tauri && cargo test --lib -- --ignored keychain)
```

Duman testi: `hakan-desktop --self-test` uygulamayı açar, sidecar'a
`version` çağrısı yapar, sonucu tek satır JSON yazar ve çıkar (0 / 1).

## Sürüm ve imzalama

`.github/workflows/desktop-release.yml` (elle ya da `desktop-v*` etiketiyle):

- macOS universal: `APPLE_CERTIFICATE`, `APPLE_CERTIFICATE_PASSWORD`,
  `APPLE_SIGNING_IDENTITY` ile imza; `APPLE_ID`, `APPLE_PASSWORD`,
  `APPLE_TEAM_ID` ile notarization. Tauri sidecar ikilisini de imzalar.
- Windows x64: `WINDOWS_CERTIFICATE` (pfx, base64) ve
  `WINDOWS_CERTIFICATE_PASSWORD`; parmak izi derlemede yapılandırmaya verilir.
- Güncelleyici: `TAURI_SIGNING_PRIVATE_KEY` (+ parola) sırrı ve
  `TAURI_UPDATER_PUBKEY` değişkeni varsa `tauri.release.conf.json` ile
  güncelleme paketleri üretilir. Anahtar yoksa güncelleyici derlemede kapalıdır.

Sırlar yoksa iş düşmez; imzasız paket üretir ve özetine yazar. Anahtarlar koda
ya da depoya girmez.
