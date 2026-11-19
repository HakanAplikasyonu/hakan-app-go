"use client";

import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import { brand } from "@/config/brand";
import type { ThemeMode, ThemePalette } from "@/config/theme";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePalette } from "@/providers/theme-provider";

type Secenek = {
  palette: ThemePalette;
  mode: "light" | "dark";
  key: string;
  zemin: string;
  kristal: readonly [string, string, string];
};

const { crystal: m3, crystalN3: n3 } = brand.colors;
const secenekler: Secenek[] = [
  {
    palette: "m3",
    mode: "light",
    key: "common.theme_m3_light",
    zemin: "#F3F3F5",
    kristal: [m3.light, m3.mid, m3.dark],
  },
  {
    palette: "m3",
    mode: "dark",
    key: "common.theme_m3_dark",
    zemin: "#0A0A0A",
    kristal: [m3.light, m3.mid, m3.dark],
  },
  {
    palette: "n3",
    mode: "light",
    key: "common.theme_n3_light",
    zemin: "#F3F3F5",
    kristal: [n3.light, n3.mid, n3.dark],
  },
  {
    palette: "n3",
    mode: "dark",
    key: "common.theme_n3_dark",
    zemin: "#0A0A0A",
    kristal: [n3.light, n3.mid, "#F3F3F5"],
  },
];

const kipler: { value: ThemeMode; key: string }[] = [
  { value: "light", key: "common.theme_light" },
  { value: "dark", key: "common.theme_dark" },
  { value: "system", key: "common.theme_follow_system" },
];

const noop = () => () => {};

/**
 * Ayarlar → Görünüm (tuval OfisAyarlar): dört tema kartı + Açık / Koyu /
 * Sistemi izle. Tema yalnızca bu tarayıcıdaki hesap için; varsayılan sistemi izle.
 */
export function GorunumAyari({ className }: { className?: string }) {
  const { t } = useLocale();
  const { theme, resolvedTheme, setTheme } = useTheme();
  const { palette, setPalette } = usePalette();
  // next-themes değerleri yalnızca istemcide belli; SSR'da seçim gösterilmez.
  const mounted = useSyncExternalStore(
    noop,
    () => true,
    () => false,
  );
  const kip = (mounted ? theme : undefined) ?? "system";
  const etkin = mounted
    ? `${palette}-${resolvedTheme === "dark" ? "dark" : "light"}`
    : null;

  return (
    <section
      className={cn(
        "bg-card flex flex-col gap-3.5 rounded-lg border p-5",
        className,
      )}
    >
      <div className="flex flex-col gap-1">
        <h2 className="font-display text-xl font-bold">
          {t("common.appearance_title")}
        </h2>
        <p className="text-text-2 text-[13px]">
          {t("common.appearance_description")}
        </p>
      </div>
      <div
        role="radiogroup"
        aria-label={t("common.theme_palette")}
        className="grid grid-cols-2 gap-3 md:grid-cols-4"
      >
        {secenekler.map((s) => {
          const secili = etkin === `${s.palette}-${s.mode}`;
          return (
            <Button
              key={s.key}
              variant="ghost"
              role="radio"
              aria-checked={secili}
              onClick={() => {
                setPalette(s.palette);
                // Kip "sistemi izle" ise ve sistem bu kartla aynıysa sistemi bozma.
                if (kip !== "system" || resolvedTheme !== s.mode)
                  setTheme(s.mode);
              }}
              className={cn(
                "h-auto min-h-11 flex-col items-stretch justify-start gap-2 rounded-lg border-2 p-2 text-left text-[13px] font-semibold whitespace-normal transition-colors hover:bg-transparent",
                secili
                  ? "border-brand"
                  : "border-line hover:border-line-strong",
              )}
            >
              <span
                aria-hidden="true"
                className="flex h-16 items-end justify-center gap-1 rounded-md pb-3"
                style={{ background: s.zemin }}
              >
                {[22, 32, 22].map((h, i) => (
                  <i
                    key={i}
                    className="block w-2.5"
                    style={{
                      height: h,
                      background: s.kristal[i],
                      clipPath:
                        "polygon(0 4px, 50% 0, 100% 4px, 100% 100%, 0 100%)",
                    }}
                  />
                ))}
              </span>
              {t(s.key)}
            </Button>
          );
        })}
      </div>
      <div
        role="radiogroup"
        aria-label={t("common.theme_mode")}
        className="bg-surface-3 flex w-max gap-0.5 rounded-md p-[3px]"
      >
        {kipler.map((k) => (
          <Button
            key={k.value}
            variant="ghost"
            role="radio"
            aria-checked={kip === k.value}
            onClick={() => setTheme(k.value)}
            className={cn(
              "h-[30px] rounded-lg px-2.5 py-0 text-xs font-semibold transition-colors max-md:h-11",
              kip === k.value
                ? "bg-card text-foreground hover:bg-card hover:text-foreground shadow-card"
                : "text-text-2 hover:text-text-2 hover:bg-transparent",
            )}
          >
            {t(k.key)}
          </Button>
        ))}
      </div>
    </section>
  );
}
