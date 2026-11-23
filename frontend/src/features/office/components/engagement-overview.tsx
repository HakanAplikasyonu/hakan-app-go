"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { Skeleton } from "@/components/ui/skeleton";
import { EngagementNextWork } from "@/features/copilot";
import { DOC_TASK_PARAM, DOC_TASKS_ANCHOR, routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import { useEngagementInvoices } from "../hooks/use-office-invoices";
import {
  usePendingDocTasks,
  type PendingDocTask,
} from "../hooks/use-pending-doc-tasks";
import { useWarningGroups } from "../hooks/use-warnings";
import { useOfficePeriod, usePeriodText } from "../period";
import { BankAccountsSection } from "./bank-accounts-section";
import { DocRequestForm, DocTaskChannels } from "./doc-request-channels";
import { DocTaskSubmission } from "./doc-task-submission";
import { EngagementMissingDocuments } from "./document-collection";
import { ENGAGEMENT_MONTHS, periodCrown } from "./engagement-layout";
import {
  EngagementMonthCards,
  EngagementOtherSections,
} from "./engagement-month-cards";
import { ImportantDocumentsSection } from "./important-documents-section";
import { LedgerTypeCard } from "./ledger-type";
import { TaxpayerInvitationsSection } from "./taxpayer-invitations-section";

function Stat({
  label,
  value,
  tone,
  href,
}: {
  label: string;
  value: ReactNode;
  tone?: "danger" | "ok";
  href?: string;
}) {
  const body = (
    <>
      <span className="text-text-2 text-sm">{label}</span>
      <span
        className={cn(
          "font-mono text-sm font-semibold tabular-nums",
          tone === "danger" && "text-danger",
          tone === "ok" && "text-ok",
        )}
      >
        {value}
      </span>
    </>
  );
  return (
    <li>
      {href ? (
        <Link
          href={href}
          className="hover:bg-surface-2 flex min-h-11 items-center justify-between gap-3 rounded-md px-2 py-1.5"
        >
          {body}
        </Link>
      ) : (
        <div className="flex min-h-11 items-center justify-between gap-3 px-2 py-1.5">
          {body}
        </div>
      )}
    </li>
  );
}

type DocTask = PendingDocTask;

/**
 * `/t/{slug}/engagements/{uuid}` (arayüz 6/9, #329; tuval OfisMukellefler):
 * başlık, sekmeler, dönem şeridi; üstte dönemin dört kartı (Fatura, Z raporu,
 * Masraf, Banka) ve "Luca'ya aktar", altında diğer bölümler, seçili dönemin
 * hazırlığı ve açık belge istekleri.
 */
export function EngagementOverviewPage({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const periodText = usePeriodText();
  const { slug } = useTenant();
  const { focus } = useOfficePeriod();
  // Başlık, sekmeler ve dönem şeridi düzende (#468); aynı sorgu önbellekten.
  const matrix = usePeriodMatrix(ENGAGEMENT_MONTHS);
  const row = matrix.data?.items.find((r) => r.engagement_uuid === engagement);
  // Matris yeniden eskiye gelir; şerit soldan sağa eskiden yeniye.
  const cells = [...(row?.cells ?? [])].sort((a, b) =>
    a.period.localeCompare(b.period),
  );
  const cell = cells.find((c) => c.period === focus) ?? cells.at(-1);
  const period = cell?.period ?? focus;

  const invoices = useEngagementInvoices(slug, engagement, {
    period,
    limit: 1,
    offset: 0,
  });
  const warnings = useWarningGroups(
    slug,
    { period, status: "open" },
    engagement,
  );
  // Seçili dönemin bekleyen istekleri: açık olanlar ve mükellefin gönderip
  // kontrol bekleyenleri. "Sıradaki iş" kartı da aynı ikisini sayar (#563).
  // Kartla aynı ay (focus): seçili ay şeritte yoksa liste başka ayı
  // gösterip kartla çelişmesin; açılan istek de listede bulunsun.
  const tasks = usePendingDocTasks(engagement, focus);
  // `?istek=<uuid>`: Sıradaki iş ya da belge toplamadan gelinen istek
  // vurgulanır ve görünür olur.
  const params = useSearchParams();
  const selectedTask = params.get(DOC_TASK_PARAM);
  const taskRows = tasks.data;
  // Liste kimlikleri değişince bir kez kaydırılır (select her çizimde yeni
  // dizi verir).
  const taskIds = taskRows?.map((x) => x.uuid).join(",");
  useEffect(() => {
    if (!selectedTask || !taskIds) return;
    document
      .getElementById(`istek-${selectedTask}`)
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [selectedTask, taskIds]);
  const selectedMissing =
    !!selectedTask &&
    !!taskRows &&
    !taskRows.some((x) => x.uuid === selectedTask);
  const openWarnings = (warnings.data?.items ?? []).reduce(
    (n, g) => n + g.count,
    0,
  );

  if (matrix.isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-16 w-80" />
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

  const taskTitle = (x: DocTask) =>
    x.kind === "undocumented_payment" || x.kind === "uninvoiced_collection"
      ? t(`office.engagement.task_${x.kind}`)
      : x.title || t("office.engagement.task_generic");

  return (
    <div className="space-y-5">
      {/* Üstte dönemin dört ana işi ve "Luca'ya aktar" (Oğuz'un demo geri
          bildirimi); altında diğer bölümler. */}
      <EngagementMonthCards
        engagement={engagement}
        period={period}
        cell={cell}
      />
      <EngagementOtherSections engagement={engagement} />

      <EngagementNextWork engagement={engagement} variant="card" />
      {/* Mükellef ayarı: defter türü (#780). */}
      <LedgerTypeCard engagement={engagement} />

      {/* Ay başı belge toplama (#780): dönemin eksik belgeleri. */}
      <EngagementMissingDocuments engagement={engagement} cell={cell} />

      <div className="grid gap-5 lg:grid-cols-2">
        <section
          aria-labelledby="readiness-title"
          className="bg-surface border-line rounded-xl border p-4 md:p-5"
        >
          <h2
            id="readiness-title"
            className="font-display mb-2 text-base font-bold"
          >
            {t("office.engagement.readiness_title", {
              period: periodText(period),
            })}
          </h2>
          {cell ? (
            <p
              className={cn(
                "mb-2 text-sm font-semibold",
                cell.open_blockers ? "text-danger" : "text-text-2",
              )}
            >
              {t(`office.engagement.state_${periodCrown(cell).key}`)}
            </p>
          ) : null}
          <ul className="divide-line divide-y">
            <Stat
              label={t("office.engagement.stat_invoices")}
              value={
                invoices.isLoading || invoices.isError
                  ? "–"
                  : (invoices.data?.total ?? 0)
              }
              href={routes.tenant.engagementInvoices(slug, engagement)}
            />
            <Stat
              label={t("office.engagement.stat_blockers")}
              value={cell?.open_blockers ?? 0}
              tone={cell?.open_blockers ? "danger" : "ok"}
              href={routes.tenant.engagementWarnings(slug, engagement)}
            />
            <Stat
              label={t("office.engagement.stat_warnings")}
              value={
                warnings.isLoading || warnings.isError ? "–" : openWarnings
              }
              href={routes.tenant.engagementWarnings(slug, engagement)}
            />
            <Stat
              label={t("office.engagement.stat_changes")}
              value={cell?.open_change_requests ?? 0}
            />
            <Stat
              label={t("office.engagement.stat_tasks")}
              value={
                tasks.isLoading || tasks.isError
                  ? "–"
                  : (tasks.data?.length ?? 0)
              }
              href={`#${DOC_TASKS_ANCHOR}`}
            />
          </ul>
        </section>

        <section
          id={DOC_TASKS_ANCHOR}
          tabIndex={-1}
          aria-labelledby="tasks-title"
          className="bg-surface border-line scroll-mt-20 rounded-xl border p-4 outline-none md:p-5"
        >
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h2 id="tasks-title" className="font-display text-base font-bold">
              {t("office.engagement.tasks_title")}
            </h2>
            {/* Belge iste: kanal, son tarih ve hatırlatma seçilir. */}
            <DocRequestForm engagement={engagement} period={focus} />
          </div>
          {selectedMissing ? (
            <p
              className="text-text-2 mb-3 text-sm"
              data-testid="doc-task-not-open"
            >
              {t("office.engagement.task_not_open")}
            </p>
          ) : null}
          {tasks.isLoading ? (
            <Skeleton className="h-24 w-full" />
          ) : tasks.isError ? (
            <ErrorState
              title={t("office.engagement.tasks_error")}
              onRetry={() => void tasks.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : (tasks.data ?? []).length === 0 ? (
            <BosDurum
              className="border-0 py-4"
              baslik={t("office.engagement.tasks_empty")}
              aciklama={t("office.engagement.tasks_empty_hint")}
              eylem={
                <Link
                  href={routes.tenant.engagementWarnings(slug, engagement)}
                  className="text-brand text-sm font-semibold underline-offset-2 hover:underline"
                >
                  {t("office.engagement.tasks_empty_action")}
                </Link>
              }
            />
          ) : (
            <ul className="divide-line divide-y">
              {tasks.data!.map((x) => (
                <li
                  key={x.uuid}
                  id={`istek-${x.uuid}`}
                  data-testid="doc-task-row"
                  data-selected={x.uuid === selectedTask ? "true" : undefined}
                  aria-current={x.uuid === selectedTask ? "true" : undefined}
                  className={cn(
                    "flex scroll-mt-24 items-center gap-3 py-2.5",
                    x.uuid === selectedTask &&
                      "bg-brand-tint ring-brand -mx-2 rounded-md px-2 ring-2",
                  )}
                >
                  <span
                    className={cn(
                      "rounded-md px-2 py-0.5 text-xs font-semibold whitespace-nowrap",
                      x.status === "submitted"
                        ? "bg-brand-tint text-brand-text"
                        : x.overdue
                          ? "bg-danger-tint text-danger"
                          : "bg-warn-tint text-warn",
                    )}
                  >
                    {x.status === "submitted"
                      ? t("office.engagement.task_submitted")
                      : x.overdue
                        ? t("office.engagement.task_overdue")
                        : t("office.engagement.task_waiting")}
                  </span>
                  <span className="min-w-0 flex-1 space-y-1 text-sm">
                    <span className="block truncate">{taskTitle(x)}</span>
                    {/* Kanal durumu, hatırlatma ve "Hatırlat". */}
                    <DocTaskChannels engagement={engagement} task={x} />
                    {/* Gelen belge burada açılır ve istek tamamlanır (#571). */}
                    {x.status === "submitted" ? (
                      <DocTaskSubmission
                        engagement={engagement}
                        task={x.uuid}
                      />
                    ) : null}
                  </span>
                  {x.due_at ? (
                    <time
                      dateTime={x.due_at}
                      className="text-text-3 text-xs whitespace-nowrap"
                    >
                      {new Date(x.due_at).toLocaleDateString(
                        locale === "tr" ? "tr-TR" : "en-GB",
                      )}
                    </time>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      {/* Mükellefin banka hesapları (#780): ekle, düzenle, pasife al. */}
      <BankAccountsSection engagement={engagement} />

      {/* Önemli belgeler (#780 B): ofis ayarından kapatılabilir. */}
      <ImportantDocumentsSection engagement={engagement} />

      {/* Mükellef uygulaması daveti: davet linki kopyala / WhatsApp ile
          paylaş; e-posta gitmezse açık uyarı (Oğuz'un geri bildirimi). */}
      <TaxpayerInvitationsSection engagement={engagement} />
    </div>
  );
}
