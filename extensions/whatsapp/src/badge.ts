// Kaydedilmiş medyanın rozeti (#780): WhatsApp Web'de fişin altında
// "Okunuyor…" → "Kaydedildi · onay bekliyor" → "Onaylandı" / "Reddedildi".
// Durum sunucudan dosya özetiyle (SHA-256) sorulur; saf yardımcılar burada,
// testleri tests/badge.test.ts.

import type { Target } from "./messages";

export type BadgeState = "reading" | "pending" | "approved" | "rejected" | "failed" | "duplicate" | "saved";

/** GET /v1/tenant/whatsapp/captures/status satırı. */
export interface CaptureStatus {
  sha256: string;
  uuid: string;
  target: Target;
  engagement_uuid: string;
  taxpayer_title: string;
  state: Exclude<BadgeState, "reading">;
  created_at: string;
}

export function badgeText(state: BadgeState, taxpayer = ""): string {
  const who = taxpayer ? `${taxpayer} · ` : "";
  switch (state) {
    case "reading":
      return "Okunuyor…";
    case "pending":
      return `${who}Kaydedildi · onay bekliyor`;
    case "approved":
      return `${who}Onaylandı ✓`;
    case "rejected":
      return `${who}Reddedildi (panelde nedenine bakın)`;
    case "failed":
      return `${who}Okunamadı · panelde elle girin`;
    case "duplicate":
      return `${who}Mükerrer: bu fiş daha önce kayıtlı`;
    case "saved":
      return `${who}Kaydedildi`;
  }
}

/** Rozetin rengi: bitti (yeşil), sorun (kırmızı), bekliyor (mavi). */
export function badgeTone(state: BadgeState): "ok" | "bad" | "wait" {
  if (state === "approved" || state === "saved") return "ok";
  if (state === "rejected" || state === "failed" || state === "duplicate") return "bad";
  return "wait";
}

/** Kaydın yanıtındaki fiş durumu → rozet (sunucunun BadgeState kuralı). */
export function stateFromCapture(target: Target, receiptStatus: string | null): Exclude<BadgeState, "reading"> {
  if (target !== "expense" && target !== "income") return "saved";
  switch (receiptStatus) {
    case "failed":
      return "failed";
    case "duplicate":
      return "duplicate";
    case "rejected":
      return "rejected";
    case "approved":
      return "approved";
    default:
      return "pending";
  }
}

/** Bir medyanın en yeni kaydı (aynı dosya birden çok türe kaydedilmiş olabilir). */
export function latestFor(statuses: CaptureStatus[], sha: string): CaptureStatus | null {
  let best: CaptureStatus | null = null;
  for (const s of statuses) {
    if (s.sha256 !== sha) continue;
    if (!best || Date.parse(s.created_at) > Date.parse(best.created_at)) best = s;
  }
  return best;
}

/** Aynı dosya aynı türe kaydedildiyse uyarı metni, yoksa null. */
export function alreadySaved(statuses: CaptureStatus[], sha: string, target: Target): string | null {
  const hit = statuses.find((s) => s.sha256 === sha && s.target === target);
  if (!hit) return null;
  return target === "expense" || target === "income"
    ? `Bu fiş zaten kaydedildi (${hit.taxpayer_title}).`
    : `Bu dosya zaten kaydedildi (${hit.taxpayer_title}).`;
}

/** Hâlâ değişebilecek durum: yoklama sürer. */
export function isSettled(state: BadgeState): boolean {
  return state !== "reading" && state !== "pending";
}

/** WhatsApp kişi sohbetinin numarası (+905321234567); grup / gizli numara → null. */
export function phoneOfChat(jid: string): string | null {
  const m = /^(\d{10,15})@c\.us$/.exec(jid);
  return m ? `+${m[1]}` : null;
}

/** Dosyanın SHA-256'sı (hex), sunucunun whatsapp_captures.sha256 değeriyle aynı. */
export async function sha256Hex(blob: Blob): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", await blob.arrayBuffer());
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
