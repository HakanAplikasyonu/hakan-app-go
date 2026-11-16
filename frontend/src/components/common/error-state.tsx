import { CloudOff } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type ErrorStateProps = {
  title: string;
  description?: string;
  onRetry?: () => void;
  retryLabel?: string;
  action?: ReactNode;
  className?: string;
};

/**
 * Hata durumu (#332, Durumlar.dc.html): tehlike tonlu ikon, ne oldu, ne
 * yapmalı, yeniden dene. Veri güvencesi ve teknik kod gerekiyorsa
 * `HataDurumu` (components/hakan) kullanılır.
 */
export function ErrorState({
  title,
  description,
  onRetry,
  retryLabel,
  action,
  className,
}: ErrorStateProps) {
  return (
    <div
      role="alert"
      className={cn(
        "bg-card flex flex-col items-center justify-center gap-3.5 rounded-xl border px-7 py-12 text-center",
        className,
      )}
    >
      <span
        aria-hidden="true"
        className="bg-danger-tint text-danger flex size-16 items-center justify-center rounded-[18px]"
      >
        <CloudOff className="size-7" strokeWidth={1.75} />
      </span>
      <h3 className="font-display text-[19px] font-bold">{title}</h3>
      {description ? (
        <p className="text-text-2 max-w-md text-sm">{description}</p>
      ) : null}
      {onRetry || action ? (
        <div className="flex flex-wrap items-center justify-center gap-2">
          {onRetry ? (
            <Button type="button" onClick={onRetry}>
              {retryLabel}
            </Button>
          ) : null}
          {action}
        </div>
      ) : null}
    </div>
  );
}
