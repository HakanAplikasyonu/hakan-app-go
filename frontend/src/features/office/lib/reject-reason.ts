import type { components } from "@/generated/api";

/** Ret gerekçesi (#780): fiş, masraf ve Z raporu ret uçlarının gövdesi. */
export type ReceiptRejection = components["schemas"]["ReceiptRejection"];
export type RejectReasonCode = NonNullable<ReceiptRejection["reason_code"]>;

/** Gerekçeler, ekrandaki sırayla. */
export const REJECT_REASON_CODES: readonly RejectReasonCode[] = [
  "okunaksiz",
  "eksik_bilgi",
  "kisisel_harcama",
  "mukerrer",
  "yanlis_mukellef",
  "diger",
];

export const MAX_REJECT_TEXT = 500;

/**
 * Ret gerekçesi formunun gövdesi: seçilen kod ve boşlukları sadeleşmiş
 * açıklama. "diger" açıklamasız ya da 500 karakterden uzun metin
 * gönderilmez; sunucu da aynı kuralı uygular.
 */
export function rejectionBody(
  code: RejectReasonCode | "",
  text: string,
):
  | { ok: true; body: ReceiptRejection }
  | { ok: false; error: "code" | "text_required" | "text_long" } {
  const clean = text.trim().replace(/\s+/g, " ");
  if (!code) return { ok: false, error: "code" };
  if (code === "diger" && !clean) return { ok: false, error: "text_required" };
  if ([...clean].length > MAX_REJECT_TEXT)
    return { ok: false, error: "text_long" };
  return {
    ok: true,
    body: clean
      ? { reason_code: code, reason_text: clean }
      : { reason_code: code },
  };
}
