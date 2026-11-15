/**
 * Komuta merkezi (#34): mükellef × dönem durum matrisi.
 *
 * Hücre durumu iki kaynaktan gelir:
 * 1. Belge verisi (fatura, öneri, uyarı) → `toplandı` / `kontrol edildi`.
 * 2. Dönem iş akışı (W7 #32 dönem yönetimi) → `onaylandı` / `çıktı alındı`
 *    / `beyan`. #32 birleşene kadar bu kaynak `PeriodWorkflow` arayüzüdür;
 *    gerçek uç geldiğinde yalnızca onu dolduran kod değişir.
 *
 * Hesap tek geçişte (O(fatura + öneri + uyarı)) yapılır; 100 mükellef × 6
 * dönem ekran 1 sn altında kalsın diye bileşen bu modeli `useMemo` ile tutar.
 */

import type {
  DataWarning,
  Invoice,
  Severity,
  Suggestion,
  Taxpayer,
} from "../types";

export const cellStatuses = [
  "empty",
  "collected",
  "checked",
  "approved",
  "exported",
  "declared",
] as const;
export type CellStatus = (typeof cellStatuses)[number];

/** #32'nin dönem kaydından gelecek aşamalar. */
export type WorkflowStage = "approved" | "exported" | "declared";

/** taxpayerId → period (YYYY-MM) → aşama. */
export type PeriodWorkflow = Record<string, Record<string, WorkflowStage>>;

export type WarningLevel = "none" | Severity;

export type PeriodFacts = {
  invoices: number;
  pendingSuggestions: number;
  openBlockers: number;
  openWarnings: number;
  openInfos: number;
};

export type Cell = PeriodFacts & {
  status: CellStatus;
  level: WarningLevel;
  /** İş akışı aşaması ilerlemiş ama sonradan engelleyici uyarı açılmış. */
  regressed: boolean;
};

export type MatrixRow = {
  taxpayer: Taxpayer;
  cells: Record<string, Cell>;
};

const rank: Record<CellStatus, number> = Object.fromEntries(
  cellStatuses.map((s, i) => [s, i]),
) as Record<CellStatus, number>;

const levelRank: Record<WarningLevel, number> = {
  none: 0,
  info: 1,
  warning: 2,
  blocker: 3,
};

/** `end` dahil geriye doğru `count` dönem, eskiden yeniye. */
export function periodsEndingAt(end: string, count: number): string[] {
  const [y, m] = end.split("-").map(Number);
  const out: string[] = [];
  for (let i = count - 1; i >= 0; i--) {
    const d = new Date(Date.UTC(y, m - 1 - i, 1));
    out.push(
      `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`,
    );
  }
  return out;
}

export function emptyFacts(): PeriodFacts {
  return {
    invoices: 0,
    pendingSuggestions: 0,
    openBlockers: 0,
    openWarnings: 0,
    openInfos: 0,
  };
}

/** Belge verisinden mükellef/dönem özetleri (tek geçiş). */
export function factsFromDocuments(data: {
  invoices: Invoice[];
  suggestions: Suggestion[];
  warnings: DataWarning[];
}): Map<string, PeriodFacts> {
  const facts = new Map<string, PeriodFacts>();
  const at = (taxpayerId: string, period: string) => {
    const k = key(taxpayerId, period);
    let f = facts.get(k);
    if (!f) {
      f = emptyFacts();
      facts.set(k, f);
    }
    return f;
  };
  const periodOfInvoice = new Map<string, string>();
  for (const inv of data.invoices) {
    const p = inv.issueDate.slice(0, 7);
    periodOfInvoice.set(inv.id, p);
    at(inv.taxpayerId, p).invoices++;
  }
  for (const s of data.suggestions) {
    if (s.status !== "pending") continue;
    const p = periodOfInvoice.get(s.invoiceId);
    if (p) at(s.taxpayerId, p).pendingSuggestions++;
  }
  for (const w of data.warnings) {
    if (w.status !== "open") continue;
    const f = at(w.taxpayerId, w.period);
    if (w.severity === "blocker") f.openBlockers++;
    else if (w.severity === "warning") f.openWarnings++;
    else f.openInfos++;
  }
  return facts;
}

export function key(taxpayerId: string, period: string) {
  return `${taxpayerId}|${period}`;
}

export function cellOf(facts: PeriodFacts, stage?: WorkflowStage): Cell {
  const level: WarningLevel = facts.openBlockers
    ? "blocker"
    : facts.openWarnings
      ? "warning"
      : facts.openInfos
        ? "info"
        : "none";
  let status: CellStatus = "empty";
  if (facts.invoices > 0)
    status =
      facts.pendingSuggestions === 0 && facts.openBlockers === 0
        ? "checked"
        : "collected";
  let regressed = false;
  if (stage) {
    // Aşama kaydı belge durumundan önce gelir; ama engelleyici uyarı
    // sonradan açıldıysa hücre işaretlenir (müşavir yeniden bakmalı).
    regressed = facts.openBlockers > 0;
    status = stage;
  }
  return { ...facts, status, level, regressed };
}

export function buildMatrix(input: {
  taxpayers: Taxpayer[];
  periods: string[];
  facts: Map<string, PeriodFacts>;
  workflow: PeriodWorkflow;
}): MatrixRow[] {
  const blank = emptyFacts();
  return input.taxpayers.map((taxpayer) => {
    const cells: Record<string, Cell> = {};
    const stages = input.workflow[taxpayer.id];
    for (const p of input.periods)
      cells[p] = cellOf(
        input.facts.get(key(taxpayer.id, p)) ?? blank,
        stages?.[p],
      );
    return { taxpayer, cells };
  });
}

export type MatrixFilter = {
  /** Atanan ofis üyesi; "all" ya da "unassigned". */
  member: string;
  status: "all" | CellStatus;
  /** En az bu seviyede açık uyarı. */
  level: "all" | Severity;
  query: string;
};

export const defaultFilter: MatrixFilter = {
  member: "all",
  status: "all",
  level: "all",
  query: "",
};

/**
 * Satır filtresi. Durum ve uyarı seviyesi `focus` dönemine bakar (ekranda
 * seçili sütun); müşavir ve arama satırın kendisine.
 */
export function filterRows(
  rows: MatrixRow[],
  filter: MatrixFilter,
  focus: string,
): MatrixRow[] {
  const q = filter.query.trim().toLocaleLowerCase("tr");
  return rows.filter(({ taxpayer, cells }) => {
    if (filter.member === "unassigned") {
      if (taxpayer.assigneeIds.length) return false;
    } else if (
      filter.member !== "all" &&
      !taxpayer.assigneeIds.includes(filter.member)
    )
      return false;
    if (
      q &&
      !taxpayer.title.toLocaleLowerCase("tr").includes(q) &&
      !taxpayer.taxId.includes(q)
    )
      return false;
    const c = cells[focus];
    if (!c) return filter.status === "all" && filter.level === "all";
    if (filter.status !== "all" && c.status !== filter.status) return false;
    if (filter.level !== "all" && levelRank[c.level] < levelRank[filter.level])
      return false;
    return true;
  });
}

/** Seçili dönemde duruma göre mükellef sayısı (filtre öncesi). */
export function statusCounts(
  rows: MatrixRow[],
  period: string,
): Record<CellStatus, number> {
  const out = Object.fromEntries(cellStatuses.map((s) => [s, 0])) as Record<
    CellStatus,
    number
  >;
  for (const r of rows) {
    const c = r.cells[period];
    if (c) out[c.status]++;
  }
  return out;
}

/** İşi en geride olan önce; eşitse engelleyici uyarı sayısı, sonra unvan. */
export function sortByProgress(rows: MatrixRow[], period: string): MatrixRow[] {
  return [...rows].sort((a, b) => {
    const ca = a.cells[period];
    const cb = b.cells[period];
    return (
      rank[ca?.status ?? "empty"] - rank[cb?.status ?? "empty"] ||
      (cb?.openBlockers ?? 0) - (ca?.openBlockers ?? 0) ||
      a.taxpayer.title.localeCompare(b.taxpayer.title, "tr")
    );
  });
}

export function statusRank(s: CellStatus) {
  return rank[s];
}
