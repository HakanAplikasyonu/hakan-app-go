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

/** Rozet noktası: tonun rengi (sade rozetin anlamını metin taşır). */
export const statusDotClass: Record<StatusTone, string> = {
  neutral: "bg-step-pending",
  info: "bg-brand-text",
  ok: "bg-ok",
  warn: "bg-warn",
  danger: "bg-danger",
};

/**
 * Durum rozeti (EasyTrade dili): ince çerçeve, yüzey zemini, küçük renkli
 * nokta ve kısa metin. Renk tek başına anlam taşımaz; metin durumu söyler.
 * `dot` geriye uyum için durur (nokta hep var).
 */
export function StatusPill({
  tone = "neutral",
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
      data-tone={tone}
      className={cn(
        "border-line bg-surface inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        tone === "neutral" ? "text-text-2" : "text-foreground",
        className,
      )}
    >
      <span
        className={cn("size-1.5 shrink-0 rounded-full", statusDotClass[tone])}
        aria-hidden
      />
      {children}
    </span>
  );
}
