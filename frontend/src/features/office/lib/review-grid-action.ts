/** Satır kararı: boş = koda göre (aynıysa onay, farklıysa düzeltme). */
export type GridDecision = "accept" | "reject";

/**
 * Izgara satırının sunucu eylemi (#394). Reddet seçiliyse ret; kod
 * önerilenden farklıysa düzeltme; aynıysa ve "Onayla" seçiliyse onay.
 * Kod değişmeden karar da yoksa kaydedilecek bir şey yoktur (null).
 */
export function gridRowAction(input: {
  proposedCode: string | undefined;
  code: unknown;
  decision: unknown;
}): "accept" | "reject" | "correct" | null {
  if (input.decision === "reject") return "reject";
  const code = typeof input.code === "string" ? input.code.trim() : "";
  if (code && code !== (input.proposedCode ?? "")) return "correct";
  if (input.decision === "accept") return "accept";
  return null;
}
