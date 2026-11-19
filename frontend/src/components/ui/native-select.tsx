import { ChevronDown } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils/index";

type NativeSelectProps = Omit<React.ComponentProps<"select">, "size"> & {
  size?: "default" | "sm";
  /** Sarmalayıcının sınıfı (genişlik buradan verilir). */
  wrapperClassName?: string;
};

/**
 * Tarayıcının kendi listesini açan seçim kutusu. Tablo satırı ve yoğun filtre
 * gibi çok sayıda örneğin aynı anda çizildiği yerlerde Radix `Select` yerine
 * kullanılır; telefonda sistemin kendi seçicisi açılır.
 */
export const NativeSelect = React.forwardRef<
  HTMLSelectElement,
  NativeSelectProps
>(({ className, wrapperClassName, size = "default", ...props }, ref) => (
  <div
    data-slot="native-select-wrapper"
    className={cn(
      "relative inline-flex max-w-full min-w-0 has-[select:disabled]:opacity-50",
      wrapperClassName,
    )}
  >
    <select
      ref={ref}
      data-slot="native-select"
      data-size={size}
      className={cn(
        "border-border bg-background focus-visible:ring-ring h-9 w-full min-w-0 appearance-none rounded-md border py-1 pr-8 pl-3 text-sm shadow-sm transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed data-[size=sm]:h-8 data-[size=sm]:pl-2 max-md:h-11",
        "aria-invalid:border-destructive aria-invalid:ring-destructive/20",
        className,
      )}
      {...props}
    />
    <ChevronDown
      aria-hidden="true"
      className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2"
    />
  </div>
));
NativeSelect.displayName = "NativeSelect";

export function NativeSelectOption(props: React.ComponentProps<"option">) {
  return <option data-slot="native-select-option" {...props} />;
}

export function NativeSelectOptGroup(props: React.ComponentProps<"optgroup">) {
  return <optgroup data-slot="native-select-optgroup" {...props} />;
}
