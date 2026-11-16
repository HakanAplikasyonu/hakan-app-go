"use client";

import { Upload } from "lucide-react";
import Link from "next/link";

import { MarkaLogosu } from "@/components/hakan";
import { UserMenu } from "@/components/layout/user-menu";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { NotificationInbox } from "@/features/notifications";
import { SearchTrigger } from "@/features/search-engine";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { PeriodPicker } from "../period";

/**
 * Ofis üst çubuğu (bilesenler.md, "Kenar menü ve kabuk"): arama (⌘K), global
 * dönem seçici, bildirimler, birincil eylem (UBL yükle). Telefonda logo ve
 * sıkışık düğmeler; menü alt sekme çubuğunda.
 */
export function OfficeTopbar() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <header className="bg-background/90 supports-[backdrop-filter]:bg-background/75 sticky top-0 z-40 border-b backdrop-blur">
      <div className="flex h-16 items-center gap-2 px-4 md:gap-3 md:px-6">
        <Link
          href={routes.tenant.home(slug)}
          className="me-1 flex h-11 items-center md:hidden"
        >
          <MarkaLogosu varyant="isaret" boyut={24} />
          <span className="sr-only">{t("layout.home")}</span>
        </Link>
        <SearchTrigger className="hidden max-w-md flex-1 md:flex" />
        <div className="flex-1 md:hidden" />
        <SearchTrigger compact className="md:hidden pointer-coarse:size-11" />
        <PeriodPicker className="hidden sm:flex" />
        <PeriodPicker compact className="sm:hidden" />
        <NotificationInbox />
        <Button
          asChild
          className="bg-brand text-on-brand hover:bg-brand/90 hidden md:inline-flex pointer-coarse:h-11"
        >
          <Link href={routes.tenant.upload(slug)}>
            <Upload className="size-4" aria-hidden />
            {t("office.nav_upload")}
          </Link>
        </Button>
        <UserMenu />
      </div>
    </header>
  );
}
