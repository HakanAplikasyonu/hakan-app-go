/**
 * Belge görüntüleyici (#780) için saf yardımcılar: dosya türü ve adı,
 * indirme hatasının Türkçe metni, önbellek temizliği, base64. React Native'e
 * bağlı değil; node --test ile sınanır.
 */
import { ApiError, errorMessage, isApiError } from '../../core/api/envelope.ts';

/** Görüntüleyicinin açtığı belge: sunucudaki dosya (`path`) ya da telefondaki dosya (`localUri`). */
export type BelgeKaynagi = {
  /** Ekran başlığı ve paylaşım adı. */
  title: string;
  /** `/v1/taxpayer/...` indirme yolu. */
  path?: string;
  /** Telefondaki dosya (ör. kuyruktaki fiş fotoğrafı); indirme yok. */
  localUri?: string;
  fileName?: string | null;
  contentType?: string | null;
  /** Bayt; biliniyorsa indirmeden önce boyut denetlenir. */
  size?: number | null;
};

export type BelgeTuru = 'pdf' | 'image' | 'other';

/** Telefonda açılan en büyük dosya (sunucu yüklemede 20 MB sınırı koyar). */
export const AZAMI_GORUNTULEME_BAYT = 30 * 1024 * 1024;

const RESIM_TURLERI: Record<string, string> = {
  'image/jpeg': '.jpg',
  'image/jpg': '.jpg',
  'image/png': '.png',
  'image/webp': '.webp',
  'image/heic': '.heic',
  'image/heif': '.heif',
  'image/gif': '.gif',
};

const UZANTI_TURU: Record<string, string> = {
  pdf: 'application/pdf',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  webp: 'image/webp',
  heic: 'image/heic',
  heif: 'image/heif',
  gif: 'image/gif',
};

/** "image/JPEG; charset=x" → "image/jpeg". */
export function temelTur(contentType: string | null | undefined): string {
  return (contentType ?? '').split(';')[0].trim().toLowerCase();
}

function uzanti(fileName: string | null | undefined): string {
  const m = /\.([A-Za-z0-9]{1,8})$/.exec((fileName ?? '').trim());
  return m ? m[1].toLowerCase() : '';
}

/**
 * Dosyanın MIME türü: önce sunucunun bildirdiği tür; genel bir tür
 * (`application/octet-stream`, boş) gelirse dosya adının uzantısı.
 */
export function belgeMime(contentType: string | null | undefined, fileName?: string | null): string {
  const t = temelTur(contentType);
  if (t && t !== 'application/octet-stream' && t !== 'binary/octet-stream') return t;
  return UZANTI_TURU[uzanti(fileName)] ?? t;
}

/** PDF mi, görsel mi, başka bir dosya mı (Excel vb.: yalnızca paylaşılır). */
export function belgeTuru(contentType: string | null | undefined, fileName?: string | null): BelgeTuru {
  const t = belgeMime(contentType, fileName);
  if (t === 'application/pdf') return 'pdf';
  if (t in RESIM_TURLERI) return 'image';
  return 'other';
}

/** Önbellek dosya adı: önek + kimlik + uzantı; yalnızca güvenli karakterler. */
export function onbellekAdi(prefix: string, id: string, fileName?: string | null, contentType?: string | null): string {
  const temiz = `${prefix}-${id}`.replace(/[^A-Za-z0-9_-]/g, '').slice(0, 96);
  const safe = /[A-Za-z0-9]/.test(temiz) ? temiz : 'belge';
  const ext = uzanti(fileName);
  if (ext) return `${safe}.${ext}`;
  const t = belgeMime(contentType, fileName);
  if (t === 'application/pdf') return `${safe}.pdf`;
  return `${safe}${RESIM_TURLERI[t] ?? ''}`;
}

/**
 * Paylaşımda görünen ad: sunucudaki dosya adı; yoksa başlıktan üretilir
 * ("Ekim bordrosu" + PDF → "Ekim bordrosu.pdf").
 */
export function paylasimAdi(title: string, fileName?: string | null, contentType?: string | null): string {
  const name = guvenliDosyaAdi(fileName ?? '');
  if (name) return name;
  const base = title.replace(/[\\/:*?"<>|]+/g, ' ').replace(/\s+/g, ' ').trim() || 'Belge';
  const ext = onbellekAdi('x', 'x', null, contentType).replace(/^x-x/, '');
  return `${base}${ext}`;
}

/** Dosya sisteminde sorun çıkarmayan ad: denetim karakteri, `/ \\ : * ? " < > |` ve baştaki noktalar gider. */
export function guvenliDosyaAdi(ad: string): string {
  return Array.from(ad)
    .filter((ch) => ch.charCodeAt(0) >= 32 && ch.charCodeAt(0) !== 127)
    .join('')
    .replace(/[\\/:*?"<>|]+/g, '_')
    .replace(/^[.\s]+/, '')
    .trim()
    .slice(0, 120);
}

/** "1536" → "1,5 KB" (tr-TR). */
export function dosyaBoyutu(bytes: number | null | undefined): string {
  if (bytes == null || !Number.isFinite(bytes) || bytes < 0) return '';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB'];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toLocaleString('tr-TR', { maximumFractionDigits: 1 })} ${units[i]}`;
}

/** Bilinen boyut telefonda açılamayacak kadar büyük mü. */
export function cokBuyuk(bytes: number | null | undefined): boolean {
  return typeof bytes === 'number' && Number.isFinite(bytes) && bytes > AZAMI_GORUNTULEME_BAYT;
}

/**
 * İndirme yolu yalnızca mükellef API'sinin bir yolu olabilir: görüntüleyici
 * oturum belirtecini gönderdiği için başka sunucuya giden ya da kullanıcı
 * bilgisi taşıyan (`@`) yol reddedilir.
 */
export function gecerliBelgeYolu(path: string | null | undefined): boolean {
  return typeof path === 'string' && /^\/v1\/taxpayer\/[A-Za-z0-9/_-]+$/.test(path) && !path.includes('//');
}

/** Telefondaki dosya yalnızca uygulamanın kendi `file://` dosyası olabilir. */
export function gecerliYerelDosya(uri: string | null | undefined): boolean {
  return typeof uri === 'string' && uri.startsWith('file://') && !uri.includes('..');
}

/** İndirmeyi büyük dosya yüzünden durduran hata. */
export function cokBuyukHatasi(bytes?: number | null): ApiError {
  return new ApiError({
    status: 413,
    code: 'FILE_TOO_LARGE',
    message: `Belge telefonda açılamayacak kadar büyük${bytes ? ` (${dosyaBoyutu(bytes)})` : ''}.`,
  });
}

const DURUM_METNI: Record<number, { code: string; message: string }> = {
  401: { code: 'UNAUTHENTICATED', message: 'Oturumunuzun süresi doldu. Çıkış yapıp yeniden giriş yapın.' },
  403: { code: 'FORBIDDEN', message: 'Bu belgeyi açma yetkiniz yok. Şirket sahibine ya da müşavirinize danışın.' },
  404: { code: 'NOT_FOUND', message: 'Belge bulunamadı. Müşaviriniz belgeyi kaldırmış ya da paylaşımı kapatmış olabilir.' },
  413: { code: 'FILE_TOO_LARGE', message: 'Belge telefonda açılamayacak kadar büyük. Müşavirinizden e-postayla göndermesini isteyin.' },
};

export type BelgeHatasi = { code: string; message: string };

/**
 * İndirme hatasının Türkçe metni ve kodu (ekranda "Hata kodu: …").
 * API zarfının kodu (ör. `NO_TAXPAYER_MEMBERSHIP`) korunur; metin duruma göre.
 */
export function belgeHatasi(e: unknown): BelgeHatasi {
  if (!isApiError(e)) return { code: 'UNEXPECTED', message: 'Belge açılamadı. Yeniden deneyin.' };
  if (e.code === 'FILE_TOO_LARGE' || e.code === 'UPLOAD_TOO_LARGE' || e.status === 413) {
    return { code: e.code === 'UPLOAD_TOO_LARGE' ? e.code : 'FILE_TOO_LARGE', message: DURUM_METNI[413].message };
  }
  if (e.status === 0) {
    return {
      code: e.code,
      message: e.code === 'TIMEOUT' ? 'İndirme çok uzun sürdü. Bağlantınızı kontrol edip yeniden deneyin.' : 'Belge indirilemedi. İnternet bağlantınızı kontrol edip yeniden deneyin.',
    };
  }
  const byStatus = DURUM_METNI[e.status];
  if (byStatus) {
    // Şirkete erişimi olmayan kullanıcı: ortak zarf metni daha açıklayıcı.
    if (e.code === 'NO_TAXPAYER_MEMBERSHIP') return { code: e.code, message: errorMessage(e) };
    // Sunucunun kodu korunur; gövdesiz yanıtta (HTTP_404) durumun adı.
    return { code: !e.code || e.code.startsWith('HTTP_') ? byStatus.code : e.code, message: byStatus.message };
  }
  if (e.status >= 500) return { code: e.code, message: 'Sunucu belgeyi şu an veremiyor. Biraz sonra yeniden deneyin.' };
  return { code: e.code, message: errorMessage(e) };
}

/** Ekrandaki tek satır: "Metin (Hata kodu: NOT_FOUND)". */
export function belgeHataMetni(e: unknown): string {
  const h = belgeHatasi(e);
  return `${h.message} (Hata kodu: ${h.code})`;
}

export type OnbellekDosyasi = { name: string; size: number; modified: number | null };

/**
 * Önbellekte silinecek belgeler: `maxAgeMs`'ten eski olanlar, sonra toplam
 * `maxBytes`'ı aşan en eskiler. `keep` (açık olan belge) hiç silinmez.
 */
export function silinecekler(
  files: readonly OnbellekDosyasi[],
  now: number,
  opts: { maxAgeMs: number; maxBytes: number; keep?: string },
): string[] {
  const out = new Set<string>();
  const kalan: OnbellekDosyasi[] = [];
  for (const f of files) {
    if (f.name === opts.keep) continue;
    if (f.modified !== null && now - f.modified > opts.maxAgeMs) out.add(f.name);
    else kalan.push(f);
  }
  const keepSize = files.find((f) => f.name === opts.keep)?.size ?? 0;
  let total = keepSize + kalan.reduce((s, f) => s + f.size, 0);
  // En eskiden başlayarak (zamanı bilinmeyen en eski sayılır) sınır altına inene kadar.
  for (const f of [...kalan].sort((a, b) => (a.modified ?? 0) - (b.modified ?? 0))) {
    if (total <= opts.maxBytes) break;
    out.add(f.name);
    total -= f.size;
  }
  return [...out];
}

const B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

/** Baytlar → base64 (Android'de WebView'e veri olarak verilir). */
export function base64(bytes: Uint8Array): string {
  const parts: string[] = [];
  const CHUNK = 3 * 4096;
  for (let start = 0; start < bytes.length; start += CHUNK) {
    const end = Math.min(start + CHUNK, bytes.length);
    let s = '';
    let i = start;
    for (; i + 2 < end; i += 3) {
      const n = (bytes[i] << 16) | (bytes[i + 1] << 8) | bytes[i + 2];
      s += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + B64[(n >> 6) & 63] + B64[n & 63];
    }
    if (i < end) {
      const n = (bytes[i] << 16) | ((i + 1 < end ? bytes[i + 1] : 0) << 8);
      s += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + (i + 1 < end ? B64[(n >> 6) & 63] : '=') + '=';
    }
    parts.push(s);
  }
  return parts.join('');
}
