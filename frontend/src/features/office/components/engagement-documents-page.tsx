"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { FileSignature } from "lucide-react";
import Link from "next/link";
import { Suspense, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  useUrlListState,
} from "@/components/entity";
import {
  createColumn,
  createDateColumn,
  createMoneyColumn,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { historyShareRangeLabel } from "@/features/taxpayer/lib/history-share";
import {
  useEngagementDocuments,
  useEngagementHistoryShares,
} from "@/features/office/hooks/use-engagement-documents";
import type { EngagementDocument } from "@/features/office/services/engagement-documents.service";
import { useOfficePeriod } from "@/features/office/period/use-office-period";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { EngagementNextWork } from "@/features/copilot";

import { InvoiceDrawer } from "./invoice-drawer";

const PAGE_SIZE = 25;

function useDocumentColumns() {
  const { t } = useLocale();
  return useMemo<ColumnDef<EngagementDocument, unknown>[]>(
    () => [
      createColumn<EngagementDocument>({
        id: "invoice_number",
        labelKey: "office.engagement_col_number",
        accessorKey: "invoice_number",
        gridPrimary: true,
        meta: { cellClassName: "font-mono" },
      }),
      createDateColumn<EngagementDocument>({
        id: "issue_date",
        labelKey: "office.engagement_col_date",
        accessor: (doc) => doc.issue_date,
        className: "text-muted-foreground",
        gridSecondary: true,
      }),
      createColumn<EngagementDocument>({
        id: "direction",
        labelKey: "office.col_direction",
        accessorKey: "direction",
        cell: ({ row }) =>
          row.original.direction === "outgoing"
            ? t("office.direction_sales")
            : t("office.direction_purchase"),
      }),
      createMoneyColumn<EngagementDocument>({
        id: "payable_amount",
        labelKey: "office.engagement_col_amount",
        accessor: (doc) => doc.payable_amount,
        currency: (doc) => doc.currency || "TRY",
      }),
    ],
    [t],
  );
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
  const { t } = useLocale();
  const { slug } = useTenant();
  // Sayfa URL'de (#381). Liste toplam vermiyor: dolu sayfa "sonraki
  // olabilir" demek; çekirdek sayfalaması toplamsız (hasNextPage) çalışır.
  const listState = useUrlListState({ initialPageSize: PAGE_SIZE });
  const { limit, offset } = listState.params;
  // Üst çubuktaki dönem listeyi süzer (#576); "Tüm dönemler"de hepsi.
  const { period } = useOfficePeriod();
  const query = useEngagementDocuments(
    slug,
    engagement,
    { limit, offset },
    enabled,
    period,
  );
  const columns = useDocumentColumns();
  const items = query.data?.items ?? [];
  // Satır belgenin yan panelini açar (#571): uyarılar, kalemler, ilişkili
  // kayıtlar ve mükelleften düzeltme isteme. Açık belge adreste; geri tuşu
  // ve paylaşılan bağlantı çalışır.
  const open = listState.getParam("belge");
  const openDocument = (uuid: string | null) =>
    listState.setParams({ belge: uuid }, { resetPage: false });

  return (
    <EntitySectionCard title={t("office.engagement_documents_title")}>
      <EntityTable<EngagementDocument>
        columns={columns}
        data={items}
        getRowId={(doc) => doc.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("office.engagement_documents_empty")}
        emptyDescription={t("office.engagement_documents_empty_hint")}
        bordered={false}
        state={{
          pagination: listState.pagination,
          onPaginationChange: listState.onPaginationChange,
        }}
        hasNextPage={!query.isFetching && items.length >= limit}
        onRowClick={(doc) => openDocument(doc.uuid)}
        isRowActive={(doc) => doc.uuid === open}
        pageSizeOptions={[25, 50, 100]}
        // Kart içinde gömülü liste: araç çubuğu ve seçim yok; sunucu
        // sıralama sunmuyor.
        features={{
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          rowSelection: false,
          viewMode: false,
          columnVisibility: false,
          columnResizing: false,
          columnPinning: false,
          columnOrdering: false,
          density: false,
        }}
      />
      <InvoiceDrawer
        engagement={engagement}
        invoice={open}
        onClose={() => openDocument(null)}
      />
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
      headingLevel={2}
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
        <EngagementNextWork engagement={engagementUuid} />
        <EntitySectionCard title={t("office.engagement_history_title")}>
          <HistorySharesLine engagement={engagementUuid} enabled={canRead} />
        </EntitySectionCard>
        <Suspense fallback={<Loading label={t("common.loading")} />}>
          <DocumentsSection engagement={engagementUuid} enabled={canRead} />
        </Suspense>
      </div>
    </EntityPage>
  );
}
