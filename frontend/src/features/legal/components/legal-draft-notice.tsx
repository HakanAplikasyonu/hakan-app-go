"use client";

import { TriangleAlert } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Shown wherever a text with `legal_review_pending` is displayed: the
 * version is a draft that a lawyer has not reviewed yet.
 */
export function LegalDraftNotice({
  compact = false,
  className,
}: {
  /** One line, for places that only link to the text. */
  compact?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  if (compact) {
    return (
      <p
        className={cn(
          "flex items-center gap-1.5 text-xs font-medium text-amber-700 dark:text-amber-300",
          className,
        )}
      >
        <TriangleAlert className="size-3.5 shrink-0" aria-hidden />
        {t("legal.draft_title")}
      </p>
    );
  }
  return (
    <Alert
      className={cn(
        "border-amber-500/40 bg-amber-500/10 text-amber-800 dark:text-amber-200",
        className,
      )}
    >
      <TriangleAlert aria-hidden />
      <AlertTitle>{t("legal.draft_title")}</AlertTitle>
      <AlertDescription className="text-amber-800/90 dark:text-amber-200/90">
        {t("legal.draft_description")}
      </AlertDescription>
    </Alert>
  );
}
