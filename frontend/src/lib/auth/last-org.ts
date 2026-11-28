/**
 * Bu tarayıcıda son açılan ofis (#890 B). Birden çok ofisi olan kullanıcı
 * girişte seçiciye düşmeden doğrudan buraya döner. Çıkışta silinmez.
 * localStorage kapalıysa (gizli pencere, kota) sessizce yok sayılır.
 */
export const LAST_ORG_KEY = "hakan:last_org";

export function readLastOrg(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(LAST_ORG_KEY);
  } catch {
    return null;
  }
}

export function rememberLastOrg(slug: string): void {
  if (typeof window === "undefined" || !slug) return;
  try {
    window.localStorage.setItem(LAST_ORG_KEY, slug);
  } catch {
    // Saklanamazsa seçici açılır; hata göstermeye gerek yok.
  }
}
