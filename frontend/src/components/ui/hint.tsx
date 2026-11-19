"use client";

import type { ReactNode } from "react";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

/**
 * Düğme ipucu (`title=` yerine): dokunmatikte ve klavyede de görünür.
 * `label` boşsa yalnızca çocuk çizilir. Devre dışı düğme fare olayı
 * almadığından `wrap` ile odaklanabilir bir sarmalayıcı tetikleyici olur
 * (ör. "neden basılamıyor" açıklaması).
 */
export function Hint({
  label,
  children,
  wrap = false,
  side = "top",
}: {
  label?: ReactNode;
  children: ReactNode;
  wrap?: boolean;
  side?: "top" | "bottom" | "left" | "right";
}) {
  if (!label) return <>{children}</>;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {wrap ? (
          <span tabIndex={0} className="inline-flex rounded-md">
            {children}
          </span>
        ) : (
          children
        )}
      </TooltipTrigger>
      <TooltipContent side={side}>{label}</TooltipContent>
    </Tooltip>
  );
}
