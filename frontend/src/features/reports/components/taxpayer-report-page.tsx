"use client";

import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useMyReport } from "@/features/reports/hooks/use-report";
import type { ReportRange } from "@/features/reports/services/reports.service";
import { useLocale } from "@/providers/locale-provider";

import { ReportRangePicker, thisMonth } from "./report-range-picker";
import { ReportSkeleton, ReportView } from "./report-view";

function previousMonth() {
  const [y, m] = thisMonth().split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** `/m/{uuid}/report` (#386): mükellefin kendi raporu (yorumsuz). */
export function TaxpayerReportPage({ taxpayer }: { taxpayer: string }) {
  const { t } = useLocale();
  const [range, setRange] = useState<ReportRange>({
    kind: "month",
    period: previousMonth(),
  });
  const report = useMyReport(taxpayer, range);
  return (
    <Card>
      <CardHeader className="space-y-3">
        <CardTitle>{t("reports.taxpayer_title")}</CardTitle>
        <p className="text-text-2 text-sm">
          {t("reports.taxpayer_description")}
        </p>
        <ReportRangePicker value={range} onChange={setRange} />
      </CardHeader>
      <CardContent>
        {report.isError ? (
          <ErrorState
            title={t("reports.error_title")}
            description={t("reports.error_description")}
            onRetry={() => void report.refetch()}
            retryLabel={t("reports.retry")}
          />
        ) : report.data ? (
          <div className={report.isPlaceholderData ? "opacity-60" : undefined}>
            <ReportView report={report.data} />
          </div>
        ) : (
          <ReportSkeleton />
        )}
      </CardContent>
    </Card>
  );
}
