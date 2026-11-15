"use client";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { useOfficeActions } from "../hooks/use-office";
import type { DataWarning } from "../types";

/** open → acknowledged / resolved; kapalıysa yeniden aç (paket 05 §1). */
export function WarningActions({ warning }: { warning: DataWarning }) {
  const { t } = useLocale();
  const { setWarningStatus } = useOfficeActions();
  if (warning.status !== "open") {
    return (
      <Button
        variant="ghost"
        size="sm"
        onClick={() => setWarningStatus(warning.id, "open")}
      >
        {t("office.warning_reopen")}
      </Button>
    );
  }
  return (
    <div className="flex gap-1">
      <Button
        variant="outline"
        size="sm"
        onClick={() => setWarningStatus(warning.id, "acknowledged")}
      >
        {t("office.warning_acknowledge")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setWarningStatus(warning.id, "resolved")}
      >
        {t("office.warning_resolve")}
      </Button>
    </div>
  );
}
