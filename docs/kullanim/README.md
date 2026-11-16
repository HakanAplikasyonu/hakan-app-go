# Kullanım kılavuzları

Uygulama içinde **Yardım** bağlantısından (`/yardim`) da okunur. Ofis panelinde
kenar çubuğunda, mükellef panelinde üst menüdedir.

| Kılavuz | Kimin için |
|---|---|
| [Ofis yöneticisi](ofis-yoneticisi.md) | Ofis sahibi ve yöneticiler: kurulum, kullanıcılar, mükellef ekleme, entegratörler, inceleme, yevmiye ve Luca / Zirve çıktısı |
| [Ofis çalışanı](ofis-calisani.md) | Muhasebe ekibi: günlük iş akışı |
| [Mizan hafızası](mizan-hafizasi.md) | Mizan yükleme, kural adayları ve sonraki aylarda otomatik kodlama |
| [Mükellef](mukellef.md) | Mükellef kullanıcıları: web paneli ve mobil uygulama |
| [Masaüstü ajanı](masaustu.md) | Ofis bilgisayarına kurulan ajan: kurulum, klasör izleme, sorun giderme |

## Bakım

- Metinler bu klasördeki `.md` dosyalarıdır. Ön yüz kopyası
  `frontend/src/features/help/content.generated.ts` üretilir:
  `node scripts/sync-kullanim.mjs` (CI `--check` ile denetler).
- Ekran görüntüleri `frontend/public/yardim/img/` altındadır ve demo
  verisinden üretilir. Yığını `scripts/e2e-up.sh` ile kurun, sonra:

  ```
  cd e2e && npx playwright test --grep @docs
  ```

  CI'daki e2e işi aynı komutu koşar ve görüntüleri `kilavuz-goruntuleri`
  artefaktı olarak yükler.
- KVKK ve hukuk metinleri burada değil, uygulamadaki `/legal/...`
  sayfalarındadır; kılavuzlar onlara yalnızca bağlantı verir.
