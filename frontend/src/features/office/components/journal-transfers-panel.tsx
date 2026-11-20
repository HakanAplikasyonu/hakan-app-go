"use client";

import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  Check,
  CircleCheck,
  RotateCcw,
  Scale,
  X,
  XCircle,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createStatusColumn,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes, TRANSFERS_ANCHOR } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useJournalActions, useJournalTransfers } from "../hooks/use-journals";
import { useOfficePeriod } from "../period";
import type { JournalTransfer } from "../services/journals.service";
import {
  type ReconciliationRun,
  reconciliationKeys,
  reconciliationService,
} from "../services/reconciliation.service";
import { Figure, Panel } from "./office-ui";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

export const TRANSFER_STATUS: Record<string, StatusVariant> = {
  queued: { labelKey: "office.journals.transfer_status_queued", tone: "info" },
  claimed: {
    labelKey: "office.journals.transfer_status_claimed",
    tone: "info",
  },
  delivered: {
    labelKey: "office.journals.transfer_status_delivered",
    tone: "warn",
  },
  imported: {
    labelKey: "office.journals.transfer_status_imported",
    tone: "ok",
  },
  failed: {
    labelKey: "office.journals.transfer_status_failed",
    tone: "danger",
  },
  cancelled: {
    labelKey: "office.journals.transfer_status_cancelled",
    tone: "neutral",
  },
};

/** Turkish text for an error code the agent or the user reported. */
function transferError(t: Translate, tr: JournalTransfer) {
  const key = `office.journals.transfer_error_${tr.error_code ?? ""}`;
  const known = tr.error_code ? t(key) : key;
  // Known code first (what to do), the agent's own message as the detail
  // (e.g. the missing folder's path).
  if (known !== key)
    return tr.error_message && tr.error_message !== known
      ? `${known} (${tr.error_message})`
      : known;
  return tr.error_message || t("office.journals.transfer_error_unknown");
}

function actionErrorText(t: Translate, err: unknown) {
  if (isApiError(err)) {
    const key = `office.journals.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message;
  }
  return t("office.api.load_error");
}

/** Program label for an export driver id; the id itself when unknown. */
function program(t: Translate, name: string) {
  const key = `office.journals.driver_${name}`;
  const msg = t(key);
  return msg === key ? name : msg;
}

/**
 * Aktarım sonrası doğrulama (#574): programa aktarılan dönemin Luca
 * mizanı uygulamadaki kayıtlarla karşılaştırıldı mı, fark var mı. Mizan
 * karşılaştırması aktarımdan önceyse ya da hiç yapılmadıysa Mutabakat
 * ekranına götürür (dönem seçili gelir). Dönemlerin son koşuları panelde
 * tek istekle gelir (#639); `run` null ise dönem karşılaştırılmadı,
 * undefined ise henüz yüklenmedi.
 */
function TransferCheck({
  engagement,
  tr,
  run,
}: {
  engagement: string;
  tr: JournalTransfer;
  run: ReconciliationRun | null | undefined;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { setPeriod } = useOfficePeriod();
  const period = tr.period ?? "";
  if (!period || run === undefined) return null;
  const stale = !!run && !!tr.completed_at && run.created_at < tr.completed_at;
  const issues = run
    ? run.summary.differences +
      run.summary.missing_in_ledger +
      run.summary.missing_in_sources
    : 0;
  const open = () => {
    setPeriod(period);
    router.push(routes.tenant.engagementReconciliation(slug, engagement));
  };
  if (run && !stale && issues === 0) {
    return (
      <span className="text-status-approved inline-flex items-center gap-1">
        <CircleCheck className="size-3.5" aria-hidden />
        {t("office.journals.transfer_check_ok")}
      </span>
    );
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-x-2">
      <span
        className={
          run && !stale ? "text-status-blocking" : "text-status-warning"
        }
      >
        {run && !stale
          ? t("office.journals.transfer_check_differences", { count: issues })
          : stale
            ? t("office.journals.transfer_check_stale")
            : t("office.journals.transfer_check_none")}
      </span>
      <Button
        size="sm"
        variant="link"
        className="h-auto p-0 text-xs"
        onClick={open}
      >
        <Scale className="size-3.5" aria-hidden />
        {run && !stale
          ? t("office.journals.transfer_check_open")
          : t("office.journals.transfer_check_run")}
      </Button>
    </span>
  );
}

function Detail({
  tr,
  engagement,
  checks,
}: {
  tr: JournalTransfer;
  engagement: string;
  checks: Record<string, ReconciliationRun | null> | undefined;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  if (tr.status === "failed") {
    return (
      <span className="text-status-blocking text-xs">
        {transferError(t, tr)}
      </span>
    );
  }
  const parts: string[] = [];
  if (tr.agent?.name) parts.push(tr.agent.name);
  if (tr.folder_label) parts.push(tr.folder_label);
  const files = tr.delivered_files.length
    ? tr.delivered_files.map((f) => f.name)
    : tr.files.map((f) => f.name);
  return (
    <span className="text-muted-foreground flex flex-col text-xs">
      {parts.length ? <span>{parts.join(" · ")}</span> : null}
      {tr.status === "queued" ? (
        <span>{t("office.journals.transfer_waiting_agent")}</span>
      ) : null}
      {tr.status === "delivered" ? (
        <span>{t("office.journals.transfer_import_hint")}</span>
      ) : null}
      {files.length ? <Figure>{files.join(", ")}</Figure> : null}
      {tr.status === "imported" &&
      can(permissions.tenant.reconciliation.read) ? (
        <TransferCheck
          engagement={engagement}
          tr={tr}
          run={tr.period ? checks?.[tr.period] : undefined}
        />
      ) : null}
    </span>
  );
}

/**
 * Transfers to Luca / Zirve through the desktop agent (#391): the history
 * and the error screen of the journals page. A delivered transfer is closed
 * by the user after importing in the program.
 */
export function JournalTransfersPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirm, prompt } = useDialogs();
  const transfers = useJournalTransfers(slug, engagement);
  const { completeTransfer, cancelTransfer, retryTransfer } = useJournalActions(
    slug,
    engagement,
  );
  const items = useMemo(
    () => transfers.data?.items ?? [],
    [transfers.data?.items],
  );
  // Aktarılmış dönemlerin mizan karşılaştırması tek istekte (#639); satır
  // başına istek atılmaz.
  const { can } = usePermission();
  const span = useMemo(() => {
    const periods = items
      .filter((tr) => tr.status === "imported" && tr.period)
      .map((tr) => tr.period as string)
      .sort();
    return periods.length
      ? { from: periods[0], to: periods[periods.length - 1] }
      : null;
  }, [items]);
  const checks = useQuery({
    queryKey: reconciliationKeys.latestByPeriod(
      slug,
      engagement,
      span?.from ?? "",
      span?.to ?? "",
    ),
    queryFn: () =>
      reconciliationService.latestByPeriod(
        slug,
        engagement,
        span!.from,
        span!.to,
      ),
    enabled: !!span && can(permissions.tenant.reconciliation.read),
    staleTime: 60_000,
  });
  const runs = checks.data?.runs;
  const busy =
    completeTransfer.isPending ||
    cancelTransfer.isPending ||
    retryTransfer.isPending;
  const actionError =
    completeTransfer.error ?? cancelTransfer.error ?? retryTransfer.error;

  // Her çıktının son görevi (liste yeniden eskiye). Hata ekranı: son görevi
  // hatalıysa ve yerine yenisi açılmadıysa listelenir. "Yeniden dene" de
  // yalnız son görevde: eskisinin yerine yenisi açıldıysa tekrar denenemez.
  const latest = useMemo(() => {
    const byBatch = new Map<string, JournalTransfer>();
    for (const tr of items)
      if (!byBatch.has(tr.batch_uuid)) byBatch.set(tr.batch_uuid, tr);
    return byBatch;
  }, [items]);
  const openFailures = useMemo(
    () => [...latest.values()].filter((tr) => tr.status === "failed"),
    [latest],
  );

  const columns = useMemo<ColumnDef<JournalTransfer, unknown>[]>(() => {
    const markImported = async (tr: JournalTransfer) => {
      const ok = await confirm({
        title: t("office.journals.transfer_imported_title"),
        description: t("office.journals.transfer_imported_description", {
          program: program(t, tr.driver),
        }),
        confirmLabel: t("office.journals.transfer_mark_imported"),
      });
      if (ok)
        completeTransfer.mutate({ transfer: tr.uuid, outcome: "imported" });
    };
    const markFailed = async (tr: JournalTransfer) => {
      const message = await prompt({
        title: t("office.journals.transfer_failed_title"),
        description: t("office.journals.transfer_failed_description", {
          program: program(t, tr.driver),
        }),
        placeholder: t("office.journals.transfer_failed_placeholder"),
        confirmLabel: t("office.journals.transfer_mark_failed"),
      });
      if (message?.trim())
        completeTransfer.mutate({
          transfer: tr.uuid,
          outcome: "failed",
          message: message.trim(),
        });
    };
    return [
      createDateColumn<JournalTransfer>({
        id: "created_at",
        labelKey: "office.journals.transfer_col_requested",
        accessor: (r) => r.created_at,
        style: "datetime",
      }),
      createColumn<JournalTransfer, unknown>({
        id: "driver",
        labelKey: "office.journals.driver",
        accessorFn: (r) => r.driver,
        cell: ({ row }) => (
          <span className="font-medium">{program(t, row.original.driver)}</span>
        ),
        gridPrimary: true,
      }),
      createColumn<JournalTransfer, unknown>({
        id: "period",
        labelKey: "office.api.col_period",
        accessorFn: (r) => r.period ?? "",
        cell: ({ row }) => <Figure>{row.original.period ?? "—"}</Figure>,
      }),
      createStatusColumn<JournalTransfer>({
        id: "status",
        labelKey: "office.col_status",
        accessor: (r) => r.status,
        variants: TRANSFER_STATUS,
        filterable: true,
      }),
      createColumn<JournalTransfer, unknown>({
        id: "detail",
        labelKey: "office.journals.transfer_col_detail",
        enableSorting: false,
        accessorFn: (r) => r.error_message ?? r.folder_label ?? "",
        cell: ({ row }) => (
          <Detail tr={row.original} engagement={engagement} checks={runs} />
        ),
      }),
      createActionsColumn<JournalTransfer>({
        cell: (tr) => (
          <span className="flex flex-wrap justify-end gap-1">
            {tr.status === "delivered" ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void markImported(tr)}
                >
                  <Check className="size-4" />
                  {t("office.journals.transfer_mark_imported")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  onClick={() => void markFailed(tr)}
                >
                  <XCircle className="size-4" />
                  {t("office.journals.transfer_mark_failed")}
                </Button>
              </>
            ) : null}
            {tr.status === "queued" ||
            tr.status === "claimed" ||
            tr.status === "delivered" ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => cancelTransfer.mutate(tr.uuid)}
              >
                <X className="size-4" />
                {t("office.journals.transfer_cancel")}
              </Button>
            ) : null}
            {(tr.status === "failed" || tr.status === "cancelled") &&
            latest.get(tr.batch_uuid)?.uuid === tr.uuid ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => retryTransfer.mutate(tr.uuid)}
              >
                <RotateCcw className="size-4" />
                {t("office.journals.transfer_retry")}
              </Button>
            ) : null}
          </span>
        ),
      }),
    ];
  }, [
    busy,
    cancelTransfer,
    completeTransfer,
    confirm,
    engagement,
    latest,
    prompt,
    retryTransfer,
    runs,
    t,
  ]);

  if (!transfers.isLoading && !transfers.isError && items.length === 0) {
    return (
      <Panel
        id={TRANSFERS_ANCHOR}
        title={t("office.journals.transfers_title")}
        description={t("office.journals.transfers_description")}
      >
        <p className="text-muted-foreground px-4 py-3 text-sm">
          {t("office.journals.transfers_empty")}
        </p>
      </Panel>
    );
  }

  return (
    <Panel
      id={TRANSFERS_ANCHOR}
      title={t("office.journals.transfers_title")}
      description={t("office.journals.transfers_description")}
    >
      <div className="space-y-3 p-4">
        {openFailures.length ? (
          <div
            role="alert"
            className="border-status-blocking/40 rounded-md border p-3 text-sm"
          >
            <p className="text-status-blocking mb-1 flex items-center gap-2 font-medium">
              <AlertTriangle className="size-4" />
              {t("office.journals.transfer_failures", {
                count: openFailures.length,
              })}
            </p>
            <ul className="space-y-0.5 text-xs">
              {openFailures.map((tr) => (
                <li key={tr.uuid}>
                  <span className="font-medium">{program(t, tr.driver)}</span>
                  {tr.period ? ` · ${tr.period}` : ""}:{" "}
                  <span>{transferError(t, tr)}</span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {actionError ? (
          <p className="text-status-blocking text-sm">
            {actionErrorText(t, actionError)}
          </p>
        ) : null}
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(r) => r.uuid}
          isLoading={transfers.isLoading}
          isError={transfers.isError}
          onRetry={() => void transfers.refetch()}
          emptyTitle={t("office.journals.transfers_empty")}
          features={{
            persistKey: "office-journal-transfers-v1",
            rowSelection: false,
          }}
        />
      </div>
    </Panel>
  );
}
