"use client";

import { useEffect, useState } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type KristalYukleyiciProps = {
  /** Orta kristalin yüksekliği (px). Varsayılan 40 (tuval). */
  boyut?: number;
  /** Erişilebilir ad; varsayılan "Yükleniyor…". */
  etiket?: string;
  /**
   * 400 ms'den kısa işlemlerde gösterilmez (README, "Yükleme animasyonu").
   * 0 verilirse hemen görünür.
   */
  gecikme?: number;
  className?: string;
};

/**
 * Kristal yükselişi: üç kristal sırayla yükselir, 1.4 sn döngü, 180 ms
 * gecikme. "Hareketi azalt" açıksa yalnızca opaklık geçişi (hakan.css).
 * Liste ve tablolar bunun yerine iskelet (Skeleton) kullanır.
 */
export function KristalYukleyici({
  boyut = 40,
  etiket,
  gecikme = 400,
  className,
}: KristalYukleyiciProps) {
  const { t } = useLocale();
  const [gorunur, setGorunur] = useState(gecikme <= 0);

  useEffect(() => {
    if (gecikme <= 0) return;
    const id = window.setTimeout(() => setGorunur(true), gecikme);
    return () => window.clearTimeout(id);
  }, [gecikme]);

  const label = etiket ?? t("common.loading");
  const w = Math.max(4, Math.round((boyut * 12) / 40));
  const yan = Math.round((boyut * 28) / 40);
  const tepe = Math.max(2, Math.round((boyut * 6) / 40));
  const gap = Math.max(2, Math.round(boyut / 10));
  const clip = `polygon(0 ${tepe}px, 50% 0, 100% ${tepe}px, 100% 100%, 0 100%)`;

  return (
    <span
      role="status"
      aria-live="polite"
      aria-label={label}
      className={cn("hk-yukleyici inline-flex items-end", className)}
      style={{ gap, height: boyut, visibility: gorunur ? "visible" : "hidden" }}
    >
      {[
        { h: yan, bg: "var(--c-light)", d: "0s" },
        { h: boyut, bg: "var(--c-mid)", d: "0.18s" },
        { h: yan, bg: "var(--c-dark)", d: "0.36s" },
      ].map((k) => (
        <i
          key={k.d}
          data-hk-motion=""
          className="block"
          style={{
            width: w,
            height: k.h,
            background: k.bg,
            clipPath: clip,
            animationDelay: k.d,
          }}
        />
      ))}
    </span>
  );
}
