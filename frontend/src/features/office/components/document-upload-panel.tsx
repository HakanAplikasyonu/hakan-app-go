"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileUp, Loader2, Lock, Upload } from "lucide-react";
import { useRef, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { periodText } from "../lib/change-request-text";
import {
  currentMonth,
  suggestPeriod,
  ublIssueDate,
} from "../lib/document-upload-period";
import {
  DOCUMENT_UPLOAD_TYPES,
  type DocumentUpload,
  type DocumentUploadType,
  type LockChoice,
  documentUploadKeys,
  documentUploadsService,
} from "../services/document-uploads.service";
import { Figure, Panel } from "./office-ui";

const ACCEPT: Record<DocumentUploadType, string> = {
  invoice: ".xml,.zip,.pdf,.jpg,.jpeg,.png",
  z_report: ".jpg,.jpeg,.png,.webp",
  expense: ".jpg,.jpeg,.png,.webp",
  bank_statement: ".pdf,.jpg,.jpeg,.png",
  other: ".pdf,.jpg,.jpeg,.png",
};

/**
 * Evrak yükle (Oğuz'un demo geri bildirimi): ofis kullanıcısı mükellef adına
 * evrak yüklerken türü ve dönemi (ayı) seçer. Ay belge tarihinden önerilir
 * (girilen tarih, e-Fatura XML'indeki tarih; Z raporu ve masrafta
 * fotoğraftan okunan tarih), yoksa içinde bulunulan ay. Seçilen ay
 * kilitliyse kilitli dönem talebindeki üç seçenek sorulur.
 */
export function DocumentUploadPanel({
  slug,
  engagement,
}: {
  slug: string;
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const settings = useQuery({
    queryKey: documentUploadKeys.settings(slug),
    queryFn: () => documentUploadsService.getSettings(slug),
  });
  const list = useQuery({
    queryKey: documentUploadKeys.list(slug, engagement),
    queryFn: () => documentUploadsService.list(slug, engagement),
    enabled: !!engagement && settings.data?.enabled !== false,
  });
  const [docType, setDocType] = useState<DocumentUploadType>("invoice");
  const [file, setFile] = useState<File | null>(null);
  const [documentDate, setDocumentDate] = useState("");
  const [fileDate, setFileDate] = useState<string | null>(null);
  const [manualPeriod, setManualPeriod] = useState<string | null>(null);
  const [lockedFor, setLockedFor] = useState<string | null>(null);
  const [choice, setChoice] = useState<LockChoice>("correction");
  const [target, setTarget] = useState(() => currentMonth());
  const [last, setLast] = useState<DocumentUpload | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const suggestion = suggestPeriod(documentDate || fileDate);
  const period = manualPeriod ?? suggestion.period;
  const locked = lockedFor === period;

  const create = useMutation({
    mutationFn: ({ f, withChoice }: { f: File; withChoice: boolean }) =>
      documentUploadsService.create(slug, engagement, {
        file: f,
        docType,
        period,
        periodAuto: manualPeriod === null,
        documentDate: documentDate || undefined,
        lockChoice: withChoice ? choice : undefined,
        targetPeriod: target,
      }),
    onSuccess: (u) => {
      setLast(u);
      setFile(null);
      setFileDate(null);
      setDocumentDate("");
      setManualPeriod(null);
      setLockedFor(null);
      if (input.current) input.current.value = "";
      void qc.invalidateQueries({
        queryKey: documentUploadKeys.list(slug, engagement),
      });
    },
    onError: (err) => {
      if (isApiError(err) && err.code === "PERIOD_LOCKED") setLockedFor(period);
    },
  });

  if (settings.data?.enabled === false) return null;

  const pick = async (f: File | null) => {
    setFile(f);
    setLast(null);
    setFileDate(null);
    if (f && /\.xml$/i.test(f.name)) {
      const text = await f.text().catch(() => "");
      setFileDate(ublIssueDate(text));
    }
  };

  const month = (p: string) => periodText(p, locale);
  const sourceText =
    manualPeriod !== null
      ? t("office.doc_upload.source_manual")
      : documentDate
        ? t("office.doc_upload.source_document_date")
        : fileDate
          ? t("office.doc_upload.source_file_date")
          : docType === "z_report" || docType === "expense"
            ? t("office.doc_upload.source_ocr_later")
            : t("office.doc_upload.source_current_month");
  const items = list.data?.items ?? [];

  return (
    <Panel
      id="evrak-yukle"
      title={t("office.doc_upload.title")}
      description={t("office.doc_upload.description")}
    >
      <div className="space-y-4 p-4" data-testid="doc-upload-panel">
        <div className="grid gap-4 sm:grid-cols-3">
          <Label className="flex flex-col gap-1.5 text-sm leading-normal font-normal">
            <span className="font-medium">
              {t("office.doc_upload.type_label")}
            </span>
            <NativeSelect
              value={docType}
              onChange={(e) => {
                setDocType(e.target.value as DocumentUploadType);
                setLockedFor(null);
              }}
              data-testid="doc-upload-type"
            >
              {DOCUMENT_UPLOAD_TYPES.map((k) => (
                <NativeSelectOption key={k} value={k}>
                  {t(`office.doc_upload.type_${k}`)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Label>
          <div
            className="flex flex-col gap-1.5 text-sm"
            data-testid="doc-upload-date"
          >
            <Label htmlFor="doc-upload-date" className="font-medium">
              {t("office.doc_upload.date_label")}
            </Label>
            <DatePicker
              id="doc-upload-date"
              value={documentDate}
              onChange={(v) => {
                setDocumentDate(v);
                setManualPeriod(null);
              }}
            />
          </div>
          <Label className="flex flex-col gap-1.5 text-sm leading-normal font-normal">
            <span className="font-medium">
              {t("office.doc_upload.period_label")}
            </span>
            <Input
              type="month"
              value={period}
              onChange={(e) => {
                if (/^\d{4}-\d{2}$/.test(e.target.value))
                  setManualPeriod(e.target.value);
              }}
              data-testid="doc-upload-period"
            />
            <span
              className="text-muted-foreground text-xs"
              data-testid="doc-upload-period-source"
            >
              {sourceText}
            </span>
          </Label>
        </div>
        <Label className="hover:border-brand/60 flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-6 py-6 text-center text-sm leading-normal font-normal transition-colors">
          <FileUp className="text-muted-foreground size-6" aria-hidden />
          <span className="font-medium">
            {file ? file.name : t("office.doc_upload.drop")}
          </span>
          <span className="text-muted-foreground text-xs">
            {t(`office.doc_upload.hint_${docType}`)}
          </span>
          <input
            ref={input}
            type="file"
            accept={ACCEPT[docType]}
            className="sr-only"
            onChange={(e) => void pick(e.target.files?.[0] ?? null)}
            data-testid="doc-upload-input"
          />
        </Label>

        {locked ? (
          <Alert data-testid="doc-upload-locked">
            <Lock className="size-4" aria-hidden />
            <AlertTitle>
              {t("office.doc_upload.locked_title", { month: month(period) })}
            </AlertTitle>
            <AlertDescription className="space-y-3">
              <p>{t("office.doc_upload.locked_body")}</p>
              <RadioGroup
                value={choice}
                onValueChange={(v) => setChoice(v as LockChoice)}
                aria-label={t("office.doc_upload.locked_title", {
                  month: month(period),
                })}
                className="space-y-2"
              >
                {(["reject", "correction", "target_period"] as const).map(
                  (c) => (
                    <Label
                      key={c}
                      className="flex items-start gap-2 leading-normal font-normal"
                    >
                      <RadioGroupItem
                        value={c}
                        data-testid={`doc-upload-choice-${c}`}
                      />
                      <span>
                        <span className="font-medium">
                          {t(`office.doc_upload.choice_${c}`)}
                        </span>
                        <span className="text-muted-foreground block text-xs">
                          {t(`office.doc_upload.choice_${c}_effect`, {
                            month: month(period),
                          })}
                        </span>
                      </span>
                    </Label>
                  ),
                )}
              </RadioGroup>
              {choice === "correction" ? (
                <p
                  className="text-status-review text-sm font-medium"
                  data-testid="doc-upload-correction-warning"
                >
                  {t("office.change_requests.correction_warning", {
                    month: month(period),
                  })}
                </p>
              ) : null}
              {choice === "target_period" ? (
                <Label className="flex max-w-xs flex-col gap-1.5 text-sm leading-normal font-normal">
                  <span className="font-medium">
                    {t("office.change_requests.target_label")}
                  </span>
                  <Input
                    type="month"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    data-testid="doc-upload-target"
                  />
                </Label>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}

        {create.isError &&
        !(isApiError(create.error) && create.error.code === "PERIOD_LOCKED") ? (
          <p className="text-status-blocking text-sm">
            {create.error instanceof Error
              ? create.error.message
              : t("office.api.upload_failed")}
          </p>
        ) : null}

        {last ? <UploadResult u={last} /> : null}

        <div className="flex justify-end gap-2">
          {locked ? (
            <Button
              variant="ghost"
              onClick={() => setLockedFor(null)}
              disabled={create.isPending}
            >
              {t("office.doc_upload.change_month")}
            </Button>
          ) : null}
          <Button
            onClick={() => {
              if (file) create.mutate({ f: file, withChoice: locked });
            }}
            disabled={create.isPending || !file || !engagement}
            data-testid="doc-upload-submit"
          >
            {create.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Upload className="size-4" />
            )}
            {locked
              ? t("office.doc_upload.submit_with_choice")
              : t("office.doc_upload.submit", { month: month(period) })}
          </Button>
        </div>

        {items.length ? (
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {t("office.doc_upload.recent")}
            </p>
            <ul className="divide-y text-sm" data-testid="doc-upload-list">
              {items.slice(0, 10).map((u) => (
                <li
                  key={u.uuid}
                  className="flex flex-wrap items-center justify-between gap-2 py-1.5"
                >
                  <Figure className="max-w-64 truncate text-xs">
                    {u.file_name}
                  </Figure>
                  <span className="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
                    <span>{t(`office.doc_upload.type_${u.doc_type}`)}</span>
                    <span>{month(u.period)}</span>
                    <span>{t(`office.doc_upload.status_${u.status}`)}</span>
                    {u.lock_choice ? (
                      <span>
                        {t(`office.doc_upload.decided_${u.lock_choice}`, {
                          month: month(u.locked_period ?? u.period),
                          target: month(u.period),
                        })}
                      </span>
                    ) : null}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </Panel>
  );
}

function UploadResult({ u }: { u: DocumentUpload }) {
  const { t, locale } = useLocale();
  const month = (p: string) => periodText(p, locale);
  return (
    <div
      className="bg-muted/40 space-y-1 rounded-lg px-3 py-2 text-sm"
      data-testid="doc-upload-result"
    >
      <p className="font-medium">
        {t(`office.doc_upload.result_${u.status}`, {
          type: t(`office.doc_upload.type_${u.doc_type}`),
          month: month(u.period),
        })}
      </p>
      {u.correction_needed ? (
        <p className="text-status-review">
          {t("office.doc_upload.result_correction", {
            month: month(u.period),
          })}
        </p>
      ) : null}
      {u.lock_choice === "target_period" && u.locked_period ? (
        <p className="text-muted-foreground">
          {t("office.doc_upload.decided_target_period", {
            month: month(u.locked_period),
            target: month(u.period),
          })}
        </p>
      ) : null}
      {u.ocr_period ? (
        <p className="text-muted-foreground">
          {t("office.doc_upload.result_ocr_locked", {
            ocr: month(u.ocr_period),
            month: month(u.period),
          })}
        </p>
      ) : null}
      {u.period_source === "ocr" ? (
        <p className="text-muted-foreground">
          {t("office.doc_upload.result_ocr")}
        </p>
      ) : null}
    </div>
  );
}
