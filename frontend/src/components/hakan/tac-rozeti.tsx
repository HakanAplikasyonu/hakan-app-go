"use client";

import type { CSSProperties } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Belge yaşam döngüsü (docs/tasarim/README.md, "Üç taraf, bir taç"). */
export type TacDurumu =
  "bekleniyor" | "yuklendi" | "onaylandi" | "aktarildi" | "sorun";

export type TacBoyutu = 16 | 30 | 72;

const etiketAnahtari: Record<TacDurumu, string> = {
  bekleniyor: "common.crown_waiting",
  yuklendi: "common.crown_uploaded",
  onaylandi: "common.crown_approved",
  aktarildi: "common.crown_exported",
  sorun: "common.crown_problem",
};

/** Ölçüler tuvalden (Kristal.dc.html): genişlik, aralık, tepe pahı. */
const olcu: Record<TacBoyutu, { w: number; gap: number; tepe: number }> = {
  16: { w: 5, gap: 2, tepe: 3 },
  30: { w: 9, gap: 3, tepe: 5 },
  72: { w: 22, gap: 6, tepe: 10 },
};

/**
 * Dolgular: [sol, orta, sağ]. Sol mükellef (`--c-light`), sağ mali müşavir
 * (`--c-dark`), orta beyanname / Luca (`--c-mid`).
 */
function dolgular(
  durum: TacDurumu,
  boyut: TacBoyutu,
): [string?, string?, string?] {
  switch (durum) {
    case "yuklendi":
      return ["var(--c-light)"];
    case "onaylandi":
      return ["var(--c-light)", undefined, "var(--c-dark)"];
    case "aktarildi":
      return ["var(--c-light)", "var(--c-mid)", "var(--c-dark)"];
    case "sorun":
      return boyut === 16
        ? ["var(--danger)", "var(--danger)", "var(--danger)"]
        : ["var(--c-light)", undefined, "var(--danger)"];
    default:
      return [];
  }
}

type TacRozetiProps = {
  durum: TacDurumu;
  boyut?: TacBoyutu;
  /** Görünür metin etiketi. Kapalıysa etiket `aria-label` olur. */
  etiket?: boolean;
  /** Varsayılan etiketin yerine (ör. sorunun nedeni: "Okunamadı"). */
  metin?: string;
  className?: string;
};

/**
 * Taç rozeti: üç kristal (yanlar 11/16, orta 16/16), belge aşamasına göre
 * dolar. Durum yalnızca renkle anlatılmaz; her zaman bir metin etiketi ya da
 * `aria-label` taşır.
 */
export function TacRozeti({
  durum,
  boyut = 16,
  etiket = false,
  metin,
  className,
}: TacRozetiProps) {
  const { t } = useLocale();
  const label = metin ?? t(etiketAnahtari[durum]);
  const { w, gap, tepe } = olcu[boyut];
  const yan = Math.round((boyut * 11) / 16);
  const bos = boyut === 72 ? "var(--surface-3)" : "var(--line-strong)";
  const [sol, orta, sag] = dolgular(durum, boyut);
  const kristal = (h: number, renk?: string): CSSProperties => ({
    width: w,
    height: h,
    background: renk ?? bos,
    opacity: durum === "sorun" && boyut === 16 ? 0.85 : undefined,
    clipPath: `polygon(0 ${tepe}px, 50% 0, 100% ${tepe}px, 100% 100%, 0 100%)`,
    transition: "background var(--dur-slow) var(--ease-out)",
  });

  const tac = (
    <span
      role={etiket ? undefined : "img"}
      aria-label={etiket ? undefined : label}
      aria-hidden={etiket ? true : undefined}
      data-durum={durum}
      className={cn("inline-flex shrink-0 items-end", !etiket && className)}
      style={{ gap, height: boyut }}
    >
      <i className="block" style={kristal(yan, sol)} />
      <i className="block" style={kristal(boyut, orta)} />
      <i className="block" style={kristal(yan, sag)} />
    </span>
  );

  if (!etiket) return tac;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 text-sm font-semibold",
        durum === "sorun" && "text-danger",
        className,
      )}
    >
      {tac}
      <span>{label}</span>
    </span>
  );
}
