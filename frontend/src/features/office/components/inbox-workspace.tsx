"use client";

import { ArrowRight, Check, Inbox } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useUrlParam } from "@/hooks/use-url-param";
import { isApiError } from "@/lib/api";
import { formatMoney } from "@/lib/decimal";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useEngagementScope } from "../engagement-scope";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import {
  INBOX_PAGE,
  INBOX_TYPES,
  useApproveInboxItem,
  useInbox,
  type InboxItem,
  type InboxType,
} from "../hooks/use-inbox";
import {
  currentPeriod,
  periodLabel,
  recentPeriods,
  useUrlOfficePeriod,
} from "../period";
import { ENGAGEMENT_PARAM } from "./engagement-picker";
import { FilterSelect } from "./office-ui";

const TYPE_TONE: Record<InboxType, StatusTone> = {
  invoice: "info",
  bank_transaction: "neutral",
  expense: "warn",
  retail_sale: "warn",
  foreign_invoice: "info",
  late_document: "danger",
};

const TYPE_LABEL: Record<InboxType, string> = {
  invoice: "office.inbox.type_invoice",
  bank_transaction: "office.inbox.type_bank_transaction",
  expense: "office.inbox.type_expense",
  retail_sale: "office.inbox.type_retail_sale",
  foreign_invoice: "office.inbox.type_foreign_invoice",
  late_document: "office.inbox.type_late_document",
};

type TypeFilter = "all" | InboxType;
const TYPE_FILTERS: readonly TypeFilter[] = ["all", ...INBOX_TYPES];

/** Satırın işlendiği ekran: mükellef sayfasındaki sekme. */
function useTargetHref() {
  const { slug } = useTenant();
  return (item: InboxItem) => {
    const eng = item.engagement_uuid;
    switch (item.target.tab) {
      case "invoices":
        return routes.tenant.engagementInvoice(slug, eng, item.target.id);
      case "bank":
        return routes.tenant.engagementBank(slug, eng);
      case "expenses":
        return routes.tenant.engagementExpenses(slug, eng);
      case "z-reports":
        return routes.tenant.engagementZReports(slug, eng);
      case "foreign-invoices":
        return routes.tenant.engagementForeignInvoices(slug, eng);
      default:
        return routes.tenant.engagementLateDocuments(slug, eng);
    }
  };
}

/**
 * Gelen kutusu (#780 b): İnceleme ekranının ikinci görünümü. Önerisi olmayan
 * ama müşavirin elini bekleyen her şey tek listede; üstte türe göre
 * sayaçlar (tıklayınca süzer), mükellef ve dönem süzgeci. Her satır işlendiği
 * ekrana gider; onay ucu olan türlerde (masraf fişi, Z raporu) satırdan
 * onaylanır. Mükellef sekmesinde mükellef sabittir.
 */
export function InboxWorkspace() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canApprove = can(permissions.tenant.receipts.approve);
  const scope = useEngagementScope();
  const directory = useEngagementDirectory(!scope);
  const office = useUrlOfficePeriod();
  const period = office.period ?? currentPeriod();
  const [type, setType] = useUrlParam<TypeFilter>("type", "all", TYPE_FILTERS);
  const [urlEngagement, setEngagement] = useUrlParam(ENGAGEMENT_PARAM, "all");
  const engagement =
    scope ?? (urlEngagement === "all" ? undefined : urlEngagement);
  const [page, setPage] = useState(0);
  const filterKey = `${type}|${engagement ?? ""}|${period}`;
  const [pageFor, setPageFor] = useState(filterKey);
  if (pageFor !== filterKey) {
    setPageFor(filterKey);
    setPage(0);
  }
  const query = useInbox({
    period,
    type: type === "all" ? undefined : type,
    engagement,
    limit: INBOX_PAGE,
    offset: page * INBOX_PAGE,
  });
  const approve = useApproveInboxItem();
  const targetHref = useTargetHref();
  const data = query.data;
  const counts = data?.counts;
  const typeLabel = (v: InboxType) => t(TYPE_LABEL[v]);

  const onApprove = (item: InboxItem) =>
    approve.mutate(item, {
      onSuccess: () => toast.success(t("office.inbox.approved")),
      onError: (e) =>
        toast.error(
          isApiError(e) && e.message
            ? e.message
            : t("office.inbox.approve_failed"),
        ),
    });

  const periods = recentPeriods(18);
  if (!periods.includes(period)) periods.unshift(period);

  return (
    <section
      aria-labelledby="inbox-title"
      className="space-y-4"
      data-testid="inbox"
    >
      <header className="flex flex-wrap items-start gap-3">
        <div className="min-w-0 flex-1">
          <h2 id="inbox-title" className="text-lg font-semibold">
            {t("office.inbox.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.inbox.description", {
              period: periodLabel(period, locale),
            })}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {scope ? null : (
            <FilterSelect
              label={t("office.col_taxpayer")}
              value={urlEngagement}
              onChange={setEngagement}
              options={[
                { value: "all", label: t("office.filter_all") },
                ...directory.list.map((x) => ({
                  value: x.engagementUuid,
                  label: x.title,
                })),
              ]}
            />
          )}
          <FilterSelect
            label={t("office.inbox.period")}
            value={period}
            onChange={(v) => office.setPeriod(v)}
            options={periods.map((p) => ({
              value: p,
              label: periodLabel(p, locale),
            }))}
          />
        </div>
      </header>

      <div
        role="group"
        aria-label={t("office.inbox.counts_label")}
        className="flex flex-wrap gap-2"
        data-testid="inbox-counts"
      >
        <FilterChip
          pressed={type === "all"}
          onPressedChange={() => setType("all")}
          data-type="all"
        >
          {t("office.inbox.all", { count: counts?.total ?? 0 })}
        </FilterChip>
        {INBOX_TYPES.map((v) => (
          <FilterChip
            key={v}
            pressed={type === v}
            onPressedChange={() => setType(type === v ? "all" : v)}
            data-type={v}
          >
            {typeLabel(v)}
            <span className="ms-1.5 tabular-nums">{counts?.[v] ?? 0}</span>
          </FilterChip>
        ))}
      </div>

      {query.isLoading ? (
        <Loading />
      ) : query.isError ? (
        <ErrorState
          title={t("office.inbox.load_failed")}
          description={query.error.message}
          onRetry={() => void query.refetch()}
        />
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          title={t("office.inbox.empty_title")}
          description={t("office.inbox.empty_description")}
        />
      ) : (
        <>
          <Table data-testid="inbox-table">
            <TableCaption className="sr-only">
              {t("office.inbox.title")}
            </TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead scope="col">{t("office.inbox.col_type")}</TableHead>
                {scope ? null : (
                  <TableHead scope="col">{t("office.col_taxpayer")}</TableHead>
                )}
                <TableHead scope="col">
                  {t("office.inbox.col_description")}
                </TableHead>
                <TableHead scope="col" className="text-right">
                  {t("office.inbox.col_amount")}
                </TableHead>
                <TableHead scope="col">{t("office.inbox.col_date")}</TableHead>
                <TableHead scope="col">
                  <span className="sr-only">
                    {t("office.inbox.col_action")}
                  </span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((item) => (
                <TableRow
                  key={`${item.type}:${item.uuid}`}
                  data-type={item.type}
                >
                  <TableCell>
                    <StatusPill tone={TYPE_TONE[item.type]}>
                      {typeLabel(item.type)}
                    </StatusPill>
                  </TableCell>
                  {scope ? null : (
                    <TableCell className="font-medium">
                      {item.taxpayer_title}
                    </TableCell>
                  )}
                  <TableCell className="max-w-80">
                    <span className="block truncate">
                      {item.description || "—"}
                    </span>
                    {item.reference ? (
                      <span className="text-text-3 block truncate font-mono text-xs">
                        {item.reference}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {item.amount
                      ? formatMoney(item.amount, item.currency)
                      : "—"}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {item.date
                      ? formatDate(item.date, "dd.MM.yyyy", locale)
                      : "—"}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1.5">
                      {item.can_approve && canApprove ? (
                        <Button
                          size="sm"
                          variant="outline"
                          className="max-md:min-h-11"
                          disabled={approve.isPending}
                          onClick={() => onApprove(item)}
                        >
                          <Check className="size-4" aria-hidden />
                          {t("office.inbox.approve")}
                        </Button>
                      ) : null}
                      <Button
                        asChild
                        size="sm"
                        variant="secondary"
                        className="max-md:min-h-11"
                      >
                        <Link href={targetHref(item)}>
                          {t("office.inbox.act")}
                          <ArrowRight className="size-4" aria-hidden />
                        </Link>
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {page > 0 || data.next_cursor ? (
            <div className="flex items-center justify-end gap-2 text-sm">
              <Button
                size="sm"
                variant="ghost"
                disabled={page === 0}
                onClick={() => setPage((p) => Math.max(0, p - 1))}
              >
                {t("office.inbox.prev")}
              </Button>
              <span className="tabular-nums">
                {t("office.inbox.page", { page: page + 1 })}
              </span>
              <Button
                size="sm"
                variant="ghost"
                disabled={!data.next_cursor}
                onClick={() => setPage((p) => p + 1)}
              >
                {t("office.inbox.next")}
              </Button>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}

/** Komuta Merkezi ve başka ekranlardan gelen kutusuna bağlantı. */
export function InboxLink({ period }: { period?: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <Button asChild size="sm" variant="outline">
      <Link href={routes.tenant.inbox(slug, { period })}>
        <Inbox className="size-4" aria-hidden />
        {t("office.inbox.open")}
      </Link>
    </Button>
  );
}
