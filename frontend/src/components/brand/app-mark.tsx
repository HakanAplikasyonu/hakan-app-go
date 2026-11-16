import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

type AppMarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
  /**
   * `token` (varsayılan): kristal yüzleri `--c-light / --c-mid / --c-dark`
   * token'larından gelir, N3 temasında kendiliğinden değişir.
   * `marka-zemin`: mavi (brand) karo üstünde; orta kristal beyaz olur.
   */
  zemin?: "token" | "marka-zemin";
};

/**
 * HAKAN işareti, M3 "Taç kristal" (docs/brand/logo-mark.svg): üç kristal,
 * yanlar 11/16, orta 16/16 oranında. Sol mükellef, sağ mali müşavir, orta
 * beyanname. Tuval: docs/tasarim/tuval/Main.dc.html.
 */
export function AppMark({
  className,
  title = brand.name,
  zemin = "token",
  ...props
}: AppMarkProps) {
  const light = "var(--c-light)";
  const mid = zemin === "marka-zemin" ? "#FFFFFF" : "var(--c-mid)";
  const dark = "var(--c-dark)";
  return (
    <svg
      viewBox="0 0 181 198"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("size-4", className)}
      {...props}
    >
      <title>{title}</title>
      <polygon points="0,86 28,59 28,198 0,198" style={{ fill: light }} />
      <polygon points="28,59 56,86 56,198 28,198" style={{ fill: mid }} />
      <polygon points="63,36 91,0 91,198 63,198" style={{ fill: mid }} />
      <polygon points="91,0 119,36 119,198 91,198" style={{ fill: dark }} />
      <polygon points="125,86 153,59 153,198 125,198" style={{ fill: dark }} />
      <polygon points="153,59 181,86 181,198 153,198" style={{ fill: light }} />
    </svg>
  );
}
