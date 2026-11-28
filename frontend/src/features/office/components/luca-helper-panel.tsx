"use client";

import { Upload } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { ColumnDef } from "@tanstack/react-table";

import { EntityTable } from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createStatusColumn,
  StatusPill,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useAgentTaskActions,
  useAgentTasks,
  useDesktopHelperSettings,
} from "../hooks/use-agent-tasks";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import { declarationPeriod, useOfficePeriod, usePeriodText } from "../period";
import {
  AGENT_FILE_KINDS,
  OPEN_TASK_STATUSES,
  type AgentTask,
  type AgentTaskFileKind,
  type AgentTaskReportRow,
  type LucaHelperConsent,
  type LucaHelperRunner,
} from "../services/agent-tasks.service";
import { Panel } from "./office-ui";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;


export const AGENT_TASK_STATUS: Record<string, StatusVariant> = {
  queued: { labelKey: "office.luca_helper.status_queued", tone: "info" },
  claimed: { labelKey: "office.luca_helper.status_claimed", tone: "info" },
  waiting_user: { labelKey: "office.luca_helper.status_waiting_user", tone: "warn" },
  running: { labelKey: "office.luca_helper.status_running", tone: "info" },
  done: { labelKey: "office.luca_helper.status_done", tone: "ok" },
  partial: { labelKey: "office.luca_helper.status_partial", tone: "warn" },
  failed: { labelKey: "office.luca_helper.status_failed", tone: "danger" },
  needs_check: { labelKey: "office.luca_helper.status_needs_check", tone: "warn" },
  cancelled: { labelKey: "office.luca_helper.status_cancelled", tone: "neutral" },
};

const FILE_TONE = {
  pending: "neutral",
  ok: "ok",
  failed: "danger",
  unknown: "warn",
} as const;

const CARI_TONE = {
  pending: "neutral",
  found: "ok",
  created: "ok",
  failed: "danger",
  conflict: "warn",
} as const;

/** Sunucu hata kodu → Türkçe ileti; bilinmeyen kodda sunucu metni. */
function apiErrorText(t: Translate, err: unknown, fallback: string) {
  if (!isApiError(err)) return fallback;
  const key = `office.luca_helper.error_${err.code}`;
  const msg = t(key);
  return msg === key ? err.message || fallback : msg;
}

/** Görev satırının kısa özeti: ilerleme, hata ya da dosya sonucu. */
function summary(t: Translate, task: AgentTask) {
  if (task.kind === "luca_cari_sync" && !task.approved && task.status === "queued")
    return t("office.luca_helper.summary_needs_approval", {
      count: task.counterparties.length,
    });
  if (OPEN_TASK_STATUSES.has(task.status)) {
    if (task.progress?.message) return task.progress.message;
    if (task.status === "queued") return t("office.luca_helper.summary_waiting_agent");
    if (task.status === "waiting_user") return t("office.luca_helper.summary_waiting_user");
    return task.progress
      ? t("office.luca_helper.summary_step", {
          step: task.progress.step,
          steps: task.progress.steps,
        })
      : "";
  }
  if (task.status === "failed" || task.status === "cancelled")
    return task.error_message || task.error_code || "";
  if (task.kind === "luca_cari_sync") {
    const ok = task.counterparties.filter(
      (c) => c.status === "found" || c.status === "created",
    ).length;
    return t("office.luca_helper.summary_cari", { ok, total: task.counterparties.length });
  }
  const failed = task.files.filter((f) => f.status !== "ok").length;
  return failed
    ? t("office.luca_helper.summary_files_failed", {
        failed,
        total: task.files.length,
      })
    : t("office.luca_helper.summary_files_ok", { count: task.files.length });
}

/**
 * Masaüstü yardımcısıyla Luca'ya tek tık aktarım (tasarım: masaüstü
 * yardımcısı, Luca). Yardımcı ofisin Luca hesabına o bilgisayarda kayıtlı
 * girişle (ya da kullanıcı girerek) girer; şifre sunucuya gelmez. Ofis ayarı
 * varsayılan kapalıdır; ofisin açık izni olmadan görev alınmaz.
 */
export function LucaHelperPanel() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canExport = can(permissions.tenant.journals.export);
  const { query: settings, save } = useDesktopHelperSettings(slug);
  const enabled = settings.data?.luca_enabled ?? false;
  const tasks = useAgentTasks(slug);
  const periodText = usePeriodText();
  const [startOpen, setStartOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const items = tasks.data?.items ?? [];
  const current = items.find((x) => x.uuid === selected) ?? null;

  const columns = useMemo<ColumnDef<AgentTask, unknown>[]>(
    () => [
      createColumn<AgentTask>({
        id: "taxpayer",
        labelKey: "office.col_taxpayer",
        accessorFn: (r) => r.taxpayer.title,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.taxpayer.title}</span>
        ),
      }),
      createColumn<AgentTask>({
        id: "period",
        labelKey: "office.api.col_period",
        accessorFn: (r) => r.period ?? "",
        cell: ({ row }) =>
          row.original.period ? periodText(row.original.period) : "—",
      }),
      createColumn<AgentTask>({
        id: "kind",
        labelKey: "office.luca_helper.col_kind",
        accessorFn: (r) => t(`office.luca_helper.kind_${r.kind}`),
      }),
      createStatusColumn<AgentTask>({
        id: "status",
        labelKey: "office.col_status",
        accessor: (r) => r.status,
        variants: AGENT_TASK_STATUS,
        gridSecondary: true,
      }),
      createColumn<AgentTask>({
        id: "summary",
        labelKey: "office.luca_helper.col_summary",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs">
            {summary(t, row.original)}
          </span>
        ),
      }),
      createDateColumn<AgentTask>({
        id: "created_at",
        labelKey: "office.journals.transfer_col_requested",
        accessor: (r) => r.created_at,
        style: "datetime",
      }),
    ],
    [t, periodText],
  );

  return (
    <Panel
      title={t("office.luca_helper.title")}
      description={t("office.luca_helper.description")}
      actions={
        <div className="flex flex-wrap items-center gap-3">
          {canExport ? (
            <Label className="flex items-center gap-2 text-xs font-normal">
              <Switch
                checked={enabled}
                disabled={settings.isLoading || save.isPending}
                onCheckedChange={(v) =>
                  save.mutate(
                    { luca_enabled: v },
                    {
                      onError: () => toast.error(t("office.luca_helper.settings_failed")),
                    },
                  )
                }
                data-testid="luca-helper-switch"
              />
              {t("office.luca_helper.switch")}
            </Label>
          ) : null}
          {canExport ? (
            <Button
              size="sm"
              disabled={!enabled}
              onClick={() => setStartOpen(true)}
              data-testid="luca-helper-start"
            >
              <Upload className="size-4" aria-hidden />
              {t("office.luca_helper.start")}
            </Button>
          ) : null}
        </div>
      }
    >
      <div className="space-y-3 p-4">
        {!enabled && !settings.isLoading ? (
          <p className="text-muted-foreground text-xs">{t("office.luca_helper.off_hint")}</p>
        ) : null}
        {settings.data ? (
          <HelperOfficeStatus
            t={t}
            locale={locale}
            enabled={enabled}
            consent={settings.data.luca_consent ?? null}
            runner={settings.data.luca_runner ?? null}
          />
        ) : null}
        <EntityTable<AgentTask>
          columns={columns}
          data={items}
          getRowId={(r) => r.uuid}
          onRowClick={(r) => setSelected(r.uuid)}
          isLoading={tasks.isLoading}
          isError={tasks.isError}
          errorTitle={t("office.load_failed")}
          onRetry={() => void tasks.refetch()}
          emptyTitle={t("office.luca_helper.empty")}
          emptyDescription=""
          manual={{ sorting: false, filtering: false, pagination: false }}
          initialState={{ pagination: { pageIndex: 0, pageSize: 25 } }}
          pageSizeOptions={[25, 50, 100]}
          features={{
            globalFilter: true,
            columnFilters: false,
            facetedFilters: false,
            rowSelection: false,
            viewMode: false,
            persistKey: "office.transfers.luca_helper",
          }}
        />
      </div>
      <LucaExportDialog open={startOpen} onOpenChange={setStartOpen} />
      <AgentTaskDialog
        task={current}
        canAct={canExport}
        onOpenChange={(o) => (o ? null : setSelected(null))}
      />
    </Panel>
  );
}

/** "Luca'ya aktar": mükellef(ler) + dönem + aktarılacak dosya türleri. */
function LucaExportDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { period: officePeriod } = useOfficePeriod();
  const directory = useEngagementDirectory(open);
  const { start } = useAgentTaskActions(slug);
  const [period, setPeriod] = useState(officePeriod ?? declarationPeriod());
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [kinds, setKinds] = useState<Set<AgentTaskFileKind>>(
    new Set(AGENT_FILE_KINDS),
  );
  const [filter, setFilter] = useState("");

  const visible = useMemo(() => {
    const q = filter.trim().toLocaleLowerCase("tr");
    return q
      ? directory.list.filter(
          (e) =>
            e.title.toLocaleLowerCase("tr").includes(q) || e.taxId.includes(q),
        )
      : directory.list;
  }, [directory.list, filter]);

  const toggle = <T,>(set: Set<T>, v: T, on: boolean) => {
    const next = new Set(set);
    if (on) next.add(v);
    else next.delete(v);
    return next;
  };

  const submit = () =>
    start.mutate(
      {
        engagement_uuids: [...chosen],
        period,
        kinds: AGENT_FILE_KINDS.filter((k) => kinds.has(k)),
      },
      {
        onSuccess: (run) => {
          toast.success(t("office.luca_helper.started", { count: run.items.length }));
          onOpenChange(false);
        },
        onError: (e) => toast.error(apiErrorText(t, e, t("office.luca_helper.start_failed"))),
      },
    );

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) {
          setPeriod(officePeriod ?? declarationPeriod());
          setChosen(new Set());
          setKinds(new Set(AGENT_FILE_KINDS));
          setFilter("");
        }
        onOpenChange(o);
      }}
    >
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office.luca_helper.start_title")}</DialogTitle>
          <DialogDescription>{t("office.luca_helper.start_hint")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1">
          <Label
            htmlFor="luca-helper-period"
            className="text-xs leading-normal font-normal"
          >
            {t("office.luca_helper.period")}
          </Label>
          <MonthPicker
            id="luca-helper-period"
            className="w-48"
            value={period}
            onChange={setPeriod}
          />
        </div>
        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs">
              {t("office.luca_helper.taxpayers", { count: chosen.size })}
            </span>
            <Button
              variant="link"
              size="sm"
              onClick={() =>
                setChosen(
                  chosen.size === visible.length && visible.length
                    ? new Set()
                    : new Set(visible.map((e) => e.engagementUuid)),
                )
              }
            >
              {chosen.size === visible.length && visible.length
                ? t("office.luca_helper.select_none")
                : t("office.luca_helper.select_all")}
            </Button>
          </div>
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t("office.luca_helper.search")}
            aria-label={t("office.luca_helper.search")}
          />
          <div className="max-h-56 overflow-y-auto rounded-lg border p-2">
            {directory.isLoading ? (
              <p className="text-muted-foreground text-xs">
                {t("common.loading")}
              </p>
            ) : (
              visible.map((e) => (
                <Label
                  key={e.engagementUuid}
                  className="flex items-center gap-2 py-1 text-sm font-normal"
                >
                  <Checkbox
                    checked={chosen.has(e.engagementUuid)}
                    onCheckedChange={(v) =>
                      setChosen(toggle(chosen, e.engagementUuid, v === true))
                    }
                  />
                  <span className="truncate">{e.title}</span>
                  <span className="text-muted-foreground ml-auto text-xs">
                    {e.taxId}
                  </span>
                </Label>
              ))
            )}
          </div>
        </div>
        <FieldSet className="gap-1">
          <FieldLegend variant="label" className="mb-1 text-xs font-normal">
            {t("office.luca_helper.kinds")}
          </FieldLegend>
          {AGENT_FILE_KINDS.map((k) => (
            <Label
              key={k}
              className="flex items-center gap-2 text-sm font-normal"
            >
              <Checkbox
                checked={kinds.has(k)}
                onCheckedChange={(v) => setKinds(toggle(kinds, k, v === true))}
              />
              {t(`office.luca_helper.file_${k}`)}
            </Label>
          ))}
        </FieldSet>
        <p className="text-muted-foreground text-xs">
          {t("office.luca_helper.responsibility")}
        </p>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("office.luca_helper.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={!chosen.size || !kinds.size || start.isPending}
            data-testid="luca-helper-submit"
          >
            {t("office.luca_helper.start_submit", { count: chosen.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ReportRows({
  title,
  rows,
}: {
  title: string;
  rows: AgentTaskReportRow[];
}) {
  const { t } = useLocale();
  if (!rows.length) return null;
  return (
    <section className="space-y-1">
      <h3 className="text-xs font-semibold">
        {title} ({rows.length})
      </h3>
      <ul className="divide-y rounded-lg border text-xs">
        {rows.map((r, i) => (
          <li key={`${r.document_no}-${i}`} className="flex flex-col p-2">
            <span className="font-medium">
              {r.document_no || "—"}
              {r.date ? ` · ${r.date}` : ""}
              {r.kind ? ` · ${t(`office.luca_helper.file_${r.kind}`)}` : ""}
            </span>
            <span className="text-muted-foreground">
              {r.reason === "NOT_EXPORTABLE"
                ? t("office.luca_helper.reason_not_exportable")
                : r.reason}
              {r.account ? ` (${r.account})` : ""}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * Görev ayrıntısı ve hatalı satır raporu: dosya başına Luca sonucu, hata
 * veren fişler (belge numarasıyla), cari eşitleme sonucu ve sunucunun
 * dosyaya almadığı belgeler.
 */
function AgentTaskDialog({
  task,
  canAct,
  onOpenChange,
}: {
  task: AgentTask | null;
  canAct: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirm } = useDialogs();
  const { approve, cancel, resolve } = useAgentTaskActions(slug);
  const periodText = usePeriodText();
  if (!task) return null;

  const fail = (e: unknown) =>
    toast.error(apiErrorText(t, e, t("office.luca_helper.action_failed")));
  const status = AGENT_TASK_STATUS[task.status];
  const needsApproval =
    task.kind === "luca_cari_sync" && !task.approved && task.status === "queued";
  const cancellable =
    task.status === "queued" ||
    task.status === "claimed" ||
    task.status === "waiting_user";
  const report = task.server_report;
  const failedEntries = task.files.flatMap((f) =>
    f.entry_results
      .filter((e) => e.status === "failed")
      .map((e) => ({ file: f.name, ...e })),
  );

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {task.taxpayer.title}
            {task.period ? ` · ${periodText(task.period)}` : ""}
          </DialogTitle>
          <DialogDescription>
            {t(`office.luca_helper.kind_${task.kind}`)}
            {task.agent?.name ? ` · ${task.agent.name}` : ""}
            {task.requested_by_name
              ? ` · ${t("office.luca_helper.requested_by", { name: task.requested_by_name })}`
              : ""}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <StatusPill tone={status?.tone}>
            {status ? t(status.labelKey) : task.status}
          </StatusPill>
          <span className="text-muted-foreground text-xs">
            {summary(t, task)}
          </span>
        </div>
        {task.status === "needs_check" ? (
          <p className="text-status-warning text-xs">
            {t("office.luca_helper.needs_check_hint")}
          </p>
        ) : null}
        {needsApproval ? (
          <p className="text-xs">{t("office.luca_helper.approve_hint")}</p>
        ) : null}

        {task.counterparties.length ? (
          <section className="space-y-1">
            <h3 className="text-xs font-semibold">
              {t("office.luca_helper.counterparties")} ({task.counterparties.length})
            </h3>
            <ul className="divide-y rounded-lg border text-xs">
              {task.counterparties.map((c) => (
                <li
                  key={`${c.tax_id}-${c.main_code}`}
                  className="flex flex-wrap items-center gap-2 p-2"
                >
                  <span className="font-medium">{c.title || c.tax_id}</span>
                  <span className="text-muted-foreground">
                    {c.tax_id} · {c.main_code}
                  </span>
                  {c.account_code ? (
                    <span className="font-mono">{c.account_code}</span>
                  ) : null}
                  <StatusPill tone={CARI_TONE[c.status]} className="ml-auto">
                    {t(`office.luca_helper.cari_${c.status}`)}
                  </StatusPill>
                  {c.message ? (
                    <span className="text-muted-foreground w-full">
                      {c.message}
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {task.files.length ? (
          <section className="space-y-1">
            <h3 className="text-xs font-semibold">
              {t("office.luca_helper.files")} ({task.files.length})
            </h3>
            <ul className="divide-y rounded-lg border text-xs">
              {task.files.map((f) => (
                <li
                  key={f.uuid}
                  className="flex flex-wrap items-center gap-2 p-2"
                >
                  <span className="font-medium">{f.name}</span>
                  <span className="text-muted-foreground">
                    {t(`office.luca_helper.file_${f.kind}`)}
                  </span>
                  <StatusPill tone={FILE_TONE[f.status]} className="ml-auto">
                    {t(`office.luca_helper.file_status_${f.status}`)}
                  </StatusPill>
                  {f.luca_message ? (
                    <span className="text-muted-foreground w-full">
                      {f.luca_message}
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {failedEntries.length ? (
          <section className="space-y-1" data-testid="luca-helper-errors">
            <h3 className="text-status-blocking text-xs font-semibold">
              {t("office.luca_helper.failed_rows")} ({failedEntries.length})
            </h3>
            <ul className="divide-y rounded-lg border text-xs">
              {failedEntries.map((e, i) => (
                <li key={`${e.file}-${i}`} className="flex flex-col p-2">
                  <span className="font-medium">
                    {e.document_no ||
                      (e.row
                        ? t("office.luca_helper.row", { row: e.row })
                        : t("office.luca_helper.unknown_document"))}
                    <span className="text-muted-foreground font-normal">
                      {" "}
                      · {e.file}
                    </span>
                  </span>
                  <span className="text-muted-foreground">
                    {e.reason || t("office.luca_helper.no_reason")}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {report ? (
          <>
            <ReportRows title={t("office.luca_helper.report_check")} rows={report.check} />
            <ReportRows
              title={t("office.luca_helper.report_skipped")}
              rows={report.skipped}
            />
            <ReportRows
              title={t("office.luca_helper.report_already")}
              rows={report.already_exported}
            />
          </>
        ) : null}

        {canAct && (needsApproval || cancellable || task.status === "needs_check") ? (
          <DialogFooter className="flex-wrap gap-2">
            {cancellable ? (
              <Button
                variant="outline"
                disabled={cancel.isPending}
                onClick={async () => {
                  const ok = await confirm({
                    title: t("office.luca_helper.cancel_title"),
                    description: t("office.luca_helper.cancel_description"),
                    confirmLabel: t("office.luca_helper.cancel_task"),
                    variant: "destructive",
                  });
                  if (ok) cancel.mutate(task.uuid, { onError: fail });
                }}
              >
                {t("office.luca_helper.cancel_task")}
              </Button>
            ) : null}
            {task.status === "needs_check" ? (
              <>
                <Button
                  variant="outline"
                  disabled={resolve.isPending}
                  onClick={() =>
                    resolve.mutate(
                      { id: task.uuid, imported: false },
                      { onError: fail },
                    )
                  }
                >
                  {t("office.luca_helper.resolve_not_imported")}
                </Button>
                <Button
                  disabled={resolve.isPending}
                  onClick={async () => {
                    const ok = await confirm({
                      title: t("office.luca_helper.resolve_title"),
                      description: t("office.luca_helper.resolve_description"),
                      confirmLabel: t("office.luca_helper.resolve_imported"),
                    });
                    if (ok)
                      resolve.mutate(
                        { id: task.uuid, imported: true },
                        { onError: fail },
                      );
                  }}
                >
                  {t("office.luca_helper.resolve_imported")}
                </Button>
              </>
            ) : null}
            {needsApproval ? (
              <Button
                disabled={approve.isPending}
                onClick={() => approve.mutate(task.uuid, { onError: fail })}
                data-testid="luca-helper-approve"
              >
                {t("office.luca_helper.approve", { count: task.counterparties.length })}
              </Button>
            ) : null}
          </DialogFooter>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function shortDateTime(iso: string | null | undefined, locale: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(locale, { dateStyle: "short", timeStyle: "short" });
}

/**
 * Ofis izni ve aktarıcı bilgisayar (sözleşme 1.3): izin masaüstünde verilir;
 * izin yoksa yardımcı görev almaz. Aktarıcı, son yoklayan masaüstüdür.
 */
function HelperOfficeStatus({
  t,
  locale,
  enabled,
  consent,
  runner,
}: {
  t: Translate;
  locale: string;
  enabled: boolean;
  consent: LucaHelperConsent | null;
  runner: LucaHelperRunner | null;
}) {
  return (
    <div className="flex flex-col gap-1 text-xs" data-testid="luca-helper-office">
      {runner ? (
        <div className="flex flex-wrap items-center gap-2" data-testid="luca-helper-runner">
          <span>
            {t("office.luca_helper.runner", { name: runner.name || "—" })}
          </span>
          <StatusPill tone={runner.online ? "ok" : "neutral"}>
            {t(runner.online ? "office.luca_helper.runner_online" : "office.luca_helper.runner_offline")}
          </StatusPill>
          {runner.last_seen_at ? (
            <span className="text-muted-foreground">
              {t("office.luca_helper.runner_seen", { time: shortDateTime(runner.last_seen_at, locale) })}
            </span>
          ) : null}
        </div>
      ) : (
        <p className="text-muted-foreground">{t("office.luca_helper.runner_none")}</p>
      )}
      {consent ? (
        <p className="text-muted-foreground" data-testid="luca-helper-consent">
          {t("office.luca_helper.consent_given", {
            name: consent.given_by_name || "—",
            time: shortDateTime(consent.given_at, locale),
          })}
        </p>
      ) : enabled ? (
        <p className="text-amber-700 dark:text-amber-400" data-testid="luca-helper-consent-missing">
          {t("office.luca_helper.consent_missing")}
        </p>
      ) : null}
    </div>
  );
}
