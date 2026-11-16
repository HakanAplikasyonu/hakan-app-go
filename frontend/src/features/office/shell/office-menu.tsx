"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { MarkaLogosu } from "@/components/hakan";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import type { NavCatalog } from "@/features/nav-engine";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { isNavHrefActive } from "@/features/nav-engine/lib/active";
import { OrganizationSwitcher } from "@/features/tenant/components/organization-switcher";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import type { ShellCounts } from "./use-shell-counts";

const fmt = (n: number) => (n > 999 ? "999+" : String(n));

/**
 * Ofis kenar menüsü (OfisNav): ≥1280 px 240 px tam menü, 768–1279 px 72 px
 * ikon rayı (aynı bileşen, `collapsible="icon"`), <768 px alt sekme
 * çubuğunun "Daha" düğmesiyle açılan çekmece.
 */
export function OfficeMenu({
  catalog,
  homeHref,
  counts,
}: {
  catalog: NavCatalog;
  homeHref: string;
  counts: ShellCounts;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const pathname = usePathname();
  const { can, canAny } = usePermission();
  const { isMobile, setOpenMobile, state } = useSidebar();
  const rail = !isMobile && state === "collapsed";

  return (
    <Sidebar collapsible="icon" aria-label={t("office.shell.menu_label")}>
      <SidebarHeader className="gap-3 px-3 pt-4">
        <Link
          href={homeHref}
          className={cn(
            "focus-visible:ring-ring flex h-11 items-center rounded-md px-2 outline-none focus-visible:ring-2",
            rail && "justify-center px-0",
          )}
          onClick={() => isMobile && setOpenMobile(false)}
        >
          <MarkaLogosu varyant={rail ? "isaret" : "tam"} boyut={24} />
          {rail ? <span className="sr-only">{t("layout.home")}</span> : null}
        </Link>
        {rail ? null : <OrganizationSwitcher />}
      </SidebarHeader>

      <SidebarContent className="px-1">
        {catalog.groups.map((group) => {
          const items = visibleNavItems(group, { can, canAny });
          if (!items.length) return null;
          return (
            <SidebarGroup key={group.id} className="py-1">
              {rail ? (
                <div className="bg-border mx-auto my-1 h-px w-8" aria-hidden />
              ) : (
                <SidebarGroupLabel className="text-muted-foreground font-mono text-[10px] font-semibold tracking-[0.08em] uppercase">
                  {t(group.labelKey)}
                </SidebarGroupLabel>
              )}
              <SidebarGroupContent>
                <SidebarMenu className={cn(rail && "items-center gap-1.5")}>
                  {items.map((item) => {
                    const active = isNavHrefActive(
                      pathname,
                      item.href,
                      homeHref,
                    );
                    const count = counts.items[item.id];
                    const title = t(item.titleKey);
                    const Icon = item.icon;
                    return (
                      <SidebarMenuItem key={item.id}>
                        <SidebarMenuButton
                          asChild
                          isActive={active}
                          tooltip={count ? `${title} · ${fmt(count)}` : title}
                          className={cn(
                            "h-9 gap-2.5 rounded-[10px] pointer-coarse:h-11",
                            "data-[active=true]:bg-brand-tint data-[active=true]:text-brand-text",
                            "group-data-[collapsible=icon]:h-11! group-data-[collapsible=icon]:w-12! group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-xl",
                          )}
                        >
                          <Link
                            href={item.href}
                            aria-current={active ? "page" : undefined}
                            onClick={() => isMobile && setOpenMobile(false)}
                          >
                            <Icon className="size-[18px]!" strokeWidth={1.75} />
                            <span className={cn("flex-1", rail && "sr-only")}>
                              {title}
                            </span>
                            {count && !rail ? (
                              <span className="text-muted-foreground font-mono text-[11px] font-semibold tabular-nums">
                                <span className="sr-only">, </span>
                                {fmt(count)}
                              </span>
                            ) : null}
                          </Link>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    );
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          );
        })}
      </SidebarContent>

      <SidebarFooter className="px-3 pb-4">
        {counts.agents ? (
          <AgentStatus
            slug={slug}
            rail={rail}
            online={counts.agents.online}
            offline={counts.agents.offline}
          />
        ) : null}
      </SidebarFooter>
    </Sidebar>
  );
}

function AgentStatus({
  slug,
  rail,
  online,
  offline,
}: {
  slug: string;
  rail: boolean;
  online: number;
  offline: number;
}) {
  const { t } = useLocale();
  const label = t("office.shell.agents_online", { count: online });
  const off = offline
    ? t("office.shell.agents_offline", { count: offline })
    : "";
  const dot = (
    <span
      aria-hidden
      className={cn(
        "size-2 shrink-0 rounded-full",
        online ? "bg-ok" : "bg-danger",
      )}
    />
  );
  if (rail) {
    return (
      <Link
        href={routes.tenant.agents(slug)}
        className="hover:bg-sidebar-accent mx-auto flex size-11 items-center justify-center rounded-xl"
        title={[label, off].filter(Boolean).join(" · ")}
      >
        {dot}
        <span className="sr-only">
          {[label, off].filter(Boolean).join(", ")}
        </span>
      </Link>
    );
  }
  return (
    <Link
      href={routes.tenant.agents(slug)}
      className="bg-surface-2 hover:border-line-strong flex min-h-11 items-center gap-2 rounded-[10px] border px-3 py-2 text-xs transition-colors"
    >
      {dot}
      <span className="font-semibold">{label}</span>
      {off ? (
        <span className="text-muted-foreground ms-auto">{off}</span>
      ) : null}
    </Link>
  );
}
