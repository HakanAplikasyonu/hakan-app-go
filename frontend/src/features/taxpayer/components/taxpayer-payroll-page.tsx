"use client";

import { useQuery } from "@tanstack/react-query";
import { Download, FileText } from "lucide-react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  taxpayerPayrollKeys,
  taxpayerPayrollService,
  type TaxpayerPayrollDocument,
} from "@/features/taxpayer/services/payroll.service";
import { useLocale } from "@/providers/locale-provider";

import { formatDate } from "./format";

function monthLabel(period: string, locale: string) {
  return new Date(`${period}-01T00:00:00`).toLocaleDateString(locale, {
    month: "long",
    year: "numeric",
  });
}

/**
 * `/m/{uuid}/payroll` (#382): ofisin hazırladığı bordro dökümleri ve
 * pusulaları, döneme göre gruplu; yalnızca okuma ve indirme.
 */
export function TaxpayerPayrollPage({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const intl = locale === "tr" ? "tr-TR" : "en-GB";
  const list = useQuery({
    queryKey: taxpayerPayrollKeys.documents(taxpayer),
    queryFn: () => taxpayerPayrollService.documents(taxpayer),
  });
  const groups = new Map<string, TaxpayerPayrollDocument[]>();
  for (const d of list.data?.items ?? []) {
    groups.set(d.period, [...(groups.get(d.period) ?? []), d]);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("taxpayer.payroll_title")}</CardTitle>
        <CardDescription>{t("taxpayer.payroll_description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {list.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : list.isError ? (
          <ErrorState
            title={t("taxpayer.payroll_error")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : groups.size === 0 ? (
          <BosDurum
            className="border-0"
            baslik={t("taxpayer.payroll_empty_title")}
            aciklama={t("taxpayer.payroll_empty")}
          />
        ) : (
          [...groups.entries()].map(([period, docs]) => (
            <section key={period} aria-labelledby={`payroll-${period}`}>
              <h2
                id={`payroll-${period}`}
                className="text-text-2 mb-2 text-sm font-semibold capitalize"
              >
                {monthLabel(period, intl)}
              </h2>
              <ul className="border-line divide-line divide-y rounded-lg border">
                {docs.map((d) => (
                  <li
                    key={d.uuid}
                    className="flex items-center gap-3 px-3 py-2.5"
                  >
                    <FileText
                      aria-hidden
                      className="text-text-3 size-5 shrink-0"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {d.title}
                      </span>
                      <span className="text-text-3 block text-xs">
                        {t(`taxpayer.payroll_kind_${d.kind}`)}
                        {d.employee_name ? ` · ${d.employee_name}` : ""} ·{" "}
                        {formatDate(d.created_at, intl)}
                      </span>
                    </span>
                    <Button
                      size="sm"
                      variant="outline"
                      className="min-h-11 sm:min-h-9"
                      onClick={() =>
                        void taxpayerPayrollService
                          .download(taxpayer, d)
                          .catch(() =>
                            toast.error(t("taxpayer.payroll_download_failed")),
                          )
                      }
                    >
                      <Download aria-hidden className="size-4" />
                      {t("taxpayer.payroll_download")}
                    </Button>
                  </li>
                ))}
              </ul>
            </section>
          ))
        )}
      </CardContent>
    </Card>
  );
}
