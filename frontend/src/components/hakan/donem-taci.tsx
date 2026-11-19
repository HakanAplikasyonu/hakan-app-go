"use client";

import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/utils";

const tepe = "polygon(0 18px, 50% 0, 100% 18px, 100% 100%, 0 100%)";

type Kristal = {
  /** 0..1; bilinmiyorsa boş kalır. */
  oran: number | null;
  /** Kristalin altındaki değer ("%92", "24/38"). */
  deger: string;
  etiket: string;
  renk: string;
  yukseklik: number;
};

function KristalSutun({ k }: { k: Kristal }) {
  const pct =
    k.oran === null ? 0 : Math.round(Math.min(1, Math.max(0, k.oran)) * 100);
  const kap: CSSProperties = {
    position: "absolute",
    inset: 0,
    background: "var(--surface-3)",
    clipPath: tepe,
  };
  return (
    <div className="flex flex-col items-center gap-2">
      <div
        aria-hidden
        className="relative flex w-12 flex-col justify-end sm:w-16"
        style={{ height: k.yukseklik }}
      >
        <span style={kap} />
        <span
          className="relative motion-safe:transition-[height] motion-safe:duration-300"
          style={{ height: `${pct}%`, background: k.renk, clipPath: tepe }}
        />
      </div>
      <span className="text-text-2 text-center text-xs leading-tight">
        <b className="text-foreground block font-mono text-sm tabular-nums">
          {k.deger}
        </b>
        {k.etiket}
      </span>
    </div>
  );
}

/**
 * Dönem tacı (bilesenler.md): pano kahraman kartı, kesik köşe. Üç kristal
 * logodaki sırayla: sol mükellef verisi (`--c-light`), orta beyana hazır
 * (`--c-mid`), sağ müşavir onayı (`--c-dark`). Değerler kristallerin altında
 * metin olarak da yazılır; görsel yalnızca süs (aria-hidden).
 */
export function DonemTaci({
  baslik,
  hazir,
  toplam,
  veri,
  onay,
  etiketler,
  ozet,
  eylemler,
  className,
}: {
  baslik: ReactNode;
  hazir: number;
  toplam: number;
  /** 0..1 ya da bilinmiyorsa null. */
  veri: number | null;
  onay: number | null;
  etiketler: {
    hazirSatiri: ReactNode;
    veri: string;
    hazir: string;
    onay: string;
  };
  ozet?: ReactNode;
  eylemler?: ReactNode;
  className?: string;
}) {
  const yuzde = (v: number | null) =>
    v === null ? "—" : `%${Math.round(v * 100)}`;
  const kristaller: Kristal[] = [
    {
      oran: veri,
      deger: yuzde(veri),
      etiket: etiketler.veri,
      renk: "var(--c-light)",
      yukseklik: 150,
    },
    {
      oran: toplam ? hazir / toplam : null,
      deger: `${hazir}/${toplam}`,
      etiket: etiketler.hazir,
      renk: "var(--c-mid)",
      yukseklik: 190,
    },
    {
      oran: onay,
      deger: yuzde(onay),
      etiket: etiketler.onay,
      renk: "var(--c-dark)",
      yukseklik: 150,
    },
  ];
  return (
    <section
      className={cn(
        "bg-card chamfer border-line-strong flex flex-wrap items-end gap-8 rounded-lg border p-6 sm:p-7",
        className,
      )}
    >
      <div className="flex min-w-60 flex-1 flex-col gap-3">
        <span className="text-muted-foreground font-mono text-[11px] font-semibold tracking-[0.08em] uppercase">
          {baslik}
        </span>
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-display text-5xl font-bold tracking-tight tabular-nums">
            {hazir}
          </span>
          <span className="text-text-2">{etiketler.hazirSatiri}</span>
        </div>
        {ozet ? (
          <p className="text-text-2 max-w-prose text-sm">{ozet}</p>
        ) : null}
        {eylemler ? (
          <div className="flex flex-wrap gap-2">{eylemler}</div>
        ) : null}
      </div>
      <div className="flex items-end gap-3 sm:gap-4">
        {kristaller.map((k) => (
          <KristalSutun key={k.etiket} k={k} />
        ))}
      </div>
    </section>
  );
}
