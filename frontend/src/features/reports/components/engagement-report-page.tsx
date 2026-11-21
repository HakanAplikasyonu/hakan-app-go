"use client";

import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { useOfficePeriod } from "@/features/office/period";
import { useEngagementReport } from "@/features/reports/hooks/use-report";
import type { ReportRange } from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { EngagementNextWork } from "@/features/copilot";

import { EngagementDeliveryHistory } from "./delivery-history";
import { ReportActions } from "./report-actions";
import { ReportScheduleCard } from "./report-schedule-card";
import { VATNoticeCard } from "./vat-notice-card";

import { ReportRangePicker } from "./report-range-picker";
import { ReportSkeleton, ReportView } from "./report-view";

/**
 * `/t/{slug}/engagements/{uuid}/report` (#386, "mükellef bazlı özel ilgi"):
 * mükellefin aylık / yıllık raporu. İlk açılışta ofisin global dönemi
 * (Tüm dönemler seçiliyse beyan dönemi). PDF indirme, tek tıkla e-posta
 * ve gönderim geçmişi aynı sayfada.
 */
export function EngagementReportPage({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  const [range, setRange] = useState<ReportRange>({
    kind: "month",
    period: focus,
  });
  const report = useEngagementReport(slug, engagement, range);

  return (
    <div className="space-y-5">
      <div className="min-w-0">
        <h2 className="font-display truncate text-xl font-bold">
          {t("reports.title")}
        </h2>
        <p className="text-text-2 text-sm">{t("reports.description")}</p>
      </div>
      <EngagementNextWork engagement={engagement} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ReportRangePicker value={range} onChange={setRange} />
        {report.data ? (
          <div className="flex flex-wrap gap-2">
            <ReportActions
              engagement={engagement}
              report={report.data}
              range={range}
              canSend={can(permissions.tenant.engagements.write)}
            />
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
      <VATNoticeCard
        engagement={engagement}
        canSend={can(permissions.tenant.engagements.write)}
      />
      <ReportScheduleCard
        engagement={engagement}
        canWrite={can(permissions.tenant.engagements.write)}
      />
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("reports.history.title")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <EngagementDeliveryHistory engagement={engagement} />
        </CardContent>
      </Card>
    </div>
  );
}
