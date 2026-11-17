"use client";

import type { ColumnDef, Row, RowData } from "@tanstack/react-table";
import type { ReactNode } from "react";

import {
  compareDate,
  compareDecimal,
  type CellDateStyle,
} from "@/components/tables/cell-format";
import {
  DateCell,
  MoneyCell,
  StatusPill,
  type StatusTone,
} from "@/components/tables/cells";
import { createColumn } from "@/components/tables/create-column";
import type { DataTableColumnMeta } from "@/components/tables/types";
import { useLocale } from "@/providers/locale-provider";

type BaseHelperOptions = {
  id: string;
  labelKey: string;
  label?: string;
  enableSorting?: boolean;
  enableHiding?: boolean;
  defaultHidden?: boolean;
  gridPrimary?: boolean;
  gridSecondary?: boolean;
  meta?: DataTableColumnMeta;
  /** Alt toplam satırı (tfoot) içeriği. */
  footer?: ColumnDef<RowData, unknown>["footer"];
  size?: number;
};

/**
 * Para kolonu (#381): ondalık string tutar, kuruş hassasiyetinde tr-TR
 * biçim, sağa hizalı başlık ve hücre, sayısal sıralama.
 */
export function createMoneyColumn<TData extends RowData>(
  options: BaseHelperOptions & {
    accessor: (row: TData) => string | number | null | undefined;
    currency?: string | ((row: TData) => string | undefined);
    /** Hücreye ek sınıf (ör. kalın toplam). */
    className?: string;
  },
): ColumnDef<TData, unknown> {
  const { accessor, currency, className, meta, footer, ...rest } = options;
  const currencyOf = (row: TData) =>
    (typeof currency === "function" ? currency(row) : currency) ?? "TRY";
  return createColumn<TData, unknown>({
    ...rest,
    accessorFn: (row) => accessor(row) ?? "",
    sortingFn: (a: Row<TData>, b: Row<TData>, columnId: string) =>
      compareDecimal(
        a.getValue(columnId) as string,
        b.getValue(columnId) as string,
      ),
    cell: ({ row }) => (
      <MoneyCell
        value={accessor(row.original)}
        currency={currencyOf(row.original)}
        className={className}
      />
    ),
    footer: footer as ColumnDef<TData, unknown>["footer"],
    meta: { align: "end", ...meta },
  });
}

/** Tarih kolonu: ISO değer, uygulama diline göre biçim, tarihe göre sıralama. */
export function createDateColumn<TData extends RowData>(
  options: BaseHelperOptions & {
    accessor: (row: TData) => string | null | undefined;
    style?: CellDateStyle;
    className?: string;
  },
): ColumnDef<TData, unknown> {
  const { accessor, style, className, meta, footer, ...rest } = options;
  return createColumn<TData, unknown>({
    ...rest,
    accessorFn: (row) => accessor(row) ?? "",
    sortingFn: (a: Row<TData>, b: Row<TData>, columnId: string) =>
      compareDate(
        a.getValue(columnId) as string,
        b.getValue(columnId) as string,
      ),
    cell: ({ row }) => (
      <DateCell
        value={accessor(row.original)}
        style={style}
        className={className}
      />
    ),
    footer: footer as ColumnDef<TData, unknown>["footer"],
    meta: { ...meta },
  });
}

export type StatusVariant = {
  /** i18n anahtarı (rozet metni ve süzgeç seçeneği). */
  labelKey: string;
  tone?: StatusTone;
  dot?: boolean;
};

function StatusCell({
  value,
  variants,
  fallback,
}: {
  value: string;
  variants: Record<string, StatusVariant>;
  fallback?: StatusVariant;
}) {
  const { t } = useLocale();
  const v = variants[value] ?? fallback;
  if (!v) return <span className="text-muted-foreground text-xs">{value}</span>;
  return (
    <StatusPill tone={v.tone} dot={v.dot}>
      {t(v.labelKey)}
    </StatusPill>
  );
}

/**
 * Durum kolonu: değer → rozet eşlemesi. `filterable` açıksa araç çubuğunda
 * seçenekleri değişkenlerden üretilen çok seçimli (faceted) süzgeç çıkar.
 */
export function createStatusColumn<TData extends RowData>(
  options: BaseHelperOptions & {
    accessor: (row: TData) => string | null | undefined;
    variants: Record<string, StatusVariant>;
    fallback?: StatusVariant;
    filterable?: boolean;
    /** Rozet yerine özel çizim (ör. mevcut alan rozeti). */
    render?: (value: string, row: TData) => ReactNode;
  },
): ColumnDef<TData, unknown> {
  const {
    accessor,
    variants,
    fallback,
    filterable,
    render,
    meta,
    footer,
    ...rest
  } = options;
  return createColumn<TData, unknown>({
    ...rest,
    accessorFn: (row) => accessor(row) ?? "",
    filterVariant: filterable ? "faceted" : undefined,
    filterOptions: filterable
      ? Object.entries(variants).map(([value, v]) => ({
          value,
          label: v.labelKey,
          labelKey: v.labelKey,
        }))
      : undefined,
    cell: ({ row, getValue }) => {
      const value = String(getValue() ?? "");
      return render ? (
        render(value, row.original)
      ) : (
        <StatusCell value={value} variants={variants} fallback={fallback} />
      );
    },
    footer: footer as ColumnDef<TData, unknown>["footer"],
    meta: { ...meta },
  });
}

/**
 * Satır aksiyonları kolonu (`id: "actions"`): sıralanmaz, gizlenmez, kartta
 * alt bilgiye yerleşir. Başlık ekran okuyucuya `labelKey` ile okunur.
 */
export function createActionsColumn<TData extends RowData>(options: {
  labelKey?: string;
  cell: (row: TData) => ReactNode;
}): ColumnDef<TData, unknown> {
  return createColumn<TData, unknown>({
    id: "actions",
    labelKey: options.labelKey ?? "common.actions",
    enableSorting: false,
    enableHiding: false,
    header: () => null,
    cell: ({ row }) => options.cell(row.original),
    meta: { align: "end", srOnlyHeader: true },
  });
}
