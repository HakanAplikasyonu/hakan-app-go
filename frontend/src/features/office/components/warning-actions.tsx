"use client";

import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useDecideWarning } from "../hooks/use-warnings";
import type {
  ApiDataWarning,
  WarningDecisionAction,
} from "../services/warnings.service";

/** Acknowledge / resolve on the API need this tenant permission. */
export const WARNINGS_WRITE_PERMISSION = Permission.TenantWarningsWrite;

/** A reason shorter than this is not a reason (#529). */
export const MIN_NOTE = 3;
export const MAX_NOTE = 500;

/**
 * Gerekçe formu (#529): "Gördüm" ve "Çözüldü" nedensiz verilmez. Tekli ve
 * toplu kararda aynı form.
 */
export function DecisionNoteForm({
  label,
  confirmLabel,
  busy,
  onConfirm,
  onCancel,
}: {
  label: string;
  confirmLabel: string;
  busy: boolean;
  onConfirm: (note: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const [note, setNote] = useState("");
  const ok = note.trim().length >= MIN_NOTE;
  return (
    <form
      className="flex w-full max-w-md flex-col gap-1.5"
      onSubmit={(e) => {
        e.preventDefault();
        if (ok) onConfirm(note.trim());
      }}
    >
      <Label htmlFor={id} className="text-text-2 text-xs font-normal">
        {label}
      </Label>
      <Textarea
        id={id}
        autoFocus
        required
        minLength={MIN_NOTE}
        maxLength={MAX_NOTE}
        rows={2}
        value={note}
        onChange={(e) => setNote(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onCancel();
          // Ctrl/⌘+Enter onaylar; düz Enter satır.
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && ok) {
            e.preventDefault();
            onConfirm(note.trim());
          }
        }}
        placeholder={t("office.warning_decision.placeholder")}
        className="min-h-0 px-2 py-1.5 shadow-none"
      />
      <div className="flex justify-end gap-1">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-11 md:min-h-8"
          onClick={onCancel}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          size="sm"
          className="min-h-11 md:min-h-8"
          disabled={busy || !ok}
        >
          {confirmLabel}
        </Button>
      </div>
    </form>
  );
}

/** Karar verilmiş uyarıda kim, ne zaman, hangi gerekçeyle (#529). */
export function WarningDecisionInfo({ warning }: { warning: ApiDataWarning }) {
  const { t, locale } = useLocale();
  if (warning.status === "open") return null;
  const who =
    warning.status === "auto_resolved"
      ? t("office.warning_decision.system")
      : (warning.resolved_by_name ?? t("office.warning_decision.unknown"));
  return (
    <div
      className="text-text-3 max-w-72 text-xs"
      data-testid="warning-decision"
    >
      <div>
        {t(`office.warning_decision.status_${warning.status}`)} · {who}
        {warning.resolved_at
          ? ` · ${datetime(warning.resolved_at, undefined, locale)}`
          : ""}
      </div>
      {warning.resolution_note ? (
        <div className="text-text-2 italic">“{warning.resolution_note}”</div>
      ) : null}
    </div>
  );
}

/**
 * API-backed actions (warnings screen): only an open warning can be decided,
 * with a reason (#529); the API has no reopen. Hidden without
 * `tenant.warnings.write`. A decided warning shows who, when and why.
 */
export function ApiWarningActions({
  warning,
  onDecided,
}: {
  warning: ApiDataWarning;
  /** Karar kaydedilince (yan panel sıradaki uyarıya geçer). */
  onDecided?: (w: ApiDataWarning) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const decide = useDecideWarning(slug);
  const [action, setAction] = useState<WarningDecisionAction | null>(null);
  if (warning.status !== "open")
    return <WarningDecisionInfo warning={warning} />;
  if (!can(WARNINGS_WRITE_PERMISSION)) return null;
  const run = (a: WarningDecisionAction, note: string) =>
    decide.mutate(
      {
        engagement: warning.engagement_uuid,
        warning: warning.uuid,
        action: a,
        note,
      },
      {
        onSuccess: () => {
          setAction(null);
          onDecided?.(warning);
        },
      },
    );
  return (
    <div className="flex flex-col items-end gap-1">
      {action ? (
        <DecisionNoteForm
          label={t(`office.warning_decision.reason_${action}`)}
          confirmLabel={t(`office.warning_decision.confirm_${action}`)}
          busy={decide.isPending}
          onConfirm={(note) => run(action, note)}
          onCancel={() => setAction(null)}
        />
      ) : (
        <div className="flex gap-1">
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 md:min-h-8"
            title={t("office.warning_decision.acknowledge_hint")}
            onClick={() => setAction("acknowledge")}
          >
            {t("office.warning_acknowledge")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 md:min-h-8"
            title={t("office.warning_decision.resolve_hint")}
            onClick={() => setAction("resolve")}
          >
            {t("office.warning_resolve")}
          </Button>
        </div>
      )}
      {decide.error ? (
        <p role="alert" className="text-destructive max-w-56 text-xs">
          {decide.error.message || t("office.warnings_api.action_error")}
        </p>
      ) : null}
    </div>
  );
}
