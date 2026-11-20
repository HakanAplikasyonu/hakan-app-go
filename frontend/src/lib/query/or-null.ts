import { ApiError } from "@/lib/api";

/**
 * Beklenen 404'ü boş veri sayar (#639). "Henüz oluşturulmadı" anlamına gelen
 * bir uç 404 dönerse sorgu hata durumuna düşmez: `null` veri olarak
 * önbelleğe alınır, yeniden bağlanmada ya da tablo yeniden çizildiğinde
 * tekrar çekilmez ve küresel hata bildirimi çıkmaz. Diğer hatalar (403,
 * 5xx, ağ) olduğu gibi fırlatılır.
 *
 *   queryFn: () => queryOrNull(() => service.latest(slug, id))
 */
export async function queryOrNull<T>(fn: () => Promise<T>): Promise<T | null> {
  try {
    return await fn();
  } catch (error) {
    if (error instanceof ApiError && error.isNotFound) return null;
    throw error;
  }
}
