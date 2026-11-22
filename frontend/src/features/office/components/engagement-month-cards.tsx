"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Download,
  FileSpreadsheet,
  FileText,
  Landmark,
  Loader2,
  Receipt,
  Wallet,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { EXPORT_ANCHOR, routes } from "@/config/routes";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useJournalActions } from "../hooks/use-journals";
import { useEngagementInvoices } from "../hooks/use-office-invoices";
import { journalErrorText } from "../lib/journal-error";
import { PERIOD_PARAM, useOfficePeriod, usePeriodText } from "../period";
import { journalService } from "../services/journals.service";
import {
  receiptKeys,
  receiptsService,
  type ExpensePage,
  type RetailSalePage,
} from "../services/receipts.service";
import type { PeriodMatrixCell } from "../services/periods.service";
import { useEngagementTabs } from "./engagement-layout";
import { useLedgerType } from "./ledger-type";

type BankSummary = {
  summary: { total: number; matched: number; unmatched: number };
};

/** Liste adresine dönemi ekler (`?period=YYYY-MM`). */
function withPeriod(href: string, period: string, anchor?: string) {
  const sep = href.includes("?") ? "&" : "?";
  return `${href}${sep}${PERIOD_PARAM}=${encodeURIComponent(period)}${
    anchor ? `#${anchor}` : ""
  }`;
}

type CardProps = {
  kind: "invoices" | "zreports" | "expenses" | "bank";
  icon: LucideIcon;
  title: string;
  href: string;
  period: string;
  count: number | undefined;
  loading: boolean;
  /** Durum satırı: kaç onay bekliyor, kaç işlendi. */
  status: string;
  tone?: "warn" | "ok";
};

function MonthCard({
  kind,
  icon: Icon,
  title,
  href,
  period,
  count,
  loading,
  status,
  tone,
}: CardProps) {
  const { setPeriod } = useOfficePeriod();
  return (
    <li>
      <Link
        href={withPeriod(href, period)}
        // Liste de aynı aya süzülsün: global dönem seçici bu aya gelir.
        onClick={() => setPeriod(period)}
        data-testid={`month-card-${kind}`}
        className="bg-surface border-line hover:border-brand focus-visible:ring-brand flex h-full min-h-32 flex-col gap-2 rounded-xl border p-4 outline-none focus-visible:ring-2"
      >
        <span className="text-text-2 flex items-center gap-2 text-sm font-semibold">
          <Icon className="size-4" aria-hidden />
          {title}
        </span>
        {loading ? (
          <Skeleton className="h-8 w-16" />
        ) : (
          <span
            data-testid="month-card-count"
            className="font-display text-3xl font-bold tabular-nums"
          >
            {count ?? "–"}
          </span>
        )}
        <span
          className={cn(
            "text-sm",
            tone === "warn" && "text-warn font-semibold",
            tone === "ok" && "text-ok",
            !tone && "text-text-2",
          )}
        >
          {status}
        </span>
      </Link>
    </li>
  );
}

/**
 * Mükellef özetinin üstü (Oğuz'un demo geri bildirimi): dönemin dört ana
 * işi — Fatura, Z raporu, Masraf, Banka — sayı ve durumla; karta tıklayınca
 * o ayın listesi açılır. Yanında "Luca'ya aktar". Mevcut liste uçları
 * (`limit=1`) ve dönem matrisinin bekleyen sayıları kullanılır.
 */
export function EngagementMonthCards({
  engagement,
  period,
  cell,
}: {
  engagement: string;
  period: string;
  cell: PeriodMatrixCell | undefined;
}) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canInvoices = can(Permission.TenantInvoicesRead);
  const canReceipts = can(Permission.TenantReceiptsRead);
  const canBank = can(Permission.TenantBankRead);

  const invoices = useEngagementInvoices(slug, canInvoices ? engagement : "", {
    period,
    limit: 1,
    offset: 0,
  });
  const incoming = useEngagementInvoices(slug, canInvoices ? engagement : "", {
    period,
    direction: "incoming",
    limit: 1,
    offset: 0,
  });
  const expenses = useQuery({
    queryKey: [
      ...receiptKeys.expenses(slug, engagement),
      "month-summary",
      period,
    ],
    queryFn: (): Promise<ExpensePage> =>
      receiptsService.expenses(slug, engagement, {
        period,
        limit: 1,
        offset: 0,
      }),
    enabled: canReceipts && !!period,
  });
  const zreports = useQuery({
    queryKey: [
      ...receiptKeys.retail(slug, engagement),
      "month-summary",
      period,
    ],
    queryFn: (): Promise<RetailSalePage> =>
      receiptsService.retailSales(slug, engagement, {
        period,
        limit: 1,
        offset: 0,
      }),
    enabled: canReceipts && !!period,
  });
  const bank = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "bank-transactions",
      engagement,
      "month-summary",
      period,
    ],
    queryFn: () =>
      tenantRequest<BankSummary>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-transactions`,
        { query: { period, limit: 1, offset: 0 } },
      ),
    enabled: canBank && !!period,
  });

  const invTotal = invoices.data?.total;
  const invIn = incoming.data?.total;
  // Yüklenemeyen sayaç "0" yazıp yanıltmaz: "–" ve hata metni.
  const failed = t("office.engagement.month.error");
  const invoiceStatus =
    invoices.isError || incoming.isError
      ? failed
      : invTotal === undefined || invIn === undefined
        ? t("office.engagement.month.loading")
        : cell?.missing_purchase_invoices
          ? t("office.engagement.month.invoices_missing")
          : t("office.engagement.month.invoices_status", {
              incoming: invIn,
              outgoing: Math.max(invTotal - invIn, 0),
            });

  const receiptStatus = (q: {
    isError: boolean;
    data?: { total: number; pending: number };
  }) => {
    const page = q.data;
    return q.isError
      ? failed
      : !page
        ? t("office.engagement.month.loading")
        : page.total === 0
          ? t("office.engagement.month.none")
          : page.pending > 0
            ? t("office.engagement.month.pending_status", {
                pending: page.pending,
                done: page.total - page.pending,
              })
            : t("office.engagement.month.all_done");
  };

  const bankWaiting = cell?.pending.bank_transactions ?? 0;
  const bankMissing = cell?.missing_bank_statements.length ?? 0;
  const bankTotal = bank.data?.summary.total;
  const bankStatus = bank.isError
    ? failed
    : bankTotal === undefined
      ? t("office.engagement.month.loading")
      : bankMissing > 0
        ? t("office.engagement.month.bank_missing", { count: bankMissing })
        : bankTotal === 0
          ? t("office.engagement.month.none")
          : bankWaiting > 0
            ? t("office.engagement.month.bank_status", {
                pending: bankWaiting,
                done: bank.data?.summary.matched ?? 0,
              })
            : t("office.engagement.month.all_done");

  const cards: CardProps[] = [];
  if (canInvoices)
    cards.push({
      kind: "invoices",
      icon: FileText,
      title: t("office.engagement.month.invoices"),
      href: routes.tenant.engagementInvoices(slug, engagement),
      period,
      count: invTotal,
      loading: invoices.isLoading,
      status: invoiceStatus,
      tone: cell?.missing_purchase_invoices ? "warn" : undefined,
    });
  if (canReceipts) {
    cards.push({
      kind: "zreports",
      icon: Receipt,
      title: t("office.engagement.month.zreports"),
      href: routes.tenant.engagementZReports(slug, engagement),
      period,
      count: zreports.data?.total,
      loading: zreports.isLoading,
      status: cell?.missing_z_reports
        ? t("office.engagement.month.zreports_missing")
        : receiptStatus(zreports),
      tone:
        cell?.missing_z_reports || (zreports.data?.pending ?? 0) > 0
          ? "warn"
          : undefined,
    });
    cards.push({
      kind: "expenses",
      icon: Wallet,
      title: t("office.engagement.month.expenses"),
      href: routes.tenant.engagementExpenses(slug, engagement),
      period,
      count: expenses.data?.total,
      loading: expenses.isLoading,
      status: cell?.missing_expenses
        ? t("office.engagement.month.expenses_missing")
        : receiptStatus(expenses),
      tone:
        cell?.missing_expenses || (expenses.data?.pending ?? 0) > 0
          ? "warn"
          : undefined,
    });
  }
  if (canBank)
    cards.push({
      kind: "bank",
      icon: Landmark,
      title: t("office.engagement.month.bank"),
      href: routes.tenant.engagementBank(slug, engagement),
      period,
      count: bankTotal,
      loading: bank.isLoading,
      status: bankStatus,
      tone: bankMissing > 0 || bankWaiting > 0 ? "warn" : undefined,
    });

  return (
    <section
      aria-labelledby="month-summary-title"
      data-testid="month-summary"
      className="space-y-3"
    >
      <div>
        <h2
          id="month-summary-title"
          className="font-display text-base font-bold"
        >
          {t("office.engagement.month.title", { period: periodText(period) })}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.engagement.month.description")}
        </p>
      </div>
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {cards.map((c) => (
            <MonthCard key={c.kind} {...c} />
          ))}
        </ul>
        <LucaExportCard engagement={engagement} period={period} />
      </div>
    </section>
  );
}

/**
 * "Luca'ya aktar": dönemin onaylı fişlerinden mevcut Luca fiş dosyası
 * üretilir ve iner (yevmiye aktarımıyla aynı uç). Luca'nın API'si yok;
 * müşavir dosyayı Luca'ya kendisi yükler. Uyarılı ya da çift kayıt şüpheli
 * belge varsa Fişler sayfasındaki ayrıntılı aktarıma yönlendirir. İşletme
 * defteri / serbest meslekte yevmiye fişi olmadığı için nedeni yazılır.
 */
export function LucaExportCard({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { singleEntry } = useLedgerType(engagement);
  const { export: run, check } = useJournalActions(slug, engagement);
  const canExport = can(Permission.TenantJournalsExport);
  const journalsHref = withPeriod(
    routes.tenant.engagementJournals(slug, engagement),
    period,
    EXPORT_ANCHOR,
  );

  const download = (batch: string, files: { name: string }[]) => {
    for (const f of files)
      void journalService
        .download(slug, engagement, batch, f.name)
        .catch(() => undefined);
  };

  const start = () => {
    run.reset();
    check.mutate(
      { driver: "luca", period },
      {
        onSuccess: (res) => {
          if (res.blocked || res.excluded.length) return;
          run.mutate(
            { driver: "luca", period },
            { onSuccess: (b) => download(b.uuid, b.files) },
          );
        },
      },
    );
  };

  const pending = run.isPending || check.isPending;
  const c = check.data;
  const review =
    c && !run.data && (c.blocked || c.excluded.length > 0) ? c : undefined;
  const err = run.error ?? check.error;
  const blockedErr = isApiError(err) && err.code === "EXPORT_BLOCKED";

  return (
    <section
      aria-labelledby="luca-export-title"
      data-testid="luca-export"
      className="bg-surface border-line flex flex-col gap-2 rounded-xl border p-4"
    >
      <h3 id="luca-export-title" className="font-display text-sm font-bold">
        {t("office.engagement.luca.title")}
      </h3>
      {singleEntry ? (
        <>
          <p className="text-text-2 text-sm" data-testid="luca-single-entry">
            {t("office.engagement.luca.single_entry")}
          </p>
          <Link
            href={routes.tenant.engagementOperatingAccount(slug, engagement)}
            className="text-brand text-sm font-semibold underline-offset-2 hover:underline"
          >
            {t("office.engagement.luca.operating_account")}
          </Link>
        </>
      ) : (
        <>
          <p className="text-text-2 text-sm" data-testid="luca-hint">
            {t("office.engagement.luca.hint")}
          </p>
          {canExport ? (
            <Button
              size="sm"
              className="self-start"
              disabled={pending || !period}
              onClick={start}
            >
              {pending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <FileSpreadsheet className="size-4" aria-hidden />
              )}
              {t("office.engagement.luca.button")}
            </Button>
          ) : (
            <p className="text-text-3 text-xs">
              {t("office.engagement.luca.no_permission")}
            </p>
          )}
          {review ? (
            <p className="text-warn text-sm" role="status">
              {review.blocked && review.entries === 0
                ? t("office.engagement.luca.nothing")
                : t("office.engagement.luca.review")}{" "}
              <Link
                href={journalsHref}
                className="text-brand font-semibold underline-offset-2 hover:underline"
              >
                {t("office.engagement.luca.open_journals")}
              </Link>
            </p>
          ) : null}
          {err ? (
            <p className="text-danger text-sm" role="alert">
              {blockedErr
                ? t("office.engagement.luca.review")
                : journalErrorText(
                    t,
                    err,
                    t("office.journals.action_error"),
                  )}{" "}
              <Link
                href={journalsHref}
                className="text-brand font-semibold underline-offset-2 hover:underline"
              >
                {t("office.engagement.luca.open_journals")}
              </Link>
            </p>
          ) : null}
          {run.data ? (
            <div className="space-y-1 text-sm" role="status">
              <p className="text-ok">
                {t("office.engagement.luca.done", {
                  count: run.data.entries,
                })}
              </p>
              <ul className="space-y-1">
                {run.data.files.map((f) => (
                  <li key={f.name}>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        download(run.data.uuid, [{ name: f.name }])
                      }
                    >
                      <Download className="size-4" aria-hidden />
                      {f.name}
                    </Button>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}

/**
 * Dört kartın altında mükellefin diğer bölümleri (Excel aktarımı, geçmiş
 * dönem, beyannameler, dilekçeler...): sekmelerle aynı liste ve aynı izin
 * kuralı, kısa açıklamayla. Sekmeler yerinde kalır.
 */
export function EngagementOtherSections({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const tabs = useEngagementTabs(engagement);
  const items = tabs
    .filter((x) => !x.exact)
    .map((x) => ({
      ...x,
      segment: (x.href.split(/[?#]/)[0] ?? "").split("/").pop() ?? "",
    }))
    .filter((x) => !CARD_SEGMENTS.has(x.segment));
  if (!items.length) return null;
  return (
    <nav
      aria-labelledby="other-sections-title"
      data-testid="other-sections"
      className="space-y-3"
    >
      <h2
        id="other-sections-title"
        className="font-display text-base font-bold"
      >
        {t("office.engagement.sections.title")}
      </h2>
      <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {items.map((x) => {
          const key = `office.engagement.sections.hint_${x.segment.replace(/-/g, "_")}`;
          const hint = t(key);
          return (
            <li key={x.href}>
              <Link
                href={x.href}
                className="bg-surface border-line hover:border-brand focus-visible:ring-brand flex h-full flex-col gap-0.5 rounded-lg border px-3 py-2.5 outline-none focus-visible:ring-2"
              >
                <span className="text-sm font-semibold">{x.label}</span>
                {hint !== key ? (
                  <span className="text-text-3 text-xs">{hint}</span>
                ) : null}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

/** Dört kartın açtığı listeler; "Diğer bölümler"de tekrarlanmaz. */
const CARD_SEGMENTS = new Set(["invoices", "z-reports", "expenses", "bank"]);
