/**
 * Defter türü (#780): bilanço, işletme defteri ya da serbest meslek kazanç
 * defteri. Saf yardımcılar.
 */
import type { components } from "@/generated/api";

export type LedgerType = components["schemas"]["LedgerTypeValue"];

export const LEDGER_TYPES: LedgerType[] = [
  "bilanco",
  "isletme",
  "serbest_meslek",
];

/** Mükellef detayının dönem şeridindeki ay sayısı (matris sorgusu). */
export const ENGAGEMENT_MONTHS = 6;

/**
 * İşletme defteri ve serbest meslekte hesap planı yok: yevmiye, mizan ve
 * hesap kodu ekranları gizlenir, işletme hesabı görünür. Bilinmeyen değer
 * (eski yanıt) bilançodur.
 */
export function isSingleEntry(t: string | null | undefined): boolean {
  return t === "isletme" || t === "serbest_meslek";
}
