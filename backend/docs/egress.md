# Sabit çıkış IP'si (fixed-egress)

Issue #45, D4. İşNet gibi entegratörler yalnızca allowlist'teki IP'lerden gelen
çağrıları kabul eder. Yanlış IP'den gelen çağrıda go-ubltr / entegratör
`ip-not-allowlisted` benzeri bir hata döner (`egress.IsIPNotAllowlisted`).

## Nasıl çalışır

| Parça | Ne yapar |
|---|---|
| `fixed-egress` kuyruğu | Entegratör senkronu (`queue.EnqueueEntegratorSync`) ve IP denetimi bu kuyrukta. |
| `WORKER_POOL` | `all` (varsayılan, tek sunucu ve geliştirme), `general` (fixed-egress hariç her şey), `fixed-egress` (yalnızca o kuyruk). API'nin süreç içi worker'ı fixed-egress'i hiç almaz. |
| `worker-egress` servisi | `compose.prod.yml`, `fixed-egress` profili. Zamanlayıcı çalıştırmaz; periyodik işleri genel worker planlar. |
| `EGRESS_PROXY_URL` | Doluysa fixed-egress çağrıları bu ileri vekilden çıkar (http/https/socks5). Ortamdaki `HTTP(S)_PROXY` hiçbir zaman kullanılmaz. |
| `EGRESS_EXPECTED_IPS` | Allowlist'teki IP'ler (virgülle). |
| IP denetimi | Her `EGRESS_CHECK_INTERVAL`'de (varsayılan 15 dk) `EGRESS_IP_CHECK_URL`'ye entegratör çağrılarıyla **aynı istemciyle** gider. IP beklenenlerden biri değilse görev düşer: hata günlüğü ve Sentry (alarm). Son sonuç Redis'te, `GET /v1/platform/egress` gösterir; iki aralık boyunca denetim yoksa `stale: true`. |
| TCMB döviz kurları (#780) | Worker'ın `app:exchange_rates:fetch` işi `www.tcmb.gov.tr/kurlar` adresine aynı egress istemcisiyle gider; üretimde `EGRESS_PROXY_HOSTS` varsayılanında `www.tcmb.gov.tr` var, vekil izin verir. Ağ yoksa iş uyarı günlüğü bırakır, uygulama çalışmaya devam eder. |
| `entegrator.Config.HTTP` | Entegratör istemcisinin taşıyıcısı; fixed-egress worker'ında egress istemcisi verilir. Adaptör dosyalarına dokunulmadı. |

## Üretimde açmak

1. Çıkış IP'sini belirleyin: ya `worker-egress` servisini statik IP'li bir
   Dokploy sunucusunda/node'unda çalıştırın, ya da statik IP'li bir ileri
   vekil kurup `EGRESS_PROXY_URL`'yi ona yöneltin.
2. `EGRESS_EXPECTED_IPS` = o IP; aynı IP'yi entegratörün allowlist'ine verin.
3. `COMPOSE_PROFILES=fixed-egress` ve ana `worker` için `WORKER_POOL=general`.
4. `GET /v1/platform/egress` ile `last.ok: true` ve `stale: false` görün.

Profil açılmazsa her şey eskisi gibi çalışır: ana worker (`WORKER_POOL=all`)
fixed-egress kuyruğunu da alır ve çağrılar onun IP'sinden çıkar.
