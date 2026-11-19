"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { FileSpreadsheet, Upload } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useId, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { BosDurum } from "@/components/hakan";
import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  duplicateFields,
  excelBusy,
  excelStep,
  missingRequired,
} from "../lib/excel-import";
import {
  excelImportKeys,
  excelImportsService,
  type ExcelDataTypeKey,
  type ExcelImport,
  type ExcelImportDataType,
  type ExcelRowResult,
} from "../services/excel-imports.service";
import { Figure } from "./office-ui";

const IMPORT_PARAM = "aktarim";
const NONE = "__none__";
const AUTO = "__auto__";
const ROWS_PAGE = 20;

const statusTone: Record<ExcelImport["status"], StatusTone> = {
  uploaded: "info",
  analyzing: "info",
  validating: "info",
  mapping_review: "warn",
  has_errors: "danger",
  preview: "ok",
  committed: "ok",
  failed: "danger",
  rolled_back: "neutral",
};

const errText = (e: unknown, fallback: string) =>
  isApiError(e) ? e.message : fallback;

/**
 * `/t/{slug}/engagements/{uuid}/excel-imports` (#518 B1): genel Excel / CSV
 * içe aktarma. Yükle → sütunları eşle → doğrula → hatalı satırı atla → işle
 * (ya da fatura aktarımını geri al). Açık aktarım adreste (`?aktarim=`).
 */
export function EngagementExcelImportsPage({
  engagement,
}: {
  engagement: string;
}) {
  return (
    <Suspense fallback={<Loading />}>
      <Inner engagement={engagement} />
    </Suspense>
  );
}

function Inner({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRead = can(Permission.TenantExcelImportsRead);
  const canWrite = can(Permission.TenantExcelImportsWrite);
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const openUuid = params.get(IMPORT_PARAM);
  const setOpen = (uuid: string | null) => {
    const next = new URLSearchParams(params.toString());
    if (uuid) next.set(IMPORT_PARAM, uuid);
    else next.delete(IMPORT_PARAM);
    const qs = next.toString();
    router.replace(`${pathname}${qs ? `?${qs}` : ""}`, { scroll: false });
  };
  const types = useQuery({
    queryKey: excelImportKeys.dataTypes(slug),
    queryFn: () => excelImportsService.dataTypes(slug),
    enabled: canRead,
    staleTime: 5 * 60_000,
  });
  const list = useQuery({
    queryKey: excelImportKeys.list(slug, engagement),
    queryFn: () => excelImportsService.list(slug, engagement),
    enabled: canRead,
  });

  if (!canRead)
    return (
      <BosDurum
        baslik={t("office.excel.forbidden_title")}
        aciklama={t("office.excel.forbidden")}
      />
    );
  const dataTypes = types.data?.items ?? [];
  const items = list.data?.items ?? [];
  return (
    <div className="space-y-5">
      <div>
        <h2 className="font-display text-xl font-bold">
          {t("office.excel.title")}
        </h2>
        <p className="text-text-2 text-sm">{t("office.excel.description")}</p>
      </div>

      {canWrite ? (
        <UploadForm
          engagement={engagement}
          dataTypes={dataTypes}
          onUploaded={(x) => setOpen(x.uuid)}
        />
      ) : null}

      {openUuid ? (
        <ImportDetail
          key={openUuid}
          engagement={engagement}
          uuid={openUuid}
          dataTypes={dataTypes}
          onClose={() => setOpen(null)}
        />
      ) : null}

      <section aria-labelledby="excel-list-title" className="space-y-3">
        <h3 id="excel-list-title" className="text-sm font-semibold">
          {t("office.excel.list_title")}
        </h3>
        {list.isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : list.isError ? (
          <ErrorState
            title={t("office.excel.load_failed")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : items.length === 0 ? (
          <BosDurum
            baslik={t("office.excel.empty_title")}
            aciklama={t("office.excel.empty")}
          />
        ) : (
          <ul
            className="bg-surface border-line divide-line divide-y rounded-xl border"
            data-testid="excel-imports"
          >
            {items.map((x) => (
              <li key={x.uuid}>
                <Button
                  variant="ghost"
                  aria-current={x.uuid === openUuid ? "true" : undefined}
                  onClick={() => setOpen(x.uuid === openUuid ? null : x.uuid)}
                  className="hover:bg-surface-2 aria-[current=true]:bg-brand-tint h-auto min-h-11 w-full flex-wrap justify-start gap-3 rounded-none px-4 py-3 text-start font-normal whitespace-normal has-[>svg]:px-4"
                >
                  <FileSpreadsheet
                    aria-hidden
                    className="text-text-3 size-4 shrink-0"
                  />
                  <span className="min-w-40 flex-1 truncate text-sm font-medium">
                    {x.file_name}
                  </span>
                  <span className="text-text-2 text-xs">
                    {typeLabel(dataTypes, x.data_type) ??
                      t("office.excel.type_unknown")}
                  </span>
                  <StatusPill tone={statusTone[x.status]}>
                    {t(`office.excel.status_${x.status}`)}
                  </StatusPill>
                  <time
                    dateTime={x.created_at}
                    className="text-text-3 text-xs whitespace-nowrap"
                  >
                    {datetime(x.created_at)}
                  </time>
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <TemplatesSection dataTypes={dataTypes} canWrite={canWrite} />
    </div>
  );
}

/**
 * Kayıtlı sütun şablonları: aynı düzendeki dosya bir dahaki sefere yapay
 * zekâsız eşlenir. Ofis şablonları silinebilir; genel şablonlar silinemez.
 */
function TemplatesSection({
  dataTypes,
  canWrite,
}: {
  dataTypes: ExcelImportDataType[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const q = useQuery({
    queryKey: excelImportKeys.templates(slug),
    queryFn: () => excelImportsService.templates(slug),
    enabled: open,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      excelImportsService.deleteTemplate(slug, uuid),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: excelImportKeys.templates(slug) });
      toast.success(t("office.excel.template_deleted"));
    },
    onError: (e) => toast.error(errText(e, t("office.excel.action_failed"))),
  });
  const items = q.data?.items ?? [];
  return (
    <section aria-labelledby="excel-templates-title" className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <h3 id="excel-templates-title" className="flex-1 text-sm font-semibold">
          {t("office.excel.templates_title")}
        </h3>
        <Button
          variant="outline"
          size="sm"
          aria-expanded={open}
          aria-controls="excel-templates-list"
          onClick={() => setOpen((v) => !v)}
        >
          {open
            ? t("office.excel.templates_hide")
            : t("office.excel.templates_show")}
        </Button>
      </div>
      {open ? (
        <div id="excel-templates-list">
          {q.isLoading ? (
            <Skeleton className="h-16 w-full" />
          ) : q.isError ? (
            <p role="alert" className="text-danger text-sm">
              {q.error.message}
            </p>
          ) : items.length === 0 ? (
            <p className="text-text-3 text-sm">
              {t("office.excel.templates_empty")}
            </p>
          ) : (
            <ul className="bg-surface border-line divide-line divide-y rounded-xl border">
              {items.map((x) => (
                <li
                  key={x.uuid}
                  className="flex flex-wrap items-center gap-3 px-4 py-3"
                >
                  <span className="min-w-40 flex-1 text-sm">
                    <span className="font-medium">
                      {typeLabel(dataTypes, x.data_type) ??
                        t("office.excel.type_unknown")}
                    </span>
                    {x.source_system ? (
                      <span className="text-text-2"> · {x.source_system}</span>
                    ) : null}
                    <span className="text-text-3 block text-xs">
                      {t("office.excel.template_columns", {
                        count: (x.columns ?? []).filter((c) => c.field).length,
                      })}
                    </span>
                  </span>
                  <StatusPill tone={x.scope === "global" ? "neutral" : "info"}>
                    {t(`office.excel.scope_${x.scope ?? "tenant"}`)}
                  </StatusPill>
                  {canWrite && x.scope !== "global" && x.uuid ? (
                    <Button
                      size="sm"
                      variant="outline"
                      className="max-md:min-h-11"
                      disabled={remove.isPending}
                      onClick={async () => {
                        const ok = await confirmDelete({
                          title: t("office.excel.template_delete_title"),
                          description: t("office.excel.template_delete_body"),
                        });
                        if (ok && x.uuid) remove.mutate(x.uuid);
                      }}
                    >
                      {t("office.excel.template_delete")}
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </section>
  );
}

function typeLabel(types: ExcelImportDataType[], key?: string | null) {
  return key ? (types.find((d) => d.data_type === key)?.label ?? key) : null;
}

function UploadForm({
  engagement,
  dataTypes,
  onUploaded,
}: {
  engagement: string;
  dataTypes: ExcelImportDataType[];
  onUploaded: (x: ExcelImport) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const [dataType, setDataType] = useState<string>(AUTO);
  const [period, setPeriod] = useState("");
  const [sheet, setSheet] = useState("");
  const upload = useMutation({
    mutationFn: (file: File) =>
      excelImportsService.upload(slug, engagement, {
        file,
        dataType:
          dataType === AUTO ? undefined : (dataType as ExcelDataTypeKey),
        period: /^\d{4}-(0[1-9]|1[0-2])$/.test(period) ? period : undefined,
        sheet: sheet.trim() || undefined,
      }),
    onSuccess: (x) => {
      void qc.invalidateQueries({
        queryKey: excelImportKeys.list(slug, engagement),
      });
      qc.setQueryData(excelImportKeys.one(slug, engagement, x.uuid), x);
      toast.success(t("office.excel.uploaded"));
      onUploaded(x);
    },
    onError: (e) => toast.error(errText(e, t("office.excel.upload_failed"))),
  });
  return (
    <section
      aria-labelledby={`${id}-title`}
      className="bg-surface border-line space-y-3 rounded-xl border p-4"
    >
      <h3 id={`${id}-title`} className="text-sm font-semibold">
        {t("office.excel.new_title")}
      </h3>
      <div className="grid gap-3 sm:grid-cols-[1fr_10rem_12rem_auto] sm:items-end">
        <div className="space-y-1">
          <Label htmlFor={`${id}-type`}>{t("office.excel.data_type")}</Label>
          <Select value={dataType} onValueChange={setDataType}>
            <SelectTrigger id={`${id}-type`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={AUTO}>
                {t("office.excel.type_auto")}
              </SelectItem>
              {dataTypes
                .filter((d) => d.available)
                .map((d) => (
                  <SelectItem key={d.data_type} value={d.data_type}>
                    {d.label}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-period`}>{t("office.excel.period")}</Label>
          <Input
            id={`${id}-period`}
            inputMode="numeric"
            placeholder="2026-10"
            pattern="\d{4}-(0[1-9]|1[0-2])"
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-sheet`}>{t("office.excel.sheet")}</Label>
          <Input
            id={`${id}-sheet`}
            placeholder={t("office.excel.sheet_placeholder")}
            value={sheet}
            onChange={(e) => setSheet(e.target.value)}
          />
        </div>
        {/* Dosya seçimi gizli girişle; tetikleyen düğme görünür. */}
        <input
          ref={input}
          type="file"
          className="sr-only"
          accept=".xlsx,.csv"
          aria-label={t("office.excel.choose_file")}
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) upload.mutate(f);
          }}
        />
        <Button
          disabled={upload.isPending}
          onClick={() => input.current?.click()}
        >
          <Upload aria-hidden className="size-4" />
          {upload.isPending
            ? t("office.excel.uploading")
            : t("office.excel.choose_file")}
        </Button>
      </div>
      <p className="text-text-3 text-xs">{t("office.excel.upload_hint")}</p>
    </section>
  );
}

function ImportDetail({
  engagement,
  uuid,
  dataTypes,
  onClose,
}: {
  engagement: string;
  uuid: string;
  dataTypes: ExcelImportDataType[];
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { confirm } = useDialogs();
  const qc = useQueryClient();
  const canWrite = can(Permission.TenantExcelImportsWrite);
  const canCommit = can(Permission.TenantExcelImportsCommit);
  const key = excelImportKeys.one(slug, engagement, uuid);
  const q = useQuery({
    queryKey: key,
    queryFn: () => excelImportsService.get(slug, engagement, uuid),
    // Sunucu çözümlerken / doğrularken durum yoklanır.
    refetchInterval: (query) =>
      query.state.data && excelBusy(query.state.data.status) ? 2000 : false,
  });
  const settle = (x: ExcelImport) => {
    qc.setQueryData(key, x);
    void qc.invalidateQueries({
      queryKey: excelImportKeys.all(slug, engagement),
    });
  };
  const validate = useMutation({
    mutationFn: () => excelImportsService.validate(slug, engagement, uuid),
    onSuccess: settle,
    onError: (e) => toast.error(errText(e, t("office.excel.action_failed"))),
  });
  const commit = useMutation({
    mutationFn: () => excelImportsService.commit(slug, engagement, uuid),
    onSuccess: (x) => {
      settle(x);
      toast.success(
        t("office.excel.committed", { count: x.summary?.added ?? 0 }),
      );
    },
    onError: (e) => toast.error(errText(e, t("office.excel.action_failed"))),
  });
  const rollback = useMutation({
    mutationFn: () => excelImportsService.rollback(slug, engagement, uuid),
    onSuccess: (x) => {
      settle(x);
      toast.success(t("office.excel.rolled_back"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.status === 409
          ? t("office.excel.rollback_not_supported")
          : errText(e, t("office.excel.action_failed")),
      ),
  });

  if (q.isLoading) return <Skeleton className="h-48 w-full" />;
  if (q.isError || !q.data)
    return (
      <ErrorState
        title={t("office.excel.load_failed")}
        description={q.error?.message}
        onRetry={() => void q.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  const x = q.data;
  const step = excelStep(x.status);
  const s = x.summary;
  const type = dataTypes.find((d) => d.data_type === x.data_type);
  const busy = validate.isPending || commit.isPending || rollback.isPending;
  const editable =
    canWrite && (step === "mapping" || step === "rows" || step === "ready");
  const onCommit = async () => {
    const ok = await confirm({
      title: t("office.excel.commit_title"),
      description: t("office.excel.commit_body", {
        count: (s?.ok ?? 0) + (s?.warnings ?? 0),
        skipped: (s?.errors ?? 0) + (s?.skipped ?? 0),
      }),
      confirmLabel: t("office.excel.commit"),
    });
    if (ok) commit.mutate();
  };
  const onRollback = async () => {
    const ok = await confirm({
      title: t("office.excel.rollback_title"),
      description: t("office.excel.rollback_body"),
      confirmLabel: t("office.excel.rollback"),
      variant: "destructive",
    });
    if (ok) rollback.mutate();
  };
  return (
    <section
      aria-labelledby="excel-detail-title"
      className="bg-surface border-brand space-y-4 rounded-xl border p-4"
      data-testid="excel-import-detail"
    >
      <div className="flex flex-wrap items-center gap-3">
        <h3
          id="excel-detail-title"
          className="min-w-40 flex-1 truncate text-base font-semibold"
        >
          {x.file_name}
        </h3>
        <StatusPill tone={statusTone[x.status]}>
          {t(`office.excel.status_${x.status}`)}
        </StatusPill>
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t("office.excel.close")}
        </Button>
      </div>
      <p className="text-text-2 text-sm" aria-live="polite">
        {t(`office.excel.step_${step}`)}
        {x.from_template ? ` ${t("office.excel.from_template")}` : ""}
        {x.ai_used ? ` ${t("office.excel.ai_used")}` : ""}
      </p>
      {x.error ? (
        <p role="alert" className="text-danger text-sm">
          {x.error}
        </p>
      ) : null}

      {s ? (
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
          {(
            [
              ["rows", s.rows],
              ["ok", s.ok],
              ["warnings", s.warnings],
              ["errors", s.errors],
              ["skipped", s.skipped],
            ] as const
          ).map(([k, v]) => (
            <div key={k} className="bg-surface-2 rounded-lg p-3">
              <dt className="text-text-3 text-xs">
                {t(`office.excel.count_${k}`)}
              </dt>
              <dd>
                <Figure className="font-display text-xl font-bold">
                  {v ?? 0}
                </Figure>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {x.status === "committed" && s ? (
        <p className="text-sm">
          {t("office.excel.result", {
            added: s.added ?? 0,
            duplicates: (s.duplicates ?? 0) + (s.existing ?? 0),
            rejected: s.rejected ?? 0,
          })}
          {s.note ? ` ${s.note}` : ""}
        </p>
      ) : null}

      {x.mapping && step !== "working" ? (
        <MappingEditor
          key={`${x.updated_at}`}
          engagement={engagement}
          item={x}
          type={type}
          dataTypes={dataTypes}
          editable={editable}
          onSaved={settle}
        />
      ) : null}

      {step === "rows" || step === "ready" || step === "done" ? (
        <RowsPanel
          engagement={engagement}
          item={x}
          editable={editable}
          onChanged={settle}
        />
      ) : null}

      <div className="flex flex-wrap gap-2">
        {editable ? (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => validate.mutate()}
          >
            {t("office.excel.validate")}
          </Button>
        ) : null}
        {step === "ready" && canCommit ? (
          <Button disabled={busy} onClick={() => void onCommit()}>
            {t("office.excel.commit")}
          </Button>
        ) : null}
        {x.status === "committed" && canCommit ? (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void onRollback()}
          >
            {t("office.excel.rollback")}
          </Button>
        ) : null}
        {step === "rows" ? (
          <span className="text-text-3 self-center text-xs">
            {t("office.excel.rows_hint")}
          </span>
        ) : null}
      </div>
    </section>
  );
}

function MappingEditor({
  engagement,
  item,
  type,
  dataTypes,
  editable,
  onSaved,
}: {
  engagement: string;
  item: ExcelImport;
  type: ExcelImportDataType | undefined;
  dataTypes: ExcelImportDataType[];
  editable: boolean;
  onSaved: (x: ExcelImport) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const initial = useMemo(
    () => item.mapping?.columns ?? [],
    [item.mapping?.columns],
  );
  const [dataType, setDataType] = useState(item.data_type ?? "");
  const [fields, setFields] = useState<Record<string, string>>(() =>
    Object.fromEntries(initial.map((c) => [c.column, c.field])),
  );
  const [template, setTemplate] = useState(false);
  const active = dataTypes.find((d) => d.data_type === dataType) ?? type;
  const columns = initial.map((c) => ({
    column: c.column,
    field: fields[c.column] ?? "",
  }));
  const missing = active ? missingRequired(active.required, columns) : [];
  const dupes = duplicateFields(columns);
  const dirty =
    dataType !== (item.data_type ?? "") ||
    initial.some((c) => (fields[c.column] ?? "") !== c.field);
  const label = (key: string) =>
    active?.fields.find((f) => f.key === key)?.description ?? key;
  const save = useMutation({
    mutationFn: () =>
      excelImportsService.setMapping(slug, engagement, item.uuid, {
        ...(dataType ? { data_type: dataType as ExcelDataTypeKey } : {}),
        columns,
        save_template: template ? "company" : "",
      }),
    onSuccess: (x) => {
      onSaved(x);
      toast.success(t("office.excel.mapping_saved"));
    },
    onError: (e) => toast.error(errText(e, t("office.excel.action_failed"))),
  });
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <h4 className="flex-1 text-sm font-semibold">
          {t("office.excel.mapping_title")}
        </h4>
        {editable ? (
          <div className="w-56 space-y-1">
            <Label htmlFor="excel-map-type">
              {t("office.excel.data_type")}
            </Label>
            <Select value={dataType || NONE} onValueChange={setDataType}>
              <SelectTrigger id="excel-map-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {dataType ? null : (
                  <SelectItem value={NONE} disabled>
                    {t("office.excel.type_unknown")}
                  </SelectItem>
                )}
                {dataTypes
                  .filter((d) => d.available)
                  .map((d) => (
                    <SelectItem key={d.data_type} value={d.data_type}>
                      {d.label}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}
      </div>
      <ul className="divide-line border-line divide-y rounded-lg border">
        {initial.map((c) => (
          <li
            key={c.column}
            className="grid items-center gap-2 px-3 py-2 sm:grid-cols-[3rem_1fr_16rem]"
          >
            <Figure className="text-text-3 text-xs">{c.column}</Figure>
            <span className="min-w-0 truncate text-sm">
              {c.header || "—"}
              {c.needs_review ? (
                <span className="text-warn ms-2 text-xs">
                  {t("office.excel.needs_review")}
                </span>
              ) : null}
            </span>
            {editable && active ? (
              <Select
                value={fields[c.column] || NONE}
                onValueChange={(v) =>
                  setFields((f) => ({ ...f, [c.column]: v === NONE ? "" : v }))
                }
              >
                <SelectTrigger
                  aria-label={t("office.excel.field_for", {
                    column: c.header || c.column,
                  })}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>
                    {t("office.excel.unmapped")}
                  </SelectItem>
                  {active.fields.map((f) => (
                    <SelectItem key={f.key} value={f.key}>
                      {f.description}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <span className="text-text-2 text-sm">
                {c.field ? label(c.field) : t("office.excel.unmapped")}
              </span>
            )}
          </li>
        ))}
      </ul>
      {missing.length ? (
        <p role="alert" className="text-danger text-sm">
          {t("office.excel.missing_required", {
            fields: missing.map((g) => g.map(label).join(" / ")).join(", "),
          })}
        </p>
      ) : null}
      {dupes.length ? (
        <p role="alert" className="text-danger text-sm">
          {t("office.excel.duplicate_fields", {
            fields: dupes.map(label).join(", "),
          })}
        </p>
      ) : null}
      {editable ? (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            disabled={save.isPending || !dirty || !dataType || dupes.length > 0}
            onClick={() => save.mutate()}
          >
            {t("office.excel.mapping_save")}
          </Button>
          <Label className="flex min-h-11 items-center gap-2 font-normal">
            <Checkbox
              checked={template}
              onCheckedChange={(v) => setTemplate(v === true)}
            />
            {t("office.excel.save_template")}
          </Label>
        </div>
      ) : null}
    </div>
  );
}

const ROW_FILTERS: (ExcelRowResult | "all")[] = [
  "error",
  "warning",
  "skipped",
  "ok",
  "all",
];

function RowsPanel({
  engagement,
  item,
  editable,
  onChanged,
}: {
  engagement: string;
  item: ExcelImport;
  editable: boolean;
  onChanged: (x: ExcelImport) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [filter, setFilter] = useState<ExcelRowResult | "all">(
    (item.summary?.errors ?? 0) > 0 ? "error" : "all",
  );
  const [offset, setOffset] = useState(0);
  const rows = useQuery({
    queryKey: excelImportKeys.rows(slug, engagement, item.uuid, filter, offset),
    queryFn: () =>
      excelImportsService.rows(slug, engagement, item.uuid, {
        result: filter === "all" ? undefined : filter,
        limit: ROWS_PAGE,
        offset,
      }),
    placeholderData: keepPreviousData,
  });
  const skip = useMutation({
    mutationFn: (v: { rowNo: number; skip: boolean }) =>
      excelImportsService.skipRow(slug, engagement, item.uuid, v.rowNo, v.skip),
    onSuccess: (x) => {
      onChanged(x);
      void qc.invalidateQueries({
        queryKey: excelImportKeys.one(slug, engagement, item.uuid),
      });
    },
    onError: (e) => toast.error(errText(e, t("office.excel.action_failed"))),
  });
  const items = rows.data?.items ?? [];
  const total = rows.data?.total ?? 0;
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="flex-1 text-sm font-semibold">
          {t("office.excel.rows_title")}
        </h4>
        <div
          role="group"
          aria-label={t("office.excel.rows_filter")}
          className="bg-surface-3 flex gap-0.5 rounded-lg p-0.5"
        >
          {ROW_FILTERS.map((f) => (
            <Button
              key={f}
              size="sm"
              variant={filter === f ? "secondary" : "ghost"}
              aria-pressed={filter === f}
              onClick={() => {
                setFilter(f);
                setOffset(0);
              }}
            >
              {t(`office.excel.filter_${f}`)}
            </Button>
          ))}
        </div>
      </div>
      {rows.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : items.length === 0 ? (
        <p className="text-text-3 text-sm">{t("office.excel.rows_empty")}</p>
      ) : (
        <ul className="divide-line border-line divide-y rounded-lg border">
          {items.map((r) => (
            <li key={r.row_no} className="space-y-1 px-3 py-2">
              <div className="flex flex-wrap items-center gap-2">
                <Figure className="text-text-3 text-xs">
                  {t("office.excel.row_no", { no: r.row_no })}
                </Figure>
                <StatusPill
                  tone={
                    r.result === "error"
                      ? "danger"
                      : r.result === "warning"
                        ? "warn"
                        : r.result === "ok"
                          ? "ok"
                          : "neutral"
                  }
                >
                  {t(`office.excel.filter_${r.result}`)}
                </StatusPill>
                {r.duplicate ? (
                  <span className="text-text-3 text-xs">
                    {t("office.excel.row_duplicate")}
                  </span>
                ) : null}
                {editable ? (
                  <Button
                    size="sm"
                    variant="outline"
                    className="ms-auto max-md:min-h-11"
                    disabled={skip.isPending}
                    onClick={() =>
                      skip.mutate({ rowNo: r.row_no, skip: !r.skipped })
                    }
                  >
                    {r.skipped
                      ? t("office.excel.row_unskip")
                      : t("office.excel.row_skip")}
                  </Button>
                ) : null}
              </div>
              {r.messages.length ? (
                <ul className="text-sm">
                  {r.messages.map((m, i) => (
                    <li
                      key={i}
                      className={
                        m.severity === "error" ? "text-danger" : "text-text-2"
                      }
                    >
                      {[m.field, m.detail || m.code].filter(Boolean).join(": ")}
                    </li>
                  ))}
                </ul>
              ) : null}
              <p className="text-text-3 truncate text-xs">
                {Object.values(r.raw).filter(Boolean).slice(0, 8).join(" · ")}
              </p>
            </li>
          ))}
        </ul>
      )}
      {total > ROWS_PAGE ? (
        <div className="flex items-center justify-end gap-2">
          <span className="text-text-3 text-xs tabular-nums">
            {offset + 1}–{Math.min(offset + ROWS_PAGE, total)} / {total}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - ROWS_PAGE))}
          >
            {t("office.excel.prev")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={offset + ROWS_PAGE >= total}
            onClick={() => setOffset(offset + ROWS_PAGE)}
          >
            {t("office.excel.next")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
