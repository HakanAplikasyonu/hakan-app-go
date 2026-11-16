import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

/** Kesik çizgili kristaller (boş durum işareti). Tuval: Kristal.dc.html. */
export function BosKristal({ className }: { className?: string }) {
  const k = (h: number) => (
    <i
      className="block w-[34px] border-2 border-b-0 border-dashed"
      style={{
        height: h,
        borderColor: "var(--line-strong)",
        background: "var(--surface-2)",
        clipPath: "polygon(0 14px, 50% 0, 100% 14px, 100% 100%, 0 100%)",
      }}
    />
  );
  return (
    <span
      aria-hidden="true"
      className={cn("flex h-20 items-end gap-2", className)}
    >
      {k(56)}
      {k(80)}
      {k(56)}
    </span>
  );
}

type BosDurumProps = {
  baslik: string;
  aciklama?: string;
  /** Tek eylem (ör. "UBL yükle" düğmesi). */
  eylem?: ReactNode;
  className?: string;
};

/** Boş durum: kesik çizgili kristaller, başlık, açıklama ve tek eylem. */
export function BosDurum({
  baslik,
  aciklama,
  eylem,
  className,
}: BosDurumProps) {
  return (
    <div
      className={cn(
        "bg-card flex flex-col items-center gap-[18px] rounded-lg border px-6 py-10 text-center",
        className,
      )}
    >
      <BosKristal />
      <h3 className="font-display text-xl font-bold">{baslik}</h3>
      {aciklama ? (
        <p className="text-text-2 max-w-[380px]">{aciklama}</p>
      ) : null}
      {eylem ? <div>{eylem}</div> : null}
    </div>
  );
}
