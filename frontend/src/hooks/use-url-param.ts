"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback } from "react";

/**
 * `useState` gibi tek bir seçim, ama değeri sorgu dizesinde (#669 D):
 * başka sayfaya gidip geri tuşuyla (ya da bağlantıyla) dönünce aynı seçim
 * açılır. Varsayılan değer adrese yazılmaz; `allowed` verilirse dışındaki
 * değerler varsayılana düşer. Yazım `router.replace` ile (geçmiş şişmez),
 * kaydırma korunur. Liste süzgeçleri için `useUrlListState` kullanın.
 *
 * `useSearchParams` kullandığı için çağıran bileşen Suspense içinde olmalı.
 */
export function useUrlParam<T extends string = string>(
  key: string,
  fallback: NoInfer<T>,
  allowed?: readonly NoInfer<T>[],
): [T, (next: T) => void] {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const raw = params.get(key);
  const value =
    raw && (!allowed || (allowed as readonly string[]).includes(raw))
      ? (raw as T)
      : fallback;

  const setValue = useCallback(
    (v: T) => {
      const next = new URLSearchParams(params.toString());
      if (!v || v === fallback) next.delete(key);
      else next.set(key, v);
      const qs = next.toString();
      if (qs === params.toString()) return;
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [fallback, key, params, pathname, router],
  );

  return [value, setValue];
}
