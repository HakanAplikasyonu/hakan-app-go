/** İnceleme ayarı satırı (ofis ya da mükellef düzeyi). */
export type ReviewRow = {
  level: "office" | "engagement";
  kind: string;
  auto_accept: boolean | null;
  threshold: string | null;
};

/**
 * Mükellefin bütün türler için ("*") kendi satırı (#518 A5); yoksa ofis
 * ayarı geçerlidir.
 */
export function engagementOverride(rows: ReviewRow[]): ReviewRow | null {
  return rows.find((r) => r.level === "engagement" && r.kind === "*") ?? null;
}

/** "0.95" → 95; bozuk değerde varsayılan. */
export function thresholdPercent(threshold: string | null | undefined) {
  const n = Math.round(Number(threshold) * 100);
  return Number.isFinite(n) && n > 0 ? n : 95;
}
