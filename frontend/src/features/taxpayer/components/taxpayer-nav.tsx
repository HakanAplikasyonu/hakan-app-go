"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { routes } from "@/config/routes";
import {
  taxpayerTaskKeys,
  taxpayerTaskService,
} from "@/features/taxpayer/services/taxpayer.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Mükellef portalının sekmeleri (#39; arayüz 8/9, #331): etkin sekmenin
 * altında marka çizgisi, İstekler'de açık istek sayısı.
 */
export function TaxpayerNav({
  taxpayer,
  owner = false,
}: {
  taxpayer: string;
  /** Bordro PDF'leri yalnızca şirketin sahibine görünür (#382). */
  owner?: boolean;
}) {
  const { t } = useLocale();
  const pathname = usePathname();
  const open = useQuery({
    queryKey: taxpayerTaskKeys.list(taxpayer, "open"),
    queryFn: () => taxpayerTaskService.list(taxpayer, "open"),
    meta: { silent: true },
  });
  const openCount = open.data?.total ?? 0;
  const items = [
    {
      href: routes.taxpayer.home(taxpayer),
      label: t("taxpayer.nav_home"),
      exact: true,
    },
    {
      href: routes.taxpayer.requests(taxpayer),
      label: t("taxpayer.nav_requests"),
      count: openCount,
    },
    {
      href: routes.taxpayer.invoices(taxpayer),
      label: t("taxpayer.nav_invoices"),
    },
    {
      href: routes.taxpayer.receipts(taxpayer),
      label: t("taxpayer.nav_receipts"),
    },
    {
      href: routes.taxpayer.uploads(taxpayer),
      label: t("taxpayer.nav_uploads"),
    },
    ...(owner
      ? [
          {
            href: routes.taxpayer.payroll(taxpayer),
            label: t("taxpayer.nav_payroll"),
          },
          {
            href: routes.taxpayer.archive(taxpayer),
            label: t("taxpayer.nav_archive"),
          },
        ]
      : []),
    {
      href: routes.taxpayer.report(taxpayer),
      label: t("taxpayer.nav_report"),
    },
    {
      // Kullanım kılavuzu (#274).
      href: routes.help.guide("mukellef"),
      label: t("help.nav"),
    },
  ];
  return (
    <nav
      aria-label={t("taxpayer.nav_label")}
      className="flex h-full items-stretch gap-1 overflow-x-auto"
    >
      {items.map((item) => {
        const active = item.exact
          ? pathname === item.href
          : pathname === item.href || pathname.startsWith(`${item.href}/`);
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "focus-visible:ring-ring relative flex min-h-11 items-center gap-2 px-3 text-sm whitespace-nowrap transition-colors focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset",
              "after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full",
              active
                ? "text-foreground after:bg-brand font-semibold"
                : "text-text-2 hover:text-foreground",
            )}
          >
            {item.label}
            {item.count ? (
              <span
                className="bg-danger text-background flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-[11px] font-bold tabular-nums"
                aria-label={t("taxpayer.nav_requests_open", {
                  count: item.count,
                })}
              >
                {item.count}
              </span>
            ) : null}
          </Link>
        );
      })}
    </nav>
  );
}
