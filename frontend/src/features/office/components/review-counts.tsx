"use client";

import { useLocale } from "@/providers/locale-provider";

import { useOfficeDashboard } from "../hooks/use-office-data";
import { useReviewSuggestions } from "../hooks/use-review";
import type { ReviewSuggestionStatus } from "../services/review.service";

type Scope = { engagement?: string; kind?: string; period?: string };

/** Yalnızca toplam için tek satırlık sayım sorgusu. */
function useStatusTotal(scope: Scope, status: ReviewSuggestionStatus) {
  // Sessiz: kuyruk yüklenemezse sayım "…" yazar; dört ayrı hata bildirimi
  // açılmaz (kuyruğun kendi hata durumu yeterli).
  const q = useReviewSuggestions(
    { ...scope, status, limit: 1, offset: 0 },
    true,
    true,
  );
  return q.data?.total;
}

/**
 * Kuyruğun durum dökümü (#669 B): bekleyen, onaylanan, düzeltilen,
 * reddedilen ve eşik üstü otomatik onaylanan öneriler, kuyruktaki süzgeçle
 * (mükellef, tür, dönem) aynı tanımla sayılır. Süzgeç ofisin tamamından
 * darsa menüdeki ve panodaki toplam (bütün mükellefler, türler ve dönemler)
 * ayrıca yazılır: iki sayı farklıysa nedeni görünür.
 */
export function ReviewCounts({
  scope,
  pending,
  filtered,
}: {
  scope: Scope;
  /** Kuyruğun bu süzgeçteki bekleyen sayısı (liste sorgusunun toplamı). */
  pending: number;
  /** Mükellef, tür ya da dönem süzgeci seçili. */
  filtered: boolean;
}) {
  const { t } = useLocale();
  const accepted = useStatusTotal(scope, "accepted");
  const corrected = useStatusTotal(scope, "corrected");
  const rejected = useStatusTotal(scope, "rejected");
  const auto = useStatusTotal(scope, "auto_accepted");
  const office = useOfficeDashboard().data?.review?.pending;
  const n = (v: number | undefined) => (v === undefined ? "…" : String(v));
  return (
    <p
      className="text-muted-foreground text-xs tabular-nums"
      data-testid="review-counts"
    >
      {t("office.review.counts", {
        pending: String(pending),
        accepted: n(accepted),
        corrected: n(corrected),
        rejected: n(rejected),
        auto: n(auto),
      })}
      {filtered && office !== undefined ? (
        <span data-testid="review-office-total">
          {" · "}
          {t("office.review.counts_office", { count: String(office) })}
        </span>
      ) : null}
    </p>
  );
}
