"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ReportRange } from "@/features/reports/services/reports.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const MONTHS_BACK = 24;
const YEARS_BACK = 5;

function shift(period: string, months: number) {
  const [y, m] = period.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1 + months, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** Ofis saatiyle bu ay (YYYY-MM). */
export function thisMonth(now = new Date()) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(now);
  return `${parts.find((p) => p.type === "year")?.value}-${parts.find((p) => p.type === "month")?.value}`;
}

export function monthLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { month: "long", year: "numeric", timeZone: "UTC" },
  );
}

/** Aylık / yıllık seçimi ve dönem listesi. */
export function ReportRangePicker({
  value,
  onChange,
}: {
  value: ReportRange;
  onChange: (next: ReportRange) => void;
}) {
  const { t, locale } = useLocale();
  const now = thisMonth();
  const months = Array.from({ length: MONTHS_BACK }, (_, i) => shift(now, -i));
  const year = Number(now.slice(0, 4));
  const years = Array.from({ length: YEARS_BACK }, (_, i) => String(year - i));
  const current = value.kind === "month" ? value.period : value.year;
  const options = value.kind === "month" ? months : years;
  if (!options.includes(current)) options.unshift(current);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div
        role="radiogroup"
        aria-label={t("reports.range_kind")}
        className="border-line bg-surface-2 inline-flex rounded-md border p-0.5"
      >
        {(["month", "year"] as const).map((kind) => (
          <button
            key={kind}
            type="button"
            role="radio"
            aria-checked={value.kind === kind}
            onClick={() =>
              onChange(
                kind === "month"
                  ? {
                      kind,
                      period:
                        value.kind === "year"
                          ? value.year === String(year)
                            ? shift(now, -1)
                            : `${value.year}-12`
                          : value.period,
                    }
                  : {
                      kind,
                      year:
                        value.kind === "month"
                          ? value.period.slice(0, 4)
                          : value.year,
                    },
              )
            }
            className={cn(
              "focus-visible:ring-ring min-h-9 rounded px-3 text-sm focus-visible:ring-2 focus-visible:outline-none",
              value.kind === kind
                ? "bg-surface text-foreground font-semibold shadow-sm"
                : "text-text-2 hover:text-foreground",
            )}
          >
            {kind === "month"
              ? t("reports.kind_month")
              : t("reports.kind_year")}
          </button>
        ))}
      </div>
      <Select
        value={current}
        onValueChange={(v) =>
          onChange(
            value.kind === "month"
              ? { kind: "month", period: v }
              : { kind: "year", year: v },
          )
        }
      >
        <SelectTrigger
          className="min-w-44"
          aria-label={t("reports.range_value")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o} value={o}>
              {value.kind === "month" ? monthLabel(o, locale) : o}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
