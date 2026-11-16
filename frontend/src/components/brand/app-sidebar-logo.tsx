import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppSidebarLogoProps = {
  className?: string;
};

/** Kenar menü başlığı: M3 işareti + ürün adı (tuval OfisNav). */
export function AppSidebarLogo({ className }: AppSidebarLogoProps) {
  return (
    <>
      <span
        className={cn(
          "flex aspect-square size-8 items-center justify-center",
          className,
        )}
      >
        <AppMark className="h-[22px] w-5" />
      </span>
      <span className="grid flex-1 text-left leading-tight">
        <span className="font-display truncate text-[19px] font-extrabold tracking-[-0.05em] lowercase">
          {brand.productName}
        </span>
      </span>
    </>
  );
}
