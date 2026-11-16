"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import { ALL_PERIODS, isPeriod, type PeriodChoice } from "./period";

type PeriodState = {
  /** Ofis (slug, küçük harf) başına seçili dönem. */
  byOffice: Record<string, PeriodChoice>;
  setPeriod: (slug: string, choice: PeriodChoice) => void;
};

/**
 * Seçim tarayıcıda ofis başına saklanır (localStorage): sayfa yenilense de
 * dönem korunur, başka ofise geçince o ofisin seçimi gelir.
 */
export const usePeriodStore = create<PeriodState>()(
  persist(
    (set) => ({
      byOffice: {},
      setPeriod: (slug, choice) =>
        set((s) => ({
          byOffice: {
            ...s.byOffice,
            [slug.toLowerCase()]:
              choice === ALL_PERIODS || isPeriod(choice) ? choice : ALL_PERIODS,
          },
        })),
    }),
    {
      name: "hakan.office-period",
      version: 1,
      storage: createJSONStorage(() => localStorage),
    },
  ),
);
