"use client";

import { useCallback } from "react";

import { useLocale } from "@/providers/locale-provider";

import { periodLabel } from "./period";

/**
 * Ekranda dönem metni (#455): "2026-10" → "Ekim 2026". Boş değer için
 * `fallback`; dönem olmayan değer aynen döner.
 */
export function usePeriodText() {
  const { locale } = useLocale();
  return useCallback(
    (period: string | null | undefined, fallback = "—") =>
      period ? periodLabel(period, locale) : fallback,
    [locale],
  );
}
