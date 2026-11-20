"use client";

import Link from "next/link";

import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { TaxReturnSection } from "../services/tax-returns.service";

const SECTION_ROUTES: Record<
  TaxReturnSection["key"],
  (slug: string, engagement: string) => string
> = {
  retail: routes.tenant.engagementZReports,
  invoices: routes.tenant.engagementInvoices,
  bank: routes.tenant.engagementBank,
  expenses: routes.tenant.engagementExpenses,
};

/**
 * Beyan sayfasında bölüm bölüm özet (#710): dönemin Z raporları, faturaları,
 * banka hareketleri ve KDV'li fişleri; her biri için "x / y işlendi", yok
 * sayılanlar ve bekleyenler. Her bölüm kendi listesine götürür. Beyan
 * hazırlansa da canlıdır: sonradan gelen kayıt burada görünür.
 */
export function TaxReturnSections({
  slug,
  engagement,
  period,
  sections,
}: {
  slug: string;
  engagement: string;
  period: string;
  sections: TaxReturnSection[];
}) {
  const { t } = useLocale();
  if (sections.length === 0) return null;
  return (
    <section
      aria-label={t("office.tax_returns.sections_label")}
      data-testid="tax-return-sections"
      className="bg-surface border-line rounded-xl border p-4"
    >
      <h3 className="mb-3 text-sm font-semibold">
        {t("office.tax_returns.sections_label")}
      </h3>
      <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {sections.map((s) => {
          const waiting = Math.max(s.total - s.processed - s.ignored, 0);
          const done = s.total > 0 && waiting === 0;
          const href = `${SECTION_ROUTES[s.key](slug, engagement)}?period=${encodeURIComponent(period)}`;
          const name = t(`office.tax_returns.section_${s.key}`);
          return (
            <li
              key={s.key}
              data-testid={`tax-return-section-${s.key}`}
              className="flex min-w-0 flex-col gap-0.5"
            >
              <Link href={href} className="text-sm font-medium underline">
                {name}
              </Link>
              <span
                className={cn(
                  "text-base",
                  done && "text-ok",
                  waiting > 0 && "text-warn",
                )}
              >
                {s.total === 0
                  ? t("office.tax_returns.section_none")
                  : t("office.tax_returns.section_processed", {
                      processed: s.processed,
                      total: s.total,
                    })}
              </span>
              {s.ignored > 0 ? (
                <span className="text-text-2 text-xs">
                  {t("office.tax_returns.section_ignored", {
                    count: s.ignored,
                  })}
                </span>
              ) : null}
              {waiting > 0 ? (
                <span className="text-text-2 text-xs">
                  {t("office.tax_returns.section_waiting", { count: waiting })}
                </span>
              ) : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
