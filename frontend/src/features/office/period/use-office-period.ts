"use client";

import { useCallback } from "react";

import { useTenant } from "@/features/tenant/providers/tenant-provider";

import {
  ALL_PERIODS,
  declarationPeriod,
  isPeriod,
  type PeriodChoice,
} from "./period";
import { usePeriodStore } from "./period-store";

export type OfficePeriod = {
  /** Seçicinin değeri: `all` ya da YYYY-MM. */
  choice: PeriodChoice;
  /** Listelere giden `period` (Tüm dönemler seçiliyse undefined). */
  period: string | undefined;
  /** Tek döneme ihtiyaç duyan ekranlar için: seçili dönem ya da beyan dönemi. */
  focus: string;
  setPeriod: (choice: PeriodChoice) => void;
};

/**
 * Üst çubuktaki global dönem seçicinin değeri. Varsayılan "Tüm dönemler":
 * listeler bugünkü gibi bütün dönemleri gösterir; seçim yapılınca hepsi
 * aynı döneme süzülür. Genel bakış ve yevmiye gibi tek dönemlik ekranlar
 * "Tüm dönemler"de beyan dönemini (önceki ay) kullanır.
 */
export function useOfficePeriod(): OfficePeriod {
  const { slug } = useTenant();
  const key = slug.toLowerCase();
  const stored = usePeriodStore((s) => s.byOffice[key]);
  const set = usePeriodStore((s) => s.setPeriod);
  const choice = isPeriod(stored) ? stored : ALL_PERIODS;
  const period = choice === ALL_PERIODS ? undefined : choice;
  const setPeriod = useCallback(
    (next: PeriodChoice) => set(key, next),
    [set, key],
  );
  return { choice, period, focus: period ?? declarationPeriod(), setPeriod };
}
