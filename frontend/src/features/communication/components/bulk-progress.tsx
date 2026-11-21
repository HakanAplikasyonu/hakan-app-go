"use client";

import { CheckCircle2, Loader2, TriangleAlert } from "lucide-react";

import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useLocale } from "@/providers/locale-provider";

import { useBulkSend } from "../hooks/use-communication";
import type { BulkSend } from "../services/communication.service";

const statusTone: Record<BulkSend["status"], StatusTone> = {
  running: "info",
  done: "ok",
  failed: "danger",
  interrupted: "warn",
};

/**
 * Toplu gönderimin canlı ilerlemesi (#780 B): sunucu tek tek gönderirken
 * çubuk akar; bitince başarısızlar ve atlananlar listelenir.
 */
export function BulkProgress({
  uuid,
  onNew,
}: {
  uuid: string;
  onNew?: () => void;
}) {
  const { t } = useLocale();
  const q = useBulkSend(uuid);
  const b = q.data;
  if (!b) {
    return (
      <p className="text-muted-foreground flex items-center gap-2 text-sm">
        <Loader2 className="size-4 animate-spin" />
        {t("common.loading")}
      </p>
    );
  }
  const pct = b.total > 0 ? Math.round((b.processed / b.total) * 100) : 100;
  return (
    <div className="flex min-w-0 flex-col gap-3" data-testid="bulk-progress">
      <div className="flex flex-wrap items-center gap-2">
        {b.status === "running" ? (
          <Loader2 className="text-brand size-4 animate-spin" />
        ) : b.status === "done" ? (
          <CheckCircle2 className="text-ok size-4" />
        ) : (
          <TriangleAlert className="text-warn size-4" />
        )}
        <span className="font-medium">{b.target_label}</span>
        <StatusPill tone={statusTone[b.status]}>
          <span data-testid="bulk-status" data-status={b.status}>
            {t(`communication.status_${b.status}`)}
          </span>
        </StatusPill>
      </div>
      <Progress value={pct} aria-label={t("communication.progress_label")} />
      <p className="text-sm" data-testid="bulk-counts">
        {t("communication.progress_counts", {
          processed: b.processed,
          total: b.total,
          sent: b.sent,
          failed: b.failed,
        })}
      </p>
      {b.duplicates > 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("communication.duplicates_dropped", { count: b.duplicates })}
        </p>
      ) : null}
      {b.error_message ? (
        <p className="text-danger text-sm">{b.error_message}</p>
      ) : null}
      {b.status !== "running" && b.failures.length > 0 ? (
        <div data-testid="bulk-failures">
          <p className="text-sm font-medium">
            {t("communication.failures_title", { count: b.failures.length })}
          </p>
          <ul className="text-muted-foreground list-disc pl-5 text-xs">
            {b.failures.map((f, i) => (
              <li key={`${f.recipient}-${i}`}>
                {f.title || f.recipient} ({f.recipient}): {f.error_message}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {b.skipped.length > 0 ? (
        <div>
          <p className="text-sm font-medium">
            {t("communication.skipped_count", { count: b.skipped.length })}
          </p>
          <ul className="text-muted-foreground list-disc pl-5 text-xs">
            {b.skipped.map((s, i) => (
              <li key={`${s.engagement_uuid}-${i}`}>{s.title}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {onNew && b.status !== "running" ? (
        <div>
          <Button variant="outline" size="sm" onClick={onNew}>
            {t("communication.new_send")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
