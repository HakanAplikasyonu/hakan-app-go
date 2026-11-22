// Ofis özeti (#780): rozetteki Gelen kutusu sayısı, açılır penceredeki KDV
// uyarıları ve açık belge istekleri. Bu dosyada yalnızca saf dönüşümler var;
// istekler src/api.ts, rozet src/background.ts içindedir.

/** GET /v1/tenant/inbox → data.counts */
export interface InboxCounts {
  invoice: number;
  bank_transaction: number;
  expense: number;
  retail_sale: number;
  foreign_invoice: number;
  late_document: number;
  total: number;
}

/** GET /v1/tenant/vat-forecast/watch-events → data.items[] */
export interface VatWatchEvent {
  uuid: string;
  period: string;
  event: string;
  engagement_uuid: string;
  taxpayer_title: string;
  created_at: string;
}

/** GET /v1/tenant/clients?filter=tasks → data.items[] (yalnızca kullanılan alanlar) */
export interface ClientWithTasks {
  engagement_uuid: string;
  title: string;
  open_tasks: number;
}

export interface SummaryItem {
  title: string;
  detail: string;
  url: string;
}

/** Bir bölüm okunamadıysa (yetki yok, uç nokta yok) null. */
export interface Summary {
  inbox: { counts: InboxCounts; url: string } | null;
  vat: { items: SummaryItem[]; url: string } | null;
  tasks: { items: SummaryItem[]; url: string } | null;
  fetchedAt: number;
}

export const SUMMARY_LIMIT = 5;
/** Otomatik yenileme en sık 5 dakikada bir. */
export const POLL_MS = 5 * 60 * 1000;

const INBOX_LABELS: Array<[Exclude<keyof InboxCounts, "total">, string]> = [
  ["invoice", "Fatura"],
  ["bank_transaction", "Banka"],
  ["expense", "Gider"],
  ["retail_sale", "Z raporu"],
  ["foreign_invoice", "Yabancı fatura"],
  ["late_document", "Geç gelen belge"],
];

const VAT_EVENT_LABELS: Record<string, string> = {
  "vat.threshold_exceeded": "KDV eşiği aşıldı",
  "vat.status_changed": "KDV durumu değişti",
  "vat.report_ready": "KDV raporu hazır",
};

/** Rozet metni: 0 → boş (rozet gizlenir), 100 ve üstü → "99+". */
export function badgeText(total: number | null | undefined): string {
  if (!total || total < 0 || !Number.isFinite(total)) return "";
  return total > 99 ? "99+" : String(Math.floor(total));
}

/** Rozet ipucu: toplam ve sıfır olmayan türlerin dökümü. */
export function badgeTitle(base: string, counts: InboxCounts | null): string {
  if (!counts) return base;
  if (!counts.total) return `${base}\nGelen kutusu boş`;
  const parts = INBOX_LABELS.filter(([k]) => counts[k] > 0).map(([k, label]) => `${label} ${counts[k]}`);
  return `${base}\nGelen kutusu: ${counts.total}${parts.length ? ` (${parts.join(", ")})` : ""}`;
}

/** Gelen kutusu dökümü, açılır pencere için (sıfırlar atlanır). */
export function inboxBreakdown(counts: InboxCounts): string {
  return INBOX_LABELS.filter(([k]) => counts[k] > 0)
    .map(([k, label]) => `${label} ${counts[k]}`)
    .join(" · ");
}

/**
 * Panel (web) adresi. Uzantı yalnızca API adresini bilir: API panelin
 * /api yolundaysa (aynı köken) onu atar; "api." alt alan adını "app." yapar.
 * Tahmin tutmazsa kullanıcı açılır pencerede panel adresini yazar.
 */
export function guessWebBase(apiBase: string): string {
  let u: URL;
  try {
    u = new URL(apiBase);
  } catch {
    return "";
  }
  const path = u.pathname.replace(/\/+$/, "").replace(/\/v1$/, "").replace(/\/api$/, "");
  const host = u.host.startsWith("api.") ? "app." + u.host.slice(4) : u.host;
  return `${u.protocol}//${host}${path}`;
}

export interface Links {
  inbox: string;
  reports: string;
  documentCollection: string;
  engagementTasks: (engagementUuid: string) => string;
}

export function links(webBase: string, slug: string): Links {
  const t = `${webBase.replace(/\/+$/, "")}/t/${encodeURIComponent(slug)}`;
  return {
    inbox: `${t}/review?view=inbox`,
    reports: `${t}/reports`,
    documentCollection: `${t}/document-collection`,
    engagementTasks: (e) => `${t}/engagements/${encodeURIComponent(e)}#belge-istekleri`,
  };
}

function shortDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("tr-TR", { day: "numeric", month: "short", timeZone: "Europe/Istanbul" });
}

export function vatItems(events: VatWatchEvent[], l: Links): SummaryItem[] {
  return events.slice(0, SUMMARY_LIMIT).map((e) => {
    const label = VAT_EVENT_LABELS[e.event] ?? "KDV uyarısı";
    const when = shortDate(e.created_at);
    return {
      title: e.taxpayer_title || "Mükellef",
      detail: [label, e.period, when].filter(Boolean).join(" · "),
      url: l.reports,
    };
  });
}

export function taskItems(clients: ClientWithTasks[], l: Links): SummaryItem[] {
  return clients
    .filter((c) => c.open_tasks > 0)
    .slice(0, SUMMARY_LIMIT)
    .map((c) => ({
      title: c.title || "Mükellef",
      detail: `${c.open_tasks} açık belge isteği`,
      url: l.engagementTasks(c.engagement_uuid),
    }));
}
