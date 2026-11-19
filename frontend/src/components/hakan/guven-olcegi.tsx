"use client";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type GuvenKaynagi = "kural" | "gecmis" | "model" | "girdi";
export type GuvenKademesi = "dusuk" | "orta" | "yuksek";

/**
 * Kademe: < .6 düşük, < .9 orta, aksi yüksek. Kademe her zaman gösterilen
 * yüzdeden gelir: "%40 · Yüksek güven" gibi çelişki güveni bozar (#576).
 * Kaynak (kural, geçmiş, model) yalnız ipucunda yazılır.
 */
export function guvenKademesi(
  deger: number,
  _kaynak?: GuvenKaynagi,
): GuvenKademesi {
  if (deger < 0.6) return "dusuk";
  if (deger < 0.9) return "orta";
  return "yuksek";
}

const renk: Record<GuvenKademesi, string> = {
  dusuk: "var(--warn)",
  orta: "var(--brand-text)",
  yuksek: "var(--ok)",
};
const dolu: Record<GuvenKademesi, number> = { dusuk: 1, orta: 2, yuksek: 3 };
const etiket: Record<GuvenKademesi, string> = {
  dusuk: "common.confidence_low",
  orta: "common.confidence_medium",
  yuksek: "common.confidence_high",
};
const kaynakEtiketi: Record<GuvenKaynagi, string> = {
  kural: "common.confidence_source_rule",
  gecmis: "common.confidence_source_history",
  model: "common.confidence_source_model",
  girdi: "common.confidence_source_input",
};

type GuvenOlcegiProps = {
  /** 0..1 */
  deger: number;
  kaynak?: GuvenKaynagi;
  /** Kademe adını yanında yazar ("Yüksek güven"). */
  etiketli?: boolean;
  className?: string;
};

/**
 * AI güven ölçeği: üç kademeli kristal (6/9/12 px). Yüzde değil kademe
 * gösterilir; yüzde yalnızca ipucunda (üzerine gelince) ve ekran okuyucuda.
 */
export function GuvenOlcegi({
  deger,
  kaynak,
  etiketli = false,
  className,
}: GuvenOlcegiProps) {
  const { t, locale } = useLocale();
  const kademe = guvenKademesi(deger, kaynak);
  const yuzde = new Intl.NumberFormat(locale === "en" ? "en-GB" : "tr-TR", {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(Math.min(1, Math.max(0, deger)));
  const ad = t(etiket[kademe]);
  const ipucu = [ad, yuzde, kaynak ? t(kaynakEtiketi[kaynak]) : null]
    .filter(Boolean)
    .join(" · ");

  return (
    <span
      role="img"
      aria-label={ipucu}
      title={ipucu}
      data-kademe={kademe}
      className={cn("inline-flex items-center gap-2", className)}
    >
      <span aria-hidden="true" className="inline-flex items-end gap-0.5">
        {[6, 9, 12].map((h, i) => (
          <i
            key={h}
            className="block w-1"
            style={{
              height: h,
              background:
                i < dolu[kademe] ? renk[kademe] : "var(--line-strong)",
              clipPath: "polygon(0 2px, 50% 0, 100% 2px, 100% 100%, 0 100%)",
            }}
          />
        ))}
      </span>
      {etiketli ? (
        <span aria-hidden="true" className="text-text-2 text-xs font-semibold">
          {ad}
        </span>
      ) : null}
    </span>
  );
}
