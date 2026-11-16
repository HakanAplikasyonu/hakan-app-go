"use client";

import { Send } from "lucide-react";
import { useRef, useState, type ChangeEvent, type ReactNode } from "react";

import { Loading } from "@/components/common/loading";
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
} from "../services/office-setup.service";
import { StepActions, StepError } from "./step-actions";
import type { SetupStepProps } from "./step-props";

const ACCEPT = ".xlsx,.csv";
const ACCEPTED_EXTENSIONS = [".xlsx", ".csv"];

const th = "py-2 pe-3 text-start font-medium";

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
  const rows = [...(batch.rows ?? [])].sort(
    (a, b) =>
      Number(b.errors.length > 0) - Number(a.errors.length > 0) ||
      a.row - b.row,
  );
  const edited = Object.keys(fixes).length;
  const fix = (row: number, base: RowFix, patch: Partial<RowFix>) =>
    setFixes((f) => ({ ...f, [row]: { ...(f[row] ?? base), ...patch } }));
  return (
    <div className="mt-5 space-y-3">
      <div className="bg-surface-2 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-[10px] border px-4 py-3">
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
        <div className="overflow-x-auto rounded-[10px] border">
          <table className="w-full text-sm">
            <caption className="sr-only">
              {t("setup.taxpayers_rows_caption")}
            </caption>
            <thead>
              <tr className="text-muted-foreground border-b font-mono text-[11px] uppercase">
                <th className={`${th} ps-3`}>
                  {t("setup.taxpayers_col_status")}
                </th>
                <th className={th}>{t("setup.taxpayers_col_title")}</th>
                <th className={th}>{t("setup.taxpayers_col_tax_id")}</th>
                <th className={`${th} hidden md:table-cell`}>
                  {t("setup.taxpayers_col_tax_office")}
                </th>
                <th className={th}>{t("setup.taxpayers_col_errors")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                const bad = r.errors.length > 0;
                const base = { tax_id: r.tax_id, title: r.title };
                const f = fixes[r.row];
                return (
                  <tr
                    key={r.row}
                    className={`border-b last:border-0 ${bad ? "bg-danger-tint/40" : ""}`}
                  >
                    <td className="py-2 ps-3 pe-3">
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ${bad ? "bg-danger-tint text-danger" : "bg-ok-tint text-ok"}`}
                      >
                        {bad ? t("setup.row_error") : t("setup.row_valid")}
                      </span>
                    </td>
                    <td className="py-2 pe-3">
                      {bad ? (
                        <Input
                          aria-label={t("setup.taxpayers_fix_title", {
                            row: r.row,
                          })}
                          className="h-9 min-w-40"
                          value={f?.title ?? r.title}
                          onChange={(e) =>
                            fix(r.row, base, { title: e.target.value })
                          }
                        />
                      ) : (
                        r.title
                      )}
                    </td>
                    <td className="figure py-2 pe-3">
                      {bad ? (
                        <Input
                          aria-label={t("setup.taxpayers_fix_tax_id", {
                            row: r.row,
                          })}
                          inputMode="numeric"
                          className="h-9 w-36 font-mono"
                          value={f?.tax_id ?? r.tax_id}
                          onChange={(e) =>
                            fix(r.row, base, { tax_id: e.target.value })
                          }
                        />
                      ) : (
                        r.tax_id
                      )}
                    </td>
                    <td className="text-text-2 hidden py-2 pe-3 md:table-cell">
                      {r.tax_office}
                    </td>
                    <td className="text-danger py-2 pe-3">
                      {r.errors.map((e) => e.message).join("; ")}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
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

function SendOutcome({ batch }: { batch: TaxpayerImport }) {
  const { t } = useLocale();
  const outcome = batch.outcome ?? [];
  const count = (status: "requested" | "skipped" | "failed") =>
    outcome.filter((o) => o.status === status).length;
  const failed = outcome.filter((o) => o.status === "failed");
  const rows = new Map((batch.rows ?? []).map((r) => [r.row, r]));
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
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <caption className="mb-2 text-start text-sm font-medium">
              {t("setup.invites_failed_title")}
            </caption>
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className={th}>{t("setup.taxpayers_col_row")}</th>
                <th className={th}>{t("setup.taxpayers_col_tax_id")}</th>
                <th className={th}>{t("setup.taxpayers_col_title")}</th>
                <th className={th}>{t("setup.invites_col_error")}</th>
              </tr>
            </thead>
            <tbody>
              {failed.map((o) => (
                <tr key={o.row} className="border-b last:border-0">
                  <td className="figure py-2 pe-3">{o.row}</td>
                  <td className="figure py-2 pe-3">
                    {rows.get(o.row)?.tax_id ?? ""}
                  </td>
                  <td className="py-2 pe-3">{rows.get(o.row)?.title ?? ""}</td>
                  <td className="text-destructive py-2 pe-3">
                    {o.error ?? ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
