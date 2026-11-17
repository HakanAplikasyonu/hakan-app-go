"use client";

import type { ReactNode } from "react";

import {
  formatCellDate,
  formatCellMoney,
  type CellDateStyle,
} from "@/components/tables/cell-format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Tutar hücresi: JetBrains Mono + hizalı rakam, satır kırılmaz. */
export function MoneyCell({
  value,
  currency = "TRY",
  className,
}: {
  value: string | number | null | undefined;
  currency?: string;
  className?: string;
}) {
  return (
    <span className={cn("figure whitespace-nowrap", className)}>
      {formatCellMoney(value, currency)}
    </span>
  );
}

/** Tarih hücresi: uygulama diline göre (tr → tr-TR). */
export function DateCell({
  value,
  style = "short",
  className,
}: {
  value: string | Date | null | undefined;
  style?: CellDateStyle;
  className?: string;
}) {
  const { locale } = useLocale();
  return (
    <span className={cn("figure whitespace-nowrap", className)}>
      {formatCellDate(value, locale, style)}
    </span>
  );
}

/** Rozet tonları: tint zemin + aynı ailenin metin token'ı (dört temada 4.5:1). */
export type StatusTone = "neutral" | "info" | "ok" | "warn" | "danger";

export const statusToneClass: Record<StatusTone, string> = {
  neutral: "bg-surface-3 text-text-2",
  info: "bg-brand-tint text-brand-text",
  ok: "bg-ok-tint text-ok",
  warn: "bg-warn-tint text-warn",
  danger: "bg-danger-tint text-danger",
};

export function StatusPill({
  tone = "neutral",
  dot,
  className,
  children,
}: {
  tone?: StatusTone;
  dot?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        statusToneClass[tone],
        className,
      )}
    >
      {dot ? (
        <span className="size-1.5 rounded-full bg-current" aria-hidden />
      ) : null}
      {children}
    </span>
  );
}
