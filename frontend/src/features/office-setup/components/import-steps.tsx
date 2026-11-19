"use client";

import { Send } from "lucide-react";
import {
  createContext,
  useContext,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type ReactNode,
} from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { Loading } from "@/components/common/loading";
import {
  createColumn,
  DataTable,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  useSaveOfficeSetup,
  useStoreTaxpayerImport,
  useTaxpayerImport,
} from "../hooks/use-office-setup";
import { setupInput } from "../lib/steps";
import {
  taxpayerImportService,
  type TaxpayerImport,
  type TaxpayerImportOutcome,
  type TaxpayerImportRow,
} from "../services/office-setup.service";
import { StepActions, StepError } from "./step-actions";
import type { SetupStepProps } from "./step-props";

const ACCEPT = ".xlsx,.csv";
const ACCEPTED_EXTENSIONS = [".xlsx", ".csv"];

// Sihirbaz adımındaki gömülü tablolar: araç çubuğu, seçim ve sayfalama yok
// (liste tek seferde gelir, hepsi gösterilir); sıralama açık.
const embeddedFeatures: DataTableFeatures = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  globalFilter: false,
  facetedFilters: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
};

/** Localized message of a failed call (codes map to errors.codes.*). */
function errorText(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

// --- Mükellef listesi ---

type RowFix = { tax_id: string; title: string };

/** Düzeltilmiş satırlarla yeni bir CSV (sunucunun başlık eşlemesine uygun). */
function correctedCsv(batch: TaxpayerImport, fixes: Record<number, RowFix>) {
  const q = (v: string) => `"${v.replace(/"/g, '""')}"`;
  const lines = ["VKN,Unvan,Vergi dairesi,E-posta,Tür"];
  for (const r of batch.rows ?? []) {
    const f = fixes[r.row];
    lines.push(
      [
        f?.tax_id ?? r.tax_id,
        f?.title ?? r.title,
        r.tax_office ?? "",
        r.contact_email ?? "",
        // Düzeltilen satırda tür boş: sunucu kimlik numarasından çıkarır
        // (VKN düzeltilip TCKN olursa eski tür çelişirdi).
        f
          ? ""
          : r.kind === "individual"
            ? "gerçek"
            : r.kind === "legal"
              ? "tüzel"
              : "",
      ]
        .map((v) => q(v ?? ""))
        .join(","),
    );
  }
  return lines.join("\n");
}

type FixState = {
  fixes: Record<number, RowFix>;
  fix: (row: number, base: RowFix, patch: Partial<RowFix>) => void;
};

// Düzeltmeler bağlamdan okunur: kolonlar sabit kalır, yazarken girdi
// yeniden bağlanmaz (odak kaybolmaz).
const FixContext = createContext<FixState>({ fixes: {}, fix: () => {} });

function FixInput({ r, field }: { r: TaxpayerImportRow; field: keyof RowFix }) {
  const { t } = useLocale();
  const { fixes, fix } = useContext(FixContext);
  const base = { tax_id: r.tax_id, title: r.title };
  const value = fixes[r.row]?.[field] ?? r[field];
  return field === "title" ? (
    <Input
      aria-label={t("setup.taxpayers_fix_title", { row: r.row })}
      className="h-9 min-w-40"
      value={value}
      onChange={(e) => fix(r.row, base, { title: e.target.value })}
    />
  ) : (
    <Input
      aria-label={t("setup.taxpayers_fix_tax_id", { row: r.row })}
      inputMode="numeric"
      className="h-9 w-36 font-mono"
      value={value}
      onChange={(e) => fix(r.row, base, { tax_id: e.target.value })}
    />
  );
}

const isBad = (r: TaxpayerImportRow) => r.errors.length > 0;

function useImportRowColumns() {
  const { t } = useLocale();
  return useMemo<ColumnDef<TaxpayerImportRow, unknown>[]>(
    () => [
      createColumn<TaxpayerImportRow>({
        id: "status",
        labelKey: "setup.taxpayers_col_status",
        accessorFn: (r) => (isBad(r) ? 0 : 1),
        cell: ({ row }) => {
          const bad = isBad(row.original);
          return (
            <span
              className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-semibold ${bad ? "bg-danger-tint text-danger" : "bg-ok-tint text-ok"}`}
            >
              {bad ? t("setup.row_error") : t("setup.row_valid")}
            </span>
          );
        },
      }),
      createColumn<TaxpayerImportRow>({
        id: "title",
        labelKey: "setup.taxpayers_col_title",
        accessorKey: "title",
        gridPrimary: true,
        cell: ({ row }) =>
          isBad(row.original) ? (
            <FixInput r={row.original} field="title" />
          ) : (
            row.original.title
          ),
      }),
      createColumn<TaxpayerImportRow>({
        id: "tax_id",
        labelKey: "setup.taxpayers_col_tax_id",
        accessorKey: "tax_id",
        gridSecondary: true,
        meta: { cellClassName: "figure" },
        cell: ({ row }) =>
          isBad(row.original) ? (
            <FixInput r={row.original} field="tax_id" />
          ) : (
            row.original.tax_id
          ),
      }),
      createColumn<TaxpayerImportRow>({
        id: "tax_office",
        labelKey: "setup.taxpayers_col_tax_office",
        accessorFn: (r) => r.tax_office ?? "",
        meta: { cellClassName: "text-text-2" },
      }),
      createColumn<TaxpayerImportRow>({
        id: "errors",
        labelKey: "setup.taxpayers_col_errors",
        accessorFn: (r) => r.errors.map((e) => e.message).join("; "),
        meta: { cellClassName: "text-danger" },
      }),
    ],
    [t],
  );
}

/**
 * Yüklenen listenin satırları (OfisKurulum): hatalılar önce, durum rozetiyle.
 * Hatalı satırda VKN ve unvan yerinde düzeltilir; "Düzeltmeleri uygula"
 * düzeltilmiş listeyi yeni bir yükleme olarak gönderir (satır düzenleme ucu
 * yok, sunucu yeniden doğrular).
 */
function ImportSummary({
  batch,
  busy,
  onReupload,
}: {
  batch: TaxpayerImport;
  busy: boolean;
  onReupload: (file: File) => void;
}) {
  const { t } = useLocale();
  const [fixes, setFixes] = useState<Record<number, RowFix>>({});
  const rows = useMemo(
    () =>
      [...(batch.rows ?? [])].sort(
        (a, b) => Number(isBad(b)) - Number(isBad(a)) || a.row - b.row,
      ),
    [batch.rows],
  );
  const columns = useImportRowColumns();
  const edited = Object.keys(fixes).length;
  const fixState = useMemo<FixState>(
    () => ({
      fixes,
      fix: (row, base, patch) =>
        setFixes((f) => ({ ...f, [row]: { ...(f[row] ?? base), ...patch } })),
    }),
    [fixes],
  );
  return (
    <div className="mt-5 space-y-3">
      <div className="bg-surface-2 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border px-4 py-3">
        <span className="font-mono text-sm font-semibold">
          {batch.file_name}
        </span>
        <span className="text-sm" data-testid="setup-import-summary">
          {t("setup.taxpayers_summary", {
            valid: batch.valid,
            invalid: batch.invalid,
          })}
        </span>
        <span className="sr-only">
          {t("setup.taxpayers_current_file", { name: batch.file_name })}
        </span>
      </div>
      {rows.length ? (
        <FixContext.Provider value={fixState}>
          {/* Mobilde kartlara geçer; kart içindeki düzeltme girdileri çalışır. */}
          <DataTable<TaxpayerImportRow>
            columns={columns}
            data={rows}
            getRowId={(r) => String(r.row)}
            caption={t("setup.taxpayers_rows_caption")}
            features={embeddedFeatures}
            getRowClassName={(r) =>
              isBad(r) ? "bg-danger-tint/40" : undefined
            }
          />
        </FixContext.Provider>
      ) : null}
      {batch.invalid > 0 ? (
        <div className="flex flex-wrap items-center justify-end gap-3">
          <span className="text-text-2 text-xs">
            {t("setup.taxpayers_fix_hint")}
          </span>
          <Button
            type="button"
            variant="outline"
            className="h-11"
            disabled={!edited || busy}
            onClick={() =>
              onReupload(
                new File(
                  [correctedCsv(batch, fixes)],
                  batch.file_name.replace(/\.(xlsx|csv)$/i, "") +
                    "-duzeltilmis.csv",
                  { type: "text/csv" },
                ),
              )
            }
          >
            {t("setup.taxpayers_fix_apply", { count: edited })}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function TaxpayersStep(props: SetupStepProps) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const save = useSaveOfficeSetup(slug);
  const store = useStoreTaxpayerImport(slug);
  const inputRef = useRef<HTMLInputElement>(null);
  // The last upload, shown even if saving its uuid to the state failed.
  const [uploaded, setUploaded] = useState<string | undefined>();
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const importUuid = uploaded ?? props.state.import_uuid;
  const batch = useTaxpayerImport(slug, importUuid);

  const upload = async (file: File) => {
    setError(null);
    setUploading(true);
    try {
      const created = await taxpayerImportService.create(slug, file);
      store(created);
      setUploaded(created.uuid);
      try {
        await save.mutateAsync(
          setupInput(props.state, { importUuid: created.uuid }),
        );
      } catch {
        // toasted by the mutation cache; the summary still shows
      }
    } catch (err) {
      setError(errorText(err, t("setup.taxpayers_upload_failed")));
    } finally {
      setUploading(false);
      // Allows choosing the same file again after fixing it.
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const onFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const name = file.name.toLowerCase();
    if (!ACCEPTED_EXTENSIONS.some((ext) => name.endsWith(ext))) {
      setError(t("setup.taxpayers_file_type"));
      e.target.value = "";
      return;
    }
    await upload(file);
  };

  return (
    <div>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("setup.taxpayers_description")}
      </p>
      <div className="grid max-w-md gap-2">
        <Label htmlFor="setup-taxpayer-file">{t("setup.taxpayers_file")}</Label>
        <Input
          ref={inputRef}
          id="setup-taxpayer-file"
          name="file"
          type="file"
          accept={ACCEPT}
          disabled={uploading}
          aria-describedby="setup-taxpayer-file-hint"
          onChange={(e) => void onFile(e)}
          className="h-auto py-1.5"
        />
        <p
          id="setup-taxpayer-file-hint"
          className="text-muted-foreground text-xs"
        >
          {t("setup.taxpayers_replace_hint")}
        </p>
      </div>
      {uploading ? (
        <Loading
          label={t("setup.taxpayers_uploading")}
          className="mt-3 justify-start"
        />
      ) : null}
      <StepError message={error} />
      {importUuid && batch.isLoading ? (
        <Loading label={t("common.loading")} className="mt-3 justify-start" />
      ) : null}
      {importUuid && batch.isError ? (
        <StepError message={t("setup.taxpayers_load_failed")} />
      ) : null}
      {batch.data ? (
        <ImportSummary
          key={batch.data.uuid}
          batch={batch.data}
          busy={uploading}
          onReupload={(file) => void upload(file)}
        />
      ) : null}
      <StepActions
        isLast={props.isLast}
        busy={props.saving || uploading}
        onSkip={() => void props.onSkip()}
        onSave={() => void props.onDone()}
      />
    </div>
  );
}

// --- Mükellef davetleri ---

function useFailedColumns(batch: TaxpayerImport) {
  const rows = useMemo(
    () => new Map((batch.rows ?? []).map((r) => [r.row, r])),
    [batch.rows],
  );
  return useMemo<ColumnDef<TaxpayerImportOutcome, unknown>[]>(
    () => [
      createColumn<TaxpayerImportOutcome>({
        id: "row",
        labelKey: "setup.taxpayers_col_row",
        accessorKey: "row",
        meta: { cellClassName: "figure" },
      }),
      createColumn<TaxpayerImportOutcome>({
        id: "tax_id",
        labelKey: "setup.taxpayers_col_tax_id",
        accessorFn: (o) => rows.get(o.row)?.tax_id ?? "",
        gridSecondary: true,
        meta: { cellClassName: "figure" },
      }),
      createColumn<TaxpayerImportOutcome>({
        id: "title",
        labelKey: "setup.taxpayers_col_title",
        accessorFn: (o) => rows.get(o.row)?.title ?? "",
        gridPrimary: true,
      }),
      createColumn<TaxpayerImportOutcome>({
        id: "error",
        labelKey: "setup.invites_col_error",
        accessorFn: (o) => o.error ?? "",
        meta: { cellClassName: "text-destructive" },
      }),
    ],
    [rows],
  );
}

function SendOutcome({ batch }: { batch: TaxpayerImport }) {
  const { t } = useLocale();
  const outcome = batch.outcome ?? [];
  const count = (status: "requested" | "skipped" | "failed") =>
    outcome.filter((o) => o.status === status).length;
  const failed = outcome.filter((o) => o.status === "failed");
  const columns = useFailedColumns(batch);
  return (
    <div className="space-y-3">
      <p className="text-sm font-medium" data-testid="setup-invites-result">
        {t("setup.invites_result", {
          requested: count("requested"),
          skipped: count("skipped"),
          failed: count("failed"),
        })}
      </p>
      {failed.length ? (
        <div className="space-y-2">
          {/* Görünür başlık; tablonun erişilebilir adı caption'da. */}
          <p className="text-sm font-medium" aria-hidden>
            {t("setup.invites_failed_title")}
          </p>
          <DataTable<TaxpayerImportOutcome>
            columns={columns}
            data={failed}
            getRowId={(o) => String(o.row)}
            caption={t("setup.invites_failed_title")}
            features={embeddedFeatures}
          />
        </div>
      ) : null}
    </div>
  );
}

export function InvitesStep(props: SetupStepProps) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const importUuid = props.state.import_uuid;
  const batch = useTaxpayerImport(slug, importUuid);
  const store = useStoreTaxpayerImport(slug);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSend = async () => {
    if (!importUuid) return;
    setError(null);
    setSending(true);
    try {
      store(await taxpayerImportService.send(slug, importUuid));
    } catch (err) {
      if (err instanceof ApiError && err.code === "IMPORT_ALREADY_SENT") {
        // Sent from another tab: show what was sent.
        await batch.refetch();
      } else {
        setError(errorText(err, t("common.error_generic")));
      }
    } finally {
      setSending(false);
    }
  };

  let body: ReactNode;
  if (!importUuid) {
    body = (
      <p className="text-muted-foreground text-sm">
        {t("setup.invites_no_import")}
      </p>
    );
  } else if (batch.isLoading) {
    body = <Loading label={t("common.loading")} className="justify-start" />;
  } else if (!batch.data) {
    body = <StepError message={t("setup.taxpayers_load_failed")} />;
  } else if (batch.data.sent_at) {
    body = <SendOutcome batch={batch.data} />;
  } else {
    body = (
      <div className="space-y-3">
        <p className="text-sm font-medium">
          {t("setup.invites_pending", { valid: batch.data.valid })}
        </p>
        <Button
          type="button"
          variant="secondary"
          disabled={sending || batch.data.valid === 0}
          onClick={() => void onSend()}
        >
          <Send className="size-4" aria-hidden />
          {t("setup.invites_send")}
        </Button>
        <StepError message={error} />
      </div>
    );
  }

  return (
    <div>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("setup.invites_description")}
      </p>
      {body}
      <StepActions
        isLast={props.isLast}
        busy={props.saving || sending}
        onSkip={() => void props.onSkip()}
        onSave={() => void props.onDone()}
      />
    </div>
  );
}
