"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Suspense, type KeyboardEvent, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/common/error-state";
import { TacRozeti, type TacDurumu } from "@/components/hakan";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { ENGAGEMENT_MONTHS } from "../lib/ledger-type";
import { visibleTabs } from "../lib/visible-tabs";

import { EngagementScopeProvider } from "../engagement-scope";

import { ChangeRequestsPanel } from "./change-requests-panel";
import { LedgerBadge, useLedgerType } from "./ledger-type";
import { usePeriodMatrix } from "../hooks/use-office-data";
import { tabState } from "../lib/engagement-nav";
import { FocusedRecord } from "./related-records";
import { LatePeriodNote } from "./late-documents";
import { LockedPeriodNote } from "./locked-period-note";
import { ALL_PERIODS, periodLabel, useOfficePeriod } from "../period";
import type { PeriodMatrixRow } from "../services/periods.service";

type Cell = PeriodMatrixRow["cells"][number];
/** Dönem şeridindeki ay sayısı (genel bakışla aynı sorgu, önbellekten). */
export { ENGAGEMENT_MONTHS };

/** Dönemde işlenmemiş kayıt sayısı (#575). */
export function pendingTotal(cell: Cell): number {
  const p = cell.pending;
  return p ? p.bank_transactions + p.expenses + p.retail_sales : 0;
}

/** Dönem hücresi → taç ve etiket (veri · onay · aktarım). */
export function periodCrown(cell: Cell): {
  durum: TacDurumu;
  key: string;
} {
  if (cell.locked) return { durum: "aktarildi", key: "locked" };
  if (cell.open_blockers > 0) return { durum: "sorun", key: "blocked" };
  // İşlenmemiş banka hareketi, fiş ya da Z raporu varsa hazır değil (#575).
  if (pendingTotal(cell) > 0) return { durum: "yuklendi", key: "pending" };
  if (cell.ready) return { durum: "onaylandi", key: "ready" };
  if (cell.open_change_requests > 0)
    return { durum: "yuklendi", key: "changes" };
  return { durum: "bekleniyor", key: "open" };
}

function initials(title: string) {
  const parts = title
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toLocaleUpperCase(
    "tr-TR",
  );
}

/** Dönem kapalı: kilitli ya da bir beyanı verilmiş (#780 C). */
function isClosed(cell: Cell | undefined): boolean {
  if (!cell) return false;
  return cell.locked || cell.returns.some((r) => r.status === "filed");
}

type EngagementTab = { href: string; label: string; exact?: boolean };

/** Mükellef ayrıntısının sekmeleri (tuval OfisMukellefler). */
export function useEngagementTabs(engagement: string): EngagementTab[] {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  // İşletme defteri / serbest meslek (#780): mizan ve fiş yerine işletme
  // hesabı.
  const { singleEntry } = useLedgerType(engagement);
  // Her sekme ana listesinin okuma iznine bağlı (#382, #390, #518 E2):
  // izni olmayan rol sekmeyi görmez, boş ya da hatalı ekrana düşmez.
  const tabs: (EngagementTab & {
    permission?: string;
    balanceOnly?: boolean;
    singleEntryOnly?: boolean;
  })[] = [
    {
      href: routes.tenant.engagement(slug, engagement),
      label: t("office.engagement.tab_overview"),
      exact: true,
    },
    {
      href: routes.tenant.engagementInvoices(slug, engagement),
      label: t("office.nav_invoices"),
      permission: Permission.TenantInvoicesRead,
    },
    {
      href: routes.tenant.engagementWarnings(slug, engagement),
      label: t("office.engagement.tab_warnings"),
      permission: Permission.TenantWarningsRead,
    },
    {
      href: routes.tenant.engagementReview(slug, engagement),
      label: t("office.engagement.tab_review"),
      permission: Permission.TenantReviewRead,
    },
    {
      href: routes.tenant.engagementBank(slug, engagement),
      label: t("office.engagement.tab_bank"),
      permission: Permission.TenantBankRead,
    },
    // Maaş verisi hassas: izni olmayan (stajyer) sekmeyi görmez (#382).
    {
      href: routes.tenant.engagementPayroll(slug, engagement),
      label: t("office.engagement.tab_payroll"),
      permission: Permission.TenantPayrollRead,
    },
    {
      href: routes.tenant.engagementEasytrade(slug, engagement),
      label: t("sources.easytrade.tab"),
    },
    {
      href: routes.tenant.engagementExpenses(slug, engagement),
      label: t("office.engagement.tab_expenses"),
      permission: Permission.TenantReceiptsRead,
    },
    {
      href: routes.tenant.engagementZReports(slug, engagement),
      label: t("office.engagement.tab_zreports"),
      permission: Permission.TenantReceiptsRead,
    },
    {
      href: routes.tenant.engagementReconciliation(slug, engagement),
      label: t("office.engagement.tab_reconciliation"),
      permission: Permission.TenantReconciliationRead,
    },
    // İşletme hesabı: gelir / gider defteri (#780).
    {
      href: routes.tenant.engagementOperatingAccount(slug, engagement),
      label: t("office.engagement.tab_operating_account"),
      permission: Permission.TenantInvoicesRead,
      singleEntryOnly: true,
    },
    // Mükellef mizanı (#669 E).
    {
      href: routes.tenant.engagementTrialBalance(slug, engagement),
      label: t("office.engagement.tab_trial_balance"),
      permission: Permission.TenantAccountPlanRead,
      balanceOnly: true,
    },
    {
      href: routes.tenant.engagementJournals(slug, engagement),
      label: t("office.engagement.tab_journals"),
      permission: Permission.TenantJournalsRead,
      balanceOnly: true,
    },
    {
      href: routes.tenant.engagementDocuments(slug, engagement),
      label: t("office.engagement.tab_documents"),
      permission: Permission.TenantDocumentsRead,
    },
    {
      href: routes.tenant.engagementArchive(slug, engagement),
      label: t("office.engagement.tab_archive"),
      permission: Permission.TenantDocumentsRead,
    },
    {
      href: routes.tenant.engagementUpload(slug, engagement),
      label: t("office.engagement.tab_upload"),
    },
    // Genel Excel / CSV içe aktarma (#518 B1).
    {
      href: routes.tenant.engagementExcelImports(slug, engagement),
      label: t("office.engagement.tab_excel"),
      permission: Permission.TenantExcelImportsRead,
    },
    // Geçmiş dönem takibi (#780 C).
    {
      href: routes.tenant.engagementLateDocuments(slug, engagement),
      label: t("office.engagement.tab_late_documents"),
      permission: Permission.TenantPeriodsRead,
    },
    // MUHSGK bordrodan beslenir: izni olmayan (stajyer) sekmeyi görmez (#390).
    {
      href: routes.tenant.engagementTaxReturns(slug, engagement),
      label: t("office.engagement.tab_tax_returns"),
      permission: Permission.TenantTaxReturnsRead,
    },
    {
      href: routes.tenant.engagementPetitions(slug, engagement),
      label: t("office.engagement.tab_petitions"),
    },
    {
      href: routes.tenant.engagementReport(slug, engagement),
      label: t("office.engagement.tab_report"),
    },
    // İletişim geçmişi (#780 D): "bana söylemediniz" diyene gösterilecek kanıt.
    {
      href: routes.tenant.engagementCommunications(slug, engagement),
      label: t("office.engagement.tab_communications"),
    },
    // Banka, entegratör ve diğer dış bağlantılar (#518).
    {
      href: routes.tenant.engagementConnections(slug, engagement),
      label: t("office.engagement.tab_connections"),
    },
    // KVKK ilgili kişi talepleri (#518 B3).
    {
      href: routes.tenant.engagementPrivacy(slug, engagement),
      label: t("office.engagement.tab_privacy"),
      permission: Permission.TenantPrivacyRequest,
    },
  ];
  return visibleTabs(
    tabs.filter((x) => (singleEntry ? !x.balanceOnly : !x.singleEntryOnly)),
    can,
  );
}

/** Ok tuşları, Home ve End sekmeler arasında odağı taşır. */
function onTabsKeyDown(e: KeyboardEvent<HTMLElement>) {
  const links = Array.from(
    e.currentTarget.querySelectorAll<HTMLAnchorElement>("a[href]"),
  );
  const i = links.indexOf(document.activeElement as HTMLAnchorElement);
  if (i < 0) return;
  const next =
    e.key === "ArrowRight"
      ? (i + 1) % links.length
      : e.key === "ArrowLeft"
        ? (i - 1 + links.length) % links.length
        : e.key === "Home"
          ? 0
          : e.key === "End"
            ? links.length - 1
            : -1;
  if (next < 0) return;
  e.preventDefault();
  links[next]?.focus();
}

export function EngagementTabs({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const pathname = usePathname();
  const tabs = useEngagementTabs(engagement);
  return (
    <nav
      aria-label={t("office.engagement.tabs")}
      onKeyDown={onTabsKeyDown}
      className="border-line flex gap-1 overflow-x-auto border-b"
      data-testid="engagement-tabs"
    >
      {tabs.map((tab) => {
        const state = tabState(pathname, tab);
        return (
          <Link
            key={tab.href}
            href={tab.href}
            aria-current={state}
            className={cn(
              "focus-visible:ring-ring relative flex min-h-11 items-center px-3 text-sm whitespace-nowrap focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset",
              "after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full",
              state
                ? "text-foreground after:bg-brand font-semibold"
                : "text-text-2 hover:text-foreground",
            )}
          >
            {tab.label}
          </Link>
        );
      })}
    </nav>
  );
}

/**
 * Dönem şeridi: son altı dönem, her biri bir taç (veri · onay · aktarım).
 * Bir döneme basınca ofisin global dönemi o olur; seçili dönem vurgulu.
 */
/**
 * Dönem şeridi. "Tüm dönemler" seçiliyken hiçbir ay seçili görünmez (#710):
 * eskiden beyan ayı seçili gibi duruyor, ama sekmeler bütün dönemleri
 * listeliyordu (şeritte "3 açık uyarı", listede 17). Seçim satırı ne
 * gösterildiğini söyler ve seçim tek tıkla kaldırılır.
 */
function PeriodStrip({
  cells,
  selected,
  onPick,
  onAll,
}: {
  cells: Cell[];
  selected: string | undefined;
  onPick: (period: string) => void;
  onAll: () => void;
}) {
  const { t, locale } = useLocale();
  const month = (p: string) =>
    new Intl.DateTimeFormat(locale, { month: "short", year: "2-digit" }).format(
      new Date(`${p}-01T00:00:00`),
    );
  return (
    <section
      aria-labelledby="period-strip-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
    >
      <div className="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2
          id="period-strip-title"
          className="font-display text-base font-bold"
        >
          {t("office.engagement.strip_title")}
        </h2>
        <span className="text-text-3 text-xs">
          {t("office.engagement.strip_hint")}
        </span>
        <span
          className="text-text-2 ms-auto flex items-center gap-2 text-xs"
          data-testid="period-strip-scope"
        >
          {selected ? (
            <>
              {t("office.engagement.strip_scope_one", {
                period: periodLabel(selected, locale),
              })}
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 max-md:min-h-11"
                onClick={onAll}
              >
                {t("office.engagement.strip_show_all")}
              </Button>
            </>
          ) : (
            t("office.engagement.strip_scope_all")
          )}
        </span>
      </div>
      <ol className="grid grid-cols-3 gap-2 sm:grid-cols-6">
        {cells.map((cell) => {
          const { durum, key } = periodCrown(cell);
          const on = cell.period === selected;
          const label = t(`office.engagement.state_${key}`);
          return (
            <li key={cell.period}>
              <Button
                variant="ghost"
                aria-pressed={on}
                onClick={() => onPick(cell.period)}
                className={cn(
                  "h-auto min-h-11 w-full flex-col gap-2 rounded-xl px-1.5 py-3 font-normal whitespace-normal transition-colors",
                  on
                    ? "bg-brand-tint hover:bg-brand-tint outline-brand outline-[1.5px]"
                    : "hover:bg-surface-2",
                )}
              >
                <span className="font-mono text-xs tabular-nums">
                  {month(cell.period)}
                </span>
                {/* Ad görünür metinden gelir (ay · durum); rozet tekrar okunmaz. */}
                <span aria-hidden>
                  <TacRozeti durum={durum} boyut={30} />
                </span>
                <span
                  className={cn(
                    "text-center text-xs leading-tight",
                    key === "blocked"
                      ? "text-danger font-semibold"
                      : "text-text-2",
                  )}
                >
                  {label}
                  {cell.open_blockers ? ` · ${cell.open_blockers}` : ""}
                </span>
                {/* Dönemde kalan açık uyarı (engelleyenler dâhil, #529). */}
                {cell.open_warnings > 0 ? (
                  <span className="text-text-3 text-center text-[11px] leading-tight tabular-nums">
                    {t("office.engagement.strip_open_warnings", {
                      count: cell.open_warnings,
                    })}
                  </span>
                ) : null}
              </Button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

/**
 * Mükellef detayının ortak düzeni (#468): başlık (unvan, VKN), sekme çubuğu
 * ve dönem şeridi her alt sayfada tek kaynaktan çizilir; alt sayfalar
 * mükellef bağlamını (`useEngagementScope`) görür.
 */
export function EngagementLayout({
  engagement,
  children,
}: {
  engagement: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const { focus, period: selected, setPeriod } = useOfficePeriod();
  const matrix = usePeriodMatrix(ENGAGEMENT_MONTHS);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  // Matris yeniden eskiye gelir; şerit soldan sağa eskiden yeniye.
  const cells = [...(row?.cells ?? [])].sort((a, b) =>
    a.period.localeCompare(b.period),
  );
  const period = (cells.find((c) => c.period === focus) ?? cells.at(-1))
    ?.period;

  if (matrix.isLoading) {
    return (
      <div className="space-y-5">
        <Skeleton className="h-12 w-80" />
        <EngagementTabs engagement={engagement} />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  // Yükleme hatası "mükellef bulunamadı" değildir (#710): yeniden denenir.
  if (matrix.isError) {
    return (
      <ErrorState
        title={t("office.api.load_error")}
        onRetry={() => void matrix.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (!row) {
    return (
      <ErrorState
        title={t("office.engagement.not_found")}
        description={t("office.engagement.not_found_hint")}
      />
    );
  }

  return (
    <EngagementScopeProvider engagement={engagement}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-4">
          <span
            aria-hidden
            className="bg-brand text-on-brand flex size-12 items-center justify-center rounded-xl text-base font-bold"
          >
            {initials(row.taxpayer_title)}
          </span>
          <div className="min-w-0 flex-1">
            <h1 className="font-display truncate text-2xl font-bold">
              {row.taxpayer_title}
            </h1>
            <p className="text-text-2 flex flex-wrap items-center gap-2 text-sm">
              <span>
                {row.taxpayer_tax_id.length === 11 ? "TCKN" : "VKN"}{" "}
                <span className="font-mono">{row.taxpayer_tax_id}</span>
              </span>
              {/* Defter türü (#780). */}
              <LedgerBadge ledgerType={row.ledger_type ?? "bilanco"} />
            </p>
          </div>
        </div>
        <EngagementTabs engagement={engagement} />
        <PeriodStrip
          cells={cells}
          selected={selected}
          onPick={setPeriod}
          onAll={() => setPeriod(ALL_PERIODS)}
        />
        {period && cells.find((c) => c.period === period)?.locked ? (
          <LockedPeriodNote engagement={engagement} period={period} />
        ) : null}
        {period ? (
          <LatePeriodNote
            engagement={engagement}
            period={period}
            closed={isClosed(cells.find((c) => c.period === period))}
          />
        ) : null}
        {/* Panel ?talepler=ac'yi arama parametrelerinden okur (#710). */}
        <Suspense fallback={null}>
          <ChangeRequestsPanel
            engagement={engagement}
            open={cells.reduce((n, c) => n + (c.open_change_requests ?? 0), 0)}
          />
        </Suspense>
        {/* Seçili kayıt (?kayit=tür:uuid) ve ilişkili kayıtları (#469). */}
        <Suspense fallback={null}>
          <FocusedRecord engagement={engagement} />
        </Suspense>
        {children}
      </div>
    </EngagementScopeProvider>
  );
}
