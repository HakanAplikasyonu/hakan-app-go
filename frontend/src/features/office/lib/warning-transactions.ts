/**
 * Dönem düzeyindeki banka uyarılarının (faturasız tahsilat, belgesiz ödeme,
 * sınıflanmamış hareket …) işaret ettiği hareketler (#570). Uyarı tek bir
 * kayda bağlı değildir; hareketler `params` içinde gelir:
 * `groups[].transactions` (karşı tarafa göre) ya da düz `transactions`.
 */
export type WarningTransaction = {
  uuid: string;
  /** Grubun karşı tarafı (varsa). */
  counterparty?: string;
};

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function uuids(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string" && UUID_RE.test(v))
    : [];
}

export function warningTransactions(
  params: Record<string, unknown> | null | undefined,
): WarningTransaction[] {
  if (!params) return [];
  const out: WarningTransaction[] = [];
  const seen = new Set<string>();
  const push = (uuid: string, counterparty?: string) => {
    if (seen.has(uuid)) return;
    seen.add(uuid);
    out.push(counterparty ? { uuid, counterparty } : { uuid });
  };
  if (Array.isArray(params.groups)) {
    for (const g of params.groups) {
      if (!g || typeof g !== "object") continue;
      const group = g as Record<string, unknown>;
      const name =
        typeof group.counterparty === "string" && group.counterparty
          ? group.counterparty
          : undefined;
      for (const id of uuids(group.transactions)) push(id, name);
    }
  }
  for (const id of uuids(params.transactions)) push(id);
  return out;
}
