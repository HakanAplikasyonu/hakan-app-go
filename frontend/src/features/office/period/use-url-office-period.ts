"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef } from "react";

import { isPeriod } from "./period";
import { useOfficePeriod, type OfficePeriod } from "./use-office-period";

/** Sayfanın dönemini taşıyan sorgu anahtarı (`?period=YYYY-MM`). */
export const PERIOD_PARAM = "period";

/**
 * `useOfficePeriod` ile aynı, ama seçili dönem adreste de durur (#669 D):
 * bağlantı ya da geri tuşuyla gelen `?period=` global seçiciye işlenir;
 * seçici (sayfadaki ya da üst çubuktaki) değişince adres güncellenir.
 * "Tüm dönemler" adrese yazılmaz. Yazım `router.replace` ile, kaydırma
 * korunur.
 *
 * `useSearchParams` kullandığı için çağıran bileşen Suspense içinde olmalı.
 */
export function useUrlOfficePeriod(): OfficePeriod {
  const office = useOfficePeriod();
  const { period: stored, setPeriod } = office;
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const raw = params.get(PERIOD_PARAM);
  const fromUrl = isPeriod(raw) ? raw : undefined;

  // Adresle gelen dönem kazanır: global seçici ona döner.
  useEffect(() => {
    if (fromUrl && fromUrl !== stored) setPeriod(fromUrl);
    // Yalnızca adres değişince; seçicinin değişimi aşağıda.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fromUrl]);

  // Seçici değişince adres izler (ilk yüklemede adrese dokunulmaz).
  const prev = useRef(stored);
  useEffect(() => {
    if (prev.current === stored) return;
    prev.current = stored;
    if (stored === fromUrl) return;
    const next = new URLSearchParams(params.toString());
    if (stored) next.set(PERIOD_PARAM, stored);
    else next.delete(PERIOD_PARAM);
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    // Yalnızca seçici değişince; adres, yol ve yönlendirici bu render'ınki.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stored]);

  const period = fromUrl ?? stored;
  return {
    ...office,
    choice: period ?? office.choice,
    period,
    focus: period ?? office.focus,
  };
}
