# hakan-app-go

Mali müşavirlik ofisleri ve mükellefleri için veri toplama, kontrol, inceleme ve muhasebe programına aktarım platformu. "HAKAN" geçici geliştirme adıdır.

Durum: **planlama.** Kod henüz yok.

## Belgeler

| Belge | İçerik |
|---|---|
| [docs/00-degerlendirme-ve-kararlar.md](docs/00-degerlendirme-ve-kararlar.md) | Entegrasyon paketinin eleştirel değerlendirmesi, ilk kararlar |
| [docs/01-cekirdek-hiyerarsi-ve-acik-kararlar.md](docs/01-cekirdek-hiyerarsi-ve-acik-kararlar.md) | Çekirdek (boilerplate, go-ubltr), hiyerarşi modeli, Basis ve rakip analizi, **güncel kararlar (§0)** |
| [docs/02-muhasebe-cikti-surucu-modeli.md](docs/02-muhasebe-cikti-surucu-modeli.md) | Luca ve formüllü Excel çıktısı için sürücü modeli |
| [docs/brand/](docs/brand/) | Logo |
| [docs/kaynak/](docs/kaynak/) | Kaynak talepler, görüşme transkripti ve entegrasyon paketi |

## Yığın

- Backend: Go, [`nextjs-go-boilerplate`](https://github.com/furkanmeclis/nextjs-go-boilerplate) tenant çekirdeği
- UBL-TR: [`go-ubltr`](https://github.com/furkanmeclis/go-ubltr)
- Web: Next.js (boilerplate)
- Mobil: Expo
- Masaüstü: Tauri v2 + Go sidecar

İş takibi: GitHub Projects (organizasyon projesi).
