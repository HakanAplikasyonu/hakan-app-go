"use client";

import { FileSignature } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import type { AppLocale } from "@/config/i18n";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { historyShareRangeLabel } from "@/features/taxpayer/lib/history-share";
import {
  useEngagementDocuments,
  useEngagementHistoryShares,
} from "@/features/office/hooks/use-engagement-documents";
import type { EngagementDocument } from "@/features/office/services/engagement-documents.service";
import { currency, date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { EngagementTabs } from "./engagement-overview";

const PAGE_SIZE = 25;

function formatAmount(doc: EngagementDocument, locale: AppLocale) {
  const value = Number(doc.payable_amount);
  if (!Number.isFinite(value)) return `${doc.payable_amount} ${doc.currency}`;
  try {
    return currency(value, doc.currency || "TRY", locale);
  } catch {
    return `${doc.payable_amount} ${doc.currency}`;
  }
}

/** The owner's consents for documents from before this engagement (D2). */
function HistorySharesLine({
  engagement,
  enabled,
}: {
  engagement: string;
  enabled: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useEngagementHistoryShares(slug, engagement, enabled);
  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const active = (query.data?.items ?? []).filter((s) => s.active);
  return (
    <p className="text-muted-foreground text-sm">
      {active.length === 0
        ? t("office.engagement_history_none")
        : t("office.engagement_history_active", {
            ranges: active
              .map((s) => historyShareRangeLabel(s, t, locale))
              .join(", "),
          })}
    </p>
  );
}

function DocumentsSection({
  engagement,
  enabled,
}: {
  engagement: string;
  enabled: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [offset, setOffset] = useState(0);
  const query = useEngagementDocuments(
    slug,
    engagement,
    { limit: PAGE_SIZE, offset },
    enabled,
  );
  const items = query.data?.items ?? [];
  // The list has no total: a full page means there may be more.
  const hasNext = items.length >= PAGE_SIZE;

  return (
    <EntitySectionCard title={t("office.engagement_documents_title")}>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("office.engagement_documents_empty")}
        </p>
      ) : null}
      {items.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  {t("office.engagement_col_number")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("office.engagement_col_date")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("office.col_direction")}
                </th>
                <th className="py-2 text-end font-medium">
                  {t("office.engagement_col_amount")}
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((doc) => (
                <tr key={doc.uuid} className="border-b last:border-0">
                  <td className="py-2 pe-3 font-mono">{doc.invoice_number}</td>
                  <td className="text-muted-foreground py-2 pe-3">
                    {date(doc.issue_date, "dd.MM.yyyy", locale)}
                  </td>
                  <td className="py-2 pe-3">
                    {doc.direction === "outgoing"
                      ? t("office.direction_sales")
                      : t("office.direction_purchase")}
                  </td>
                  <td className="py-2 text-end tabular-nums">
                    {formatAmount(doc, locale)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      {offset > 0 || hasNext ? (
        <div className="mt-4 flex items-center justify-end gap-2 text-sm">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={offset === 0 || query.isFetching}
            onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
          >
            {t("office.engagement_prev")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!hasNext || query.isFetching}
            onClick={() => setOffset((o) => o + PAGE_SIZE)}
          >
            {t("office.engagement_next")}
          </Button>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/**
 * `/t/{slug}/engagements/{engagementUuid}/documents` (issue #8): the
 * documents this office may see for one mükellef, and whether the owner
 * shared data from before the engagement. No nav entry yet.
 */
export function EngagementDocumentsPage({
  engagementUuid,
}: {
  engagementUuid: string;
}) {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.engagements.read);

  return (
    <EntityPage
      title={t("office.engagement_documents_page_title")}
      description={t("office.engagement_documents_page_description")}
      permission={permissions.tenant.engagements.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("errors.forbidden_body")}
        />
      }
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("office.engagement_documents_page_title") },
      ]}
      actions={
        <Button asChild variant="outline" size="sm">
          <Link
            href={routes.tenant.engagementPetitions(
              tenant.slug,
              engagementUuid,
            )}
          >
            <FileSignature className="size-4" />
            {t("petitions.documents_page_link")}
          </Link>
        </Button>
      }
    >
      <div className="space-y-6">
        <EngagementTabs engagement={engagementUuid} />
        <EntitySectionCard title={t("office.engagement_history_title")}>
          <HistorySharesLine engagement={engagementUuid} enabled={canRead} />
        </EntitySectionCard>
        <DocumentsSection engagement={engagementUuid} enabled={canRead} />
      </div>
    </EntityPage>
  );
}
