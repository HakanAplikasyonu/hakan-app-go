import type { ComponentProps, ReactNode } from "react";

import { FieldDescription } from "@/components/ui/field";
import { cn } from "@/lib/utils";

/**
 * Giriş formu bölümü (#332, Giris.dc.html): kart yok; sağ paneldeki başlık
 * (Inter Tight 28) ve altında form.
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
      <div className="flex flex-col gap-1.5">
        <h1 className="font-display text-[28px] leading-tight font-bold">
          {title}
        </h1>
        {description ? (
          <p className="text-text-2 text-sm">{description}</p>
        ) : null}
      </div>
      <div>{children}</div>
      {footer ? (
        <FieldDescription className="text-center">{footer}</FieldDescription>
      ) : null}
    </div>
  );
}
