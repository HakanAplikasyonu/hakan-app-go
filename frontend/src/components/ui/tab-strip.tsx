import * as React from "react";

import { cn } from "@/lib/utils/index";

/**
 * Alt çizgili görünüm şeridi (`role="tablist"`): aynı sayfanın görünümleri
 * arasında geçiş. İçerik panelleri sayfa tarafından çizilir (adres
 * parametresine bağlı), bu yüzden Radix `Tabs` yerine yalın sekme düğmeleri.
 * `aria-label` çağıran yerden verilir.
 */
export function TabStrip({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      role="tablist"
      data-slot="tab-strip"
      className={cn(
        "border-line flex gap-1 overflow-x-auto border-b",
        className,
      )}
      {...props}
    />
  );
}

export function TabStripItem({
  selected,
  className,
  type,
  ...props
}: Omit<React.ComponentProps<"button">, "role" | "aria-selected"> & {
  selected: boolean;
}) {
  return (
    <button
      role="tab"
      type={type ?? "button"}
      aria-selected={selected}
      data-slot="tab-strip-item"
      className={cn(
        "focus-visible:ring-ring -mb-px min-h-11 border-b-2 px-3 text-sm font-medium whitespace-nowrap focus-visible:ring-2 focus-visible:outline-none md:min-h-9",
        selected
          ? "border-brand text-foreground"
          : "text-text-2 hover:text-foreground border-transparent",
        className,
      )}
      {...props}
    />
  );
}
