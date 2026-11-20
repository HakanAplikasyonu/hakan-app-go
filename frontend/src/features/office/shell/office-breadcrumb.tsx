"use client";

import { Check, ChevronsUpDown } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { routes } from "@/config/routes";
import type { NavCatalog } from "@/features/nav-engine";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementTabs } from "../components/engagement-layout";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import { FOCUS_PARAM } from "../lib/related-href";
import {
  engagementInPath,
  matchByPath,
  swapEngagement,
} from "../lib/office-location";
import { isPeriod, PERIOD_PARAM } from "../period";

/**
 * Üst çubuğun ekmek kırıntısı (#521): ofis → mükellef → sayfa → işlem (#710). Kullanıcı
 * nerede olduğunu buradan okur; mükellef ayrıntısında mükellef adı hızlı
 * değiştiricidir (aynı sekmede başka mükellefe geçer). Dar ekranda yalnızca
 * son iki, telefonda yalnızca en yakın bağlam görünür.
 */
export function OfficeBreadcrumb({
  catalog,
  className,
}: {
  catalog: NavCatalog;
  className?: string;
}) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const pathname = usePathname();
  const engagement = engagementInPath(pathname, slug);
  const home = routes.tenant.home(slug);
  const page = useMemo(() => {
    const items = catalog.groups.flatMap((g) =>
      g.items.map((i) => ({
        href: i.href,
        titleKey: i.titleKey,
        exact: i.href === home,
      })),
    );
    return matchByPath(pathname, items);
  }, [catalog, pathname, home]);

  return (
    <Breadcrumb
      aria-label={t("office.shell.breadcrumb_label")}
      className={cn("min-w-0", className)}
    >
      <BreadcrumbList className="flex-nowrap">
        <BreadcrumbItem className="hidden shrink-0 lg:inline-flex">
          <BreadcrumbLink asChild className="max-w-48">
            <Link href={home}>{organization.name}</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator className="hidden lg:list-item" />
        {engagement ? (
          <EngagementCrumbs engagement={engagement} />
        ) : (
          <BreadcrumbItem>
            <BreadcrumbPage className="font-medium">
              {page ? t(page.titleKey) : t("layout.home")}
            </BreadcrumbPage>
          </BreadcrumbItem>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}

function EngagementCrumbs({ engagement }: { engagement: string }) {
  const pathname = usePathname();
  const tabs = useEngagementTabs(engagement);
  const tab = matchByPath(pathname, tabs);
  return (
    <>
      <BreadcrumbItem>
        <TaxpayerSwitcher engagement={engagement} />
      </BreadcrumbItem>
      {tab ? (
        <Suspense fallback={<TabCrumb tab={tab} deeper={false} back="" />}>
          <TabCrumbs tab={tab} />
        </Suspense>
      ) : null}
    </>
  );
}

/**
 * Sekme ve işlem kırıntısı (#710): sekmenin altındaki bir kayıttaysa
 * (alt yol ya da `?kayit=`) sekme adı listeye geri dönüş bağlantısıdır
 * (dönem korunur), son kırıntı "Kayıt"tır. Ofis → mükellef → sayfa →
 * işlem; dönem hemen yandaki seçicidedir.
 */
function TabCrumbs({ tab }: { tab: { href: string; label: string } }) {
  const pathname = usePathname();
  const params = useSearchParams();
  const base = tab.href.split("?")[0]!.replace(/\/+$/, "");
  const path = pathname.replace(/\/+$/, "");
  const deeper = path !== base || params.has(FOCUS_PARAM);
  const period = params.get(PERIOD_PARAM);
  const back =
    period && isPeriod(period)
      ? `${base}?${PERIOD_PARAM}=${encodeURIComponent(period)}`
      : base;
  return <TabCrumb tab={tab} deeper={deeper} back={back} />;
}

function TabCrumb({
  tab,
  deeper,
  back,
}: {
  tab: { href: string; label: string };
  deeper: boolean;
  back: string;
}) {
  const { t } = useLocale();
  if (!deeper) {
    return (
      <>
        <BreadcrumbSeparator className="hidden sm:list-item" />
        <BreadcrumbItem className="hidden sm:inline-flex">
          <BreadcrumbPage>{tab.label}</BreadcrumbPage>
        </BreadcrumbItem>
      </>
    );
  }
  return (
    <>
      <BreadcrumbSeparator className="hidden sm:list-item" />
      <BreadcrumbItem className="hidden sm:inline-flex">
        <BreadcrumbLink asChild>
          <Link
            href={back}
            aria-label={t("office.shell.breadcrumb_back", { page: tab.label })}
          >
            {tab.label}
          </Link>
        </BreadcrumbLink>
      </BreadcrumbItem>
      <BreadcrumbSeparator className="hidden md:list-item" />
      <BreadcrumbItem className="hidden md:inline-flex">
        <BreadcrumbPage>{t("office.shell.breadcrumb_record")}</BreadcrumbPage>
      </BreadcrumbItem>
    </>
  );
}

/** Mükellef adı + açılır liste: aynı sekmede başka mükellefe geçer. */
function TaxpayerSwitcher({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const pathname = usePathname();
  const router = useRouter();
  const directory = useEngagementDirectory();
  const [open, setOpen] = useState(false);
  const current = directory.entries.get(engagement);
  const title =
    current?.title ??
    (directory.isLoading ? "…" : t("office.shell.taxpayer_unknown"));
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          role="combobox"
          aria-expanded={open}
          aria-label={t("office.shell.taxpayer_switch", { name: title })}
          className="text-foreground -mx-2 max-w-[42vw] gap-1 px-2 font-medium sm:max-w-64"
        >
          <span className="truncate">{title}</span>
          <ChevronsUpDown
            aria-hidden
            className="text-muted-foreground size-3.5 shrink-0"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        <Command>
          <CommandInput placeholder={t("office.shell.taxpayer_search")} />
          <CommandList>
            <CommandEmpty>{t("office.shell.taxpayer_none")}</CommandEmpty>
            <CommandGroup>
              {directory.list.map((x) => (
                <CommandItem
                  key={x.engagementUuid}
                  value={`${x.title} ${x.taxId}`}
                  onSelect={() => {
                    setOpen(false);
                    if (x.engagementUuid !== engagement)
                      router.push(
                        swapEngagement(pathname, slug, x.engagementUuid),
                      );
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">{x.title}</span>
                  <span className="text-muted-foreground font-mono text-xs tabular-nums">
                    {x.taxId}
                  </span>
                  <Check
                    aria-hidden
                    className={cn(
                      "size-4",
                      x.engagementUuid === engagement
                        ? "opacity-100"
                        : "opacity-0",
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
