"use client";

import { LifeBuoy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { openClickFix } from "@/lib/support/clickfix";
import { useSupportStore } from "@/lib/support/support-store";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Destek" düğmesi (#387): ClickFix widget'ını açar. Widget yüklü değilse
 * (kimlik boş) hiç görünmez; okunmamış yanıt sayısını rozetle gösterir.
 */
export function SupportButton({ className }: { className?: string }) {
  const { t } = useLocale();
  const available = useSupportStore((s) => s.available);
  const unread = useSupportStore((s) => s.unread);
  if (!available) return null;
  const label = unread
    ? t("layout.support_unread", { count: unread })
    : t("layout.support");
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn("relative pointer-coarse:size-11", className)}
      aria-label={label}
      title={label}
      onClick={() => openClickFix()}
    >
      <LifeBuoy className="size-5" aria-hidden />
      {unread ? (
        <span
          aria-hidden
          className="bg-danger text-background absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-bold tabular-nums"
        >
          {unread > 9 ? "9+" : unread}
        </span>
      ) : null}
    </Button>
  );
}
