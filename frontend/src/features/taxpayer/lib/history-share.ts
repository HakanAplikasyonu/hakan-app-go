import type { AppLocale } from "@/config/i18n";
import type { HistoryShare } from "@/features/taxpayer/services/history-shares.service";
import { date } from "@/lib/utils/format";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** "Tümü", "01.01.2025 – 31.12.2025", "01.01.2025 sonrası", … */
export function historyShareRangeLabel(
  share: Pick<HistoryShare, "from_date" | "to_date">,
  t: Translate,
  locale: AppLocale,
) {
  const from = share.from_date
    ? date(share.from_date, "dd.MM.yyyy", locale)
    : null;
  const to = share.to_date ? date(share.to_date, "dd.MM.yyyy", locale) : null;
  if (from && to) return `${from} – ${to}`;
  if (from) return t("taxpayer.history_range_from", { from });
  if (to) return t("taxpayer.history_range_to", { to });
  return t("taxpayer.history_range_all");
}
