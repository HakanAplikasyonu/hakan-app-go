import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppWordmarkProps = {
  title?: string;
  className?: string;
};

/** İşaret + küçük harfli "hakan" yazısı (Inter Tight 800, tuval OfisNav). */
export function AppWordmark({
  className,
  title = brand.productName,
}: AppWordmarkProps) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <AppMark className="h-[22px] w-5" title={title} />
      <span
        aria-hidden="true"
        className="font-display text-[19px] leading-none font-extrabold tracking-[-0.05em] lowercase"
      >
        {title}
      </span>
    </span>
  );
}
