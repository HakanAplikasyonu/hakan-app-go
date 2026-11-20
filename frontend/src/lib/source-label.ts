/**
 * Verinin geldiği yerin Türkçe adı (#710): `manual_upload`, `bank_ziraat`,
 * `entegrator_izibiz` gibi kaynak kodları ve `ubl_upload` gibi alış yolları
 * ekranda hiç ham görünmez. Saf modül; çeviri dışarıdan.
 */

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

function known(t: Translate, key: string): string | null {
  const v = t(key);
  return v === key ? null : v;
}

/** "vakif_katilim" → "Vakif Katilim" (bilinmeyen banka / entegratör adı). */
function titled(code: string) {
  return code
    .split("_")
    .filter(Boolean)
    .map((w) => w.charAt(0).toLocaleUpperCase("tr-TR") + w.slice(1))
    .join(" ");
}

/** Kaynak sistemin adı: "Elle yükleme", "Ziraat bankası dosyası" … */
export function sourceSystemLabel(
  code: string | null | undefined,
  t: Translate,
): string {
  if (!code) return "";
  const direct = known(t, `sources.system.${code}`);
  if (direct) return direct;
  if (code.startsWith("entegrator_")) {
    return t("sources.system_integrator", {
      name: titled(code.slice("entegrator_".length)),
    });
  }
  if (code.startsWith("bank_")) {
    return t("sources.system_bank", {
      name: titled(code.slice("bank_".length)),
    });
  }
  return t("sources.system_other");
}

/** Alış yolu: "UBL dosyası", "Excel aktarımı", "Elle giriş" … */
export function captureMethodLabel(
  code: string | null | undefined,
  t: Translate,
): string {
  if (!code) return "";
  return known(t, `sources.capture.${code}`) ?? t("sources.capture_other");
}

/** "Elle yükleme · UBL dosyası"; boş parça atlanır. */
export function sourceText(
  system: string | null | undefined,
  method: string | null | undefined,
  t: Translate,
): string {
  return [sourceSystemLabel(system, t), captureMethodLabel(method, t)]
    .filter(Boolean)
    .join(" · ");
}
