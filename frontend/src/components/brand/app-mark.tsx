import type { SVGProps } from "react";

import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

type AppMarkProps = SVGProps<SVGSVGElement> & {
  title?: string;
};

/**
 * HAKAN işareti (docs/brand/logo-mark.svg, D19): borç/alacak sütunları ve
 * onay çizgisi bir arada "H" kurar. Sütunlar `currentColor`, onay çizgisi
 * `--brand-check` tokenıdır; bu yüzden işaret `bg-primary` karonun üstünde
 * açık ve koyu temada doğru renk alır.
 */
export function AppMark({
  className,
  title = brand.name,
  ...props
}: AppMarkProps) {
  return (
    <svg
      viewBox="12 11 40 42"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("size-4", className)}
      {...props}
    >
      <title>{title}</title>
      <rect x="15" y="14" width="8" height="36" rx="4" fill="currentColor" />
      <rect x="41" y="14" width="8" height="36" rx="4" fill="currentColor" />
      <path
        d="M21 33 L29 41 L43 25"
        stroke="var(--brand-check)"
        strokeWidth="6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
