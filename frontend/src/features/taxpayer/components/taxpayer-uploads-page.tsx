"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
  type TaxpayerUploadJob,
} from "@/features/taxpayer/services/taxpayer.service";
import { ApiError } from "@/lib/api/errors";
import { useLocale } from "@/providers/locale-provider";

import { formatDate } from "./format";
import { TaxpayerForeignInvoicesCard } from "./taxpayer-foreign-invoices-card";

/** `/m/{uuid}/uploads`: send e-invoice XML / ZIP files to the office. */
export function TaxpayerUploadsPage({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [last, setLast] = useState<TaxpayerUploadJob | null>(null);
  const [error, setError] = useState<string | null>(null);
  const list = useQuery({
    queryKey: taxpayerPanelKeys.uploads(taxpayer),
    queryFn: () => taxpayerPanelService.uploads(taxpayer),
  });
  const send = useMutation({
    mutationFn: (file: File) => taxpayerPanelService.upload(taxpayer, file),
    onSuccess: (job) => {
      setLast(job);
      setError(null);
      void qc.invalidateQueries({
        queryKey: taxpayerPanelKeys.uploads(taxpayer),
      });
    },
    onError: (err) => {
      setLast(null);
      const code = err instanceof ApiError ? err.code : undefined;
      setError(
        code === "NO_ACTIVE_ENGAGEMENT"
          ? t("taxpayer.upload_no_office")
          : code === "UNSUPPORTED_FILE"
            ? t("taxpayer.upload_unsupported")
            : t("taxpayer.upload_failed"),
      );
    },
  });

  return (
    <div className="space-y-6">
      <TaxpayerForeignInvoicesCard taxpayer={taxpayer} />
      <Card>
        <CardHeader>
          <CardTitle>{t("taxpayer.upload_title")}</CardTitle>
          <CardDescription>{t("taxpayer.upload_description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <input
            ref={input}
            type="file"
            accept=".xml,.zip,application/xml,text/xml,application/zip"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) send.mutate(file);
              e.target.value = "";
            }}
          />
          <Button
            onClick={() => input.current?.click()}
            disabled={send.isPending}
          >
            {send.isPending
              ? t("taxpayer.upload_sending")
              : t("taxpayer.upload_choose")}
          </Button>
          {error ? (
            <p className="text-destructive text-sm" role="alert">
              {error}
            </p>
          ) : null}
          {last ? <UploadSummary job={last} /> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("taxpayer.uploads_history")}</CardTitle>
        </CardHeader>
        <CardContent>
          {list.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("common.loading")}
            </p>
          ) : !list.data || list.data.items.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("taxpayer.uploads_empty")}
            </p>
          ) : (
            <ul className="divide-y">
              {list.data.items.map((job) => (
                <li
                  key={job.uuid}
                  className="flex items-center justify-between gap-3 py-2 text-sm"
                >
                  <div>
                    <div>{job.file_name}</div>
                    <div className="text-muted-foreground text-xs">
                      {formatDate(job.created_at, locale)}
                    </div>
                  </div>
                  <UploadCounts job={job} />
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function UploadCounts({ job }: { job: TaxpayerUploadJob }) {
  const { t } = useLocale();
  if (job.status !== "done" || !job.report) {
    return (
      <Badge variant={job.status === "failed" ? "danger" : "secondary"}>
        {t(`taxpayer.upload_status_${job.status}`)}
      </Badge>
    );
  }
  return (
    <div className="flex gap-1">
      <Badge variant="success">
        {t("taxpayer.upload_added", { count: job.report.added })}
      </Badge>
      {job.report.duplicate > 0 ? (
        <Badge variant="secondary">
          {t("taxpayer.upload_duplicate", { count: job.report.duplicate })}
        </Badge>
      ) : null}
      {job.report.failed > 0 ? (
        <Badge variant="danger">
          {t("taxpayer.upload_failed_count", { count: job.report.failed })}
        </Badge>
      ) : null}
    </div>
  );
}

function UploadSummary({ job }: { job: TaxpayerUploadJob }) {
  const { t } = useLocale();
  return (
    <div className="space-y-2 rounded-md border p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span>{job.file_name}</span>
        <UploadCounts job={job} />
      </div>
      {job.report?.items
        ?.filter((i) => i.status === "failed")
        .map((i) => (
          <div key={i.name} className="text-destructive text-xs">
            {i.name}:{" "}
            {i.error_code
              ? t(`taxpayer.upload_error_${i.error_code}`)
              : i.error}
          </div>
        ))}
    </div>
  );
}
