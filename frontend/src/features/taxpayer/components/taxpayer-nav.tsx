"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Section links of the mükellef panel (issue #39). */
export function TaxpayerNav({ taxpayer }: { taxpayer: string }) {
  const { t } = useLocale();
  const pathname = usePathname();
  const items = [
    {
      href: routes.taxpayer.home(taxpayer),
      label: t("taxpayer.nav_home"),
      exact: true,
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
  ];
  return (
    <nav
      aria-label={t("taxpayer.nav_label")}
      className="flex gap-1 overflow-x-auto"
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
              "rounded-md px-3 py-1.5 text-sm whitespace-nowrap transition-colors",
              active
                ? "bg-secondary text-foreground font-medium"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
