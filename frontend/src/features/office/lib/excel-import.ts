/** Aktarımın sihirbazdaki adımı (#518 B1). */
export type ExcelStep = "working" | "mapping" | "rows" | "ready" | "done";

type Status =
  | "uploaded"
  | "analyzing"
  | "mapping_review"
  | "validating"
  | "has_errors"
  | "preview"
  | "committed"
  | "failed"
  | "rolled_back";

/** Sunucu durumunu sihirbaz adımına çevirir. */
export function excelStep(status: Status): ExcelStep {
  switch (status) {
    case "uploaded":
    case "analyzing":
    case "validating":
      return "working";
    case "mapping_review":
      return "mapping";
    case "has_errors":
      return "rows";
    case "preview":
      return "ready";
    default:
      return "done";
  }
}

/** Sunucu hâlâ çalışıyor: ekran durumu yoklar. */
export function excelBusy(status: Status) {
  return excelStep(status) === "working";
}

type ColumnMap = { column: string; field: string };

/**
 * Zorunlu alan gruplarından eşlenmemiş olanlar: her gruptan en az bir alan
 * bir sütuna bağlı olmalı.
 */
export function missingRequired(
  required: string[][],
  columns: ColumnMap[],
): string[][] {
  const mapped = new Set(columns.map((c) => c.field).filter(Boolean));
  return required.filter((group) => !group.some((f) => mapped.has(f)));
}

/** Aynı alana bağlanmış birden çok sütun (kullanıcı hatası). */
export function duplicateFields(columns: ColumnMap[]): string[] {
  const seen = new Map<string, number>();
  for (const c of columns)
    if (c.field) seen.set(c.field, (seen.get(c.field) ?? 0) + 1);
  return [...seen].filter(([, n]) => n > 1).map(([f]) => f);
}
