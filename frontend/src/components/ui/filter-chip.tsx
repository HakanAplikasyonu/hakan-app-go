"use client";

import { Toggle as TogglePrimitive } from "radix-ui";
import * as React from "react";

import { cn } from "@/lib/utils/index";

/**
 * Süzgeç çipi: basılıyken dolu, değilken çerçeveli yuvarlak düğme
 * (`aria-pressed`). Liste üstündeki "Tümü / Bekleyen / …" süzgeçlerinde
 * kullanılır; `role="group"` ve etiketi sarmalayan öğe verir.
 */
export function FilterChip({
  className,
  type,
  ...props
}: React.ComponentProps<typeof TogglePrimitive.Root>) {
  return (
    <TogglePrimitive.Root
      data-slot="filter-chip"
      type={type ?? "button"}
      className={cn(
        "focus-visible:ring-ring min-h-11 rounded-full border px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 md:min-h-9",
        "data-[state=on]:border-brand data-[state=on]:bg-brand data-[state=on]:text-on-brand",
        "data-[state=off]:border-line data-[state=off]:bg-surface data-[state=off]:text-text-2 data-[state=off]:hover:text-foreground",
        className,
      )}
      {...props}
    />
  );
}
