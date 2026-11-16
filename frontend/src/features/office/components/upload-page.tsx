"use client";

import { FileUp, Loader2, Upload } from "lucide-react";
import Link from "next/link";
import { Suspense, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useCreateUBLUploads,
  useUBLUpload,
  useUBLUploads,
} from "../hooks/use-office-invoices";
import { isRealUUID, useOfficeMessages } from "../lib/messages";
import { severityOf } from "../lib/warnings";
import type {
  UBLUploadItem,
  UBLUploadJob,
} from "../services/office-invoices.service";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import { Figure, Panel, SeverityBadge, TableBox } from "./office-ui";

const itemClass: Record<UBLUploadItem["status"], string> = {
  added: "bg-status-approved/12 text-status-approved",
  duplicate: "bg-secondary text-secondary-foreground",
  failed: "bg-status-blocking/12 text-status-blocking",
  held: "bg-status-review/12 text-status-review",
};

const jobClass: Record<UBLUploadJob["status"], string> = {
  queued: "bg-secondary text-secondary-foreground",
  processing: "bg-status-review/12 text-status-review",
  done: "bg-status-approved/12 text-status-approved",
  failed: "bg-status-blocking/12 text-status-blocking",
};

function JobStatus({ status }: { status: UBLUploadJob["status"] }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium",
        jobClass[status],
      )}
    >
      {status === "queued" || status === "processing" ? (
        <Loader2 className="size-3 animate-spin" aria-hidden />
      ) : null}
      {t(`office.api.upload_status_${status}`)}
    </span>
  );
}

function JobReport({
  engagement,
  jobId,
}: {
  engagement: string;
  jobId: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { uploadError } = useOfficeMessages();
  const { data: job } = useUBLUpload(slug, engagement, jobId);
  if (!job) return <Skeleton className="h-24 w-full" />;
  const items = job.report?.items ?? [];
  return (
    <Panel
      title={`${t("office.upload_report_title")} · ${job.file_name}`}
      description={
        job.report
          ? t("office.api.upload_report_summary", {
              added: job.report.added,
              duplicate: job.report.duplicate,
              failed: job.report.failed,
            })
          : undefined
      }
      actions={<JobStatus status={job.status} />}
    >
      {job.status === "failed" ? (
        <p className="text-status-blocking px-4 py-3 text-sm">
          {t("office.errors.upload.job_failed")}
        </p>
      ) : null}
      {items.length ? (
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.col_file")}</th>
              <th>{t("office.col_result")}</th>
              <th>{t("office.col_number")}</th>
              <th>{t("office.col_warnings")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((f, i) => (
              <tr key={i}>
                <td className="max-w-80 truncate">
                  <Figure className="text-xs">{f.name}</Figure>
                </td>
                <td>
                  <span
                    className={cn(
                      "rounded-md px-2 py-0.5 text-xs font-medium",
                      itemClass[f.status],
                    )}
                  >
                    {t(`office.api.item_${f.status}`)}
                  </span>
                  {f.status === "failed" ? (
                    <span
                      className="text-muted-foreground mt-0.5 block text-[11px]"
                      title={f.error_code}
                    >
                      {uploadError(f.error_code)}
                    </span>
                  ) : f.status === "held" ? (
                    <span className="text-muted-foreground mt-0.5 block text-[11px]">
                      {t("office.errors.upload.held")}
                    </span>
                  ) : null}
                </td>
                <td>
                  {isRealUUID(f.invoice_uuid) ? (
                    <Link
                      className="hover:underline"
                      href={routes.tenant.invoice(
                        slug,
                        f.invoice_uuid,
                        engagement,
                      )}
                    >
                      <Figure>
                        {f.invoice_number || t("office.api.open_invoice")}
                      </Figure>
                    </Link>
                  ) : (
                    "—"
                  )}
                  {isRealUUID(f.ettn) ? (
                    <Figure className="text-muted-foreground block text-[11px]">
                      {f.ettn}
                    </Figure>
                  ) : null}
                </td>
                <td>
                  <div className="flex flex-wrap gap-1">
                    {f.warnings?.length
                      ? f.warnings.map((c) => (
                          <span
                            key={c}
                            className="inline-flex items-center gap-1"
                          >
                            <SeverityBadge severity={severityOf(c)} />
                            <Figure className="text-xs">{c}</Figure>
                          </span>
                        ))
                      : "—"}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </TableBox>
      ) : null}
    </Panel>
  );
}

function UploadInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { apiError } = useOfficeMessages();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug);
  const engagement = selected?.engagement ?? "";
  const uploads = useUBLUploads(slug, engagement);
  const create = useCreateUBLUploads(slug, engagement);
  const [files, setFiles] = useState<File[]>([]);
  const [dragging, setDragging] = useState(false);
  const [shown, setShown] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const accept = (list: FileList | null) => {
    if (!list) return;
    setFiles(Array.from(list).filter((f) => /\.(xml|zip)$/i.test(f.name)));
  };

  const run = () => {
    if (!engagement || files.length === 0) return;
    create.mutate(files, {
      onSuccess: (jobs) => {
        setFiles([]);
        if (input.current) input.current.value = "";
        if (jobs.length) setShown(jobs[jobs.length - 1].uuid);
      },
    });
  };

  const header = (
    <PageHeader
      title={t("office.upload_title")}
      description={t("office.upload_description")}
      icon={<Upload className="size-6" />}
    />
  );
  if (isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }
  if (isError || options.length === 0) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={
            isError
              ? t("office.api.load_error")
              : t("office.api.no_engagements")
          }
        />
      </div>
    );
  }

  const jobs = uploads.data?.items ?? [];
  return (
    <div className="space-y-4">
      {header}
      <Panel>
        <div className="space-y-4 p-4">
          <EngagementPicker
            options={options}
            selected={selected}
            onSelect={(v) => {
              setShown(null);
              select(v);
            }}
          />
          <label
            onDragOver={(e) => {
              e.preventDefault();
              setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragging(false);
              accept(e.dataTransfer.files);
            }}
            className={cn(
              "hover:border-brand/60 flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors",
              dragging && "border-brand bg-brand/5",
            )}
          >
            <FileUp className="text-muted-foreground size-8" aria-hidden />
            <span className="font-medium">{t("office.upload_drop")}</span>
            <span className="text-muted-foreground text-xs">
              {t("office.upload_hint")}
            </span>
            <input
              ref={input}
              type="file"
              multiple
              accept=".xml,.zip,application/xml,text/xml,application/zip"
              className="sr-only"
              onChange={(e) => accept(e.target.files)}
              data-testid="ubl-input"
            />
          </label>
          {files.length ? (
            <ul className="text-sm">
              {files.map((f) => (
                <li key={f.name} className="flex justify-between gap-4">
                  <Figure className="truncate text-xs">{f.name}</Figure>
                  <Figure className="text-muted-foreground text-xs">
                    {Math.ceil(f.size / 1024)} KB
                  </Figure>
                </li>
              ))}
            </ul>
          ) : null}
          {create.isError ? (
            <p className="text-status-blocking text-sm">
              {apiError(create.error)}
            </p>
          ) : null}
          <div className="flex justify-end">
            <Button
              onClick={run}
              disabled={create.isPending || !engagement || files.length === 0}
            >
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Upload className="size-4" />
              )}
              {t("office.upload_submit", { count: files.length })}
            </Button>
          </div>
        </div>
      </Panel>

      {shown ? <JobReport engagement={engagement} jobId={shown} /> : null}

      {jobs.length ? (
        <Panel title={t("office.api.upload_jobs_title")}>
          <ul className="divide-y text-sm">
            {jobs.map((j) => (
              <li
                key={j.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-4 py-2"
              >
                <Figure className="max-w-80 truncate text-xs">
                  {j.file_name}
                </Figure>
                <span className="text-muted-foreground flex items-center gap-2 text-xs">
                  <JobStatus status={j.status} />
                  {j.report
                    ? t("office.api.upload_report_summary", {
                        added: j.report.added,
                        duplicate: j.report.duplicate,
                        failed: j.report.failed,
                      })
                    : null}
                  <Figure>
                    {new Date(j.created_at).toLocaleString(
                      locale === "tr" ? "tr-TR" : "en-GB",
                    )}
                  </Figure>
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShown(j.uuid)}
                >
                  {t("office.upload_show_report")}
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}

      <p className="text-muted-foreground text-xs">
        {t("office.api.upload_footnote")}{" "}
        <Link
          className="text-brand hover:underline"
          href={`${routes.tenant.invoices(slug)}?taxpayer=${encodeURIComponent(engagement)}`}
        >
          {t("office.nav_invoices")}
        </Link>
      </p>
    </div>
  );
}

export function UploadPage() {
  return (
    <Suspense fallback={<Skeleton className="h-48 w-full" />}>
      <UploadInner />
    </Suspense>
  );
}
