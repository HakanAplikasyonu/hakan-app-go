import { AppMark } from "@/components/brand/app-mark";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

type MarkaLogosuProps = {
  /** `isaret`: yalnız kristaller · `yazi`: yalnız "hakan" · `tam`: ikisi. */
  varyant?: "isaret" | "yazi" | "tam";
  /** `marka`: mavi (brand) zemin üstünde; orta kristal beyaz olur. */
  zemin?: "normal" | "marka";
  /** İşaret yüksekliği (px). */
  boyut?: number;
  className?: string;
};

/** M3 logosu. Kristal yüzleri token'dan gelir (N3'te kendiliğinden değişir). */
export function MarkaLogosu({
  varyant = "tam",
  zemin = "normal",
  boyut = 22,
  className,
}: MarkaLogosuProps) {
  const isaret = (
    <AppMark
      zemin={zemin === "marka" ? "marka-zemin" : "token"}
      style={{ height: boyut, width: Math.round((boyut * 181) / 198) }}
      aria-hidden={varyant === "tam" ? true : undefined}
    />
  );
  if (varyant === "isaret") return <span className={className}>{isaret}</span>;
  const yazi = (
    <span
      className="font-display leading-none font-extrabold tracking-[-0.05em] lowercase"
      style={{ fontSize: Math.round(boyut * 0.86) }}
    >
      {brand.name}
    </span>
  );
  if (varyant === "yazi") return <span className={className}>{yazi}</span>;
  return (
    <span
      role="img"
      aria-label={brand.name}
      className={cn("inline-flex items-center gap-2.5", className)}
    >
      {isaret}
      {yazi}
    </span>
  );
}
