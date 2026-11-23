"use client";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatMoney, sum } from "../lib/decimal";
import type { Expense } from "../services/receipts.service";

type Line = Expense["vat_lines"][number];

/**
 * Masrafın KDV oran kırılımı (#780): her oran için matrah ve KDV, birden
 * çok oranda toplam satırı. Kırılımı olmayan masrafta (kaynak program oran
 * göndermedi, fişten okunamadı) "kırılım yok" yazar; toplam KDV'den oran
 * türetilmez.
 */
export function ExpenseVatBreakdown({
  lines,
  currency,
  className,
}: {
  lines: Line[];
  currency: string;
  className?: string;
}) {
  const { t } = useLocale();
  if (!lines.length)
    return (
      <span
        className={cn("text-text-3 text-xs", className)}
        data-testid="expense-vat-none"
      >
        {t("office.expenses.vat_no_breakdown")}
      </span>
    );
  const money = (v: string) => formatMoney(v, currency);
  return (
    <div
      role="table"
      aria-label={t("office.expenses.vat_breakdown")}
      className={cn(
        "grid grid-cols-[auto_auto_auto] gap-x-3 gap-y-0.5 text-xs tabular-nums",
        className,
      )}
      data-testid="expense-vat-breakdown"
    >
      <div role="row" className="text-text-3 contents">
        <span role="columnheader">{t("office.expenses.vat_rate")}</span>
        <span role="columnheader" className="text-end">
          {t("office.expenses.vat_base")}
        </span>
        <span role="columnheader" className="text-end">
          {t("office.expenses.col_tax")}
        </span>
      </div>
      {lines.map((l) => (
        <div role="row" key={l.tax_rate} className="contents">
          <span role="cell" className="text-text-2">
            %{l.tax_rate}
          </span>
          <span role="cell" className="text-end whitespace-nowrap">
            {money(l.tax_base)}
          </span>
          <span role="cell" className="text-end whitespace-nowrap">
            {money(l.tax_amount)}
          </span>
        </div>
      ))}
      {lines.length > 1 ? (
        <div role="row" className="contents font-semibold">
          <span role="cell">{t("office.expenses.vat_total_row")}</span>
          <span role="cell" className="text-end whitespace-nowrap">
            {money(sum(lines.map((l) => l.tax_base)))}
          </span>
          <span role="cell" className="text-end whitespace-nowrap">
            {money(sum(lines.map((l) => l.tax_amount)))}
          </span>
        </div>
      ) : null}
    </div>
  );
}
