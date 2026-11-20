"use client";

import { useId, useState } from "react";

import { Hint } from "@/components/ui/hint";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useDecideWarning } from "../hooks/use-warnings";
import { createDocumentRequest } from "../services/engagement-documents.service";
import type {
  ApiDataWarning,
  WarningDecisionAction,
} from "../services/warnings.service";

/** Acknowledge / resolve on the API need this tenant permission. */
export const WARNINGS_WRITE_PERMISSION = Permission.TenantWarningsWrite;

/** A reason shorter than this is not a reason (#529). */
const INVOICE_ENTITIES = new Set(["invoice", "invoice_item"]);

/**
 * Belgesi eksik uyarılar (#570): panelden mükelleften belge istenir; istek
 * açılınca uyarı "Gördüm" olarak, isteğin gerekçesiyle kapanır.
 */
export const DOC_REQUEST_CODES = new Set([
  "BNK_UNDOCUMENTED_PAYMENT",
  "BNK_UNINVOICED_COLLECTION",
  "UBL_MISSING",
  "MISSING_AT_SOURCE",
]);

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
  presets,
}: {
  label: string;
  confirmLabel: string;
  busy: boolean;
  /** Hazır gerekçeler (#546): tıklanınca metne yazılır, düzenlenebilir. */
  presets?: string[];
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
      {presets?.length ? (
        <div
          className="flex flex-wrap gap-1"
          role="group"
          aria-label={t("office.warning_decision.presets_label")}
        >
          {presets.map((p) => (
            <Button
              key={p}
              type="button"
              size="sm"
              variant={note === p ? "secondary" : "outline"}
              className="h-auto min-h-8 px-2 py-1 text-left text-xs font-normal whitespace-normal"
              onClick={() => setNote(p)}
            >
              {p}
            </Button>
          ))}
        </div>
      ) : null}
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
  requestText,
}: {
  warning: ApiDataWarning;
  /**
   * Uyarının okunur metni (yan panel): verilirse belgesi eksik uyarıda
   * "Mükelleften belge iste" çıkar (#570).
   */
  requestText?: string;
  /** Karar kaydedilince (yan panel sıradaki uyarıya geçer). */
  onDecided?: (w: ApiDataWarning) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const decide = useDecideWarning(slug);
  const [action, setAction] = useState<WarningDecisionAction | null>(null);
  const [requesting, setRequesting] = useState(false);
  const [requestError, setRequestError] = useState("");
  // Fatura kaynağından gelir, ofiste değiştirilmez (#546): karar gerekçeyle
  // verilir; sık gerekçeler hazır gelir.
  const presets = INVOICE_ENTITIES.has(warning.entity_type)
    ? (warning.code === "POSSIBLE_DUPLICATE"
        ? ["duplicate_keep", "duplicate_cancel"]
        : ["checked", "correction_requested", "rounding"]
      ).map((k) => t(`office.warning_decision.preset_${k}`))
    : undefined;
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
  const canRequest =
    requestText !== undefined &&
    DOC_REQUEST_CODES.has(warning.code) &&
    can(Permission.TenantDocTasksWrite);
  // Belge isteği açılır, uyarı isteğin notuyla "Gördüm" olarak kapanır.
  const requestDocument = async () => {
    setRequesting(true);
    setRequestError("");
    try {
      await createDocumentRequest(slug, warning.engagement_uuid, {
        title: t("office.warning_request.title", {
          period: warning.period ?? "",
        }).trim(),
        note: requestText || undefined,
        due_days: 7,
      });
      run("acknowledge", t("office.warning_request.note"));
    } catch (e) {
      setRequestError(
        (e instanceof Error && e.message) || t("office.warning_request.error"),
      );
    } finally {
      setRequesting(false);
    }
  };
  return (
    <div className="flex flex-col items-end gap-1">
      {action ? (
        <DecisionNoteForm
          label={t(`office.warning_decision.reason_${action}`)}
          confirmLabel={t(`office.warning_decision.confirm_${action}`)}
          busy={decide.isPending}
          presets={presets}
          onConfirm={(note) => run(action, note)}
          onCancel={() => setAction(null)}
        />
      ) : (
        <div className="flex flex-wrap justify-end gap-1">
          {canRequest ? (
            <Button
              size="sm"
              className="min-h-11 md:min-h-8"
              disabled={requesting || decide.isPending}
              onClick={() => void requestDocument()}
            >
              {t("office.warning_request.action")}
            </Button>
          ) : null}
          <Hint label={t("office.warning_decision.acknowledge_hint")}>
            <Button
              variant="outline"
              size="sm"
              className="min-h-11 md:min-h-8"
              onClick={() => setAction("acknowledge")}
            >
              {t("office.warning_acknowledge")}
            </Button>
          </Hint>
          <Hint label={t("office.warning_decision.resolve_hint")}>
            <Button
              variant="outline"
              size="sm"
              className="min-h-11 md:min-h-8"
              onClick={() => setAction("resolve")}
            >
              {t("office.warning_resolve")}
            </Button>
          </Hint>
        </div>
      )}
      {requestError ? (
        <p role="alert" className="text-destructive max-w-56 text-xs">
          {requestError}
        </p>
      ) : null}
      {decide.error ? (
        <p role="alert" className="text-destructive max-w-56 text-xs">
          {decide.error.message || t("office.warnings_api.action_error")}
        </p>
      ) : null}
    </div>
  );
}
