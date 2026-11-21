import { isApiError } from "@/lib/api";

type Translate = (key: string) => string;

/**
 * API hatasının Türkçesi (#161, #710): önce fişe özel çeviri
 * (`office.journals.error_CODE`), sonra ortak kod sözlüğü
 * (`errors.codes.CODE`), yoksa işe uygun yedek metin. Sunucunun İngilizce
 * mesajı ve ağ hatasının ham metni ekrana çıkmaz.
 */
export function journalErrorText(t: Translate, err: unknown, fallback: string) {
  if (isApiError(err) && err.code) {
    for (const key of [
      `office.journals.error_${err.code}`,
      `errors.codes.${err.code}`,
    ]) {
      const msg = t(key);
      if (msg !== key) return msg;
    }
  }
  return fallback;
}
