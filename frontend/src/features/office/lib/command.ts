/**
 * Komuta merkezi (#34): mükellef × dönem durum matrisi.
 *
 * Kaynak dönem yönetimi (#32) `GET /v1/tenant/periods/matrix`. Hücre durumu:
 * - kilitli dönem → onaylandı
 * - ayı bitmemiş dönem → ay sürüyor (henüz "hazır" denemez, #576)
 * - hazır (açık engelleyici uyarı ve açık değişiklik talebi yok) → kontrol edildi
 * - diğerleri → toplandı (engelleyici ya da talep sayısıyla)
 *
 * Çıktı alındı ve beyan aşamalarının kaynağı henüz yok (muhasebe çıktısı ve
 * beyan kaydı dönem kaydına bağlanınca eklenir).
 */

import { routes } from "@/config/routes";

import { currentPeriod } from "../period/period";
import type {
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";

export const cellStatuses = [
  "open",
  "collected",
  "checked",
  "approved",
] as const;
export type CellStatus = (typeof cellStatuses)[number];

export function statusOf(
  cell: PeriodMatrixCell,
  current = currentPeriod(),
): CellStatus {
  if (cell.locked) return "approved";
  if (cell.period >= current) return "open";
  if (cell.ready) return "checked";
  return "collected";
}

/**
 * Hücreye tıklayınca açılacak ekran (#567, #710): problemden çözüm ekranına.
 * Kapalı dönemde açık talep varsa önce talep listesi; engelleyici uyarı
 * varsa o uyarılar; açık değişiklik talebi mükellef
 * ekranında talep listesi açık gelir; yalnız engellemeyen uyarı varsa uyarı
 * listesi; iş yoksa mükellef özeti.
 */
export type CellTarget =
  "blockers" | "warnings" | "change_requests" | "overview";

/** Mükellef ekranında talep listesini açık getiren sorgu (#710). */
export const OPEN_CHANGE_REQUESTS = "talepler=ac";

export function cellTarget(
  cell: Pick<
    PeriodMatrixCell,
    "open_blockers" | "open_warnings" | "open_change_requests"
  > &
    Partial<Pick<PeriodMatrixCell, "locked">>,
): CellTarget {
  // Kapalı dönemde ilk iş açık talebin kararıdır: talep uygulanmadan
  // dönemdeki engel giderilemez (#710).
  if (cell.locked && cell.open_change_requests > 0) return "change_requests";
  if (cell.open_blockers > 0) return "blockers";
  if (cell.open_change_requests > 0) return "change_requests";
  if (cell.open_warnings > 0) return "warnings";
  return "overview";
}

/** Hedef ekranın adresi (Komuta Merkezi ve Mükellefler aynı kural, #710). */
export function cellHref(
  slug: string,
  engagement: string,
  target: CellTarget,
): string {
  switch (target) {
    case "blockers":
      return routes.tenant.engagementBlockers(slug, engagement);
    case "warnings":
      return routes.tenant.engagementWarnings(slug, engagement);
    case "change_requests":
      return `${routes.tenant.engagement(slug, engagement)}?${OPEN_CHANGE_REQUESTS}`;
    default:
      return routes.tenant.engagement(slug, engagement);
  }
}

const rank: Record<CellStatus, number> = {
  open: 0,
  collected: 0,
  checked: 1,
  approved: 2,
};

export function statusRank(s: CellStatus) {
  return rank[s];
}

export type MatrixFilter = {
  status: "all" | CellStatus;
  /** Yalnızca seçili dönemde açık engelleyici uyarısı olanlar. */
  blockersOnly: boolean;
  query: string;
};

export const defaultFilter: MatrixFilter = {
  status: "all",
  blockersOnly: false,
  query: "",
};

export function cellAt(row: PeriodMatrixRow, period: string) {
  return row.cells.find((c) => c.period === period);
}

/** Durum ve engelleyici filtresi seçili döneme (`focus`) uygulanır. */
export function filterRows(
  rows: PeriodMatrixRow[],
  filter: MatrixFilter,
  focus: string,
): PeriodMatrixRow[] {
  const q = filter.query.trim().toLocaleLowerCase("tr");
  return rows.filter((row) => {
    if (
      q &&
      !row.taxpayer_title.toLocaleLowerCase("tr").includes(q) &&
      !row.taxpayer_tax_id.includes(q)
    )
      return false;
    const c = cellAt(row, focus);
    if (!c) return filter.status === "all" && !filter.blockersOnly;
    if (filter.status !== "all" && statusOf(c) !== filter.status) return false;
    if (filter.blockersOnly && c.open_blockers === 0) return false;
    return true;
  });
}

export function statusCounts(
  rows: PeriodMatrixRow[],
  period: string,
): Record<CellStatus, number> {
  const out: Record<CellStatus, number> = {
    open: 0,
    collected: 0,
    checked: 0,
    approved: 0,
  };
  for (const r of rows) {
    const c = cellAt(r, period);
    if (c) out[statusOf(c)]++;
  }
  return out;
}

/** İşi en geride olan önce; eşitse engelleyici sayısı, sonra unvan. */
export function sortByProgress(
  rows: PeriodMatrixRow[],
  period: string,
): PeriodMatrixRow[] {
  return [...rows].sort((a, b) => {
    const ca = cellAt(a, period);
    const cb = cellAt(b, period);
    return (
      (ca ? rank[statusOf(ca)] : -1) - (cb ? rank[statusOf(cb)] : -1) ||
      (cb?.open_blockers ?? 0) - (ca?.open_blockers ?? 0) ||
      a.taxpayer_title.localeCompare(b.taxpayer_title, "tr")
    );
  });
}
