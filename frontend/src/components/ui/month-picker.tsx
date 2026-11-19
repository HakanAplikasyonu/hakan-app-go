"use client";

import { format } from "date-fns";
import { CalendarIcon, ChevronLeft, ChevronRight } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { dateFnsLocale } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type MonthPickerProps = {
  id?: string;
  /** `yyyy-MM`; boşsa seçim yok. */
  value?: string;
  onChange?: (value: string) => void;
  /** Seçilebilecek en erken ve en geç ay (`yyyy-MM`, dâhil). */
  min?: string;
  max?: string;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  align?: React.ComponentProps<typeof PopoverContent>["align"];
  "aria-label"?: string;
  "aria-invalid"?: boolean;
};

const MONTHS = Array.from({ length: 12 }, (_, i) => i);

export function parseMonthValue(
  value: string | undefined,
): { year: number; month: number } | undefined {
  const m = /^(\d{4})-(\d{2})$/.exec(value ?? "");
  if (!m) return undefined;
  const month = Number(m[2]) - 1;
  if (month < 0 || month > 11) return undefined;
  return { year: Number(m[1]), month };
}

export function formatMonthValue(year: number, month: number): string {
  return `${String(year).padStart(4, "0")}-${String(month + 1).padStart(2, "0")}`;
}

/**
 * Ay seçici (Popover + ay ızgarası); native `type="month"` yerine kullanılır.
 * Değer her zaman `yyyy-MM` biçimindedir. Ay adları uygulama diline göre gelir.
 */
export function MonthPicker({
  id,
  value,
  onChange,
  min,
  max,
  placeholder,
  disabled,
  className,
  align = "start",
  "aria-label": ariaLabel,
  "aria-invalid": ariaInvalid,
}: MonthPickerProps) {
  const { locale, t } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const selected = parseMonthValue(value);
  const [open, setOpen] = React.useState(false);
  const [year, setYear] = React.useState(
    () => selected?.year ?? new Date().getFullYear(),
  );

  const handleOpenChange = (next: boolean) => {
    if (next) setYear(selected?.year ?? new Date().getFullYear());
    setOpen(next);
  };

  const isDisabled = (month: number) => {
    const v = formatMonthValue(year, month);
    return (min != null && v < min) || (max != null && v > max);
  };
  const minYear = parseMonthValue(min)?.year;
  const maxYear = parseMonthValue(max)?.year;

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={ariaLabel}
          aria-invalid={ariaInvalid}
          data-empty={!selected}
          className={cn(
            "data-[empty=true]:text-muted-foreground h-9 w-full justify-start gap-2 px-3 font-normal",
            className,
          )}
        >
          <CalendarIcon className="text-muted-foreground size-4" />
          {selected
            ? format(new Date(selected.year, selected.month, 1), "LLLL yyyy", {
                locale: dfLocale,
              })
            : (placeholder ?? t("form.pick_month"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-3" align={align}>
        <div className="mb-2 flex items-center justify-between">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("form.prev_year")}
            disabled={minYear != null && year <= minYear}
            onClick={() => setYear((y) => y - 1)}
          >
            <ChevronLeft />
          </Button>
          <span className="text-sm font-medium tabular-nums" aria-live="polite">
            {year}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("form.next_year")}
            disabled={maxYear != null && year >= maxYear}
            onClick={() => setYear((y) => y + 1)}
          >
            <ChevronRight />
          </Button>
        </div>
        <div className="grid grid-cols-3 gap-1">
          {MONTHS.map((month) => {
            const active = selected?.year === year && selected.month === month;
            return (
              <Button
                key={month}
                variant={active ? "default" : "ghost"}
                size="sm"
                aria-pressed={active}
                disabled={isDisabled(month)}
                onClick={() => {
                  onChange?.(formatMonthValue(year, month));
                  setOpen(false);
                }}
              >
                {format(new Date(year, month, 1), "LLL", { locale: dfLocale })}
              </Button>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}
