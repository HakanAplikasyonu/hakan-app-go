"use client";

import { FileUp, Loader2, Upload } from "lucide-react";
import Link from "next/link";
import { useRef, useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOffice, useOfficeActions } from "../hooks/use-office";
import { severityOf } from "../lib/warnings";
import type { UploadFileResult, UploadReport } from "../types";
import {
  DemoNotice,
  FilterSelect,
  Figure,
  OfficeDataGate,
  Panel,
  SeverityBadge,
  TableBox,
} from "./office-ui";

const outcomeClass: Record<UploadFileResult["outcome"], string> = {
  imported: "bg-status-approved/12 text-status-approved",
  duplicate: "bg-secondary text-secondary-foreground",
  not_ubl: "bg-status-warning/12 text-status-warning",
  error: "bg-status-blocking/12 text-status-blocking",
};

function ReportTable({ report }: { report: UploadReport }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const count = (o: UploadFileResult["outcome"]) =>
    report.files.filter((f) => f.outcome === o).length;
  return (
    <Panel
      title={t("office.upload_report_title")}
      description={t("office.upload_report_summary", {
        imported: count("imported"),
        duplicate: count("duplicate"),
        skipped: count("not_ubl") + count("error"),
        warnings: report.files.reduce((n, f) => n + f.warningCodes.length, 0),
      })}
    >
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
          {report.files.map((f, i) => (
            <tr key={i}>
              <td className="max-w-80 truncate">
                <Figure className="text-xs">{f.fileName}</Figure>
              </td>
              <td>
                <span
                  className={cn(
                    "rounded-md px-2 py-0.5 text-xs font-medium",
                    outcomeClass[f.outcome],
                  )}
                >
                  {t(`office.outcome_${f.outcome}`)}
                </span>
              </td>
              <td>
                {f.invoiceId ? (
                  <Link
                    className="hover:underline"
                    href={routes.tenant.invoice(slug, f.invoiceId)}
                  >
                    <Figure>{f.number}</Figure>
                  </Link>
                ) : (
                  "—"
                )}
                {f.ettn ? (
                  <Figure className="text-muted-foreground block text-[11px]">
                    {f.ettn}
                  </Figure>
                ) : null}
              </td>
              <td>
                <div className="flex flex-wrap gap-1">
                  {f.warningCodes.length
                    ? f.warningCodes.map((c) => (
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
    </Panel>
  );
}

function UploadInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const taxpayers = useOffice((s) => s.taxpayers);
  const uploads = useOffice((s) => s.uploads);
  const { importFiles } = useOfficeActions();
  const [taxpayer, setTaxpayer] = useState(taxpayers[0]?.id ?? "");
  const [files, setFiles] = useState<File[]>([]);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [last, setLast] = useState<UploadReport | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const accept = (list: FileList | null) => {
    if (!list) return;
    setFiles(Array.from(list).filter((f) => /\.(xml|zip)$/i.test(f.name)));
  };

  const run = async () => {
    if (!taxpayer || files.length === 0) return;
    setBusy(true);
    try {
      setLast(await importFiles(taxpayer, files));
      setFiles([]);
      if (input.current) input.current.value = "";
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.upload_title")}
        description={t("office.upload_description")}
        icon={<Upload className="size-6" />}
      />
      <DemoNotice />
      <Panel>
        <div className="space-y-4 p-4">
          <FilterSelect
            label={t("office.col_taxpayer")}
            value={taxpayer}
            onChange={setTaxpayer}
            options={taxpayers.map((x) => ({
              value: x.id,
              label: `${x.title} · ${x.taxId}`,
            }))}
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
          <div className="flex justify-end">
            <Button
              onClick={run}
              disabled={busy || !taxpayer || files.length === 0}
            >
              {busy ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Upload className="size-4" />
              )}
              {t("office.upload_submit", { count: files.length })}
            </Button>
          </div>
        </div>
      </Panel>

      {last ? <ReportTable report={last} /> : null}

      {uploads.length ? (
        <Panel title={t("office.upload_history")}>
          <ul className="divide-y text-sm">
            {uploads.map((u) => (
              <li
                key={u.id}
                className="flex flex-wrap items-center justify-between gap-2 px-4 py-2"
              >
                <span>
                  {taxpayers.find((x) => x.id === u.taxpayerId)?.title}
                </span>
                <span className="text-muted-foreground text-xs">
                  {t("office.upload_history_row", {
                    files: u.files.length,
                    imported: u.files.filter((f) => f.outcome === "imported")
                      .length,
                  })}{" "}
                  ·{" "}
                  <Figure>
                    {new Date(u.createdAt).toLocaleString(
                      locale === "tr" ? "tr-TR" : "en-GB",
                    )}
                  </Figure>
                </span>
                <Button variant="ghost" size="sm" onClick={() => setLast(u)}>
                  {t("office.upload_show_report")}
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}

      <p className="text-muted-foreground text-xs">
        {t("office.upload_footnote")}{" "}
        <Link
          className="text-brand hover:underline"
          href={routes.tenant.invoices(slug)}
        >
          {t("office.nav_invoices")}
        </Link>
      </p>
    </div>
  );
}

export function UploadPage() {
  return (
    <OfficeDataGate>
      <UploadInner />
    </OfficeDataGate>
  );
}
