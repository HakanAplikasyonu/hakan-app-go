"use client";

import { Lock } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { periodText } from "../lib/change-request-text";

/**
 * Seçili dönem kapalıyken (#710): kayıtlar salt okunur görünür; fişler,
 * faturalar ve beyan tek tıkla açılır. Değişiklik için dönem gerekçeyle
 * açılır (fişler ekranındaki "Dönemi aç").
 */
export function LockedPeriodNote({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const month = periodText(period, locale);
  const links = [
    {
      href: routes.tenant.engagementJournals(slug, engagement),
      label: t("office.locked_period.journals"),
    },
    {
      href: routes.tenant.engagementInvoices(slug, engagement),
      label: t("office.locked_period.invoices"),
    },
    {
      href: routes.tenant.engagementTaxReturns(slug, engagement),
      label: t("office.locked_period.returns"),
    },
  ];
  return (
    <section
      data-testid="locked-period-note"
      aria-labelledby="locked-period-title"
      className="border-line bg-surface-2 flex flex-wrap items-center gap-3 rounded-xl border p-3"
    >
      <Lock aria-hidden className="text-text-3 size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <h2 id="locked-period-title" className="text-sm font-semibold">
          {t("office.locked_period.title", { month })}
        </h2>
        <p className="text-text-2 text-sm">{t("office.locked_period.body")}</p>
      </div>
      <nav
        aria-label={t("office.locked_period.nav", { month })}
        className="flex flex-wrap gap-2"
      >
        {links.map((l) => (
          <Button key={l.href} asChild variant="outline" size="sm">
            <Link href={l.href}>{l.label}</Link>
          </Button>
        ))}
      </nav>
    </section>
  );
}
