"use client";

import { CalendarRange } from "lucide-react";
import { useMemo } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { ALL_PERIODS, periodLabel, recentPeriods } from "./period";
import { useOfficePeriod } from "./use-office-period";

/**
 * Üst çubuktaki global dönem seçici (#326). Değer bütün listelere `period`
 * olarak gider; "Tüm dönemler" süzmez.
 */
export function PeriodPicker({
  className,
  compact = false,
}: {
  className?: string;
  /** Telefon: yalnızca değer (etiket ekran okuyucuda). */
  compact?: boolean;
}) {
  const { t, locale } = useLocale();
  const { choice, setPeriod } = useOfficePeriod();
  const options = useMemo(() => {
    const list = recentPeriods(18);
    return choice !== ALL_PERIODS && !list.includes(choice)
      ? [choice, ...list]
      : list;
  }, [choice]);

  return (
    <Select value={choice} onValueChange={setPeriod}>
      <SelectTrigger
        aria-label={t("office.period.label")}
        className={cn(
          "bg-card w-auto gap-2 pointer-coarse:h-11",
          compact ? "px-2.5" : "px-3",
          className,
        )}
      >
        <CalendarRange className="text-muted-foreground size-4" aria-hidden />
        {compact ? null : (
          <span className="text-muted-foreground text-xs">
            {t("office.period.label")}
          </span>
        )}
        <span className="font-mono text-[13px] font-semibold tabular-nums">
          {choice === ALL_PERIODS ? t("office.period.all_short") : choice}
        </span>
      </SelectTrigger>
      <SelectContent className="max-h-80">
        <SelectItem value={ALL_PERIODS}>{t("office.period.all")}</SelectItem>
        {options.map((p) => (
          <SelectItem key={p} value={p}>
            <span className="font-mono tabular-nums">{p}</span>
            <span className="text-muted-foreground ms-2 capitalize">
              {periodLabel(p, locale)}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
