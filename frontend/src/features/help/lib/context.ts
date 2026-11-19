import { primaryGuide } from "./guides";

/**
 * Ekrana özel yardım (#522): ofis panelindeki sayfa → rolün kılavuzundaki
 * bölüm. Anahtar, `/t/{ofis}` ya da `/t/{ofis}/engagements/{id}` sonrasındaki
 * ilk yol parçasıdır. Eşleşme yoksa kılavuzun başı açılır.
 */
const SECTIONS: Record<string, Record<string, string>> = {
  "ofis-calisani": {
    upload: "ubl-yükleme",
    invoices: "faturalar",
    review: "inceleme",
    warnings: "uyarılar",
    journals: "yevmiye",
    bank: "banka-luca-ekstresi-csv",
    reconciliation: "mutabakat",
    report: "mükellef-raporu",
  },
  "ofis-yoneticisi": {
    settings: "2-ofis-ayarları",
    members: "3-kullanıcılar-ve-roller",
    roles: "3-kullanıcılar-ve-roller",
    taxpayers: "4-mükellef-ekleme",
    connections: "5-entegratör-ve-kaynak-bağlantıları",
    review: "6-inceleme-kuyruğu",
    journals: "7-yevmiye-ve-luca--zirve-çıktısı",
  },
};

export const HELP_CONTEXT_SECTIONS = SECTIONS;

/** Ofis adresinden sayfa anahtarı (`warnings`, `bank` …); yoksa null. */
export function pageKey(pathname: string): string | null {
  const parts = pathname.split("/").filter(Boolean);
  if (parts[0] !== "t" || !parts[1]) return null;
  const rest = parts.slice(2);
  if (rest[0] === "engagements") return rest[2] ?? null;
  return rest[0] ?? null;
}

/**
 * Kullanıcının yardım hedefi: rolünün kılavuzu ve, o an bir ofis sayfasında
 * ise, kılavuzun ilgili bölümü. Rolüne yazılmış kılavuz yoksa (ziyaretçi,
 * platform yöneticisi) yardım merkezi.
 */
export function helpTarget(
  roles: readonly string[],
  pathname?: string | null,
): { slug: string; section?: string } | null {
  const guide = primaryGuide(roles);
  if (!guide) return null;
  const key = pathname ? pageKey(pathname) : null;
  const section = key ? SECTIONS[guide.slug]?.[key] : undefined;
  return { slug: guide.slug, section };
}

/** `helpTarget` sonucunun adresi. */
export function helpHref(
  target: { slug: string; section?: string } | null,
): string {
  if (!target) return "/yardim";
  const base = `/yardim/${encodeURIComponent(target.slug)}`;
  return target.section
    ? `${base}#${encodeURIComponent(target.section)}`
    : base;
}
