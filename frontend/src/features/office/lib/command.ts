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
