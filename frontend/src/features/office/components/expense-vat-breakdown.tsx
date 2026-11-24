"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatMoney, sum } from "../lib/decimal";
import type { Expense } from "../services/receipts.service";

type Line = Expense["vat_lines"][number];
type ReceiptRead = Expense["receipt_read"];

/**
 * Ön muhasebe programından gelen masrafın oranları fiş görselinden okundu
 * mu (#780): "fişten okundu", "fiş okunamadı / tutmadı" (programın tek oranı
 * kaldı) ya da "fiş okunuyor".
 */
export function ReceiptReadBadge({ status }: { status: ReceiptRead }) {
  const { t } = useLocale();
  if (!status) return null;
  const variant =
    status === "read"
      ? "success"
      : status === "pending"
        ? "secondary"
        : "warning";
  const label =
    status === "read"
      ? t("office.expenses.receipt_read")
      : status === "pending"
        ? t("office.expenses.receipt_pending")
        : t("office.expenses.receipt_not_read");
  return (
    <Badge
      variant={variant}
      className="w-fit text-[11px] font-normal"
      title={
        status === "mismatch" || status === "unreadable"
          ? t("office.expenses.receipt_not_read_hint")
          : undefined
      }
      data-testid="expense-receipt-read"
      data-status={status}
    >
      {label}
    </Badge>
  );
}

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
  receiptRead,
}: {
  lines: Line[];
  currency: string;
  className?: string;
  receiptRead?: ReceiptRead;
}) {
  const { t } = useLocale();
  const badge = receiptRead ? <ReceiptReadBadge status={receiptRead} /> : null;
  if (!lines.length)
    return (
      <span
        className={cn("text-text-3 flex flex-col gap-1 text-xs", className)}
        data-testid="expense-vat-none"
      >
        {t("office.expenses.vat_no_breakdown")}
        {badge}
      </span>
    );
  const money = (v: string) => formatMoney(v, currency);
  const table = (
    <div
      role="table"
      aria-label={t("office.expenses.vat_breakdown")}
      className={cn(
        "grid grid-cols-[auto_auto_auto] gap-x-3 gap-y-0.5 text-xs tabular-nums",
        badge ? undefined : className,
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
  if (!badge) return table;
  return (
    <div className={cn("flex flex-col gap-1", className)}>
      {table}
      {badge}
    </div>
  );
}
