/**
 * Belgenin yetkili indirilmesi ve önbellek (#780). Dosya uygulamanın kendi
 * önbellek klasörüne (`<cache>/belgeler`) iner; işletim sistemi yer
 * daraldığında bu klasörü boşaltabilir, biz de bir günden eski ya da toplamı
 * 100 MB'ı aşan belgeleri siliriz. Başka hiçbir klasöre dokunulmaz.
 */
import { fetch } from 'expo/fetch';
import { Directory, File, Paths } from 'expo-file-system';

import { api } from '@/core/api/client';
import { ApiError, unwrap } from '@/core/api/envelope';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';

import { belgeMime, cokBuyuk, cokBuyukHatasi, gecerliBelgeYolu, guvenliDosyaAdi, onbellekAdi, silinecekler, type BelgeKaynagi } from './belge';

const KLASOR = 'belgeler';
const BIR_GUN = 24 * 60 * 60 * 1000;
const AZAMI_ONBELLEK = 100 * 1024 * 1024;
/** Büyük PDF'ler yavaş bağlantıda uzun sürer; JSON isteklerinden uzun. */
const INDIRME_SURESI = 90_000;

export type IndirilenBelge = { file: File; bytes: Uint8Array; contentType: string };

function klasor(): Directory {
  const d = new Directory(Paths.cache, KLASOR);
  if (!d.exists) d.create({ intermediates: true, idempotent: true });
  return d;
}

/** Erişim belirteci bitmek üzereyse (ya da 401 geldiyse) ortak istemci yeniler. */
async function belirteciTazele(taxpayer: string, zorla: boolean): Promise<void> {
  const s = session.get();
  if (!s) return;
  if (!zorla && s.accessExpiresAt - Date.now() > 60_000) return;
  // Hafif bir istek: istemci süresi dolan belirteci yeniler (401'de bir kez daha dener).
  await api.get('/v1/taxpayer/notifications/unread-count', { taxpayer }).catch(() => undefined);
}

async function getir(url: string, taxpayer: string) {
  const token = session.get()?.accessToken;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), INDIRME_SURESI);
  try {
    return await fetch(url, {
      headers: { 'X-Taxpayer': taxpayer, Accept: '*/*', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      signal: controller.signal,
    });
  } catch {
    throw new ApiError({ status: 0, code: controller.signal.aborted ? 'TIMEOUT' : 'NETWORK_ERROR', message: 'Network request failed' });
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Belgeyi indirir ve önbelleğe yazar. Hata API zarfıyla aynı biçimde
 * `ApiError` olur (durum, sunucunun kodu); ekran `belgeHatasi` ile Türkçe
 * metne çevirir.
 */
export async function belgeyiIndir(src: BelgeKaynagi & { path: string }, id: string, prefix: string, taxpayer: string): Promise<IndirilenBelge> {
  if (!gecerliBelgeYolu(src.path)) throw new ApiError({ status: 400, code: 'INVALID_DOCUMENT_PATH', message: 'Belge bağlantısı geçersiz.' });
  if (cokBuyuk(src.size)) throw cokBuyukHatasi(src.size);
  const url = `${env.apiUrl.replace(/\/$/, '')}${src.path}`;

  await belirteciTazele(taxpayer, false);
  let res = await getir(url, taxpayer);
  if (res.status === 401) {
    await belirteciTazele(taxpayer, true);
    res = await getir(url, taxpayer);
  }
  const requestId = res.headers.get('X-Request-ID') ?? undefined;
  if (!res.ok) {
    const payload: unknown = await res.json().catch(() => undefined);
    // Hata zarfı (ya da gövdesiz yanıt) ortak istemcideki gibi ApiError olur.
    unwrap(res.status, payload, requestId);
    throw new ApiError({ status: res.status, code: `HTTP_${res.status}`, message: `Request failed (${res.status})`, requestId });
  }
  const length = Number(res.headers.get('Content-Length') ?? '');
  if (cokBuyuk(length)) throw cokBuyukHatasi(length);

  let bytes: Uint8Array;
  try {
    bytes = new Uint8Array(await res.arrayBuffer());
  } catch {
    throw new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Download interrupted', requestId });
  }
  if (cokBuyuk(bytes.byteLength)) throw cokBuyukHatasi(bytes.byteLength);

  const contentType = belgeMime(res.headers.get('Content-Type') ?? src.contentType, src.fileName) || 'application/octet-stream';
  const file = new File(klasor(), onbellekAdi(prefix, id, src.fileName, contentType));
  try {
    if (file.exists) file.delete();
    file.create();
    file.write(bytes);
  } catch {
    throw new ApiError({ status: 0, code: 'STORAGE_FULL', message: 'Telefonda belgeyi kaydedecek yer yok.', requestId });
  }
  return { file, bytes, contentType };
}

/** Telefondaki dosyayı (kuyruktaki fiş fotoğrafı) okur; indirme yok. */
export async function yerelDosyayiOku(uri: string, contentType: string | null | undefined, fileName?: string | null): Promise<IndirilenBelge> {
  const file = new File(uri);
  if (!file.exists) throw new ApiError({ status: 404, code: 'NOT_FOUND', message: 'Dosya telefonda yok.' });
  if (cokBuyuk(file.size)) throw cokBuyukHatasi(file.size);
  const bytes = await file.bytes();
  return { file, bytes, contentType: belgeMime(contentType, fileName ?? file.name) || 'application/octet-stream' };
}

/**
 * Paylaşımda görünen adla önbellekte bir kopya ("bordro-<uuid>.pdf" yerine
 * "Ekim 2026 bordro.pdf"). Kopyalanamazsa dosyanın kendisi paylaşılır.
 */
export function paylasimKopyasi(file: File, ad: string): File {
  const safe = guvenliDosyaAdi(ad);
  if (!safe || safe === file.name) return file;
  try {
    const hedef = new File(klasor(), safe);
    if (hedef.exists) hedef.delete();
    file.copy(hedef);
    return hedef;
  } catch {
    return file;
  }
}

/** Görüntüleyici kapanınca indirilen kopya silinir (yalnızca önbellek klasöründekiler). */
export function onbellektenSil(file: File | null | undefined): void {
  if (!file) return;
  try {
    if (file.uri.startsWith(klasor().uri) && file.exists) file.delete();
  } catch {
    // zaten silinmiş
  }
}

/** Bir günden eski ve 100 MB'ı aşan önbellek belgelerini siler; hata sessizce yutulur. */
export function onbellegiTemizle(keep?: string): void {
  try {
    const files = klasor()
      .list()
      .filter((e): e is File => e instanceof File);
    const names = silinecekler(
      files.map((f) => ({ name: f.name, size: f.size ?? 0, modified: f.modificationTime ?? null })),
      Date.now(),
      { maxAgeMs: BIR_GUN, maxBytes: AZAMI_ONBELLEK, keep },
    );
    for (const f of files) {
      if (names.includes(f.name)) {
        try {
          f.delete();
        } catch {
          // başka bir ekran kullanıyor olabilir
        }
      }
    }
  } catch {
    // önbellek okunamadı: temizlik bir dahaki açılışta
  }
}
