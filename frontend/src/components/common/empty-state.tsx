import type { ReactNode } from "react";

import { BosKristal } from "@/components/hakan/bos-durum";
import { cn } from "@/lib/utils";

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
};

/**
 * Boş durum (#332, Durumlar.dc.html): kesik çizgili kristaller + ne olduğu
 * + neden + tek düğme.
 */
export function EmptyState({
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3.5 rounded-xl border border-dashed px-6 py-12 text-center",
        className,
      )}
    >
      <BosKristal className="h-[72px]" />
      <h3 className="font-display text-[19px] font-bold">{title}</h3>
      {description ? (
        <p className="text-text-2 max-w-md text-sm">{description}</p>
      ) : null}
      {action ? <div>{action}</div> : null}
    </div>
  );
}
