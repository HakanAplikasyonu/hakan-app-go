"use client";

import {
  Building2,
  ClipboardCheck,
  LayoutDashboard,
  Menu,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { useSidebar } from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { isNavHrefActive } from "@/features/nav-engine/lib/active";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { ShellCounts } from "./use-shell-counts";

/**
 * Telefon alt sekme çubuğu (OfisDuyarli, <768 px): Pano / Kuyruk / Mükellef /
 * Uyarı / Daha. "Daha" kenar menüyü çekmece olarak açar.
 */
export function OfficeTabbar({ counts }: { counts: ShellCounts }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const pathname = usePathname();
  const { setOpenMobile, openMobile } = useSidebar();
  const home = routes.tenant.home(slug);
  const tabs = [
    {
      id: "home",
      href: home,
      icon: LayoutDashboard,
      label: t("office.shell.tab_home"),
    },
    {
      id: "review",
      href: routes.tenant.review(slug),
      icon: ClipboardCheck,
      label: t("office.shell.tab_review"),
    },
    {
      id: "taxpayers",
      href: routes.tenant.taxpayers(slug),
      icon: Building2,
      label: t("office.shell.tab_taxpayers"),
    },
    {
      id: "warnings",
      href: routes.tenant.warnings(slug),
      icon: TriangleAlert,
      label: t("office.shell.tab_warnings"),
    },
  ];
  return (
    <nav
      aria-label={t("office.shell.tabs_label")}
      className="bg-card fixed inset-x-0 bottom-0 z-40 border-t pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      <ul className="grid grid-cols-5">
        {tabs.map(({ id, href, icon: Icon, label }) => {
          const active = isNavHrefActive(pathname, href, home);
          const count = counts.items[id];
          return (
            <li key={id}>
              <Link
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex min-h-14 flex-col items-center justify-center gap-0.5 text-[11px] font-medium",
                  active ? "text-brand-text" : "text-muted-foreground",
                )}
              >
                <span className="relative">
                  <Icon className="size-5" strokeWidth={1.75} aria-hidden />
                  {count ? (
                    <span className="bg-foreground text-background absolute -end-3 -top-1.5 min-w-4 rounded-full px-1 text-center font-mono text-[9px] leading-4 tabular-nums">
                      {count > 99 ? "99+" : count}
                    </span>
                  ) : null}
                </span>
                {label}
                {count ? <span className="sr-only">, {count}</span> : null}
              </Link>
            </li>
          );
        })}
        <li>
          <button
            type="button"
            aria-expanded={openMobile}
            onClick={() => setOpenMobile(true)}
            className="text-muted-foreground flex min-h-14 w-full flex-col items-center justify-center gap-0.5 text-[11px] font-medium"
          >
            <Menu className="size-5" strokeWidth={1.75} aria-hidden />
            {t("office.shell.tab_more")}
          </button>
        </li>
      </ul>
    </nav>
  );
}
