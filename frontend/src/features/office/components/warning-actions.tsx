"use client";

import { Button } from "@/components/ui/button";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useDecideWarning } from "../hooks/use-warnings";
import type {
  ApiDataWarning,
  WarningDecisionAction,
} from "../services/warnings.service";

/** Acknowledge / resolve on the API need this tenant permission. */
export const WARNINGS_WRITE_PERMISSION = "tenant.warnings.write";

/**
 * API-backed actions (warnings screen): only an open warning can be decided;
 * the API has no reopen. Hidden without `tenant.warnings.write`. The error of
 * a failed decision is shown under the buttons.
 */
export function ApiWarningActions({ warning }: { warning: ApiDataWarning }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const decide = useDecideWarning(slug);
  if (warning.status !== "open" || !can(WARNINGS_WRITE_PERMISSION)) {
    return null;
  }
  const run = (action: WarningDecisionAction) =>
    decide.mutate({
      engagement: warning.engagement_uuid,
      warning: warning.uuid,
      action,
    });
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex gap-1">
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 md:min-h-8"
          disabled={decide.isPending}
          onClick={() => run("acknowledge")}
        >
          {t("office.warning_acknowledge")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 md:min-h-8"
          disabled={decide.isPending}
          onClick={() => run("resolve")}
        >
          {t("office.warning_resolve")}
        </Button>
      </div>
      {decide.error ? (
        <p role="alert" className="text-destructive max-w-56 text-xs">
          {decide.error.message || t("office.warnings_api.action_error")}
        </p>
      ) : null}
    </div>
  );
}
