"use client";

import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EngagementTabs } from "@/features/office/components/engagement-overview";
import { usePeriodMatrix } from "@/features/office/hooks/use-office-data";
import { useOfficePeriod } from "@/features/office/period";
import { useEngagementReport } from "@/features/reports/hooks/use-report";
import type {
  ReportRange,
  TaxpayerReport,
} from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { ReportRangePicker } from "./report-range-picker";
import { ReportSkeleton, ReportView } from "./report-view";

/**
 * `/t/{slug}/engagements/{uuid}/report` (#386, "mükellef bazlı özel ilgi"):
 * mükellefin aylık / yıllık raporu. İlk açılışta ofisin global dönemi
 * (Tüm dönemler seçiliyse beyan dönemi). `actions` raporun yanındaki
 * düğmeler (PDF, e-posta).
 */
export function EngagementReportPage({
  engagement,
  actions,
}: {
  engagement: string;
  actions?: (report: TaxpayerReport, range: ReportRange) => ReactNode;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  const matrix = usePeriodMatrix(1);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  const [range, setRange] = useState<ReportRange>({
    kind: "month",
    period: focus,
  });
  const report = useEngagementReport(slug, engagement, range);

  return (
    <div className="space-y-5">
      <div className="min-w-0">
        <h1 className="font-display truncate text-2xl font-bold">
          {row?.taxpayer_title ?? t("reports.title")}
        </h1>
        <p className="text-text-2 text-sm">{t("reports.description")}</p>
      </div>
      <EngagementTabs engagement={engagement} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ReportRangePicker value={range} onChange={setRange} />
        {report.data && actions ? (
          <div className="flex flex-wrap gap-2">
            {actions(report.data, range)}
          </div>
        ) : null}
      </div>
      {report.isError ? (
        <ErrorState
          title={t("reports.error_title")}
          description={t("reports.error_description")}
          onRetry={() => void report.refetch()}
          retryLabel={t("reports.retry")}
        />
      ) : report.data ? (
        <div
          className={report.isPlaceholderData ? "opacity-60" : undefined}
          aria-busy={report.isPlaceholderData || undefined}
        >
          <h2 className="mb-3 text-lg font-semibold">{report.data.label}</h2>
          <ReportView report={report.data} showDraftCommentary />
        </div>
      ) : (
        <ReportSkeleton />
      )}
    </div>
  );
}
