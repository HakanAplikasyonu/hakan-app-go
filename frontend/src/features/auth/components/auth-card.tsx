import type { ComponentProps, ReactNode } from "react";

import { FieldDescription } from "@/components/ui/field";
import { cn } from "@/lib/utils";

/**
 * Giriş kabuğundaki form bölümü (tuval Giris): kart çerçevesi yok; sayfa
 * başlığı (h1), kısa açıklama, içerik ve dipnot. Kabuk zemini zaten yüzey.
 */
export function AuthCard({
  title,
  description,
  children,
  footer,
  className,
  ...props
}: ComponentProps<"div"> & {
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <div className="space-y-1.5">
        <h1 className="font-display text-[28px] leading-tight font-bold tracking-tight">
          {title}
        </h1>
        {description ? (
          <p className="text-text-2 text-sm">{description}</p>
        ) : null}
      </div>
      <div>{children}</div>
      {footer ? (
        <FieldDescription className="text-text-3 text-xs">
          {footer}
        </FieldDescription>
      ) : null}
    </div>
  );
}
