"use client";

import { Upload } from "lucide-react";
import Link from "next/link";

import { SupportButton } from "@/components/support";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/config/routes";
import type { NavCatalog } from "@/features/nav-engine";
import { NotificationInbox } from "@/features/notifications";
import { SearchTrigger } from "@/features/search-engine";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { PeriodPicker } from "../period";
import { OfficeBreadcrumb } from "./office-breadcrumb";
import { OfficeUserMenu } from "./office-user-menu";

/**
 * Ofis üst çubuğu (#521). Soldan sağa üç bölge:
 *
 * 1. **Bağlam:** ekmek kırıntısı (ofis → mükellef → sayfa) ve hemen yanında
 *    dönem seçici: dönem, görünen bağlamın süzgecidir.
 * 2. **Bulma:** arama (⌘K); geniş ekranda kısayol ipucuyla, dar ekranda ikon.
 * 3. **Eylemler:** ikincil ikon düğmeleri (destek, bildirim) bir grupta,
 *    birincil eylem (UBL yükle) belirgin, en sağda kullanıcı menüsü.
 *
 * Düzenler: ≥1024 px tam; 768–1023 px ofis adı gizli, arama ikon; <768 px
 * tek satır bağlam + dönem + arama + bildirim + kullanıcı (destek ve yükleme
 * kullanıcı menüsünde ve alt sekme çubuğunda). Dokunma hedefleri ≥44 px.
 */
export function OfficeTopbar({ catalog }: { catalog: NavCatalog }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <header className="bg-background/90 supports-[backdrop-filter]:bg-background/75 sticky top-0 z-40 border-b backdrop-blur">
      <div className="flex h-16 items-center gap-2 px-4 md:gap-3 md:px-6">
        <div className="flex min-w-0 flex-1 items-center gap-2 md:gap-3">
          <OfficeBreadcrumb catalog={catalog} className="shrink" />
          <PeriodPicker className="hidden shrink-0 sm:flex" />
          <PeriodPicker compact className="shrink-0 sm:hidden" />
        </div>

        <SearchTrigger className="hidden w-64 shrink-0 lg:flex" />
        <SearchTrigger
          compact
          className="shrink-0 max-md:size-11 lg:hidden pointer-coarse:size-11"
        />

        <div
          role="group"
          aria-label={t("office.shell.secondary_actions")}
          className="flex shrink-0 items-center"
        >
          <SupportButton className="hidden sm:inline-flex" />
          <NotificationInbox />
        </div>
        <Separator
          orientation="vertical"
          className="hidden h-6 md:block"
          decorative
        />
        <Button
          asChild
          className="bg-brand text-on-brand hover:bg-brand/90 hidden shrink-0 md:inline-flex pointer-coarse:h-11"
        >
          <Link href={routes.tenant.upload(slug)}>
            <Upload className="size-4" aria-hidden />
            {t("office.nav_upload")}
          </Link>
        </Button>
        <OfficeUserMenu />
      </div>
    </header>
  );
}
